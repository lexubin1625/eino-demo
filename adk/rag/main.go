package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	ccb "github.com/cloudwego/eino-ext/callbacks/cozeloop"
	"github.com/cloudwego/eino-ext/components/embedding/openai"
	chatOpenAi "github.com/cloudwego/eino-ext/components/model/openai"
	es8retriever "github.com/cloudwego/eino-ext/components/retriever/es8"
	"github.com/cloudwego/eino-ext/components/retriever/es8/search_mode"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/coze-dev/cozeloop-go"
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/typedapi/types"
)

var (
	// ES 配置
	indexName          = "eino_rag_demo"         // ES索引
	fieldContent       = "content"               // 内容字段
	fieldContentVector = "content_vector"        // 向量字段
	esAddress          = "http://localhost:9200" // ES 地址
	esUsername         = ""                      // 如果需要认证
	esPassword         = ""                      // 如果需要认证

	// 千问llm
	llmKey         = os.Getenv("DASHSCOPE_API_KEY")
	llmApi         = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	embeddingModel = "text-embedding-v3" // Embedding 模型配置
	chatModel      = "qwen-plus"         // chat模型
	evalModel      = "qwen-plus"         // 评估模型

	// 检索配置
	minAccuracy = 0.95 // 最低准确率要求
	maxRetries  = 5    // 最大重试次数
	initialTopK = 3    // 初始检索数量
)

// RAGQueryParams RAG 检索工具的参数
type RAGQueryParams struct {
	Query string `json:"query" jsonschema:"required,description=要检索的问题或关键词"`
	TopK  int    `json:"top_k,omitempty" jsonschema:"description=返回的文档数量，默认为3"`
}

// RAGEvaluationParams RAG 评估工具的参数
type RAGEvaluationParams struct {
	Query      string   `json:"query" jsonschema:"required,description=原始问题"`
	Documents  []string `json:"documents" jsonschema:"required,description=检索到的文档内容列表"`
	RetryCount int      `json:"retry_count,omitempty" jsonschema:"description=当前重试次数"`
}

// RAGRetrieveTool RAG 检索工具
type RAGRetrieveTool struct{}

func (r *RAGRetrieveTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "rag_retrieve",
		Desc: "从知识库中检索与查询相关的文档。输入查询问题或关键词，返回最相关的文档片段。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Desc:     "要检索的问题或关键词",
				Required: true,
				Type:     schema.String,
			},
			"top_k": {
				Desc:     "返回的文档数量，默认为3",
				Required: false,
				Type:     schema.Integer,
			},
		}),
	}, nil
}

func (r *RAGRetrieveTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var params RAGQueryParams
	if err := sonic.UnmarshalString(argumentsInJSON, &params); err != nil {
		return "", fmt.Errorf("解析参数失败: %w", err)
	}

	if strings.TrimSpace(params.Query) == "" {
		return `{"error": "query is required"}`, nil
	}

	topK := params.TopK
	if topK <= 0 {
		topK = initialTopK
	}

	// 动态调整检索器的 TopK
	retriever, err := createRetriever(ctx, topK)
	if err != nil {
		return fmt.Sprintf(`{"error": "创建检索器失败: %v"}`, err), nil
	}

	// 执行检索
	docs, err := retriever.Retrieve(ctx, params.Query)
	if err != nil {
		return fmt.Sprintf(`{"error": "检索失败: %v"}`, err), nil
	}

	// 格式化检索结果
	if len(docs) == 0 {
		return `{"found": false, "message": "未找到相关文档"}`, nil
	}

	// 构建结果
	var results []map[string]interface{}
	for i, doc := range docs {
		result := map[string]interface{}{
			"index":   i + 1,
			"content": doc.Content,
			"score":   doc.Score(),
		}
		if doc.MetaData != nil {
			result["metadata"] = doc.MetaData
		}
		results = append(results, result)
	}

	result := map[string]interface{}{
		"found": true,
		"count": len(docs),
		"docs":  results,
		"query": params.Query,
		"top_k": topK,
	}

	// 将结果转换为 JSON 字符串
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf(`{"error": "序列化结果失败: %v"}`, err), nil
	}

	return string(jsonBytes), nil
}

// RAGEvaluationTool RAG 评估工具
type RAGEvaluationTool struct {
	evalModel *chatOpenAi.ChatModel
}

func (e *RAGEvaluationTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "rag_evaluate",
		Desc: "评估检索结果的准确率，特别关注 top1（最相关第一个文档）的准确率。输入原始问题和检索到的文档，返回整体准确率和 top1 准确率（0-1之间）和评估结果。只有 top1 准确率 >= 95% 才算通过。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Desc:     "原始问题",
				Required: true,
				Type:     schema.String,
			},
			"documents": {
				Desc:     "检索到的文档内容列表",
				Required: true,
				Type:     schema.Array,
			},
			"retry_count": {
				Desc:     "当前重试次数",
				Required: false,
				Type:     schema.Integer,
			},
		}),
	}, nil
}

func (e *RAGEvaluationTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var params RAGEvaluationParams
	if err := sonic.UnmarshalString(argumentsInJSON, &params); err != nil {
		return "", fmt.Errorf("解析参数失败: %w", err)
	}

	if strings.TrimSpace(params.Query) == "" {
		return `{"error": "query is required"}`, nil
	}

	if len(params.Documents) == 0 {
		return `{"accuracy": 0.0, "top1_accuracy": 0.0, "passed": false, "reason": "没有检索到文档"}`, nil
	}

	// 构建评估提示词
	documentsText := ""
	for i, doc := range params.Documents {
		documentsText += fmt.Sprintf("文档 %d:\n%s\n\n", i+1, doc)
	}

	prompt := fmt.Sprintf(`请评估以下检索结果与问题的相关性，并给出准确率分数。

问题：
%s

检索到的文档：
%s

重要：请特别关注第一个文档（top1）的准确率评估。top1 是最相关的文档，必须单独评估。

请从以下维度评估：
1. 文档内容与问题的相关性（0-1分）
2. 文档是否能够回答该问题（0-1分）
3. 文档的完整性和准确性（0-1分）

特别要求：
- 必须单独评估 top1（第一个文档）的准确率
- top1 准确率必须 >= 0.95 才算通过
- 整体准确率是所有文档的平均准确率

请以JSON格式返回评估结果，格式：
{
  "accuracy": 0.xx,  // 整体准确率分数（0-1之间，保留2位小数）
  "top1_accuracy": 0.xx,  // top1文档的准确率分数（0-1之间，保留2位小数）
  "passed": true/false,  // 是否通过（top1_accuracy >= 0.95 才为 true）
  "reason": "评估原因说明",
  "top1_reason": "top1文档的评估原因",
  "details": {
    "relevance": 0.xx,  // 整体相关性分数
    "answerability": 0.xx,  // 整体可回答性分数
    "completeness": 0.xx,  // 整体完整性分数
    "top1_relevance": 0.xx,  // top1相关性分数
    "top1_answerability": 0.xx,  // top1可回答性分数
    "top1_completeness": 0.xx  // top1完整性分数
  }
}`, params.Query, documentsText)

	msg, err := e.evalModel.Generate(ctx, []*schema.Message{
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return "", fmt.Errorf("评估失败: %w", err)
	}

	// 尝试解析返回的 JSON
	var evalResult map[string]interface{}
	content := strings.TrimSpace(msg.Content)

	// 尝试提取 JSON 部分（如果 LLM 返回了额外的文本）
	if strings.Contains(content, "{") {
		start := strings.Index(content, "{")
		end := strings.LastIndex(content, "}") + 1
		if start >= 0 && end > start {
			content = content[start:end]
		}
	}

	if err := sonic.UnmarshalString(content, &evalResult); err != nil {
		// 如果解析失败，返回原始内容，但标记为未通过
		return fmt.Sprintf(`{"accuracy": 0.0, "top1_accuracy": 0.0, "passed": false, "reason": "无法解析评估结果: %s", "raw_response": "%s"}`, err.Error(), msg.Content), nil
	}

	// 确保返回格式正确
	result := map[string]interface{}{
		"accuracy": evalResult["accuracy"],
		"passed":   evalResult["passed"],
		"reason":   evalResult["reason"],
	}

	// 添加 top1_accuracy 字段
	if top1Acc, ok := evalResult["top1_accuracy"].(float64); ok {
		result["top1_accuracy"] = top1Acc
		// 如果 LLM 没有正确设置 passed，根据 top1_accuracy 自动判断
		if passed, ok := evalResult["passed"].(bool); !ok || !passed {
			result["passed"] = top1Acc >= minAccuracy
		}
	} else {
		// 如果没有 top1_accuracy，尝试从整体 accuracy 推断（但标记为未通过）
		result["top1_accuracy"] = evalResult["accuracy"]
		result["passed"] = false
	}

	// 添加 top1_reason
	if top1Reason, ok := evalResult["top1_reason"].(string); ok {
		result["top1_reason"] = top1Reason
	}

	if details, ok := evalResult["details"].(map[string]interface{}); ok {
		result["details"] = details
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf(`{"accuracy": 0.0, "top1_accuracy": 0.0, "passed": false, "reason": "序列化结果失败: %v"}`, err), nil
	}

	return string(jsonBytes), nil
}

// createESClient 创建 ES 客户端
func createESClient() (*elasticsearch.Client, error) {
	client, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{esAddress},
		Username:  esUsername,
		Password:  esPassword,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 ES 客户端失败: %w", err)
	}

	// 测试连接
	res, err := client.Info()
	if err != nil {
		return nil, fmt.Errorf("连接 ES 失败: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("ES 返回错误: %s", res.String())
	}

	return client, nil
}

// createEmbedder 创建 Embedder
func createEmbedder(ctx context.Context) (embedding.Embedder, error) {
	if llmKey == "" {
		return nil, fmt.Errorf("未设置 DASHSCOPE_API_KEY 环境变量")
	}

	embedder, err := openai.NewEmbedder(ctx, &openai.EmbeddingConfig{
		APIKey:  llmKey,
		Model:   embeddingModel,
		Timeout: 60 * time.Second,
		ByAzure: false,
		BaseURL: llmApi,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 embedder 失败: %w", err)
	}

	return embedder, nil
}

var (
	esClient  *elasticsearch.Client
	embedder  embedding.Embedder
	clientErr error
	embedErr  error
)

// initClients 初始化 ES 客户端和 Embedder（只初始化一次）
func initClients(ctx context.Context) error {
	if esClient == nil {
		esClient, clientErr = createESClient()
		if clientErr != nil {
			return clientErr
		}
	}
	if embedder == nil {
		embedder, embedErr = createEmbedder(ctx)
		if embedErr != nil {
			return embedErr
		}
	}
	return nil
}

// createRetriever 创建 Retriever（支持动态 TopK）
func createRetriever(ctx context.Context, topK int) (*es8retriever.Retriever, error) {
	if err := initClients(ctx); err != nil {
		return nil, err
	}

	ret, err := es8retriever.NewRetriever(ctx, &es8retriever.RetrieverConfig{
		Client:    esClient,
		Index:     indexName,
		Embedding: embedder,
		TopK:      topK,
		SearchMode: search_mode.SearchModeApproximate(&search_mode.ApproximateConfig{
			QueryFieldName:  fieldContent,
			VectorFieldName: fieldContentVector,
			Hybrid:          true,
			RRF:             false,
		}),
		ResultParser: func(ctx context.Context, hit types.Hit) (doc *schema.Document, err error) {
			if hit.Source_ == nil {
				return nil, fmt.Errorf("hit source is nil")
			}

			var source map[string]interface{}
			if err := json.Unmarshal(hit.Source_, &source); err != nil {
				return nil, fmt.Errorf("unmarshal source failed: %w", err)
			}

			content, ok := source[fieldContent].(string)
			if !ok {
				return nil, fmt.Errorf("content field not found or not a string")
			}

			docID := ""
			if hit.Id_ != nil {
				docID = *hit.Id_
			}

			doc = &schema.Document{
				ID:       docID,
				Content:  content,
				MetaData: map[string]any{},
			}

			if hit.Score_ != nil {
				doc.WithScore(float64(*hit.Score_))
			}
			if source["paragraph_index"] != nil {
				doc.MetaData["paragraph_index"] = source["paragraph_index"]
			}
			return doc, nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("创建检索器失败: %w", err)
	}

	return ret, nil
}

// createChatModel 创建对话模型
func createChatModel(ctx context.Context, modelName string) (*chatOpenAi.ChatModel, error) {
	llm, err := chatOpenAi.NewChatModel(ctx, &chatOpenAi.ChatModelConfig{
		APIKey:  llmKey,
		Model:   modelName,
		Timeout: 60 * time.Second,
		BaseURL: llmApi,
	})
	return llm, err
}

// runRAGWithEvaluation 执行带评估的 RAG 检索
func runRAGWithEvaluation(ctx context.Context, question string) error {
	fmt.Println("=" + strings.Repeat("=", 80))
	fmt.Println("RAG 检索与评估流程开始")
	fmt.Println("=" + strings.Repeat("=", 80))
	fmt.Printf("\n问题：%s\n\n", question)

	// 创建评估模型
	evalModel, err := createChatModel(ctx, evalModel)
	if err != nil {
		return fmt.Errorf("创建评估模型失败: %w", err)
	}

	// 创建工具
	ragTool := &RAGRetrieveTool{}
	evalTool := &RAGEvaluationTool{evalModel: evalModel}

	// 创建主 Agent 模型
	agentModel, err := createChatModel(ctx, chatModel)
	if err != nil {
		return fmt.Errorf("创建 Agent 模型失败: %w", err)
	}

	// 绑定工具
	toolInfos := []*schema.ToolInfo{}
	for _, t := range []tool.BaseTool{ragTool, evalTool} {
		info, err := t.Info(ctx)
		if err != nil {
			return fmt.Errorf("获取工具信息失败: %w", err)
		}
		toolInfos = append(toolInfos, info)
	}
	if err := agentModel.BindTools(toolInfos); err != nil {
		return fmt.Errorf("绑定工具失败: %w", err)
	}

	// 创建 RAG Agent
	ragAgent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "RAG检索评估Agent",
		Description: "负责执行RAG检索并对结果进行评估，如果准确率达不到95%%则继续检索",
		Instruction: `你是 RAG 检索评估 Agent，负责执行检索并评估结果质量。

重要要求：
- 必须评估 top1（最相关第一个文档）的准确率
- 只有 top1 准确率 >= 95%% 才算通过，才能结束检索
- 整体准确率仅供参考，不作为判断标准

工作流程：
1. 使用 rag_retrieve 工具检索相关文档（初始 top_k=3）
2. 使用 rag_evaluate 工具评估检索结果，特别关注 top1_accuracy
3. 如果 top1_accuracy < 95%%：
   - 增加 top_k 参数（例如：5, 7, 10等）重新检索
   - 再次评估 top1_accuracy
4. 如果 top1_accuracy >= 95%% 或达到最大重试次数：
   - 使用 exit 工具输出最终结果

输出格式：
- 每次检索后，显示检索到的文档数量和评估结果
- 必须显示 top1_accuracy 的值
- 最终输出应包含：
  * 检索到的文档内容（特别是 top1）
  * top1 准确率
  * 是否达到要求

请严格按照流程执行，使用提供的工具完成任务。`,
		Model: agentModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: []tool.BaseTool{ragTool, evalTool},
			},
		},
		MaxIterations: 30, // 允许更多迭代以支持多次检索和评估
		Exit:          adk.ExitTool{},
	})
	if err != nil {
		return fmt.Errorf("创建 RAG Agent 失败: %w", err)
	}

	// 运行 Agent
	fmt.Println("[开始执行] RAG 检索与评估...")
	fmt.Println()

	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           ragAgent,
		EnableStreaming: false,
	})

	iterator := runner.Query(ctx, question)

	var finalResult string
	bestAccuracy := 0.0

	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}

		if event.Err != nil {
			return fmt.Errorf("流程执行失败: %w", event.Err)
		}

		// 处理消息输出
		if event.Output != nil && event.Output.MessageOutput != nil {
			msg, err := event.Output.MessageOutput.GetMessage()
			if err == nil && msg != nil && msg.Content != "" {
				fmt.Printf("[%s] %s\n", event.AgentName, msg.Content)
				finalResult = msg.Content
			}
		}

		// 处理退出
		if event.Action != nil && event.Action.Exit {
			fmt.Println("[完成] 流程执行完成")
			break
		}
	}

	if finalResult == "" {
		return fmt.Errorf("未能生成最终结果")
	}

	// 输出最终结果
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("最终结果")
	fmt.Println(strings.Repeat("=", 80))
	if bestAccuracy > 0 {
		fmt.Printf("最佳准确率: %.2f%%\n", bestAccuracy*100)
	}
	fmt.Println("\n检索结果:")
	fmt.Println(finalResult)

	return nil
}

func main() {
	ctx := context.Background()

	client, err := cozeloop.NewClient()
	if err != nil {
		panic(err)
	}
	defer client.Close(ctx)
	handler := ccb.NewLoopHandler(client)
	callbacks.AppendGlobalHandlers(handler)

	// 示例问题
	question := "风寒感冒的症状有哪些？"

	// 执行 RAG 检索与评估
	if err := runRAGWithEvaluation(ctx, question); err != nil {
		log.Fatalf("RAG 检索与评估失败: %v", err)
	}
}

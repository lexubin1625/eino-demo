package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/embedding/openai"
	es8retriever "github.com/cloudwego/eino-ext/components/retriever/es8"
	"github.com/cloudwego/eino-ext/components/retriever/es8/search_mode"
	duckduckgo "github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/typedapi/types"
)

// RAGQueryParams RAG 检索工具的参数
type RAGQueryParams struct {
	Query string `json:"query" jsonschema:"required,description=要检索的问题或关键词"`
}

// ReActRag 使用 eino react 实现 RAG 检索
func ReActRag(ctx context.Context, question string) error {
	// 1. 创建 ES 客户端
	esClient, err := createESClient()
	if err != nil {
		return fmt.Errorf("创建 ES 客户端失败: %w", err)
	}
	log.Println("成功连接 Elasticsearch")

	// 2. 创建 Embedder
	embedder, err := createEmbedder(ctx)
	if err != nil {
		return fmt.Errorf("创建 embedder 失败: %w", err)
	}
	log.Println("成功初始化 Embedding 模型")

	// 3. 创建 Retriever
	retriever, err := createRetriever(ctx, esClient, embedder)
	if err != nil {
		return fmt.Errorf("创建 retriever 失败: %w", err)
	}
	log.Println("成功创建 RAG 检索器")

	// 4. 将 Retriever 包装成工具
	ragTool, err := createRAGTool(retriever)
	if err != nil {
		return fmt.Errorf("创建 RAG 工具失败: %w", err)
	}
	log.Println("成功创建 RAG 工具")

	// 5. 创建聊天模型
	cm, err := createChatModel(ctx)
	if err != nil {
		return fmt.Errorf("创建聊天模型失败: %w", err)
	}

	//  创建搜索工具（可选，用于补充知识库中没有的信息）
	searchTool, err := duckduckgo.NewTextSearchTool(ctx, &duckduckgo.Config{
		MaxResults: 3,
		Region:     duckduckgo.RegionCN,
	})
	if err != nil {
		return fmt.Errorf("创建搜索工具失败: %w", err)
	}

	// 8. 创建 ReAct Agent
	agent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: cm, // 使用 ToolCallingModel 而不是 Model
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: []tool.BaseTool{ragTool, searchTool},
		},
		MaxStep: 15, // 增加最大步数，避免过早终止
		MessageModifier: react.NewPersonaModifier(`你是一个专业的 RAG（检索增强生成）代理，擅长从知识库中检索相关信息并结合外部搜索回答问题。

工作流程：
1. **思考（Think）**：分析用户问题，确定需要什么信息
2. **第一步行动（Act）**：优先使用 RAG 检索工具从知识库中检索相关文档（如症状、疾病信息等）
3. **观察（Observe）**：分析 RAG 检索到的文档内容
4. **再思考**：判断是否需要外部信息
   - 如果问题涉及治疗建议、最新治疗方案、用药指导等，需要调用搜索工具获取外部信息
   - 如果 RAG 检索结果已足够回答问题，直接给出答案
5. **第二步行动（如需要）**：使用搜索工具获取治疗建议等外部信息
6. **最终回答**：综合 RAG 检索结果和外部搜索结果，给出完整、准确的答案

规则：
- **必须先用 RAG 检索工具**从知识库中检索相关信息（症状、疾病描述等）
- **治疗建议、用药指导、最新治疗方案**等需要从外部获取的信息，在 RAG 检索后使用搜索工具
- RAG 检索结果可能包含多个相关文档片段，需要综合分析
- 答案必须基于检索到的文档内容和搜索结果，不要编造信息
- 综合信息时，要区分哪些来自知识库，哪些来自外部搜索
- 在得到足够信息后，立即给出最终答案，避免不必要的重复检索
- 最终答案要准确、完整，结构清晰（如：症状描述、治疗建议等）`),
	})
	if err != nil {
		return fmt.Errorf("创建 ReAct Agent 失败: %w", err)
	}

	// 8. 执行代理
	fmt.Println("ReAct RAG Agent 开始工作")
	fmt.Printf("问题: %s\n\n", question)

	response, err := agent.Generate(ctx, []*schema.Message{
		{Role: schema.User, Content: question},
	})
	if err != nil {
		return fmt.Errorf("Agent 执行失败: %w", err)
	}

	// 9. 输出结果
	fmt.Println("\n最终答案:")
	if response != nil && response.Content != "" {
		fmt.Println(response.Content)
	} else {
		fmt.Println("未能生成最终答案")
	}

	return nil
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

// createRetriever 创建 Retriever
func createRetriever(ctx context.Context, client *elasticsearch.Client, embedder embedding.Embedder) (*es8retriever.Retriever, error) {
	ret, err := es8retriever.NewRetriever(ctx, &es8retriever.RetrieverConfig{
		Client:    client,
		Index:     indexName,
		Embedding: embedder,
		TopK:      3, // 返回最相关的3个文档
		SearchMode: search_mode.SearchModeApproximate(&search_mode.ApproximateConfig{
			QueryFieldName:  fieldContent,
			VectorFieldName: fieldContentVector,
			Hybrid:          true,
			RRF:             false, // 不启用RRF以避免许可证问题
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

// ragRetrieve 执行 RAG 检索的函数
func ragRetrieve(ctx context.Context, retriever *es8retriever.Retriever, params *RAGQueryParams) (string, error) {
	if params == nil || strings.TrimSpace(params.Query) == "" {
		return `{"error": "query is required"}`, nil
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
	}

	// 将结果转换为 JSON 字符串
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf(`{"error": "序列化结果失败: %v"}`, err), nil
	}

	return string(jsonBytes), nil
}

// createRAGTool 将 Retriever 包装成工具
func createRAGTool(retriever *es8retriever.Retriever) (tool.InvokableTool, error) {
	// 创建一个闭包函数，将 retriever 绑定到检索函数
	retrieveFunc := func(ctx context.Context, params *RAGQueryParams) (string, error) {
		return ragRetrieve(ctx, retriever, params)
	}

	// 使用 InferTool 快速构建可调用工具
	ragTool, err := utils.InferTool(
		"rag_retrieve",
		"从知识库中检索与查询相关的文档。输入查询问题或关键词，返回最相关的文档片段。",
		retrieveFunc,
	)
	if err != nil {
		return nil, fmt.Errorf("创建 RAG 工具失败: %w", err)
	}

	return ragTool, nil
}

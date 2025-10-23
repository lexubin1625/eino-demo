package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/document/loader/file"
	"github.com/cloudwego/eino-ext/components/embedding/openai"
	es8indexer "github.com/cloudwego/eino-ext/components/indexer/es8"
	chatOpenAi "github.com/cloudwego/eino-ext/components/model/openai"
	es8retriever "github.com/cloudwego/eino-ext/components/retriever/es8"
	"github.com/cloudwego/eino-ext/components/retriever/es8/search_mode" // 导入 search_mode 包
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/typedapi/types" // 用于 Hit 类型
)

var (
	indexName          = "eino_rag_demo"  // es索引
	fieldContent       = "content"        // 内容字段
	fieldContentVector = "content_vector" // 向量字段

	// ES 配置
	esAddress  = "http://localhost:9200" // ES 地址
	esUsername = ""                      // 如果需要认证
	esPassword = ""                      // 如果需要认证

	// 千问llm
	llmKey = os.Getenv("DASHSCOPE_API_KEY")
	llmApi = "https://dashscope.aliyuncs.com/compatible-mode/v1" //千问系列API

	embeddingModel = "text-embedding-v3" // Embedding 模型配置
	chatModel      = "qwen-plus"         // chat模型
)

func main() {
	ctx := context.Background()

	// 加载文档
	docs, err := loadDocuments(ctx)
	if err != nil {
		log.Fatalf("加载文档失败: %v", err)
	}
	log.Printf("成功加载 %d 个文档", len(docs))

	// 文档分块
	chunkedDocs := chunkDocuments(docs)
	log.Printf("成功分块，共 %d 个文本块", len(chunkedDocs))

	// 连接 Elasticsearch
	client, err := createESClient()
	if err != nil {
		log.Fatalf("连接 ES 失败: %v", err)
	}

	//  创建 Embedder
	embedder, err := createEmbedder(ctx)
	if err != nil {
		log.Fatalf("创建 embedder 失败: %v", err)
	}
	log.Println("成功初始化 Embedding 模型")

	// 创建 LLM 模型
	llmModel, err := createChatModel(ctx)
	if err != nil {
		log.Fatalf("创建 chat model 模型失败: %v", err)
	}
	log.Println("成功初始化 LLM 模型")

	//  创建索引并存储文档
	log.Println("步骤 5: 创建索引并存储文档到 ES...")
	ids, err := indexDocuments(ctx, client, embedder, chunkedDocs)
	if err != nil {
		log.Fatalf("索引文档失败: %v", err)
	}
	log.Printf("成功索引 %d 个文档块", len(ids))

	qurey := "风寒感冒 症状"
	//  演示混合搜索（向量检索 + BM25）
	doc, err := demonstrateHybridSearch(ctx, client, embedder, qurey)
	if err != nil {
		log.Printf("混合检索演示失败: %v", err)
	}

	// 对话模版
	chatMessages, err := buildChatMessages(ctx, doc, qurey)
	if err != nil {
		log.Printf("构建提示词消息: %v", err)
	}

	// 对话输出
	chat(ctx, llmModel, chatMessages)

}

func createTemplate() prompt.ChatTemplate {
	// 创建模板，使用 FString 格式
	return prompt.FromMessages(schema.FString,
		// 系统消息模板
		schema.SystemMessage("你是专业的老中医,专注于用户问题回答,不要回答医学以外问题"),

		// 插入需要的对话历史（新对话的话这里不填）
		schema.MessagesPlaceholder("chat_history", true),

		// 用户消息模板
		schema.UserMessage(`获取的文档: 
		{context}

		用户问题:
		{question}`),
	)
}

// buildChatContext 构建聊天上下文
func buildChatContext(docs []*schema.Document) (content string) {
	for _, doc := range docs {
		content += doc.Content + "\n\n"
	}
	return content
}

func buildChatMessages(ctx context.Context, docs []*schema.Document, query string) ([]*schema.Message, error) {
	buildChatContext := buildChatContext(docs)
	template := createTemplate()
	messages, err := template.Format(ctx, map[string]any{
		"context":  buildChatContext,
		"question": query,
	})
	return messages, err
}

func chat(ctx context.Context, chatModel *chatOpenAi.ChatModel, messages []*schema.Message) {
	streamMsgs, err := chatModel.Stream(ctx, messages)
	if err != nil {
		log.Fatalf("生成聊天结果失败: %v", err)
	}

	if err != nil {
		log.Fatalf("Stream of openai failed, err=%v", err)
	}

	defer streamMsgs.Close()

	for {
		msg, err := streamMsgs.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("Recv of streamMsgs failed, err=%v", err)
		}
		fmt.Print(msg.Content)
	}
}

// loadDocuments 加载文档
func loadDocuments(ctx context.Context) ([]*schema.Document, error) {
	loader, err := file.NewFileLoader(ctx, &file.FileLoaderConfig{
		UseNameAsID: true,
	})
	if err != nil {
		return nil, fmt.Errorf("创建文件加载器失败: %w", err)
	}

	filePath := "../data/tcm.txt"
	docs, err := loader.Load(ctx, document.Source{
		URI: filePath,
	})
	if err != nil {
		return nil, fmt.Errorf("加载文件失败: %w", err)
	}

	if len(docs) == 0 {
		return nil, fmt.Errorf("未加载到任何文档")
	}

	return docs, nil
}

// chunkDocuments 将文档按段落分块
func chunkDocuments(docs []*schema.Document) []*schema.Document {
	var chunkedDocs []*schema.Document

	for _, doc := range docs {
		content := doc.Content

		// 按段落分割（段落之间用双换行符分隔）
		paragraphs := strings.Split(content, "\n\n")

		// 遍历每个段落
		for idx, paragraph := range paragraphs {
			// 去除段落前后的空白字符
			paragraph = strings.TrimSpace(paragraph)

			// 跳过空段落
			if paragraph == "" {
				continue
			}

			// 创建段落文档块
			chunkDoc := &schema.Document{
				ID:      fmt.Sprintf("%s_para_%d", doc.ID, idx),
				Content: paragraph,
				MetaData: map[string]any{
					"source":          doc.ID,
					"paragraph_index": idx,
				},
			}
			chunkedDocs = append(chunkedDocs, chunkDoc)
			if idx == 10 {
				break
			}
		}
	}

	return chunkedDocs
}

// createESClient 创建 ES 客户端
func createESClient() (*elasticsearch.Client, error) {
	client, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{esAddress},
		Username:  esUsername,
		Password:  esPassword,
		//Logger:    &elastictransport.ColorLogger{Output: os.Stdout, EnableRequestBody: true, EnableResponseBody: true},
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

	// DashScope Embedding 模型配置
	embedder, err := openai.NewEmbedder(ctx, &openai.EmbeddingConfig{
		APIKey:  llmKey,
		Model:   embeddingModel,
		Timeout: 60 * time.Second, // 增加超时时间
		ByAzure: false,            // DashScope 不是 Azure
		BaseURL: llmApi,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 embedder 失败: %w", err)
	}

	log.Printf("  - 使用模型: %s", embeddingModel)

	return embedder, nil
}

// createChatModel 创建对话模型
func createChatModel(ctx context.Context) (*chatOpenAi.ChatModel, error) {
	// 创建 LLM
	llm, err := chatOpenAi.NewChatModel(ctx, &chatOpenAi.ChatModelConfig{
		APIKey:  llmKey,
		Model:   chatModel,
		Timeout: 60 * time.Second, // 添加超时
		BaseURL: llmApi,
	})
	return llm, err
}

// indexDocuments 索引文档到 ES
func indexDocuments(ctx context.Context, client *elasticsearch.Client, embedder embedding.Embedder, docs []*schema.Document) ([]string, error) {
	indexer, err := es8indexer.NewIndexer(ctx, &es8indexer.IndexerConfig{
		Client:    client,
		Index:     indexName,
		BatchSize: 10,
		DocumentToFields: func(ctx context.Context, doc *schema.Document) (map[string]es8indexer.FieldValue, error) {
			fields := map[string]es8indexer.FieldValue{
				fieldContent: {
					Value:    doc.Content,
					EmbedKey: fieldContentVector,
				},
				"id":              {Value: doc.ID},
				"paragraph_index": {Value: doc.MetaData["paragraph_index"]},
			}

			return fields, nil
		},
		Embedding: embedder,
	})
	if err != nil {
		return nil, fmt.Errorf("创建索引器失败: %w", err)
	}

	ids, err := indexer.Store(ctx, docs)
	if err != nil {
		return nil, fmt.Errorf("存储文档失败: %w", err)
	}

	return ids, nil
}

// demonstrateHybridSearch 演示混合搜索(向量检索 + BM25)
// 注意:如果ES没有企业许可证,RRF功能将不可用,这里使用不需要RRF的Hybrid模式
func demonstrateHybridSearch(ctx context.Context, client *elasticsearch.Client, embedder embedding.Embedder, query string) ([]*schema.Document, error) {
	// 创建混合检索器(不使用RRF,使用KNN+Filter混合模式)
	ret, err := es8retriever.NewRetriever(ctx, &es8retriever.RetrieverConfig{
		Client:    client,
		Index:     indexName,
		Embedding: embedder,
		TopK:      3, // 返回最相关的3个文档
		// 使用混合搜索模式:向量相似度 + BM25(不启用RRF以避免许可证问题)
		SearchMode: search_mode.SearchModeApproximate(&search_mode.ApproximateConfig{
			QueryFieldName:  fieldContent,       // BM25搜索字段
			VectorFieldName: fieldContentVector, // 向量字段
			Hybrid:          true,               // 启用混合搜索
			RRF:             false,              // 不启用RRF(避免许可证问题)
		}),
		// 自定义结果解析器
		ResultParser: func(ctx context.Context, hit types.Hit) (doc *schema.Document, err error) {
			if hit.Source_ == nil {
				return nil, fmt.Errorf("hit source is nil")
			}

			// 反序列化 JSON 源数据
			var source map[string]interface{}
			if err := json.Unmarshal(hit.Source_, &source); err != nil {
				return nil, fmt.Errorf("unmarshal source failed: %w", err)
			}

			// 解析文档内容
			content, ok := source[fieldContent].(string)
			if !ok {
				return nil, fmt.Errorf("content field not found or not a string")
			}

			// 获取文档 ID
			docID := ""
			if hit.Id_ != nil {
				docID = *hit.Id_
			}

			// 创建文档
			doc = &schema.Document{
				ID:       docID,
				Content:  content,
				MetaData: map[string]any{},
			}

			if hit.Score_ != nil {
				doc.WithScore(float64(*hit.Score_))
			}
			if source["paragraph_index"] != nil {
				doc.MetaData["paragraph_index"] = source["paragraph_index"].(float64)
			}
			return doc, nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("创建混合检索器失败: %w", err)
	}

	// 执行检索
	docs, err := ret.Retrieve(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("  检索失败: %v", err)
	}

	// 显示结果
	log.Printf("  找到 %d 个相关文档:", len(docs))
	for j, doc := range docs {
		content := doc.Content
		if len(content) > 100 {
			content = content[:100] + "..."
		}
		content = strings.ReplaceAll(content, "\n", " ")
		log.Printf("    %d. 混合分数: %.4f, 内容: %s", j+1, doc.Score(), content)
	}

	return docs, nil
}

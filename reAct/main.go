package main

import (
	"context"
	"log"
	"os"
	"time"

	ccb "github.com/cloudwego/eino-ext/callbacks/cozeloop"
	chatOpenAi "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/callbacks"
	"github.com/coze-dev/cozeloop-go"
)

func init() {
	os.Setenv("https_proxy", "http://127.0.0.1:33210")
	os.Setenv("http_proxy", "http://127.0.0.1:33210")
	os.Setenv("all_proxy", "socks5://127.0.0.1:33211")
}

var (
	llmKey    = os.Getenv("DASHSCOPE_API_KEY")
	llmApi    = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	chatModel = "qwen-plus"

	// RAG 相关配置
	indexName          = "eino_rag_demo"         // ES索引
	fieldContent       = "content"               // 内容字段
	fieldContentVector = "content_vector"        // 向量字段
	esAddress          = "http://localhost:9200" // ES 地址
	esUsername         = ""                      // ES 用户名
	esPassword         = ""                      // ES 密码
	embeddingModel     = "text-embedding-v3"     // Embedding 模型
)

// createChatModel 创建对话模型
func createChatModel(ctx context.Context) (*chatOpenAi.ChatModel, error) {
	llm, err := chatOpenAi.NewChatModel(ctx, &chatOpenAi.ChatModelConfig{
		APIKey:  llmKey,
		Model:   chatModel,
		Timeout: 60 * time.Second,
		BaseURL: llmApi,
	})
	return llm, err
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

	question := "我在海淀区，给我推荐一些菜，需要有口味辣一点的菜，至少推荐有 2 家餐厅"
	if err := ReActAgent(ctx, question); err != nil {
		log.Fatalf("运行 ReAct 代理失败: %v", err)
	}
	//  运行 自定义 ReAct agent
	// if err := ReActCustom(ctx, question); err != nil {
	// 	log.Fatalf("运行 ReAct 代理失败: %v", err)
	// }

	//  运行 ReAct RAG 代理
	// question = "感冒了有什么症状,并给出治疗建议"
	// if err := ReActRag(ctx, question); err != nil {
	// 	log.Fatalf("运行 ReAct RAG 代理失败: %v", err)
	// }
}

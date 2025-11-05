package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	chatOpenAi "github.com/cloudwego/eino-ext/components/model/openai"
	duckduckgo "github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

var (
	// 千问llm
	llmKey = os.Getenv("DASHSCOPE_API_KEY")
	llmApi = "https://dashscope.aliyuncs.com/compatible-mode/v1" //千问系列API

	embeddingModel = "text-embedding-v3" // Embedding 模型配置
	chatModel      = "qwen-plus"         // chat模型
)

func main() {
	ctx := context.Background()

	// Create configuration
	config := &duckduckgo.Config{
		MaxResults: 3, // Limit to return 3 results
		Region:     duckduckgo.RegionCN,
	}

	// Create search client
	textSearchTool, err := duckduckgo.NewTextSearchTool(ctx, config)
	if err != nil {
		log.Fatalf("NewTool of duckduckgo failed, err=%v", err)
	}
	// Create search request
	searchReq := &duckduckgo.TextSearchRequest{
		Query: "北京天气",
	}

	jsonReq, err := json.Marshal(searchReq)
	if err != nil {
		log.Fatalf("Marshal of search request failed, err=%v", err)
	}

	toolInfo, err := textSearchTool.Info(ctx)
	if err != nil {
		log.Fatalf("Info of duckduckgo failed, err=%v", err)
	}
	// Execute search
	resp, err := textSearchTool.InvokableRun(ctx, string(jsonReq))
	if err != nil {
		log.Fatalf("Search of duckduckgo failed, err=%v", err)
	}

	var searchResp duckduckgo.TextSearchResponse
	if err := json.Unmarshal([]byte(resp), &searchResp); err != nil {
		log.Fatalf("Unmarshal of search response failed, err=%v", err)
	}

	// Print results
	fmt.Println("Search Results:")
	fmt.Println("==============")
	for i, result := range searchResp.Results {
		fmt.Printf("\n%d. Title: %s\n", i+1, result.Title)
		fmt.Printf("   URL: %s\n", result.URL)
		fmt.Printf("\n%d. Summary: %s\n", i+1, result.Summary)
	}
	fmt.Println("")
	fmt.Println("==============")

	// 创建工具节点
	toolsNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
		Tools: []tool.BaseTool{
			textSearchTool,
		},
	})
	if err != nil {
		panic(err)
	}

	// // Mock LLM 输出作为输入
	// input := &schema.Message{
	// 	Role: schema.Assistant,
	// 	ToolCalls: []schema.ToolCall{
	// 		{
	// 			Function: schema.FunctionCall{
	// 				Name:      "duckduckgo_text_search",
	// 				Arguments: `{"query": "深圳", "date": "tomorrow"}`,
	// 			},
	// 		},
	// 	},
	// }

	// toolMessages, err := toolsNode.Invoke(ctx, input)
	// fmt.Println("tool messages: ", toolMessages)

	chatModel, err := createChatModel(ctx)
	if err != nil {
		log.Fatalf("Failed to create chat model: %v", err)
	}
	chatModel.BindTools([]*schema.ToolInfo{toolInfo})

	// Build the chain with the ChatModel and the Tools node.
	// First the tools node, then the chat model to properly handle the types
	chain := compose.NewChain[[]*schema.Message, []*schema.Message]()
	chain.
		AppendToolsNode(toolsNode, compose.WithNodeName("search")).
		AppendChatModel(chatModel, compose.WithNodeName("chat_model"))

	// Compile the chain to obtain the agent.
	agent, err := chain.Compile(ctx)
	if err != nil {
		log.Fatalf("Failed to compile chain: %v", err)
	}
	outMsg, err := agent.Invoke(ctx, []*schema.Message{{
		Role:    schema.User,
		Content: "查询北京天气,给出建议",
	}})

	if err != nil {
		log.Fatalf("Failed to invoke agent: %v", err)
	}

	// Since the output is []*schema.Message, we need to print the content of the first message
	if len(outMsg) > 0 {
		fmt.Println("outMsg: ", outMsg[0].Content)
	} else {
		fmt.Println("No output message received")
	}
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

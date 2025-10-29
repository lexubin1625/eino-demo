package main

import (
	"context"
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
	llmKey    = os.Getenv("DASHSCOPE_API_KEY")
	llmApi    = "https://dashscope.aliyuncs.com/compatible-mode/v1" //千问系列API
	chatModel = "qwen-plus"                                         // chat模型
)

func main() {
	ctx := context.Background()

	if llmKey == "" {
		log.Fatal("DASHSCOPE_API_KEY 未设置，请在环境变量中配置后再运行")
	}

	// 读取用户题目：命令行参数或交互输入
	question := "一个矩形的长是宽的2倍，周长是30厘米，求长和宽分别是多少？"

	// Create search client
	textSearchTool, err := duckduckgo.NewTextSearchTool(ctx, &duckduckgo.Config{
		MaxResults: 3,                   // Limit to return 3 results to reduce load
		Region:     duckduckgo.RegionUS, // Use US region to avoid 202 issues
	})
	if err != nil {
		log.Fatalf("NewTool of duckduckgo failed, err=%v", err)
	}

	// Create tool info
	toolInfo, err := textSearchTool.Info(ctx)
	if err != nil {
		log.Fatalf("Info of duckduckgo failed, err=%v", err)
	}

	// Create tools node
	toolsNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
		Tools: []tool.BaseTool{
			textSearchTool,
		},
	})
	if err != nil {
		log.Fatalf("Failed to create tools node: %v", err)
	}

	// Create chat model
	chatModel, err := createChatModel(ctx)
	if err != nil {
		log.Fatalf("Failed to create chat model: %v", err)
	}
	chatModel.BindTools([]*schema.ToolInfo{toolInfo})

	// 使用一个 Graph：START → tools → build_messages(lambda) → chat_model → END
	graph := compose.NewGraph[*schema.Message, *schema.Message]()
	_ = graph.AddToolsNode("tools", toolsNode, compose.WithNodeName("tools"))
	buildMessages := compose.InvokableLambda(func(ctx context.Context, toolMsgs []*schema.Message) ([]*schema.Message, error) {
		// 打印工具返回结果（在同一个 Graph 流程中）
		fmt.Println("工具返回结果：")
		var searchContext string
		for i, m := range toolMsgs {
			fmt.Printf("  [%d] role=%s\n", i+1, m.Role)
			if m.Content != "" {
				fmt.Println(m.Content)
				searchContext += m.Content + "\n\n"
			}
		}

		// 构造喂给聊天模型的上下文
		messages := []*schema.Message{
			{
				Role:    schema.System,
				Content: "仅基于给定的搜索内容中文分步解题，并在答案结尾给出来源链接。不得使用外部知识。答案通俗易懂,不要包含特殊格式",
			},
			{
				Role:    schema.User,
				Content: fmt.Sprintf("题目：%s\n\n搜索内容：\n%s", question, searchContext),
			},
		}
		return messages, nil
	})
	_ = graph.AddLambdaNode("build_messages", buildMessages, compose.WithNodeName("build_messages"))
	_ = graph.AddChatModelNode("chat_model", chatModel, compose.WithNodeName("chat_model"))
	_ = graph.AddEdge(compose.START, "tools")
	_ = graph.AddEdge("tools", "build_messages")
	_ = graph.AddEdge("build_messages", "chat_model")
	_ = graph.AddEdge("chat_model", compose.END)

	// 编译 Graph 得到 agent
	agent, err := graph.Compile(ctx, compose.WithMaxRunSteps(10))
	if err != nil {
		log.Fatalf("Failed to compile graph: %v", err)
	}

	// 构造 assistant.tool_calls，直接触发搜索
	toolCallMsg := &schema.Message{
		Role:    schema.Assistant,
		Content: "",
		ToolCalls: []schema.ToolCall{
			{
				Function: schema.FunctionCall{
					Name:      toolInfo.Name,
					Arguments: fmt.Sprintf(`{"query": %q}`, question),
				},
			},
		},
	}

	finalMsg, err := agent.Invoke(ctx, toolCallMsg)
	if err != nil {
		fmt.Println("流程失败：", err)
		return
	}
	fmt.Println("解题过程和答案：")
	fmt.Print(finalMsg.Content)
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

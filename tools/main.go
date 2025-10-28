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
	llmKey    = os.Getenv("DASHSCOPE_API_KEY")
	llmApi    = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	chatModel = "qwen-plus"
)

func main() {

}

func search() {
	ctx := context.Background()

	// 网页查询工具
	textSearchTool, err := duckduckgo.NewTextSearchTool(ctx, &duckduckgo.Config{
		MaxResults: 3,
		Region:     duckduckgo.RegionUS,
	})
	if err != nil {
		log.Fatalf("NewTool of duckduckgo failed, err=%v", err)
	}
	toolInfo, _ := textSearchTool.Info(ctx)

	// 创建
	cm, err := createChatModel(ctx)
	if err != nil {
		log.Fatalf("Failed to create chat model: %v", err)
	}
	cm.BindTools([]*schema.ToolInfo{toolInfo})

	// 创建工具节点，用于执行模型发起的 tool_calls
	toolsNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{Tools: []tool.BaseTool{textSearchTool}})
	if err != nil {
		log.Fatalf("创建工具节点失败: %v", err)
	}

	// 3) 构造用户消息，提示模型可以调用工具
	messages := []*schema.Message{
		{Role: schema.System, Content: "你可以使用提供的工具回答问题。尽量检索最新网页再给出结论。"},
		{Role: schema.User, Content: "查询北京天气，并给出出行建议。"},
	}

	var toolOutMsgs []*schema.Message // 工具输出消息
	var toolsMessage *schema.Message  // 工具会话消息
	// 直接拼装function调用
	// toolsMessage := &schema.Message{
	// 	Role:    schema.Assistant,
	// 	Content: "",
	// 	ToolCalls: []schema.ToolCall{
	// 		{
	// 			Function: schema.FunctionCall{
	// 				Name:      toolInfo.Name,
	// 				Arguments: fmt.Sprintf(`{"query": %q}`, "北京天气"),
	// 			},
	// 		},
	// 	},
	// }
	// toolOutMsgs, err = toolsNode.Invoke(ctx, toolsMessage)

	//  模型生成，若包含 tool_calls 则执行工具
	firstResp, err := cm.Generate(ctx, messages)
	if err != nil {
		log.Fatalf("模型生成失败: %v", err)
	}

	if len(firstResp.ToolCalls) > 0 {
		toolsMessage = &schema.Message{Role: schema.Assistant, ToolCalls: firstResp.ToolCalls}
		toolOutMsgs, err = toolsNode.Invoke(ctx, toolsMessage)
		if err != nil {
			log.Fatalf("工具执行失败: %v", err)
		}
	}

	fmt.Println("工具返回结果：")
	for i, m := range toolOutMsgs {
		fmt.Printf("  [%d] role=%s\n", i+1, m.Role)
		if m.Content != "" {
			fmt.Println(m.Content)
		}
	}
	fmt.Println("")

	// 组织工具结果进入上下文，再次让模型生成最终回答
	finalMessages := messages
	finalMessages = append(finalMessages, toolsMessage)
	finalMessages = append(finalMessages, toolOutMsgs...)

	finalResp, err := cm.Generate(ctx, finalMessages)
	if err != nil {
		log.Fatalf("最终生成失败: %v", err)
	}

	fmt.Println("最终回答：")
	fmt.Println(finalResp.Content)
}

func createChatModel(ctx context.Context) (*chatOpenAi.ChatModel, error) {
	llm, err := chatOpenAi.NewChatModel(ctx, &chatOpenAi.ChatModelConfig{
		APIKey:  llmKey,
		Model:   chatModel,
		Timeout: 60 * time.Second,
		BaseURL: llmApi,
	})
	return llm, err
}

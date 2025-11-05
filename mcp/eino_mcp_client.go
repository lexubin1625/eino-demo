package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	chatOpenAi "github.com/cloudwego/eino-ext/components/model/openai"
	mcpp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func einoMcpClient() {
	ctx := context.Background()
	mcpTools := getMCPTool(ctx)

	// 创建工具节点，用于执行工具调用
	toolsNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{Tools: mcpTools})
	if err != nil {
		log.Fatalf("创建工具节点失败: %v", err)
	}

	// 创建模型并绑定工具信息（模型需要 ToolInfo 进行参数对齐）
	cm, err := createChatModel(ctx)
	if err != nil {
		log.Fatalf("创建聊天模型失败: %v", err)
	}
	var toolInfos []*schema.ToolInfo
	for _, t := range mcpTools {
		info, err := t.Info(ctx)
		if err != nil {
			log.Fatalf("获取工具信息失败: %v", err)
		}
		toolInfos = append(toolInfos, info)
	}
	if err := cm.BindTools(toolInfos); err != nil {
		log.Fatalf("绑定工具到模型失败: %v", err)
	}

	messages := []*schema.Message{
		{Role: schema.System, Content: "你可使用提供的工具回答问题。尽量直接查出"},
		{Role: schema.User, Content: "查询北京经纬度"},
	}
	//  模型生成，若包含 tool_calls 则执行工具
	firstResp, err := cm.Generate(ctx, messages)
	if err != nil {
		log.Fatalf("模型生成失败: %v", err)
	}

	fmt.Println("First ToolCalls:", firstResp.ToolCalls)

	var toolOutMsgs []*schema.Message
	if len(firstResp.ToolCalls) > 0 {
		toolOutMsgs, err = toolsNode.Invoke(ctx, &schema.Message{Role: schema.Assistant, ToolCalls: firstResp.ToolCalls})
	}
	if err != nil {
		log.Fatalf("工具执行失败: %v", err)
	}

	fmt.Println("工具返回结果：")
	for i, m := range toolOutMsgs {
		fmt.Printf("  [%d] role=%s\n", i+1, m.Role)
		if m.Content != "" {
			fmt.Println(m.Content)
		}
	}
	fmt.Println("")

	printToolDesc(ctx, mcpTools)
}

func printToolDesc(ctx context.Context, mcpTools []tool.BaseTool) {
	// 显示tool列表
	for _, mcpTool := range mcpTools {
		info, err := mcpTool.Info(ctx)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("Name:", info.Name)
		fmt.Println("Desc:", info.Desc)
		params, _ := info.ParamsOneOf.ToJSONSchema()
		// 尝试输出更可读的参数约束（如有）
		if b, err := json.MarshalIndent(params, "", "  "); err == nil {
			fmt.Println("ParamsOneOf(JSONSchema):")
			fmt.Println(string(b))
		} else {
			fmt.Printf("ParamsOneOf: %#v\n", params)
		}

		// 根据工具名称选择更可能正确的示例参数
		// var args string
		// switch info.Name {
		// case "maps_direction_bicycling":
		// 	// 很多地图路径规划接口要求坐标对象包含 x/y 或 lng/lat
		// 	// 这里提供北京丰台→朝阳的大致坐标示例（请按 JSONSchema 调整）
		// 	args = `{"origin":"116.287,39.865","destination":"116.469,39.921"}`
		// default:
		// 	// 回退到简单文本地址形式（若工具支持地址解析）
		// 	args = `{"origin":"北京市丰台区","destination":"北京市朝阳区"}`
		// }

		// result, err := mcpTool.(tool.InvokableTool).InvokableRun(ctx, args)
		// if err != nil {
		// 	fmt.Println("failed to call mcp tool:", err)
		// 	// 不中断后续工具执行
		// 	continue
		// }
		//fmt.Println("Result:", result)
		fmt.Println()
	}
}
func getMCPTool(ctx context.Context) []tool.BaseTool {
	client, err := client.NewSSEMCPClient(fmt.Sprintf(amapUrl, amapApiKey))
	if err != nil {
		log.Fatal(err)
	}
	err = client.Start(ctx)
	if err != nil {
		log.Fatal(err)
	}

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "example-client",
		Version: "1.0.0",
	}

	_, err = client.Initialize(ctx, initRequest)
	if err != nil {
		log.Fatal(err)
	}

	tools, err := mcpp.GetTools(ctx, &mcpp.Config{Cli: client})
	if err != nil {
		log.Fatal(err)
	}

	return tools
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

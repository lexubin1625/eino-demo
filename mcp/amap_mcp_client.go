package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	openai "github.com/sashabaranov/go-openai"
)

var (
	amapApiKey = os.Getenv("AMAP_API_KEY")
	amapUrl    = "https://mcp.amap.com/sse?key=%s"
)

func amapMCPClient() {
	ctx := context.Background()

	// 创建MCP客户端连接
	mcpClient, err := client.NewSSEMCPClient(fmt.Sprintf(amapUrl, amapApiKey))
	if err != nil {
		log.Fatalf("Failed to create MCP client: %v", err)
	}

	defer mcpClient.Close()

	// 启动客户端
	err = mcpClient.Start(ctx)
	if err != nil {
		log.Fatalf("Failed to start MCP client: %v", err)
	}

	// 初始化请求
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "amap-mcp-client",
		Version: "1.0.0",
	}

	_, err = mcpClient.Initialize(ctx, initRequest)
	if err != nil {
		log.Fatalf("Failed to initialize MCP client: %v", err)
	}

	fmt.Println("\n=== 通过对话选择并调用工具（示例：查询北京天气） ===")

	listToolsResult, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		log.Fatalf("Failed to list tools: %v", err)
	}

	// 组织工具信息（名称、描述、参数）
	var toolsDesc []string
	for _, t := range listToolsResult.Tools {
		td := map[string]any{
			"name":        t.Name,
			"desc":        t.Description,
			"inputSchema": t.InputSchema,
		}
		toolJSON, _ := json.MarshalIndent(t, "", "  ")
		fmt.Printf("Tool[%d]: %s\n", len(toolsDesc), string(toolJSON))
		b, _ := json.Marshal(td)
		toolsDesc = append(toolsDesc, string(b))
	}

	// 首选使用 LLM 决策；若无 API Key 则关键词回退
	userInput := "查询朝阳公园到奥森公园的骑行路线"
	choice, err := chooseToolByLLM(ctx, toolsDesc, userInput)
	if err != nil || choice == nil || choice.Tool == "" {
		log.Fatalf("Failed to choose tool by LLM: %v", err)
	}

	// 生成并调用 MCP 工具
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      choice.Tool,
			Arguments: choice.Arguments,
		},
	}
	result, err := mcpClient.CallTool(ctx, req)
	if err != nil {
		log.Fatalf("Failed to call tool %s: %v", choice.Tool, err)
	}

	PrintToolResult(result)
}

// PrintToolResult 打印工具调用结果
func PrintToolResult(result *mcp.CallToolResult) {
	if result == nil {
		fmt.Println("No result returned")
		return
	}

	fmt.Printf("Tool Result:\n")
	if result.Content != nil {
		for i, content := range result.Content {
			fmt.Printf("Content[%d]: %+v\n", i, content)
		}
	}

	if result.IsError {
		fmt.Printf("Error occurred in tool execution\n")
	}
}

// ToolChoice 由模型决定使用的工具及参数
type ToolChoice struct {
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
}

// chooseToolByLLM 使用开放式对话模型选择工具
func chooseToolByLLM(ctx context.Context, toolsDesc []string, userInput string) (*ToolChoice, error) {

	llmConfig := openai.DefaultConfig(llmKey)
	llmConfig.BaseURL = llmApi
	llm := openai.NewClientWithConfig(llmConfig)

	sys := "你是一位工具选择助手。根据用户问题和提供的工具列表，选择最合适的一个工具，并以严格的JSON输出：{\"tool\": <工具名>, \"arguments\": <参数对象>}。不要输出除JSON以外的内容。"
	toolList := strings.Join(toolsDesc, "\n")
	prompt := fmt.Sprintf("可用工具列表(包含名称、描述、输入Schema)：\n%s\n\n用户问题：%s\n\n仅输出JSON。", toolList, userInput)

	resp, err := llm.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: chatModel,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: sys},
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
		Temperature: 0.0,
	})
	if err != nil || len(resp.Choices) == 0 {
		return nil, fmt.Errorf("llm completion failed: %v", err)
	}
	content := resp.Choices[0].Message.Content
	var choice ToolChoice
	if err := json.Unmarshal([]byte(content), &choice); err != nil {
		return nil, fmt.Errorf("parse llm json failed: %w", err)
	}
	return &choice, nil
}

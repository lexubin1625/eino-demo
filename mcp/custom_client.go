package main

import (
	"context"
	"fmt"
	"log"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func customClient() {
	// 创建 StreamableHTTP 客户端
	c, err := client.NewStreamableHttpClient("http://localhost:8080/mcp/")
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	ctx := context.Background()

	// 初始化
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "HTTP Test Client",
		Version: "1.0.0",
	}

	initResult, err := c.Initialize(ctx, initReq)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Connected to server: %s %s\n", initResult.ServerInfo.Name, initResult.ServerInfo.Version)

	// 列出可用的 Tools
	toolsResult, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\nAvailable Tools (%d):\n", len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		fmt.Printf("- %s: %s\n", tool.Name, tool.Description)
	}

	// 列出可用的 Resources
	resourcesResult, err := c.ListResources(ctx, mcp.ListResourcesRequest{})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\nAvailable Resources (%d):\n", len(resourcesResult.Resources))
	for _, resource := range resourcesResult.Resources {
		fmt.Printf("- %s: %s (MIME: %s)\n", resource.URI, resource.Name, resource.MIMEType)
	}

	// 列出可用的 Prompts
	promptsResult, err := c.ListPrompts(ctx, mcp.ListPromptsRequest{})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\nAvailable Prompts (%d):\n", len(promptsResult.Prompts))
	for _, prompt := range promptsResult.Prompts {
		fmt.Printf("- %s: %s\n", prompt.Name, prompt.Description)
	}

	// 测试调用 calculate 工具
	fmt.Println("\n=== Testing calculate tool ===")
	callReq := mcp.CallToolRequest{}
	callReq.Params.Name = "calculate"
	callReq.Params.Arguments = map[string]interface{}{
		"operation": "add",
		"a":         10.0,
		"b":         5.0,
	}

	toolResult, err := c.CallTool(ctx, callReq)
	if err != nil {
		log.Fatal(err)
	}

	// 访问 TextContent 的正确方式
	if len(toolResult.Content) > 0 {
		if textContent, ok := toolResult.Content[0].(mcp.TextContent); ok {
			fmt.Printf("Calculate tool result: %+v\n", textContent.Text)
		} else {
			fmt.Printf("Calculate tool result: %+v\n", toolResult.Content[0])
		}
	}

	// 测试读取服务器配置资源
	fmt.Println("\n=== Testing config resource read ===")
	readReq := mcp.ReadResourceRequest{}
	readReq.Params.URI = "config://server"

	resourceContents, err := c.ReadResource(ctx, readReq)
	if err != nil {
		log.Fatal(err)
	}

	// 访问 TextResourceContents 的正确方式
	if len(resourceContents.Contents) > 0 {
		if textResource, ok := resourceContents.Contents[0].(mcp.TextResourceContents); ok {
			fmt.Printf("Server config resource content: %+v\n", textResource.Text)
		} else {
			fmt.Printf("Server config resource content: %+v\n", resourceContents.Contents[0])
		}
	}

	// 测试获取代码审查 Prompt
	fmt.Println("\n=== Testing code review prompt ===")
	promptReq := mcp.GetPromptRequest{}
	promptReq.Params.Name = "code_review"
	promptReq.Params.Arguments = map[string]string{
		"code": "func add(a, b int) int { return a + b }",
	}

	promptResult, err := c.GetPrompt(ctx, promptReq)
	if err != nil {
		log.Fatal(err)
	}

	// 访问 PromptMessage Content 的正确方式
	if len(promptResult.Messages) > 0 {
		if textContent, ok := promptResult.Messages[0].Content.(mcp.TextContent); ok {
			fmt.Printf("Code review prompt result: %s\n", textContent.Text)
		} else {
			fmt.Printf("Code review prompt result: %s\n", promptResult.Messages[0].Content)
		}
	}
}

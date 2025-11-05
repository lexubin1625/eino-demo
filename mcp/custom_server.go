package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func customServer() {
	// 创建一个支持 Resources、Tools 和 Prompts 的 MCP Server
	s := server.NewMCPServer("Custom MCP Server", "1.0.0",
		server.WithResourceCapabilities(true, true), // 支持静态和动态资源
		server.WithPromptCapabilities(true),         // 支持 Prompts
		server.WithToolCapabilities(true),           // 支持 Tools
		server.WithLogging(),                        // 启用日志
	)

	// 添加 Tools
	addTools(s)

	// 添加 Resources
	addResources(s)

	// 添加 Prompts
	addPrompts(s)

	// 创建自定义 HTTP 服务器，添加 MCP 处理器和健康检查端点
	mux := http.NewServeMux()

	// 添加 MCP 处理器
	mcpHandler := server.NewStreamableHTTPServer(s)
	mux.Handle("/mcp/", mcpHandler)

	// 添加健康检查端点
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "healthy",
			"server": "Custom MCP Server",
		})
	})

	// 添加 capabilities 端点
	mux.HandleFunc("/capabilities", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		capabilities := map[string]interface{}{
			"resources": true,
			"tools":     true,
			"prompts":   true,
		}
		json.NewEncoder(w).Encode(capabilities)
	})

	// 启动 Server
	fmt.Println("Starting Custom MCP Server on :8080...")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}

// 添加 Tools 到 Server
func addTools(s *server.MCPServer) {
	// 添加一个简单的计算器工具
	s.AddTool(
		mcp.NewTool("calculate",
			mcp.WithDescription("执行基本数学计算"),
			mcp.WithString("operation", mcp.Required(), mcp.Enum("add", "sub", "mul", "div")),
			mcp.WithNumber("a", mcp.Required()),
			mcp.WithNumber("b", mcp.Required()),
		),
		handleCalculate,
	)
}

// 添加 Resources 到 Server
func addResources(s *server.MCPServer) {
	// 添加静态资源配置信息资源
	s.AddResource(
		mcp.NewResource(
			"config://server",
			"服务器配置",
			mcp.WithResourceDescription("当前服务器配置信息"),
			mcp.WithMIMEType("application/json"),
		),
		handleServerConfig,
	)
}

// 添加 Prompts 到 Server
func addPrompts(s *server.MCPServer) {
	// 添加代码审查 Prompt
	s.AddPrompt(
		mcp.NewPrompt("code_review",
			mcp.WithPromptDescription("代码审查助手"),
			mcp.WithArgument("code",
				mcp.ArgumentDescription("需要审查的代码"),
				mcp.RequiredArgument(),
			),
			mcp.WithArgument("language",
				mcp.ArgumentDescription("编程语言"),
			),
		),
		handleCodeReview,
	)
}

// Tool 处理函数
func handleCalculate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	op := req.GetString("operation", "")
	a := req.GetFloat("a", 0)
	b := req.GetFloat("b", 0)

	var result float64
	switch op {
	case "add":
		result = a + b
	case "sub":
		result = a - b
	case "mul":
		result = a * b
	case "div":
		if b == 0 {
			return mcp.NewToolResultError("除数不能为零"), nil
		}
		result = a / b
	default:
		return mcp.NewToolResultError("不支持的操作: " + op), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("计算结果: %.2f", result)), nil
}

// Resource 处理函数
func handleServerConfig(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	config := map[string]interface{}{
		"name":    "Custom MCP Server",
		"version": "1.0.0",
		"uptime":  time.Now().String(),
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(configJSON),
		},
	}, nil
}

// Prompt 处理函数
func handleCodeReview(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	code := ""
	if args := req.Params.Arguments; args != nil {
		if c, ok := args["code"]; ok {
			code = c
		}
	}

	return &mcp.GetPromptResult{
		Description: "代码审查",
		Messages: []mcp.PromptMessage{
			{
				Role: mcp.RoleUser,
				Content: mcp.NewTextContent(fmt.Sprintf(
					"请审查以下代码并提供改进建议：\n\n``code\n%s\n```\n\n请关注代码质量、最佳实践和潜在问题。",
					code,
				)),
			},
		},
	}, nil
}

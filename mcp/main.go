package main

import (
	"os"
)

var (
	llmKey    = os.Getenv("DASHSCOPE_API_KEY")
	llmApi    = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	chatModel = "qwen-plus"
)

func main() {
	// 检查命令行参数来决定运行哪个功能
	if len(os.Args) == 0 {
		panic("请指定运行模式: custom-server, custom-client, eino-client")
	}
	switch os.Args[1] {
	case "custom-server":
		// 启动自定义 MCP Server
		customServer()
	case "custom-client":
		// 运行HTTP测试客户端
		customClient()
	case "eino-client":
		einoMcpClient()
	default:
		amapMCPClient()
	}
}

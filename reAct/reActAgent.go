package main

import (
	"context"
	"fmt"

	duckduckgo "github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

// ReActAgent 使用 eino 的 react.NewAgent 实现 ReAct 代理
func ReActAgent(ctx context.Context, question string) error {
	// 1. 创建聊天模型
	cm, err := createChatModel(ctx)
	if err != nil {
		return fmt.Errorf("创建聊天模型失败: %w", err)
	}

	// 2. 创建搜索工具
	searchTool, err := duckduckgo.NewTextSearchTool(ctx, &duckduckgo.Config{
		MaxResults: 3,
		Region:     duckduckgo.RegionCN,
	})
	if err != nil {
		return fmt.Errorf("创建搜索工具失败: %w", err)
	}

	// 3. 创建 ReAct Agent
	// react.NewAgent 会自动处理：思考 -> 行动 -> 观察 -> 再思考的循环
	agent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: cm, // 支持工具调用的模型
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: []tool.BaseTool{searchTool},
		},
		MaxStep: 10, // 最多迭代 10 次
		MessageModifier: react.NewPersonaModifier(`你是一个 ReAct（Reasoning and Acting）代理。

工作流程：
1. **思考（Think）**：分析用户问题，确定需要什么信息
2. **行动（Act）**：如果需要外部信息，调用搜索工具获取
3. **观察（Observe）**：分析工具返回的结果
4. **再思考**：基于观察结果，决定是否需要更多信息或可以给出最终答案
5. **回答**：当有足够信息时，给出完整、准确的答案

规则：
- 对于需要实时信息的问题（如天气、新闻、最新数据、餐厅推荐），必须使用搜索工具
- 对于一般知识问题，可以直接回答
- 在调用工具前，先思考为什么需要这个工具
- 工具返回结果后，仔细分析并决定下一步行动
- 最终答案要基于工具返回的事实，不要编造信息`),
	})
	if err != nil {
		return fmt.Errorf("创建 ReAct Agent 失败: %w", err)
	}

	// 4. 执行代理
	fmt.Println("ReAct Agent 开始工作（使用 react.NewAgent）")
	fmt.Printf("问题: %s\n\n", question)
	// 调用 Generate 方法执行 ReAct 循环
	response, err := agent.Generate(ctx, []*schema.Message{
		{Role: schema.User, Content: question},
	})
	if err != nil {
		return fmt.Errorf("Agent 执行失败: %w", err)
	}

	// 5. 输出结果
	fmt.Println("最终答案:")
	if response != nil && response.Content != "" {
		fmt.Println(response.Content)
	} else {
		fmt.Println("未能生成最终答案")
	}

	return nil
}

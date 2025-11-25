package main

import (
	"context"
	"fmt"

	duckduckgo "github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// ReActCustom 运行 ReAct 代理
// ReAct 模式：Reasoning（推理） + Acting（行动）
// 循环进行：思考问题 -> 调用工具 -> 观察结果 -> 继续思考，直到得到最终答案
func ReActCustom(ctx context.Context, question string) error {
	// 1. 创建聊天模型
	cm, err := createChatModel(ctx)
	if err != nil {
		return fmt.Errorf("创建聊天模型失败: %w", err)
	}

	// 2. 创建搜索工具（用于行动阶段）
	searchTool, err := duckduckgo.NewTextSearchTool(ctx, &duckduckgo.Config{
		MaxResults: 3,
		Region:     duckduckgo.RegionCN,
	})
	if err != nil {
		return fmt.Errorf("创建搜索工具失败: %w", err)
	}

	// 3. 获取工具信息并绑定到模型
	toolInfo, err := searchTool.Info(ctx)
	if err != nil {
		return fmt.Errorf("获取工具信息失败: %w", err)
	}

	// 4. 绑定工具到模型（让模型知道可以调用哪些工具）
	if err := cm.BindTools([]*schema.ToolInfo{toolInfo}); err != nil {
		return fmt.Errorf("绑定工具失败: %w", err)
	}

	// 5. 创建工具节点，用于执行工具调用
	toolsNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
		Tools: []tool.BaseTool{searchTool},
	})
	if err != nil {
		return fmt.Errorf("创建工具节点失败: %w", err)
	}

	// 6. 初始化消息历史
	messages := []*schema.Message{
		{
			Role: schema.System,
			Content: `你是一个 ReAct（Reasoning and Acting）代理。

工作流程：
1. **思考（Think）**：分析用户问题，确定需要什么信息
2. **行动（Act）**：如果需要外部信息，调用搜索工具获取
3. **观察（Observe）**：分析工具返回的结果
4. **再思考**：基于观察结果，决定是否需要更多信息或可以给出最终答案
5. **回答**：当有足够信息时，给出完整、准确的答案

规则：
- 对于需要实时信息的问题（如天气、新闻、最新数据），必须使用搜索工具
- 对于一般知识问题，可以直接回答
- 在调用工具前，先思考为什么需要这个工具
- 工具返回结果后，仔细分析并决定下一步行动
- 最终答案要基于工具返回的事实，不要编造信息`,
		},
		{
			Role:    schema.User,
			Content: question,
		},
	}

	// 7. ReAct 循环：最多迭代 10 次
	fmt.Println("ReAct 代理开始工作")
	fmt.Printf("问题: %s\n\n", question)

	maxIterations := 10
	var finalAnswer string

	for iteration := 0; iteration < maxIterations; iteration++ {
		fmt.Printf("\n[迭代 %d] 思考阶段 - 分析问题...\n", iteration+1)

		// 思考阶段：模型生成响应（可能包含工具调用）
		resp, err := cm.Generate(ctx, messages)
		if err != nil {
			return fmt.Errorf("模型生成失败: %w", err)
		}

		// 检查是否有工具调用（行动阶段）
		if len(resp.ToolCalls) > 0 {
			fmt.Printf("\n[迭代 %d] 行动阶段 - 调用工具:\n", iteration+1)
			for _, tc := range resp.ToolCalls {
				fmt.Printf("  - 工具: %s\n", tc.Function.Name)
				fmt.Printf("    参数: %s\n", tc.Function.Arguments)
			}

			// 将模型的响应（包含工具调用）添加到消息历史
			messages = append(messages, &schema.Message{
				Role:      schema.Assistant,
				Content:   resp.Content,
				ToolCalls: resp.ToolCalls,
			})

			// 执行工具调用
			toolMessage := &schema.Message{
				Role:      schema.Assistant,
				ToolCalls: resp.ToolCalls,
			}

			fmt.Printf("\n[迭代 %d] 观察阶段 - 执行工具并获取结果...\n", iteration+1)

			// 执行工具调用：toolsNode.Invoke 会执行工具并返回结果
			// 返回的 toolResults 是 []*schema.Message 类型
			// 每个消息的 Role 通常是 schema.Tool，Content 包含工具返回的数据
			toolResults, err := toolsNode.Invoke(ctx, toolMessage)
			if err != nil {
				return fmt.Errorf("工具执行失败: %w", err)
			}

			// 显示观察结果（工具返回的数据）
			fmt.Printf("  收到 %d 个工具结果:\n", len(toolResults))
			for i, tr := range toolResults {
				fmt.Printf("  [结果 %d] role=%s", i+1, tr.Role)
				if tr.ToolCallID != "" {
					fmt.Printf(", tool_call_id=%s", tr.ToolCallID)
				}
				fmt.Println()

				// 显示工具返回的内容
				if tr.Content != "" {
					// 截断过长的内容以便显示
					content := tr.Content
					displayContent := content
					if len(content) > 500 {
						displayContent = content[:500] + "..."
					}
					fmt.Printf("    内容预览: %s\n", displayContent)
					fmt.Printf("    内容长度: %d 字符\n", len(content))
				}
			}

			// 关键：将工具结果（观察结果）添加到消息历史
			// 这样在下一轮循环中，模型就能看到这些观察结果，并基于它们进行下一步思考
			messages = append(messages, toolResults...)

			fmt.Printf("  ✓ 观察结果已添加到消息历史，模型将在下一轮思考中使用这些信息\n")

			// 继续下一轮循环（思考 -> 行动 -> 观察）
			continue
		}

		// 没有工具调用，说明模型已经给出最终答案
		if resp.Content != "" {
			finalAnswer = resp.Content
			fmt.Printf("\n[迭代 %d] 最终答案阶段 - 生成回答\n", iteration+1)
			break
		}

		// 如果既没有工具调用也没有内容，可能是错误
		if resp.Content == "" && len(resp.ToolCalls) == 0 {
			return fmt.Errorf("模型未返回内容或工具调用")
		}
	}

	// 8. 输出最终答案
	fmt.Println("最终答案:")
	if finalAnswer != "" {
		fmt.Println(finalAnswer)
	} else {
		fmt.Println("未能生成最终答案（可能达到最大迭代次数）")
	}

	return nil
}

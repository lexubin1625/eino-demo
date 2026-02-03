package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	chatOpenAi "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

var (
	llmKey    = os.Getenv("DASHSCOPE_API_KEY")
	llmApi    = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	chatModel = "qwen-plus"
)

// createChatModel 创建对话模型
func createChatModel(ctx context.Context) (*chatOpenAi.ChatModel, error) {
	llm, err := chatOpenAi.NewChatModel(ctx, &chatOpenAi.ChatModelConfig{
		APIKey:  llmKey,
		Model:   chatModel,
		Timeout: 60 * time.Second,
		BaseURL: llmApi,
	})
	return llm, err
}

// registerAllSkills 注册所有技能
func registerAllSkills(sm *SkillManager) error {
	// 注册计算器技能
	calcSkill, err := CalculatorSkill()
	if err != nil {
		return fmt.Errorf("注册计算器技能失败: %w", err)
	}
	if err := sm.RegisterSkill("calculator", calcSkill); err != nil {
		return err
	}

	// 注册文本处理技能
	textSkill, err := TextProcessingSkill()
	if err != nil {
		return fmt.Errorf("注册文本处理技能失败: %w", err)
	}
	if err := sm.RegisterSkill("text_processing", textSkill); err != nil {
		return err
	}

	// 注册数据分析技能
	dataSkill, err := DataAnalysisSkill()
	if err != nil {
		return fmt.Errorf("注册数据分析技能失败: %w", err)
	}
	if err := sm.RegisterSkill("data_analysis", dataSkill); err != nil {
		return err
	}

	return nil
}

func main() {
	ctx := context.Background()

	// 1. 创建技能管理器并注册所有技能
	skillManager := NewSkillManager()
	if err := registerAllSkills(skillManager); err != nil {
		log.Fatalf("注册技能失败: %v", err)
	}

	fmt.Println("已注册的技能:")
	for _, name := range skillManager.ListSkills() {
		fmt.Printf("  - %s\n", name)
	}
	fmt.Println()

	// 2. 创建聊天模型
	cm, err := createChatModel(ctx)
	if err != nil {
		log.Fatalf("创建聊天模型失败: %v", err)
	}

	// 3. 获取所有工具信息并绑定到模型
	toolInfos, err := skillManager.GetToolInfos(ctx)
	if err != nil {
		log.Fatalf("获取工具信息失败: %v", err)
	}
	if err := cm.BindTools(toolInfos); err != nil {
		log.Fatalf("绑定工具到模型失败: %v", err)
	}

	// 4. 创建工具节点
	toolsNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
		Tools: skillManager.GetAllSkills(),
	})
	if err != nil {
		log.Fatalf("创建工具节点失败: %v", err)
	}

	// 5. 示例：使用技能进行计算
	fmt.Println("=== 示例 1: 使用计算器技能 ===")
	exampleCalculator(ctx, cm, toolsNode)

	// 6. 示例：使用文本处理技能
	fmt.Println("\n=== 示例 2: 使用文本处理技能 ===")
	exampleTextProcessing(ctx, cm, toolsNode)

	// 7. 示例：使用数据分析技能
	fmt.Println("\n=== 示例 3: 使用数据分析技能 ===")
	exampleDataAnalysis(ctx, cm, toolsNode)

	// 8. 示例：多技能组合使用
	fmt.Println("\n=== 示例 4: 多技能组合使用 ===")
	exampleMultiSkills(ctx, cm, toolsNode)
}

// exampleCalculator 计算器技能示例
func exampleCalculator(ctx context.Context, cm *chatOpenAi.ChatModel, toolsNode *compose.ToolsNode) {
	messages := []*schema.Message{
		{Role: schema.System, Content: "你可以使用计算器技能进行数学计算。"},
		{Role: schema.User, Content: "帮我计算 123 乘以 456，然后对结果开平方根"},
	}

	// 第一轮：模型生成并调用工具
	resp, err := cm.Generate(ctx, messages)
	if err != nil {
		log.Fatalf("模型生成失败: %v", err)
	}

	if len(resp.ToolCalls) > 0 {
		toolMsg := &schema.Message{Role: schema.Assistant, ToolCalls: resp.ToolCalls}
		toolOutMsgs, err := toolsNode.Invoke(ctx, toolMsg)
		if err != nil {
			log.Fatalf("工具执行失败: %v", err)
		}

		// 将工具结果添加到消息历史
		messages = append(messages, toolMsg)
		messages = append(messages, toolOutMsgs...)

		// 第二轮：模型生成最终回答
		finalResp, err := cm.Generate(ctx, messages)
		if err != nil {
			log.Fatalf("最终生成失败: %v", err)
		}

		fmt.Println("用户问题:", messages[1].Content)
		fmt.Println("最终回答:", finalResp.Content)
	}
}

// exampleTextProcessing 文本处理技能示例
func exampleTextProcessing(ctx context.Context, cm *chatOpenAi.ChatModel, toolsNode *compose.ToolsNode) {
	messages := []*schema.Message{
		{Role: schema.System, Content: "你可以使用文本处理技能处理文本。"},
		{Role: schema.User, Content: "请统计这句话有多少个字：'Hello World, 这是一个测试文本'，然后把它转换成大写"},
	}

	resp, err := cm.Generate(ctx, messages)
	if err != nil {
		log.Fatalf("模型生成失败: %v", err)
	}

	if len(resp.ToolCalls) > 0 {
		toolMsg := &schema.Message{Role: schema.Assistant, ToolCalls: resp.ToolCalls}
		toolOutMsgs, err := toolsNode.Invoke(ctx, toolMsg)
		if err != nil {
			log.Fatalf("工具执行失败: %v", err)
		}

		messages = append(messages, toolMsg)
		messages = append(messages, toolOutMsgs...)

		finalResp, err := cm.Generate(ctx, messages)
		if err != nil {
			log.Fatalf("最终生成失败: %v", err)
		}

		fmt.Println("用户问题:", messages[1].Content)
		fmt.Println("最终回答:", finalResp.Content)
	}
}

// exampleDataAnalysis 数据分析技能示例
func exampleDataAnalysis(ctx context.Context, cm *chatOpenAi.ChatModel, toolsNode *compose.ToolsNode) {
	messages := []*schema.Message{
		{Role: schema.System, Content: "你可以使用数据分析技能分析数字数据。"},
		{Role: schema.User, Content: "分析这组数据：10, 20, 30, 40, 50，计算平均值和标准差"},
	}

	resp, err := cm.Generate(ctx, messages)
	if err != nil {
		log.Fatalf("模型生成失败: %v", err)
	}

	if len(resp.ToolCalls) > 0 {
		toolMsg := &schema.Message{Role: schema.Assistant, ToolCalls: resp.ToolCalls}
		toolOutMsgs, err := toolsNode.Invoke(ctx, toolMsg)
		if err != nil {
			log.Fatalf("工具执行失败: %v", err)
		}

		messages = append(messages, toolMsg)
		messages = append(messages, toolOutMsgs...)

		finalResp, err := cm.Generate(ctx, messages)
		if err != nil {
			log.Fatalf("最终生成失败: %v", err)
		}

		fmt.Println("用户问题:", messages[1].Content)
		fmt.Println("最终回答:", finalResp.Content)
	}
}

// exampleMultiSkills 多技能组合示例
func exampleMultiSkills(ctx context.Context, cm *chatOpenAi.ChatModel, toolsNode *compose.ToolsNode) {
	messages := []*schema.Message{
		{Role: schema.System, Content: "你可以使用多种技能来解决问题。包括计算器、文本处理和数据分析。"},
		{Role: schema.User, Content: "我有一个数字列表：5, 15, 25, 35, 45。请先计算它们的平均值，然后计算平均值乘以2的结果"},
	}

	resp, err := cm.Generate(ctx, messages)
	if err != nil {
		log.Fatalf("模型生成失败: %v", err)
	}

	// 可能需要多轮工具调用
	maxIterations := 5
	for i := 0; i < maxIterations; i++ {
		if len(resp.ToolCalls) > 0 {
			toolMsg := &schema.Message{Role: schema.Assistant, ToolCalls: resp.ToolCalls}
			toolOutMsgs, err := toolsNode.Invoke(ctx, toolMsg)
			if err != nil {
				log.Fatalf("工具执行失败: %v", err)
			}

			messages = append(messages, toolMsg)
			messages = append(messages, toolOutMsgs...)

			// 继续生成
			resp, err = cm.Generate(ctx, messages)
			if err != nil {
				log.Fatalf("模型生成失败: %v", err)
			}

			// 如果没有工具调用，说明已经完成
			if len(resp.ToolCalls) == 0 {
				break
			}
		} else {
			break
		}
	}

	fmt.Println("用户问题:", messages[1].Content)
	fmt.Println("最终回答:", resp.Content)
}


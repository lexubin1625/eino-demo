package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	ccb "github.com/cloudwego/eino-ext/callbacks/cozeloop"
	chatOpenAi "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/coze-dev/cozeloop-go"
)

var (
	llmKey         = os.Getenv("DASHSCOPE_API_KEY")
	llmApi         = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	chatModel      = "qwen-plus"  // 主模型（生成答案 A）
	verifyModel    = "qwen-plus"  // 自验证模型（生成答案 B，可以使用更大模型）
	logicModel     = "qwen-turbo" // 逻辑校验模型（小模型）
	arbitrateModel = "qwen-plus"  // 仲裁模型（第三个模型）
	maxLoops       = 3            // 最多循环3次
)

// createChatModel 创建对话模型
func createChatModel(ctx context.Context, modelName string) (*chatOpenAi.ChatModel, error) {
	llm, err := chatOpenAi.NewChatModel(ctx, &chatOpenAi.ChatModelConfig{
		APIKey:  llmKey,
		Model:   modelName,
		Timeout: 60 * time.Second,
		BaseURL: llmApi,
	})
	return llm, err
}

// 相似度比较工具
type SimilarityTool struct{}

func (s *SimilarityTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "compare_similarity",
		Desc: "比较两个答案的相似度，返回相似度分数（0-1之间）和是否一致的判断",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"answer_a": {
				Desc:     "答案 A 的内容",
				Required: true,
				Type:     schema.String,
			},
			"answer_b": {
				Desc:     "答案 B 的内容",
				Required: true,
				Type:     schema.String,
			},
		}),
	}, nil
}

func (s *SimilarityTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	// 这里应该使用实际的相似度计算逻辑
	// 为了演示，我们使用 LLM 来判断相似度
	cm, err := createChatModel(ctx, "qwen-turbo")
	if err != nil {
		return "", err
	}

	var params struct {
		AnswerA string `json:"answer_a"`
		AnswerB string `json:"answer_b"`
	}
	if err := sonic.UnmarshalString(argumentsInJSON, &params); err != nil {
		return "", err
	}

	prompt := fmt.Sprintf(`请比较以下两个答案的相似度，并给出：
1. 相似度分数（0-1之间，保留2位小数）
2. 是否一致（true/false，相似度>=0.8认为一致）

答案A：
%s

答案B：
%s

请以JSON格式返回，格式：{"similarity": 0.xx, "consistent": true/false}`, params.AnswerA, params.AnswerB)

	msg, err := cm.Generate(ctx, []*schema.Message{
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return "", err
	}

	return msg.Content, nil
}

// 逻辑校验工具
type LogicValidationTool struct{}

func (l *LogicValidationTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "validate_logic",
		Desc: "校验答案的解题逻辑是否流畅、合理",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"answer": {
				Desc:     "需要校验的答案内容",
				Required: true,
				Type:     schema.String,
			},
			"question": {
				Desc:     "原始问题",
				Required: true,
				Type:     schema.String,
			},
		}),
	}, nil
}

func (l *LogicValidationTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	cm, err := createChatModel(ctx, logicModel)
	if err != nil {
		return "", err
	}

	var params struct {
		Answer   string `json:"answer"`
		Question string `json:"question"`
	}
	if err := sonic.UnmarshalString(argumentsInJSON, &params); err != nil {
		return "", err
	}

	prompt := fmt.Sprintf(`请评估以下答案的解题逻辑是否流畅、合理。

问题：
%s

答案：
%s

请以JSON格式返回，格式：{"valid": true/false, "reason": "原因说明"}`, params.Question, params.Answer)

	msg, err := cm.Generate(ctx, []*schema.Message{
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return "", err
	}

	return msg.Content, nil
}

// 仲裁工具
type ArbitrateTool struct{}

func (a *ArbitrateTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "arbitrate",
		Desc: "当两个答案的逻辑校验结果冲突时，使用第三个模型进行仲裁",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"answer_a": {
				Desc:     "答案 A 的内容",
				Required: true,
				Type:     schema.String,
			},
			"answer_b": {
				Desc:     "答案 B 的内容",
				Required: true,
				Type:     schema.String,
			},
			"question": {
				Desc:     "原始问题",
				Required: true,
				Type:     schema.String,
			},
			"validation_a": {
				Desc:     "答案 A 的逻辑校验结果",
				Required: true,
				Type:     schema.String,
			},
			"validation_b": {
				Desc:     "答案 B 的逻辑校验结果",
				Required: true,
				Type:     schema.String,
			},
		}),
	}, nil
}

func (a *ArbitrateTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	cm, err := createChatModel(ctx, arbitrateModel)
	if err != nil {
		return "", err
	}

	var params struct {
		AnswerA     string `json:"answer_a"`
		AnswerB     string `json:"answer_b"`
		Question    string `json:"question"`
		ValidationA string `json:"validation_a"`
		ValidationB string `json:"validation_b"`
	}
	if err := sonic.UnmarshalString(argumentsInJSON, &params); err != nil {
		return "", err
	}

	prompt := fmt.Sprintf(`两个答案的逻辑校验结果存在冲突，请进行仲裁判断。

问题：
%s

答案A：
%s
逻辑校验结果：%s

答案B：
%s
逻辑校验结果：%s

请判断哪个答案更合理，并给出最终判断。以JSON格式返回，格式：{"chosen": "A"或"B", "reason": "选择原因"}`,
		params.Question, params.AnswerA, params.ValidationA, params.AnswerB, params.ValidationB)

	msg, err := cm.Generate(ctx, []*schema.Message{
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return "", err
	}
	return msg.Content, nil
}

// 质检-验收流程主函数
func qualityCheckAndAcceptance(ctx context.Context, question string) error {
	// 1. 创建主模型生成答案 A
	mainModel, err := createChatModel(ctx, chatModel)
	if err != nil {
		return fmt.Errorf("创建主模型失败: %w", err)
	}

	fmt.Println("=" + strings.Repeat("=", 80))
	fmt.Println("质检-验收流程开始")
	fmt.Println("=" + strings.Repeat("=", 80))
	fmt.Printf("\n问题：%s\n\n", question)

	// 生成答案 A
	fmt.Println("[阶段1] 生成答案 A...")
	answerA, err := mainModel.Generate(ctx, []*schema.Message{
		{Role: schema.User, Content: question},
	})
	if err != nil {
		return fmt.Errorf("生成答案 A 失败: %w", err)
	}
	fmt.Printf("答案 A: %s\n\n", answerA.Content)

	// 2. 创建自验证模型（用于生成答案 B）
	verifyModel, err := createChatModel(ctx, verifyModel)
	if err != nil {
		return fmt.Errorf("创建自验证模型失败: %w", err)
	}

	// 3. 创建质检 Agent（包含相似度比较和逻辑校验工具）
	similarityTool := &SimilarityTool{}
	logicTool := &LogicValidationTool{}
	arbitrateTool := &ArbitrateTool{}

	qualityCheckModel, err := createChatModel(ctx, chatModel)
	if err != nil {
		return fmt.Errorf("创建质检模型失败: %w", err)
	}

	// 获取工具信息并绑定到模型
	toolInfos := []*schema.ToolInfo{}
	for _, t := range []tool.BaseTool{similarityTool, logicTool, arbitrateTool} {
		info, err := t.Info(ctx)
		if err != nil {
			return fmt.Errorf("获取工具信息失败: %w", err)
		}
		toolInfos = append(toolInfos, info)
	}
	if err := qualityCheckModel.BindTools(toolInfos); err != nil {
		return fmt.Errorf("绑定工具失败: %w", err)
	}

	qualityCheckAgent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "验收Agent",
		Description: "负责答案相似度比较和逻辑校验",
		Instruction: `你是验收 Agent，负责执行质检-验收流程。

输入说明：
- 你的输入可能包含：问题、答案A、答案B（从前一个Agent的输出中获取）
- 如果输入格式为"问题：xxx\n答案A：xxx\n答案B：xxx"，请提取这些信息
- 如果输入只包含答案B，你需要使用已知的问题和答案A（已在上下文中）

可用工具：
1. compare_similarity - 比较两个答案的相似度
2. validate_logic - 校验答案的逻辑是否流畅
3. arbitrate - 当逻辑校验冲突时进行仲裁

工作流程：
1. 从输入中提取问题、答案 A 和答案 B
2. 使用 compare_similarity 工具比较答案 A 和 B 的相似度
3. 根据相似度结果：
   - 如果相似度 >= 0.8（一致）：
     * 使用 validate_logic 工具校验答案 A 的逻辑
     * 如果逻辑通过，使用 exit 工具输出最终结果
   - 如果相似度 < 0.8（不一致）：
     * 分别使用 validate_logic 工具校验答案 A 和 B 的逻辑
     * 如果逻辑校验结果冲突（一个通过一个不通过）：
       - 使用 arbitrate 工具进行仲裁
       - 根据仲裁结果，使用 exit 工具输出最终答案
     * 如果逻辑校验结果一致，使用 exit 工具输出最终结果

请严格按照流程执行，使用提供的工具完成质检任务。`,
		Model: qualityCheckModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: []tool.BaseTool{similarityTool, logicTool, arbitrateTool},
			},
		},
		MaxIterations: 10,
		Exit:          adk.ExitTool{},
	})
	if err != nil {
		return fmt.Errorf("创建质检 Agent 失败: %w", err)
	}

	// 4.
	// 需要修改自验证Agent的输出格式，使其包含问题、答案A和答案B
	// 创建一个包装自验证Agent，使其输出包含完整上下文
	selfVerifyWithContext, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "自验证Agent",
		Description: "生成答案B并输出完整上下文",
		Instruction: fmt.Sprintf(`你的任务是生成答案B。

已知信息：
- 问题：%s
- 答案A：%s

工作流程：
1. 接收输入（可能是问题或空）
2. 使用已知的问题生成一个新的答案（答案B）
3. 生成完成后，直接输出以下格式的内容（不要使用任何工具，包括exit工具）：
   问题：%s
   答案A：%s
   答案B：[你生成的答案B]

重要：
- 禁止调用任何工具（包括exit工具）
- 禁止使用transfer_to_agent
- 直接生成答案并输出，不要退出
- 输出格式必须严格按照上述格式`, question, answerA.Content, question, answerA.Content),
		Model:         verifyModel,
		ToolsConfig:   adk.ToolsConfig{},
		MaxIterations: 3,
		// 移除 Exit 工具，让 LoopAgent 可以继续执行下一个 Agent
	})
	if err != nil {
		return fmt.Errorf("创建自验证Agent（带上下文）失败: %w", err)
	}

	// 使用 LoopAgent 按顺序执行
	loopAgent, err := adk.NewLoopAgent(ctx, &adk.LoopAgentConfig{
		Name:          "质检-验收流程Agent",
		Description:   "按顺序执行自验证和质检流程",
		SubAgents:     []adk.Agent{selfVerifyWithContext, qualityCheckAgent},
		MaxIterations: 3,
	})
	if err != nil {
		return fmt.Errorf("创建LoopAgent失败: %w", err)
	}

	// 5. 运行流程
	fmt.Println("[阶段2] 执行质检-验收流程...")
	fmt.Println()

	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           loopAgent,
		EnableStreaming: false,
	})

	// 输入问题，LoopAgent会按顺序执行子Agent
	// 第一个Agent会接收问题并生成答案B
	// 第二个Agent（会接收第一个Agent的输出（包含问题、答案A、答案B）
	iterator := runner.Query(ctx, question)

	var finalResult string
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}

		if event.Err != nil {
			return fmt.Errorf("流程执行失败: %w", event.Err)
		}

		if event.Output != nil && event.Output.MessageOutput != nil {
			msg, err := event.Output.MessageOutput.GetMessage()
			if err == nil && msg != nil && msg.Content != "" {
				fmt.Printf("[%s] %s\n", event.AgentName, msg.Content)
				finalResult = msg.Content
			}
		}

		if event.Action != nil && event.Action.Exit {
			fmt.Println("流程完成")
			break
		}
	}

	if finalResult == "" {
		return fmt.Errorf("未能生成最终结果")
	}

	// 6. 输出最终结果
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("最终结果")
	fmt.Println(strings.Repeat("=", 80))
	if finalResult != "" {
		fmt.Println(finalResult)
	} else {
		fmt.Println("未能生成最终结果")
	}

	return nil
}

func main() {
	ctx := context.Background()

	client, err := cozeloop.NewClient()
	if err != nil {
		panic(err)
	}
	defer client.Close(ctx)
	handler := ccb.NewLoopHandler(client)
	callbacks.AppendGlobalHandlers(handler)

	question := `临床上常根据病人病情需要，有针对性地选用不同的成分输血。对于血小板功能低下、贫血、创伤性失血的患者，应分别给他们输入
选项
- 全血、红细胞、血小板
- 血小板、红细胞、全血
- 血浆、红细胞、血小板
- 血小板、全血、红细胞`
	message := fmt.Sprintf(`对如下问题生成中学生物题解，问题：%s`, question)

	// 执行质检-验收流程
	if err := qualityCheckAndAcceptance(ctx, message); err != nil {
		log.Fatalf("质检-验收流程失败: %v", err)
	}
}

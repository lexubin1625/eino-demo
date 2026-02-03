package main

import (
	"context"
	"time"

	ccb "github.com/cloudwego/eino-ext/callbacks/cozeloop"
	chatOpenAi "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/supervisor"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/coze-dev/cozeloop-go"
)

var (
	llmKey    = "sk-0e3cfa922d45938a2085b01979e853b17d7b0e92dae2fc279af9d87a3e3dc76f"
	llmApi    = "https://api.qnaigc.com/v1"
	chatModel = "deepseek-v3" // 主模型（生成答案 A）
)

// 调研 Agent：生成研究计划
func NewResearchAgent(model model.ToolCallingChatModel) adk.Agent {
	agent, _ := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Name:        "ResearchAgent",
		Description: "为指定主题制定一份详细的研究计划",
		Instruction: `你是一名研究规划师。给定一个主题，输出包含关键阶段和里程碑的分步研究计划。仅输出计划，不要额外文本`,
		Model:       model,
	})
	return agent
}

// 撰写 Agent：根据研究计划撰写报告
func NewWriterAgent(model model.ToolCallingChatModel) adk.Agent {
	agent, _ := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Name:        "WriterAgent",
		Description: "基于一份研究计划撰写报告.",
		Instruction: `
你是一名学术撰稿人。给定一份研究计划，将其拓展为包含细节和分析的结构化报告。仅输出报告，不要额外文本.`,
		Model: model,
	})
	return agent
}

// Supervisor Agent：协调调研和撰写任务
func NewReportSupervisor(model model.ToolCallingChatModel) adk.Agent {
	agent, _ := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Name:        "ReportSupervisor",
		Description: "协调研究与写作工作以生成报告.",
		Instruction: `
你是一名项目主管。你的任务是协调两名次级代理：
研究代理（ResearchAgent）：制定研究计划。
写作代理（WriterAgent）：基于该计划撰写报告。
工作流程：
收到主题后，首先将任务移交至研究代理。
研究代理完成工作后，将任务连同研究计划作为输入，移交至写作代理。
写作代理完成工作后，输出最终报告`,
		Model: model,
	})
	return agent
}

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

func main() {
	ctx := context.Background()
	if false {
		client, err := cozeloop.NewClient()
		if err != nil {
			panic(err)
		}
		defer client.Close(ctx)
		handler := ccb.NewLoopHandler(client)
		callbacks.AppendGlobalHandlers(handler)
	}
	model, err := createChatModel(ctx, chatModel)
	if err != nil {
		panic(err)
	}
	// 2. 创建子 Agent 和 Supervisor
	researchAgent := NewResearchAgent(model)
	writerAgent := NewWriterAgent(model)
	reportSupervisor := NewReportSupervisor(model)

	// 3. 组合 Supervisor 与子 Agent
	supervisorAgent, _ := supervisor.New(ctx, &supervisor.Config{
		Supervisor: reportSupervisor,
		SubAgents:  []adk.Agent{researchAgent, writerAgent},
	})

	// 4. 运行 Supervisor 模式
	iter := supervisorAgent.Run(ctx, &adk.AgentInput{
		Messages: []adk.Message{
			schema.UserMessage("生成一份关于大型语言模型的历史报告."),
		},
	})

	// 5. 消费事件流（打印结果）
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Output != nil && event.Output.MessageOutput != nil {
			msg, _ := event.Output.MessageOutput.GetMessage()
			println("Agent[" + event.AgentName + "]:\n" + msg.Content + "\n===========")
		}
	}
}

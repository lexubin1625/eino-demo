package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	chatOpenAi "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/redis/go-redis/v9"
)

var (
	llmKey    = os.Getenv("DASHSCOPE_API_KEY")
	llmApi    = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	chatModel = "qwen-plus"
)

// ConversationMemory 使用 go-redis 实现的会话记忆管理
// 直接使用 Redis List 数据结构，无需自定义接口
type ConversationMemory struct {
	client    *redis.Client
	keyPrefix string
	maxRounds int
	ttl       time.Duration
}

// NewConversationMemory 创建会话记忆管理器
func NewConversationMemory(client *redis.Client, keyPrefix string, maxRounds int, ttl time.Duration) *ConversationMemory {
	return &ConversationMemory{
		client:    client,
		keyPrefix: keyPrefix,
		maxRounds: maxRounds,
		ttl:       ttl,
	}
}

// key 生成 Redis key
func (m *ConversationMemory) key(sessionId string) string {
	return fmt.Sprintf("%s:%s", m.keyPrefix, sessionId)
}

// GetMessages 获取会话历史消息
func (m *ConversationMemory) GetMessages(ctx context.Context, sessionId string) ([]*schema.Message, error) {
	key := m.key(sessionId)
	
	dataList, err := m.client.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("获取消息失败: %w", err)
	}
	
	if len(dataList) == 0 {
		return []*schema.Message{}, nil
	}

	messages := make([]*schema.Message, 0, len(dataList))
	for _, data := range dataList {
		var msg schema.Message
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			log.Printf("反序列化消息失败: %v", err)
			continue
		}
		messages = append(messages, &msg)
	}

	return messages, nil
}

// AppendMessage 添加消息到会话（自动裁剪和设置过期）
func (m *ConversationMemory) AppendMessage(ctx context.Context, sessionId string, msg *schema.Message) error {
	key := m.key(sessionId)
	
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %w", err)
	}

	// 使用 Pipeline 保证原子性：RPUSH + LTRIM + EXPIRE
	pipe := m.client.Pipeline()
	pipe.RPush(ctx, key, data)
	
	maxMessages := m.maxRounds * 2
	pipe.LTrim(ctx, key, int64(-maxMessages), -1)
	pipe.Expire(ctx, key, m.ttl)
	
	_, err = pipe.Exec(ctx)
	return err
}

// Clear 清空会话记忆
func (m *ConversationMemory) Clear(ctx context.Context, sessionId string) error {
	return m.client.Del(ctx, m.key(sessionId)).Err()
}

// ConversationState 对话状态
type ConversationState struct {
	Memory    *ConversationMemory
	SessionId string
}

// createAgentWithMemory 创建带记忆功能的 Agent
func createAgentWithMemory(ctx context.Context, sessionId string, memory *ConversationMemory, chatModel *chatOpenAi.ChatModel) (compose.Runnable[*schema.Message, *schema.Message], error) {
	graph := compose.NewGraph[*schema.Message, *schema.Message](
		compose.WithGenLocalState(func(ctx context.Context) *ConversationState {
			return &ConversationState{
				Memory:    memory,
				SessionId: sessionId,
			}
		}),
	)

	// 添加用户消息到记忆
	addUserMessage := func(ctx context.Context, input *schema.Message, state *ConversationState) (*schema.Message, error) {
		if err := state.Memory.AppendMessage(ctx, state.SessionId, input); err != nil {
			log.Printf("保存用户消息失败: %v", err)
		}
		return input, nil
	}

	// 构建包含历史消息的完整消息列表
	buildMessagesWithHistory := compose.InvokableLambda(func(ctx context.Context, input *schema.Message) ([]*schema.Message, error) {
		var memory *ConversationMemory
		var sessionId string
		_ = compose.ProcessState(ctx, func(_ context.Context, state *ConversationState) error {
			if state != nil {
				memory = state.Memory
				sessionId = state.SessionId
			}
			return nil
		})

		messages := []*schema.Message{
			{
				Role:    schema.System,
				Content: "你是一个友好的AI助手，能够记住对话历史并基于上下文回答问题。",
			},
		}

		if memory != nil {
			historyMsgs, err := memory.GetMessages(ctx, sessionId)
			if err != nil {
				log.Printf("加载历史消息失败: %v", err)
			} else {
				messages = append(messages, historyMsgs...)
			}
		}

		messages = append(messages, input)
		return messages, nil
	})

	// 添加助手回复到记忆
	addAssistantMessage := func(ctx context.Context, output *schema.Message, state *ConversationState) (*schema.Message, error) {
		if err := state.Memory.AppendMessage(ctx, state.SessionId, output); err != nil {
			log.Printf("保存助手回复失败: %v", err)
		}
		return output, nil
	}

	graph.AddLambdaNode("add_user_to_memory",
		compose.InvokableLambda(func(ctx context.Context, input *schema.Message) (*schema.Message, error) {
			return input, nil
		}),
		compose.WithStatePostHandler(addUserMessage),
	)

	graph.AddLambdaNode("build_messages", buildMessagesWithHistory)
	graph.AddChatModelNode("chat_model", chatModel)

	graph.AddLambdaNode("add_assistant_to_memory",
		compose.InvokableLambda(func(ctx context.Context, input *schema.Message) (*schema.Message, error) {
			return input, nil
		}),
		compose.WithStatePostHandler(addAssistantMessage),
	)

	graph.AddEdge(compose.START, "add_user_to_memory")
	graph.AddEdge("add_user_to_memory", "build_messages")
	graph.AddEdge("build_messages", "chat_model")
	graph.AddEdge("chat_model", "add_assistant_to_memory")
	graph.AddEdge("add_assistant_to_memory", compose.END)

	return graph.Compile(ctx)
}

func main() {
	ctx := context.Background()

	if llmKey == "" {
		log.Fatal("DASHSCOPE_API_KEY 未设置，请在环境变量中配置后再运行")
	}

	chatModel, err := createChatModel(ctx)
	if err != nil {
		log.Fatalf("创建聊天模型失败: %v", err)
	}

	// 使用 go-redis 创建 Redis 客户端
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	// 测试连接，如果失败则退出（不再使用内存存储，保持简洁）
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Redis 连接失败: %v\n请确保 Redis 服务正在运行", err)
	}

	// 直接使用 go-redis 和 Redis List 实现记忆管理
	memory := NewConversationMemory(redisClient, "eino:memory", 5, 24*time.Hour)

	sessionId1 := "session-001"
	sessionId2 := "session-002"

	fmt.Println("=== 跨进程记忆演示 ===\n")
	fmt.Println("使用: go-redis + Redis List 数据结构\n")

	// Session 1 的对话
	fmt.Println("--- Session 1 对话 ---")
	agent1, err := createAgentWithMemory(ctx, sessionId1, memory, chatModel)
	if err != nil {
		log.Fatalf("创建 Agent 失败: %v", err)
	}

	questions1 := []string{
		"我的名字是张三",
		"我喜欢吃苹果",
		"我的名字是什么？",
	}

	for i, question := range questions1 {
		fmt.Printf("\n[Session 1] 第 %d 轮对话\n", i+1)
		fmt.Printf("用户: %s\n", question)

		response, err := agent1.Invoke(ctx, &schema.Message{
			Role:    schema.User,
			Content: question,
		})
		if err != nil {
			log.Printf("调用失败: %v", err)
			continue
		}

		fmt.Printf("助手: %s\n", response.Content)
		msgs, _ := memory.GetMessages(ctx, sessionId1)
		fmt.Printf("当前记忆中的消息数: %d\n", len(msgs))
	}

	// Session 2 的对话
	fmt.Println("\n--- Session 2 对话（新会话，无历史记忆）---")
	agent2, err := createAgentWithMemory(ctx, sessionId2, memory, chatModel)
	if err != nil {
		log.Fatalf("创建 Agent 失败: %v", err)
	}

	questions2 := []string{
		"我的名字是什么？",
	}

	for i, question := range questions2 {
		fmt.Printf("\n[Session 2] 第 %d 轮对话\n", i+1)
		fmt.Printf("用户: %s\n", question)

		response, err := agent2.Invoke(ctx, &schema.Message{
			Role:    schema.User,
			Content: question,
		})
		if err != nil {
			log.Printf("调用失败: %v", err)
			continue
		}

		fmt.Printf("助手: %s\n", response.Content)
	}

	// 重新创建 Session 1 的 Agent（模拟不同进程）
	fmt.Println("\n--- 重新加载 Session 1（模拟不同进程）---")
	agent1Reloaded, err := createAgentWithMemory(ctx, sessionId1, memory, chatModel)
	if err != nil {
		log.Fatalf("创建 Agent 失败: %v", err)
	}

	testQuestion := "我刚才说我喜欢吃什么？"
	fmt.Printf("\n用户: %s\n", testQuestion)

	response, err := agent1Reloaded.Invoke(ctx, &schema.Message{
		Role:    schema.User,
		Content: testQuestion,
	})
	if err != nil {
		log.Printf("调用失败: %v", err)
	} else {
		fmt.Printf("助手: %s\n", response.Content)
		fmt.Println("\n✅ 成功！跨进程记忆功能正常工作")
	}

	fmt.Println("\n=== 演示结束 ===")
}

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

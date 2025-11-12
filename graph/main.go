package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	chatOpenAi "github.com/cloudwego/eino-ext/components/model/openai"
	duckduckgo "github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

var (
	// 千问llm
	llmKey    = os.Getenv("DASHSCOPE_API_KEY")
	llmApi    = "https://dashscope.aliyuncs.com/compatible-mode/v1" //千问系列API
	chatModel = "qwen-plus"                                         // chat模型
)

func main() {
	tag()
	return
	ctx := context.Background()

	type UserState struct {
		Messages []*schema.Message
		Subject  string
	}

	type UserParams struct {
		Subject  string
		Question string
	}

	gen := func(ctx context.Context) *UserState {
		return &UserState{Messages: make([]*schema.Message, 0)}
	}

	graph := compose.NewGraph[*schema.Message, *schema.Message](compose.WithGenLocalState(gen))
	statePost := func(ctx context.Context, out UserParams, state *UserState) (UserParams, error) {
		state.Messages = append(state.Messages, &schema.Message{Role: schema.User, Content: out.Question})
		return out, nil
	}

	msgToHistory := func(ctx context.Context, out *schema.Message, state *UserState) (*schema.Message, error) {
		state.Messages = append(state.Messages, out)
		return out, nil
	}

	// 学科识别：根据输入内容判断学科，输出到 UserParams 结构
	subjectIdentify := compose.InvokableLambda(func(ctx context.Context, input *schema.Message) (UserParams, error) {
		out := UserParams{Subject: "other", Question: input.Content}
		if strings.Contains(input.Content, "数学") {
			out.Subject = "math"
			return out, nil
		}
		if strings.Contains(input.Content, "英文") || strings.Contains(strings.ToLower(input.Content), "english") {
			out.Subject = "english"
			return out, nil
		}
		return out, nil
	})

	branch := compose.NewGraphBranch(func(ctx context.Context, in UserParams) (endNode string, err error) {
		switch in.Subject {
		case "math":
			return "mathNode", nil
		case "english":
			return "englishNode", nil
		default:
			return "otherNode", nil
		}
	}, map[string]bool{"mathNode": true, "englishNode": true, "otherNode": true})

	// 数学节点：使用 compose.ProcessState 读取 TopicState
	mathNode := compose.InvokableLambda(func(ctx context.Context, in UserParams) (*schema.Message, error) {
		subject := in.Subject
		// _ = compose.ProcessState(ctx, func(_ context.Context, st *TopicState) error {
		// 	if st != nil && st.Subject != "" {
		// 		subject = st.Subject
		// 	}
		// 	return nil
		// })
		result := "数学题解答（学科：" + subject + ")"
		return &schema.Message{Role: schema.Assistant, Content: result}, nil
	})

	// 英语节点：演示翻译（此处为示例 stub，可接入 LLM 或翻译 API）
	englishNode := compose.InvokableLambda(func(ctx context.Context, in UserParams) (*schema.Message, error) {
		translated := "翻译结果（示例）"
		return &schema.Message{Role: schema.Assistant, Content: translated}, nil
	})

	// 其它节点：返回无法解答
	otherNode := compose.InvokableLambda(func(ctx context.Context, in UserParams) (*schema.Message, error) {
		return &schema.Message{Role: schema.Assistant, Content: "抱歉，该问题暂时无法解答"}, nil
	})

	graph.AddLambdaNode("subjectIdentify", subjectIdentify,
		compose.WithStatePostHandler(statePost),
	)
	graph.AddLambdaNode("mathNode", mathNode, compose.WithStatePostHandler(msgToHistory))
	graph.AddLambdaNode("englishNode", englishNode, compose.WithStatePostHandler(msgToHistory))
	graph.AddLambdaNode("otherNode", otherNode)

	graph.AddEdge(compose.START, "subjectIdentify")
	graph.AddBranch("subjectIdentify", branch)
	graph.AddEdge("mathNode", compose.END)
	graph.AddEdge("englishNode", compose.END)
	graph.AddEdge("otherNode", compose.END)
	agent, err := graph.Compile(ctx)
	if err != nil {
		panic(err)
	}
	input := &schema.Message{
		Role:    schema.User,
		Content: "请解答数学题:一个矩形的长是宽的2倍，周长是30厘米，求长和宽分别是多少？",
	}
	output, err := agent.Invoke(ctx, input, compose.WithCallbacks(genCallback()))
	if err != nil {
		panic(err)
	}

	fmt.Println(output.Content)
}

func genCallback() callbacks.Handler {
	startKey := "node_start_time"
	handler := callbacks.NewHandlerBuilder().OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
		// 记录开始时间
		fmt.Printf("当前%s节点输入:%s\n", info.Component, input)
		return context.WithValue(ctx, startKey, time.Now())
	}).OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
		// 计算耗时
		if v := ctx.Value(startKey); v != nil {
			if t, ok := v.(time.Time); ok {
				cost := time.Since(t)
				fmt.Printf("当前%s节点耗时:%s\n", info.Component, cost)
			}
		}
		fmt.Println(info, output)

		fmt.Printf("当前%s节点输出:%s\n", info.Component, output)
		return ctx
	}).OnEndWithStreamOutputFn(func(ctx context.Context, info *callbacks.RunInfo, output *schema.StreamReader[callbacks.CallbackOutput]) context.Context {
		fmt.Println(info, output)
		return ctx
	}).Build()
	return handler
}

func tag() {
	// 使用 Eino + 对话模型，构建基于词库的文本打标签 Graph（词库可达上千）
	ctx := context.Background()

	// 本地状态：保存短语词库
	type TagState struct {
		Lexicon []string
	}

	// 初始化本地状态（可替换为从文件/DB加载）
	gen := func(ctx context.Context) *TagState {
		return &TagState{Lexicon: []string{
			"人工智能", "大模型", "云原生", "矩形", "周长", "翻译", "数学", "英语",
		}}
	}

	// Graph：输入 string（文本），输出 []string（标签）
	g := compose.NewGraph[string, []string](compose.WithGenLocalState(gen))

	// 预选节点：基于词库做快速匹配，选出候选（限制上限，避免提示超长）
	type TagCandidates struct {
		Text       string
		Candidates []string
	}

	preselect := compose.InvokableLambda(func(ctx context.Context, text string) (TagCandidates, error) {
		cand := TagCandidates{Text: text}
		if text == "" {
			return cand, nil
		}
		lower := strings.ToLower(text)
		// 并发安全地读取词库
		_ = compose.ProcessState[*TagState](ctx, func(_ context.Context, st *TagState) error {
			if st == nil || len(st.Lexicon) == 0 {
				return nil
			}
			// 简单子串匹配作为候选，限制最多 200 个（示例）
			const limit = 200
			for _, phrase := range st.Lexicon {
				p := strings.TrimSpace(phrase)
				if p == "" {
					continue
				}
				if strings.Contains(lower, strings.ToLower(p)) {
					cand.Candidates = append(cand.Candidates, p)
					if len(cand.Candidates) >= limit {
						break
					}
				}
			}
			return nil
		})
		return cand, nil
	})

	// 构造对话消息：把文本和候选列表提供给模型，只允许输出候选中的标签
	buildMessages := compose.InvokableLambda(func(ctx context.Context, in TagCandidates) ([]*schema.Message, error) {
		// 严格限制模型：只能从候选中选择；若候选为空则输出空
		var msgs []*schema.Message
		if len(in.Candidates) == 0 {
			sys := &schema.Message{Role: schema.System, Content: "你是一个文本标签助手。\n严格要求：候选为空时请输出空字符串，不要输出任何标签或解释。"}
			user := &schema.Message{Role: schema.User, Content: fmt.Sprintf("文本：%s\n候选：", in.Text)}
			msgs = []*schema.Message{sys, user}
		} else {
			candStr := strings.Join(in.Candidates, ", ")
			sys := &schema.Message{Role: schema.System, Content: "你是一个文本标签助手。\n严格要求：只输出提供的候选标签，用中文逗号分隔，不要包含解释或多余字符。不得输出候选之外的标签。"}
			user := &schema.Message{Role: schema.User, Content: fmt.Sprintf("文本：%s\n候选：%s", in.Text, candStr)}
			msgs = []*schema.Message{sys, user}
		}
		return msgs, nil
	})

	// 创建对话模型
	cm, err := createChatModel(ctx)
	if err != nil {
		log.Fatalf("Failed to create chat model: %v", err)
	}

	// 解析模型输出为标签数组，并对齐到词库（过滤掉非词库项）
	parse := compose.InvokableLambda(func(ctx context.Context, msg *schema.Message) ([]string, error) {
		out := strings.TrimSpace(msg.Content)
		if out == "" {
			return nil, nil
		}
		// 支持逗号/换行分隔
		parts := strings.FieldsFunc(out, func(r rune) bool { return r == ',' || r == '，' || r == '\n' })
		// 去重 + 过滤到词库
		uniq := make(map[string]struct{}, len(parts))
		var tags []string
		_ = compose.ProcessState[*TagState](ctx, func(_ context.Context, st *TagState) error {
			if st == nil {
				return nil
			}
			// 构造词库集合（小写匹配）
			lex := make(map[string]struct{}, len(st.Lexicon))
			for _, p := range st.Lexicon {
				lex[strings.ToLower(strings.TrimSpace(p))] = struct{}{}
			}
			for _, it := range parts {
				t := strings.TrimSpace(it)
				if t == "" {
					continue
				}
				if _, seen := uniq[t]; seen {
					continue
				}
				if _, ok := lex[strings.ToLower(t)]; ok {
					uniq[t] = struct{}{}
					tags = append(tags, t)
				}
			}
			return nil
		})
		return tags, nil
	})

	_ = g.AddLambdaNode("preselect", preselect)
	_ = g.AddLambdaNode("build_messages", buildMessages)
	_ = g.AddChatModelNode("chat_model", cm)
	_ = g.AddLambdaNode("parse", parse)

	_ = g.AddEdge(compose.START, "preselect")
	_ = g.AddEdge("preselect", "build_messages")
	_ = g.AddEdge("build_messages", "chat_model")
	_ = g.AddEdge("chat_model", "parse")
	_ = g.AddEdge("parse", compose.END)

	runnable, err := g.Compile(ctx)
	if err != nil {
		log.Fatalf("Failed to compile tag graph: %v", err)
	}

	// 示例：运行一次，打印标签
	sample := "请解答数学题：一个矩形的周长是30厘米，这是一个云原生示例。"
	out, err := runnable.Invoke(ctx, sample)
	if err != nil {
		log.Fatalf("Tag graph run failed: %v", err)
	}
	fmt.Printf("文本: %s\n标签: %s\n", sample, strings.Join(out, ", "))
}

func answer() {
	ctx := context.Background()

	if llmKey == "" {
		log.Fatal("DASHSCOPE_API_KEY 未设置，请在环境变量中配置后再运行")
	}

	// 读取用户题目：命令行参数或交互输入
	question := "一个矩形的长是宽的2倍，周长是30厘米，求长和宽分别是多少？"

	// Create search client
	textSearchTool, err := duckduckgo.NewTextSearchTool(ctx, &duckduckgo.Config{
		MaxResults: 3,                   // Limit to return 3 results to reduce load
		Region:     duckduckgo.RegionUS, // Use US region to avoid 202 issues
	})
	if err != nil {
		log.Fatalf("NewTool of duckduckgo failed, err=%v", err)
	}

	// Create tool info
	toolInfo, err := textSearchTool.Info(ctx)
	if err != nil {
		log.Fatalf("Info of duckduckgo failed, err=%v", err)
	}

	// Create tools node
	toolsNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
		Tools: []tool.BaseTool{
			textSearchTool,
		},
	})
	if err != nil {
		log.Fatalf("Failed to create tools node: %v", err)
	}

	// Create chat model
	chatModel, err := createChatModel(ctx)
	if err != nil {
		log.Fatalf("Failed to create chat model: %v", err)
	}
	chatModel.BindTools([]*schema.ToolInfo{toolInfo})

	// 使用一个 Graph：START → tools → build_messages(lambda) → chat_model → END
	graph := compose.NewGraph[*schema.Message, *schema.Message]()
	_ = graph.AddToolsNode("tools", toolsNode, compose.WithNodeName("tools"))
	buildMessages := compose.InvokableLambda(func(ctx context.Context, toolMsgs []*schema.Message) ([]*schema.Message, error) {
		// 打印工具返回结果（在同一个 Graph 流程中）
		fmt.Println("工具返回结果：")
		var searchContext string
		for i, m := range toolMsgs {
			fmt.Printf("  [%d] role=%s\n", i+1, m.Role)
			if m.Content != "" {
				fmt.Println(m.Content)
				searchContext += m.Content + "\n\n"
			}
		}

		// 构造喂给聊天模型的上下文
		messages := []*schema.Message{
			{
				Role:    schema.System,
				Content: "仅基于给定的搜索内容中文分步解题，并在答案结尾给出来源链接。不得使用外部知识。答案通俗易懂,不要包含特殊格式",
			},
			{
				Role:    schema.User,
				Content: fmt.Sprintf("题目：%s\n\n搜索内容：\n%s", question, searchContext),
			},
		}
		return messages, nil
	})
	_ = graph.AddLambdaNode("build_messages", buildMessages, compose.WithNodeName("build_messages"))
	_ = graph.AddChatModelNode("chat_model", chatModel, compose.WithNodeName("chat_model"))
	_ = graph.AddEdge(compose.START, "tools")
	_ = graph.AddEdge("tools", "build_messages")
	_ = graph.AddEdge("build_messages", "chat_model")
	_ = graph.AddEdge("chat_model", compose.END)

	// 编译 Graph 得到 agent
	agent, err := graph.Compile(ctx, compose.WithMaxRunSteps(10))
	if err != nil {
		log.Fatalf("Failed to compile graph: %v", err)
	}

	// 构造 assistant.tool_calls，直接触发搜索
	toolCallMsg := &schema.Message{
		Role:    schema.Assistant,
		Content: "",
		ToolCalls: []schema.ToolCall{
			{
				Function: schema.FunctionCall{
					Name:      toolInfo.Name,
					Arguments: fmt.Sprintf(`{"query": %q}`, question),
				},
			},
		},
	}

	finalMsg, err := agent.Invoke(ctx, toolCallMsg)
	if err != nil {
		fmt.Println("流程失败：", err)
		return
	}
	fmt.Println("解题过程和答案：")
	fmt.Print(finalMsg.Content)
}

// createChatModel 创建对话模型
func createChatModel(ctx context.Context) (*chatOpenAi.ChatModel, error) {
	// 创建 LLM
	llm, err := chatOpenAi.NewChatModel(ctx, &chatOpenAi.ChatModelConfig{
		APIKey:  llmKey,
		Model:   chatModel,
		Timeout: 60 * time.Second, // 添加超时
		BaseURL: llmApi,
	})
	return llm, err
}

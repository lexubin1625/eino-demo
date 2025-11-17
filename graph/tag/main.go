package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	ccb "github.com/cloudwego/eino-ext/callbacks/cozeloop"
	"github.com/cloudwego/eino/callbacks"

	chatOpenAi "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/coze-dev/cozeloop-go"
)

// PhraseLibrary 短语库结构
type PhraseLibrary struct {
	Phrases []string // 所有短语
}

// TagResult 标签结果
type TagResult struct {
	Text string   // 原始文本
	Tags []string // 匹配到的标签
}

func main() {
	ctx := context.Background()

	// 增加扣子罗盘trace上报
	client, err := cozeloop.NewClient()
	if err != nil {
		panic(err)
	}
	defer client.Close(ctx)
	handler := ccb.NewLoopHandler(client)
	callbacks.AppendGlobalHandlers(handler)

	phraseLib := &PhraseLibrary{
		Phrases: []string{
			"加减消元法解二元一次方程组",
			"弧长及计算公式",
			"圆的相关概念及圆的中心对称性",
			"旋转及旋转的三要素",
		},
	}

	// 测试多个用户习题文本
	question := "求解一个矩形的长是宽的2倍，周长是30厘米，求长和宽分别是多少？这是一个关于数学几何的问题。"
	// 基于短语库打标签
	tags, err := tagWithGraph(ctx, question, phraseLib)
	if err != nil {
		fmt.Printf("打标签失败: %v\n", err)
		return
	}
	fmt.Printf("标签: %v\n", tags.Tags)

}

// 基于eino graph的标签功能
func tagWithGraph(ctx context.Context, text string, lib *PhraseLibrary) (*TagResult, error) {
	llmKey := os.Getenv("DASHSCOPE_API_KEY")

	chatModel, err := chatOpenAi.NewChatModel(ctx, &chatOpenAi.ChatModelConfig{
		APIKey:  llmKey,
		Model:   "qwen-plus",
		BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
	})
	if err != nil {
		return nil, fmt.Errorf("create chat model failed: %v", err)
	}

	g := compose.NewGraph[string, *TagResult]()

	build := compose.InvokableLambda(func(ctx context.Context, input string) ([]*schema.Message, error) {
		library := strings.Join(lib.Phrases, "，")
		tmpl := prompt.FromMessages(schema.FString,
			schema.SystemMessage("只从短语库选择标签，返回中文逗号分隔，不要解释。短语库：{library}"),
			schema.UserMessage("文本：{text}"),
		)
		return tmpl.Format(ctx, map[string]any{"library": library, "text": input})
	})

	parse := compose.InvokableLambda(func(ctx context.Context, input *schema.Message) (*TagResult, error) {
		r := &TagResult{Text: text, Tags: make([]string, 0)}
		out := strings.TrimSpace(input.Content)
		out = strings.ReplaceAll(out, "，", ",")
		r.Tags = strings.Split(out, ",")
		return r, nil
	})

	_ = g.AddLambdaNode("build", build)
	_ = g.AddChatModelNode("chat", chatModel)
	_ = g.AddLambdaNode("parse", parse)

	_ = g.AddEdge(compose.START, "build")
	_ = g.AddEdge("build", "chat")
	_ = g.AddEdge("chat", "parse")
	_ = g.AddEdge("parse", compose.END)

	runnable, err := g.Compile(ctx, compose.WithGraphName("六类标签"))
	if err != nil {
		return nil, fmt.Errorf("failed to compile graph: %v", err)
	}
	return runnable.Invoke(ctx, text)
}

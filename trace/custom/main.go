// 自定义上报 trace 示例，使用火山引擎 Ark 模型。
//
// 通过 eino-ext 的 ark ChatModel 调用火山模型，不经过 eino callback，
// 使用 cozeloop-go 手写 span 将 input/output/tokens 上报到扣子罗盘。
// 文档：https://loop.coze.cn/open/docs/cozeloop/go-sdk
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/schema"
	"github.com/coze-dev/cozeloop-go"
	"github.com/coze-dev/cozeloop-go/spec/tracespec"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
)

func main() {
	ctx := context.Background()
	client, err := cozeloop.NewClient()
	if err != nil {
		panic(err)
	}
	defer client.Close(ctx)

	modelName := os.Getenv("ARK_MODEL")
	spanName := "one_span_demo"
	threadID := "session_1234567890"
	userID := "user_1234"
	// spanName/UserID/ThreadID 从 ctx 注入，供 callVolcanoArk 上报
	ctx = WithLoopTrace(ctx, spanName, threadID, userID)

	question := "用一句话介绍 eino"
	content, err := callVolcanoArk(ctx, client, modelName, question)
	if err != nil {
		panic(err)
	}
	fmt.Println("回复:", content)
	fmt.Println("单 span（含一次模型调用）已上报到扣子罗盘")

	stream, err := callVolcanoArkStream(ctx, client, modelName, question)
	if err != nil {
		panic(err)
	}
	defer stream.Close()
	fmt.Print("回复: ")
	for {
		msg, recvErr := stream.Recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			panic(recvErr)
		}
		if msg != nil && msg.Content != "" {
			fmt.Print(msg.Content)
		}
	}
	fmt.Println()
	fmt.Println("流式单 span 已上报到扣子罗盘")

}

// callVolcanoArk 使用火山方舟 Ark 模型发一次对话，并将 input/output/tokens 等 trace 上报封装在函数内。
// spanName/UserID/ThreadID/MessageID 从 ctx 注入（见 loop.LoopTrace），未设置时使用默认空或占位。
func callVolcanoArk(ctx context.Context, client cozeloop.Client, endpointID, userMessage string) (content string, err error) {
	t := getLoopTrace(ctx)
	ctx, span := client.StartSpan(ctx, t.SpanName, tracespec.VModelSpanType)
	span.SetModelName(ctx, endpointID)
	span.SetModelProvider(ctx, "Ark")
	if t.UserID != "" {
		span.SetUserID(ctx, t.UserID)
	}
	if t.ThreadID != "" {
		span.SetThreadID(ctx, t.ThreadID)
	}
	span.SetBaggage(ctx, map[string]string{"deployment_env": "test"})
	defer span.Finish(ctx)
	chatModel, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:  os.Getenv("ARK_API_KEY"),
		Model:   endpointID,
		BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
		Thinking: &model.Thinking{
			Type: model.ThinkingTypeDisabled,
		},
	})
	if err != nil {
		span.SetError(ctx, err)
		return "", err
	}

	inputs := []*schema.Message{
		{Role: schema.User, Content: userMessage},
	}
	msg, err := chatModel.Generate(ctx, inputs)
	span.SetInput(ctx, inputs)
	if err != nil {
		span.SetError(ctx, err)
		return "", err
	}

	content = msg.Content
	var promptTokens, completionTokens int
	if msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
		promptTokens = msg.ResponseMeta.Usage.PromptTokens
		completionTokens = msg.ResponseMeta.Usage.CompletionTokens
	}
	span.SetStartTimeFirstResp(ctx, time.Now().UnixMicro())
	span.SetOutput(ctx, msg)
	span.SetInputTokens(ctx, promptTokens)
	span.SetOutputTokens(ctx, completionTokens)
	return content, nil
}

// arkStreamReporter 包装流式 Reader，在 Recv 时内部累积 fullContent/tokens/firstRespTime，Close 时统一上报，业务侧无需关心。
type arkStreamReporter struct {
	raw  *schema.StreamReader[*schema.Message]
	span cozeloop.Span
	ctx  context.Context

	fullContent      string
	promptTokens     int
	completionTokens int
	firstRespTime    int64
}

// Recv 转发到底层 stream，并在内部累积内容与 usage，供 Close 时上报。
func (s *arkStreamReporter) Recv() (*schema.Message, error) {
	msg, err := s.raw.Recv()
	if err != nil {
		return msg, err
	}
	if msg != nil {
		if msg.Content != "" && s.firstRespTime == 0 {
			s.firstRespTime = time.Now().UnixMicro()
		}
		s.fullContent += msg.Content
		if msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
			s.promptTokens = msg.ResponseMeta.Usage.PromptTokens
			s.completionTokens = msg.ResponseMeta.Usage.CompletionTokens
		}
	}
	return msg, nil
}

// Close 关闭底层 stream，并将 fullContent/promptTokens/completionTokens/firstRespTime 上报到罗盘，业务侧无需显式上报。
func (s *arkStreamReporter) Close() error {
	s.raw.Close()
	if s.firstRespTime > 0 {
		s.span.SetStartTimeFirstResp(s.ctx, s.firstRespTime)
	}
	outMsg := &schema.Message{Role: schema.Assistant, Content: s.fullContent}
	s.span.SetOutput(s.ctx, outMsg)
	s.span.SetInputTokens(s.ctx, s.promptTokens)
	s.span.SetOutputTokens(s.ctx, s.completionTokens)
	s.span.Finish(s.ctx)
	return nil
}

// callVolcanoArkStream 流式调用火山 Ark 模型，返回带 Close 的 stream。
// 业务侧只需循环 Recv 并打印内容，最后 defer stream.Close()；fullContent/tokens/firstRespTime 在 Close 内自动上报。
func callVolcanoArkStream(ctx context.Context, client cozeloop.Client, endpointID, userMessage string) (*arkStreamReporter, error) {
	t := getLoopTrace(ctx)
	ctx, span := client.StartSpan(ctx, t.SpanName, tracespec.VModelSpanType)
	span.SetModelName(ctx, endpointID)
	span.SetModelProvider(ctx, "Ark")
	if t.UserID != "" {
		span.SetUserID(ctx, t.UserID)
	}
	if t.ThreadID != "" {
		span.SetThreadID(ctx, t.ThreadID)
	}
	span.SetBaggage(ctx, map[string]string{"deployment_env": "test"})

	chatModel, createErr := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:  os.Getenv("ARK_API_KEY"),
		Model:   endpointID,
		BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
		Thinking: &model.Thinking{
			Type: model.ThinkingTypeDisabled,
		},
	})
	if createErr != nil {
		span.SetError(ctx, createErr)
		span.Finish(ctx)
		return nil, createErr
	}

	inputs := []*schema.Message{
		{Role: schema.User, Content: userMessage},
	}
	span.SetInput(ctx, inputs)

	streamMsgs, streamErr := chatModel.Stream(ctx, inputs)
	if streamErr != nil {
		span.SetError(ctx, streamErr)
		span.Finish(ctx)
		return nil, streamErr
	}

	return &arkStreamReporter{
		raw:  streamMsgs,
		span: span,
		ctx:  ctx,
	}, nil
}

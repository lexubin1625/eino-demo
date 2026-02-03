// 使用 eino callback 自动上报 trace（含流式输出）。
//
// 仅注册 eino-ext LoopHandler，不手写 span。eino 的 ChatModel.Stream 调用会由 callback 自动上报到扣子罗盘。
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	ccb "github.com/cloudwego/eino-ext/callbacks/cozeloop"
	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/schema"
	"github.com/coze-dev/cozeloop-go"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
)

func main() {
	ctx := context.Background()

	client, err := cozeloop.NewClient()
	if err != nil {
		panic(err)
	}
	defer client.Close(ctx)

	handler := ccb.NewLoopHandler(client)
	callbacks.AppendGlobalHandlers(handler)

	chatModel, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:  os.Getenv("ARK_API_KEY"),
		Model:   os.Getenv("ARK_MODEL"),
		BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
		Thinking: &model.Thinking{
			Type: model.ThinkingTypeDisabled,
		},
	})
	if err != nil {
		panic(err)
	}

	// msg, err := chatModel.Generate(ctx, []*schema.Message{
	// 	{Role: schema.User, Content: "用一句话介绍 eino"},
	// })
	// if err != nil {
	// 	panic(err)
	// }
	// fmt.Println(msg.Content)

	question := "用一句话介绍 eino"
	streamMsgs, err := chatModel.Stream(ctx, []*schema.Message{
		{Role: schema.User, Content: question},
	})
	if err != nil {
		panic(err)
	}
	defer streamMsgs.Close()

	fmt.Print("流式输出: ")
	for {
		msg, err := streamMsgs.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		fmt.Print(msg.Content)
	}
	fmt.Println()

	// 流式场景下 token 与 LatencyFirstResp 由 LoopHandler 在 goroutine 中读完 stream 后上报，
	// 主流程需稍作等待再退出，否则 callback 可能尚未执行 SetTags/Finish 进程就结束，导致罗盘看不到 token 和首包时延。
	time.Sleep(2 * time.Second)
}

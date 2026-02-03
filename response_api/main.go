package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	ccb "github.com/cloudwego/eino-ext/callbacks/cozeloop"
	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/schema"
	"github.com/coze-dev/cozeloop-go"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
)

// loggingTransport 用于打印HTTP请求数据的Transport
type loggingTransport struct {
	transport http.RoundTripper
}

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// 打印请求基本信息
	fmt.Println("========== HTTP 请求数据 ==========")
	fmt.Printf("方法: %s\n", req.Method)
	fmt.Printf("URL: %s\n", req.URL.String())
	fmt.Printf("协议: %s\n", req.Proto)

	// 打印请求头
	fmt.Println("\n请求头:")
	for key, values := range req.Header {
		for _, value := range values {
			fmt.Printf("  %s: %s\n", key, value)
		}
	}

	// 打印请求体
	if req.Body != nil {
		// 读取请求体内容
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			fmt.Printf("读取请求体失败: %v\n", err)
		} else {
			fmt.Println("\n请求体:")
			fmt.Println(string(bodyBytes))
			// 重新设置请求体，因为已经被读取了
			req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}
	}

	fmt.Println("==================================\n")

	// 执行实际的请求
	return t.transport.RoundTrip(req)
}

func main() {
	ctx := context.Background()
	arkModel(ctx)
}

func openaiModel(ctx context.Context) (*openai.ChatModel, error) {
	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  "8ceb8471-e3bf-4fe5-9fff-eaa9b37ec08d",
		Model:   os.Getenv("ARK_MODEL"),
		BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
	})
	return chatModel, err
}
func arkModel(ctx context.Context) (*ark.ChatModel, error) {
	client, err := cozeloop.NewClient()
	if err != nil {
		panic(err)
	}
	defer client.Close(ctx)
	handler := ccb.NewLoopHandler(client)
	callbacks.AppendGlobalHandlers(handler)

	apiType := ark.ResponsesAPI
	// 创建带日志记录的Transport
	loggingTransport := &loggingTransport{
		transport: http.DefaultTransport,
	}
	chatModel, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:  "8ceb8471-e3bf-4fe5-9fff-eaa9b37ec08d",
		Model:   os.Getenv("ARK_MODEL"),
		BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
		HTTPClient: &http.Client{
			Transport: loggingTransport,
		},
		Cache: &ark.CacheConfig{
			APIType: &apiType,
			SessionCache: &ark.SessionCacheConfig{
				EnableCache: true,
				TTL:         500,
			},
		},
		Thinking: &model.Thinking{
			Type: model.ThinkingTypeDisabled,
		},
	})
	if err != nil {
		log.Fatalf("create chat model failed: %v", err)
	}
	outMsg, err := chatModel.Generate(ctx, []*schema.Message{
		{
			Role:    schema.User,
			Content: "你好，我是张三",
		},
	})

	if err != nil {
		log.Fatalf("generate failed: %v", err)
	}
	responseID, ok := ark.GetResponseID(outMsg)
	if !ok {
		log.Fatalf("get response id failed")
	}
	//interface {}(github.com/cloudwego/eino-ext/components/model/ark.arkResponseID) "resp_02176616102793341889a72faac00ad203e1fe6556332ab2167b8"
	fmt.Println("outMsg: ", outMsg.Extra)
	fmt.Println("chatModel type: ", chatModel.GetType())

	chatModel, err = ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:  "8ceb8471-e3bf-4fe5-9fff-eaa9b37ec08d",
		Model:   os.Getenv("ARK_MODEL"),
		BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
		HTTPClient: &http.Client{
			Transport: loggingTransport,
		},
		Thinking: &model.Thinking{
			Type: model.ThinkingTypeDisabled,
		},
		// Cache: &ark.CacheConfig{
		// 	APIType: &apiType,
		// 	SessionCache: &ark.SessionCacheConfig{
		// 		EnableCache: true,
		// 		TTL:         500,
		// 	},
		// },
		//ExtraFields: map[string]any{
		//	"caching": map[string]any{"type": "enabled"}},
		// ResponseFormat: &openai.ChatCompletionResponseFormat{
		// 	Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		// },
	})

	outMsg, err = chatModel.Generate(ctx, []*schema.Message{
		{
			Role:    schema.User,
			Content: "你好，我是谁？",
		},
	}, ark.WithCache(&ark.CacheOption{
		HeadPreviousResponseID: &responseID,
		APIType:                ark.ResponsesAPI,
	}))
	if err != nil {
		log.Fatalf("generate failed: %v", err)
	}
	fmt.Println("outMsg: ", outMsg)
	return chatModel, nil
}

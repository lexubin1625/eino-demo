# Eino Callback 自动上报 Trace（含流式输出）

本示例**仅使用 eino callback 自动上报**，不手写 span。注册 LoopHandler 后，eino 的 `ChatModel.Stream` 调用会由 callback 自动上报到扣子罗盘。

## 原理

1. 创建 cozeloop 客户端：`cozeloop.NewClient()`
2. 用 eino-ext 的 `LoopHandler` 包装：`ccb.NewLoopHandler(client)`，`callbacks.AppendGlobalHandlers(handler)`
3. 直接调用 `chatModel.Stream()` 做流式对话，trace 由 eino callback 自动上报，**无需自定义 span**

## 环境变量

- `DASHSCOPE_API_KEY` 或 `OPENAI_API_KEY`：LLM API Key
- 扣子罗盘：按 [cozeloop-go](https://github.com/coze-dev/cozeloop-go) 文档配置

## 运行

```bash
cd trace/callback
export DASHSCOPE_API_KEY=your_key
go run .
```

## 关键代码

```go
client, _ := cozeloop.NewClient()
defer client.Close(ctx)
handler := ccb.NewLoopHandler(client)
callbacks.AppendGlobalHandlers(handler)
// 流式对话，trace 由 callback 自动上报，无自定义 span
fullContent, _ := streamChat(ctx, chatModel, question)
```

与 `trace/custom`（手写 span 上报）的区别：本方式完全依赖 eino 的 callback 机制，不对 LLM 调用手写 `StartSpan`/`SetInput`/`SetOutput`/`Finish`。

## 流式时 token 与 LatencyFirstResp 未上报

- **原因**：LoopHandler 在流式场景下在 **goroutine** 里消费 stream，读完后再调用 `SetTags`（含 token、LatencyFirstResp）和 `Finish`。若主流程在读完 stream 后立刻退出，该 goroutine 可能尚未执行完，罗盘就收不到这些字段。
- **处理**：读完 stream 后**稍等再退出**（示例中 `time.Sleep(2 * time.Second)`），让 callback 有机会完成上报。
- **Token 来源**：eino-ext ark 流式已设置 `StreamOptions.IncludeUsage: true`，token 来自接口最后一 chunk 的 usage；若仍无 token，需确认当前火山 Ark 流式接口是否返回 usage。

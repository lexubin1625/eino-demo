# 自定义上报 Trace（火山模型）

本示例使用 **火山方舟 Ark 模型**（eino-ext ark）调用 LLM，并通过 **cozeloop 自定义 span** 上报 trace 到扣子罗盘，不依赖 eino callback。

## 环境变量

| 变量 | 说明 |
|------|------|
| `ARK_API_KEY` | 火山方舟 API Key（必填） |
| `ARK_MODEL` | 模型 endpoint id，如 `ep-xxx`（可选，有默认示例值） |
| 扣子罗盘 | 按 [cozeloop-go](https://github.com/coze-dev/cozeloop-go) 文档配置 |

## 运行

```bash
cd trace/custom
export ARK_API_KEY=your_volcano_ark_key
go run .
```

## 关键代码

- 使用 `ark.NewChatModel` 创建火山模型，`chatModel.Generate` 发请求。
- 使用 `client.StartSpan` 创建根 span 与 `llm_call` span，`SetInput`/`SetOutput`/`SetInputTokens`/`SetOutputTokens` 后 `Finish`，将本次调用的 input/output/tokens 上报到罗盘。

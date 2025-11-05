# MCP 模块功能说明

本模块演示了多种与 MCP（Model Context Protocol）交互的方式：
- 启动自定义 MCP Server（含 Tools/Resources/Prompts）
- 使用自定义 HTTP 客户端测试该 Server
- 连接高德地图 MCP（SSE）并通过对话式决策选择工具
- 集成 CloudWeGo Eino，将 MCP 工具作为 ToolNode 由聊天模型自动调用

## 概览
- 入口文件：`mcp/main.go` 根据命令行参数选择运行模式：`custom-server`、`custom-client`、`eino-client`，其他值默认运行高德 MCP 客户端。
- 所需环境变量：
  - `DASHSCOPE_API_KEY`：聊天模型 API Key（阿里 DashScope 兼容 OpenAI 接口）
  - `AMAP_API_KEY`：高德 MCP 接入密钥

## 关键文件与功能

### `main.go`
- 读取环境变量并设置模型 BaseURL 和模型名称。
- 根据命令行参数选择执行：
  - `custom-server`：启动内置 MCP 服务端
  - `custom-client`：运行内置 HTTP 测试客户端
  - `eino-client`：运行 Eino 集成示例，模型可自动决定并调用 MCP 工具
  - 其他值：运行高德 MCP 客户端示例
- 注意：运行时需传入参数，例如 `go run . custom-server`，否则会因访问 `os.Args[1]` 导致索引错误。

### `custom_server.go`
- 使用 `mark3labs/mcp-go` 启动一个支持 Resources、Tools、Prompts 的 MCP Server。
- 暴露端点：
  - `POST /mcp/`：MCP 协议入口（Streamable HTTP）
  - `GET /health`：健康检查
  - `GET /capabilities`：能力说明（是否开启 resources/tools/prompts）
- 内置能力：
  - Tool：`calculate` 基本四则运算（参数：`operation`、`a`、`b`）
  - Resource：`config://server` 返回服务配置信息（JSON）
  - Prompt：`code_review` 生成代码审查消息（支持 `code`、`language`）

### `custom_client.go`
- 连接 `http://localhost:8080/mcp/` 的自定义服务端，完成：
  - 初始化并打印服务信息
  - 列出 Tools/Resources/Prompts
  - 调用 `calculate` 工具并打印结果
  - 读取 `config://server` 资源
  - 获取 `code_review` Prompt 并打印消息内容
- 演示了正确解包 `mcp-go` 返回内容的方式（如 `TextContent`、`TextResourceContents`）。

### `amap_mcp_client.go`
- 通过 SSE 连接高德 MCP：`https://mcp.amap.com/sse?key=%s`
- 列出远端提供的工具，并将工具的名称、描述、输入 Schema 整理为文本。
- 使用聊天模型根据用户输入自动选择一个最合适的工具及其参数（严格 JSON 输出），然后调用该工具并打印结果。
- 依赖：
  - `AMAP_API_KEY`（必需）
  - `DASHSCOPE_API_KEY`（用于 LLM 工具选择）

### `eino_mcp_client.go`
- 使用 CloudWeGo Eino：
  - 连接高德 MCP，获取所有 MCP 工具并封装为 Eino 的 `BaseTool`
  - 创建 `ToolsNode` 用于实际工具执行
  - 创建聊天模型并绑定工具的 `ToolInfo`，保证参数对齐
  - 先让模型生成（可能包含 `tool_calls`），再把这些调用交给 `ToolsNode` 执行
  - 打印工具返回结果与所有工具的参数约束（JSON Schema）
- 示例对话：系统提醒可使用工具，用户提问“查询北京经纬度”，模型决定是否调用相关 MCP 工具来获取答案。

## 运行方式

在模块目录 `mcp/` 下运行（该目录有独立的 `go.mod`）：

- 启动自定义 MCP Server
  - `cd mcp`
  - `go run . custom-server`

- 运行自定义客户端（需服务端已启动）
  - `cd mcp`
  - `go run . custom-client`

- 运行 Eino 集成客户端（需配置环境变量）
  - `export DASHSCOPE_API_KEY=你的key`
  - `export AMAP_API_KEY=你的key`
  - `cd mcp`
  - `go run . eino-client`

- 运行高德 MCP 客户端示例（需 `AMAP_API_KEY`）
  - `export AMAP_API_KEY=你的key`
  - `cd mcp`
  - `go run . amap`
  - 或传入任意非上述模式字符串，默认落到高德示例分支

## 环境变量说明
- `DASHSCOPE_API_KEY`：用于对话模型推理和工具选择（兼容 OpenAI 接口）。
- `AMAP_API_KEY`：用于连接高德 MCP（SSE）。
- 模型 BaseURL：`https://dashscope.aliyuncs.com/compatible-mode/v1`（在代码中设定）。

## 依赖
- `github.com/mark3labs/mcp-go`：MCP Server/Client 实现
- `github.com/cloudwego/eino` 与 `github.com/cloudwego/eino-ext`：Eino 及其扩展（OpenAI 模型、MCP 工具适配器）
- `github.com/sashabaranov/go-openai`：OpenAI 兼容客户端（用于工具选择示例）

## 注意事项与排错
- 端口占用：自定义 MCP Server 默认监听 `:8080`，请确保端口空闲。
- 客户端连接失败：检查服务端是否已启动、URL 是否正确（`http://localhost:8080/mcp/`）。
- 工具选择失败：确认已设置 `DASHSCOPE_API_KEY`，并且外网网络可访问模型 API。
- 高德 MCP 连接失败：确认 `AMAP_API_KEY` 有效且网络可访问高德 MCP。
- 无参数运行将报错：请始终为 `go run .` 传入模式参数，如 `custom-server`。

## 示例输出（节选）
- `custom-client` 列出 Tools/Resources/Prompts，并打印 `calculate` 的结果与 `config://server` 的内容。
- `amap` 根据“查询朝阳公园到奥森公园的骑行路线”选择最合适的工具，调用并打印返回内容结构。
- `eino-client` 打印模型生成的 `tool_calls`，随后执行 MCP 工具并展示工具返回消息。

---
如需扩展：可在 `custom_server.go` 中继续添加更多工具、资源或提示模板，或在 `eino_mcp_client.go` 中增加更复杂的消息流与错误处理。
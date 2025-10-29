# Eino 工具示例：网页搜索与用户信息数据库查询

本目录包含两个示例：
- `main.go`：演示如何使用模型触发工具调用（网页搜索 DuckDuckGo）和如何调用自定义数据库查询工具。
- `search_user_from_db.go`：实现一个自定义 Eino Tool，按用户名查询用户信息（公司、职位、邮箱）。

## 文件结构与职责
- `tools/main.go`
  - `searchWeb()`：初始化聊天模型与 DuckDuckGo 搜索工具，模型生成 `tool_calls` 后通过 `ToolsNode` 执行，并将工具输出再次喂给模型得到最终回答。
  - `searchDB()`：初始化聊天模型与自定义数据库查询工具，模型可生成 `tool_calls`，也可以直接构造 `ToolCalls` 触发查询，返回 JSON 结果。
  - `createChatModel(...)`：创建聊天模型并绑定工具信息（用于意图识别与参数生成）。
- `tools/search_user_from_db.go`
  - 定义 `UserQueryParams`（查询参数）与 `UserInfo`（返回结构）。
  - 实现查询处理函数 `search_user_info_from_db(ctx, params)`（当前使用内存模拟数据库）。
  - 通过 `utils.InferTool` 构建可调用工具 `search_user_info()`，自动生成 `ToolInfo`（含参数约束）。

## 运行示例
### 环境准备
- Go 环境（建议 Go 1.20+）。
- 依赖安装：在 `tools` 目录执行 `go mod download`。
- 若运行网页搜索示例，需要配置模型的访问密钥：
  - 设置环境变量 `DASHSCOPE_API_KEY`。

### 运行网页搜索（默认）
- 当前 `main.go` 默认在 `main()` 中调用 `searchWeb()`。
- 执行：
  - `cd tools`
  - `go run .`
- 预期输出：
  - 控制台显示工具返回的搜索结果摘要，然后模型给出的最终建议。

### 运行数据库查询工具
- 修改 `tools/main.go` 中的 `main()`：将 `searchWeb()` 注释，取消 `searchDB()` 的注释，或在 `main()` 中直接调用 `searchDB()`。
- 执行：
  - `cd tools`
  - `go run .`
- 预期输出：
  - 控制台打印数据库工具的 JSON 返回，例如：
    ```json
    {"found":true,"name":"张三","user":{"company":"阿里巴巴","title":"后端工程师","email":"zhangsan@example.com"}}
    ```

## 自定义工具 `search_user_info`
- 工具名称：`search_user_info`
- 功能：根据用户名查询用户信息（公司、职位、邮箱）。
- 参数结构：
  - `UserQueryParams`：
    - `name`（string，必填）：要查询的用户名。
- 返回结构：JSON 字符串，示例：
  - 查询成功：
    ```json
    {"found":true,"name":"张三","user":{"company":"阿里巴巴","title":"后端工程师","email":"zhangsan@example.com"}}
    ```
  - 查询失败：
    ```json
    {"found":false,"msg":"user not found"}
    ```
  - 参数缺失：
    ```json
    {"error":"name is required"}
    ```

## 关键调用流程
1. 构建工具：在 `search_user_from_db.go` 中使用 `utils.InferTool(name, desc, handler)` 根据参数结构和处理函数自动生成 `ToolInfo` 与工具实例。
2. 绑定工具：在 `searchDB()` 中，调用 `cm.BindTools([]*schema.ToolInfo{toolInfo})` 让模型了解工具的存在与参数约束。
3. 执行工具：
   - 方式 A（模型触发）：模型 `Generate` 返回包含 `ToolCalls` 时，使用 `ToolsNode.Invoke(...)` 执行。
   - 方式 B（手动触发）：直接构造 `assistant.tool_calls` 消息（函数名取自 `toolInfo.Name`，参数为 `{"name":"张三"}`），调用 `ToolsNode.Invoke(...)`。

## 常见问题与排查
- 报错 `undefined: search_user_info`：
  - 请在 `tools` 目录使用 `go run .`，而不是单独运行 `main.go`；确保同包下的 `search_user_from_db.go` 一起编译。
- 模型未触发工具调用：
  - 可调整系统提示词以明确指示模型使用工具，或改用“方式 B”手动构造 `ToolCalls` 直接调用工具。
- 网页搜索偶发失败：
  - 可能受限于网络/站点限制；可减少 `MaxResults` 或修改 `Region`，并考虑加重试机制。

## 后续扩展
- 替换模拟数据库为真实数据源（如 MySQL/PostgreSQL），在 `search_user_info_from_db` 中执行实际查询。
- 增加更多查询条件（如邮箱、手机号），并扩展参数结构与工具描述。
- 将工具集成到更复杂的编排（Graph/Chain/ReAct Agent），按意图识别自动路由到网页或数据库。
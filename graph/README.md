# Graph 介绍

Graph 模块展示了如何使用 CloudWeGo Eino 进行可组合的对话与工具编排。通过节点化（Node）和边（Edge）连接的方式，将学科识别、工具调用、提示词构建、模型对话等能力组织成明确的有向流程。

## 什么是 Graph
- Graph 是一种有向流程编排方式，将多个节点按类型转换与边连接，形成从 `START` 到 `END` 的可执行流。
- 每个 Graph 都有统一的输入与输出类型约束，便于静态检查与运行时保证。

## 核心概念
- 输入/输出类型：`Graph[In, Out]` 声明图的整体类型，例如 `Graph[*schema.Message, *schema.Message]`。
- 节点：处理输入并产出输出，常见有 Lambda、ChatModelNode、ToolsNode。
- 边：连接节点并传递数据，必须满足类型匹配。
- 分支：根据条件将流程路由到不同的节点。
- 本地状态：在图中多次节点间维护上下文（如历史消息、学科）。

## 节点类型
- Lambda Node：将函数封装为节点，适合轻量计算或转换。
- Chat Model Node：封装聊天模型（如 Qwen），用于问答/推理。
- Tools Node：封装工具集合，执行工具调用并返回消息。
- 子图：将一个图当作节点嵌入另一个图，形成层次化结构。

## 状态管理
- 通过 `compose.WithGenLocalState` 定义图的状态结构，并在节点执行后通过 Handler 更新状态。
- 示例中的 `UserState` 保存历史消息与学科：`graph/main.go:36-47`。

## 分支控制
- 使用 `compose.NewGraphBranch` 根据条件路由到不同节点：`graph/main.go:78-88`。
- 分支需定义可达的目标节点集合，防止不可达或歧义。

## 编译与执行
- 编译：`graph.Compile(ctx)` 完成图的连通性与类型检查：`graph/main.go:125-129`。
- 执行：`agent.Invoke(ctx, input)` 返回最终输出：`graph/main.go:133-139`。

## 示例一：学科识别与应答
- 定义状态 `UserState`，维护历史与学科：`graph/main.go:36-47`。
- Lambda：`subjectIdentify` 基于文本判断学科：`graph/main.go:65-76`。
- 分支：学科路由到不同节点：`graph/main.go:78-88`。
- 节点：
  - `mathNode` 读取状态并输出解答：`graph/main.go:90-100`。
  - `englishNode` 输出翻译结果示例：`graph/main.go:103-106`。
  - `otherNode` 输出无法回答：`graph/main.go:109-111`。
- 图构建与执行：添加节点与边 `graph/main.go:113-124`；编译与运行 `graph/main.go:125-139`。

## 示例二：工具 + 模型联合流程
- 创建网页搜索工具并绑定：`graph/main.go:176-207`。
- 图结构：`START → tools → build_messages(lambda) → chat_model → END`，添加节点与边：`graph/main.go:208-241`。
- 直接触发工具调用（Assistant tool_calls）：`graph/main.go:249-261`。
- 打印模型的最终回答：`graph/main.go:268-270`。

## 与工具结合
- ToolsNode 在图中作为能力调用点，支持模型生成的 `tool_calls` 或直接构建函数调用。
- 参考工具示例模块：`tools/` 目录包含网页搜索与自定义数据库查询工具。
- 按 ID 查询用户信息可采用同样方式集成：构建工具 → ToolsNode → 触发调用 → 将结果并入上下文。

## 回调与观测
- 使用 `callbacks.Handler` 记录各节点输入、输出与耗时：`graph/main.go:141-164`。
- 回调帮助排查性能与数据流问题，建议在生产中开启必要的观测管线。

## 运行指南
- 依赖：`DASHSCOPE_API_KEY`（聊天模型密钥）。
- 运行学科识别示例：切换 `main()` 到 `SubjectAnswer()`：`graph/main.go:26-31`；`cd graph && go run .`。
- 运行工具 + 模型流程示例：切换 `main()` 到 `QuestionAnswer()`：`graph/main.go:26-31`；`cd graph && go run .`。

## 最佳实践
- 明确图的输入/输出类型，避免隐式类型转换。
- 节点命名与边连接保持一致、可读。
- 分支返回值必须命中可达节点集合。
- 使用状态处理器维护对话上下文，避免在节点中散落状态操作。
- 为复杂流程设置 `compose.WithMaxRunSteps` 限制运行步数：`graph/main.go:243-247`。
- 充分使用回调进行观测与调试。

## 参考与扩展
- 代码引用：
  - 学科识别：`graph/main.go:65-76`
  - 分支路由：`graph/main.go:78-88`
  - 节点添加：`graph/main.go:113-118`
  - 边连接与编译执行：`graph/main.go:120-139`
  - 工具 + 模型流程：`graph/main.go:176-241`、`graph/main.go:249-270`
- 更多说明：`graph/eino-graph.md` 提供概念与图示对照。
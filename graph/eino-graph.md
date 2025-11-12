# Eino Graph 实践与指南（中文）

本文系统介绍 CloudWeGo Eino 的 Graph 编排能力，结合当前项目的 `graph/main.go` 示例，讲解如何用图结构组织多阶段的数据处理与决策，并给出扩展思路与常见问题排查建议。

## 为什么使用 Graph
- 明确流程：将复杂链路拆为节点与边，使可视化与维护更容易。
- 组合复用：节点可以是函数、模型、工具等，易于重用与扩展。
- 条件分支：根据输入/上下文动态选择不同路径，提升灵活性。
- 类型安全：通过泛型约束输入输出类型，降低运行时错误概率。

## 核心概念
- Graph：由多个节点（Node）与边（Edge）组成的有向图，包含 `START` 与 `END`。
- Node：处理单一职责的计算单元，可以是 Lambda、模型节点、工具节点等。
- Edge：连接节点的有向边，指明执行顺序。
- Branch：在某个节点后进行条件分流，决定下一跳节点。

## 快速上手（与示例对应）
当前示例：
- 图的类型为 `compose.NewGraph[map[string]any, *schema.Message]()`，表示图整体输入类型为 `map[string]any`、最终输出为 `*schema.Message`。
- 三个 Lambda 节点：`node1`、`node2`、`node3`，以及一个分支 `branch`。
- 简单边关系：`START -> node1 -> branch -> node1/node2 -> node3 -> END`。

关键片段（摘要）：
```go
graph := compose.NewGraph[map[string]any, *schema.Message]()

node1 := compose.InvokableLambda(func(ctx context.Context, input map[string]any) (map[string]any, error) {
    fmt.Println("node1")
    return nil, nil
})

node2 := compose.InvokableLambda(func(ctx context.Context, input map[string]any) (*schema.Message, error) {
    fmt.Println("node2")
    return &schema.Message{Role: schema.User, Content: "矩形的长和宽分别是15厘米和7.5厘米"}, nil
})

node3 := compose.InvokableLambda(func(ctx context.Context, input *schema.Message) (*schema.Message, error) {
    fmt.Println("node3")
    return input, nil
})

branch := compose.NewGraphBranch(func(ctx context.Context, in map[string]any) (endNode string, err error) {
    fmt.Println("branch")
    if _, ok := in["question"]; ok {
        return "node1", nil
    }
    return "node2", nil
}, map[string]bool{"node1": true, "node2": true})

graph.AddLambdaNode("node1", node1)
graph.AddLambdaNode("node2", node2)
graph.AddLambdaNode("node3", node3)

graph.AddEdge(compose.START, "node1")
graph.AddBranch("node1", branch)
graph.AddEdge("node1", "node2")
graph.AddEdge("node2", "node3")
graph.AddEdge("node3", compose.END)

agent, _ := graph.Compile(ctx)
output, _ := agent.Invoke(ctx, map[string]any{"question": "..."})
```

## 节点类型与组合
- Lambda 节点（示例所用）：最轻量，定义输入输出并在函数中处理逻辑。
- 模型节点（可扩展）：封装聊天模型，接收消息并返回模型输出，用于问答/推理。
- 工具节点（可扩展）：对接外部工具（如 MCP 工具），在图中作为能力调用点。
- 子图（进阶）：将一个图当作节点嵌入到另一个图，实现层次化编排。

## 分支控制（Branch）
- 定义：在某节点完成后，根据条件决定下一节点。
- 约束：需明确可达节点集合（如示例中的 `{"node1": true, "node2": true}`）。
- 典型用法：根据输入是否包含某字段、根据上一步执行结果的状态码、根据策略标记等。

## 数据流与类型参数
- 图的泛型 `Graph[In, Out]`：约束整个图的输入与输出类型。
- 节点间数据传递：边连接要求上下游节点的类型匹配；Branch 决策函数的输入类型等同于前一节点的输出类型。
- 示例中：
  - `node1` 输出 `map[string]any`（此处返回 `nil`，但类型仍为 `map[string]any`）。
  - `branch` 函数依据 `map[string]any` 决策。
  - `node2` 接受 `map[string]any` 并输出 `*schema.Message`。
  - `node3` 接受 `*schema.Message` 并输出 `*schema.Message`。

## 编译与执行
- `Compile(ctx)`：将图结构检查、固化为可执行 `agent`，在编译阶段会做连通性与类型匹配校验（不同版本细节可能略有差异）。
- `Invoke(ctx, input)`：运行图，`input` 类型必须与图的输入类型匹配，最终返回图的输出类型。

## 与主示例的对照与思考
- 分支回到 `node1` 的设计：展示了条件循环的可能性，但实际业务中应谨慎避免无限循环，可引入迭代计数或状态标记。
- `node2` 固定消息：可替换为模型推理或外部工具调用，将 “计算矩形长宽” 这类任务交给模型或工具。
- `node3` 透传：可在此做后处理，如格式化结果、补充解释或调用下游服务。

## 扩展示例
1) 引入模型节点（伪代码，仅示意）：
```go
modelNode := compose.InvokableLambda(func(ctx context.Context, msg *schema.Message) (*schema.Message, error) {
    // 使用已配置的聊天模型生成答案
    resp, err := chatModel.Generate(ctx, []*schema.Message{msg})
    if err != nil { return nil, err }
    return &schema.Message{Role: schema.Assistant, Content: resp.Content}, nil
})

graph.AddLambdaNode("model", modelNode)
graph.AddEdge("node2", "model")
graph.AddEdge("model", "node3")
```

2) 引入工具节点（结合 MCP）：
```go
// 工具节点可参考 mcp/eino_mcp_client.go 的 ToolsNode 用法
// 在 Graph 中封装调用，将某一步的输出转为工具参数并执行，再返回结果消息
```

3) 错误与重试：
```go
nodeX := compose.InvokableLambda(func(ctx context.Context, in map[string]any) (map[string]any, error) {
    res, err := riskyCall()
    if err != nil { return nil, fmt.Errorf("nodeX failed: %w", err) }
    return res, nil
})
```
- 在上层可用分支对错误进行兜底处理，或在节点内部加入重试逻辑。

## 常见坑与建议
- 类型不匹配：确保节点输入输出类型与边连接一致，必要时引入转换节点。
- 无限循环：分支回跳需设置退出条件或最大迭代计数。
- 资源释放：节点中若创建连接（DB/HTTP 客户端等），注意关闭或复用。
- 日志与可视化：为每个节点打印关键信息，配合 Mermaid 在文档中同步维护流程图（见 `graph/README.ms`）。

## 如何运行
- 在 `graph/` 目录下执行：
  - `go run .`
- 示例输入：
```go
map[string]any{
  "question": "一个矩形的长是宽的2倍，周长是30厘米，求长和宽分别是多少？",
}
```
- 示例输出：
  - 控制台依次打印 `node1 -> branch -> node1 -> node2 -> node3`。
  - 最终输出的 `*schema.Message.Content` 为 `node2` 返回的文本。

## 总结
Eino Graph 提供了对复杂流程进行图式化编排的能力，强调类型安全与组合复用。你可以从当前示例出发，将 Lambda 替换为模型与工具节点，逐步构建出可维护、可扩展的智能工作流。建议在文档中持续维护 Mermaid 流程图，并在代码中为关键节点与分支加入日志，便于调试与协作。
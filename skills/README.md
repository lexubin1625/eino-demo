# Skills 技能模块

本模块实现了一个可扩展的技能系统，允许注册和管理多个可复用的技能（Skills），并通过 AI Agent 自动调用这些技能来解决问题。

## 功能特性

- **技能管理**：统一的技能注册和管理机制
- **多种技能**：内置计算器、文本处理、数据分析等技能
- **自动调用**：AI 模型可以自动识别并调用合适的技能
- **易于扩展**：可以轻松添加新的技能

## 已实现的技能

### 1. 计算器技能 (Calculator)
执行基本数学计算，支持：
- `add`: 加法
- `sub`: 减法
- `mul`: 乘法
- `div`: 除法
- `pow`: 幂运算
- `sqrt`: 开方

### 2. 文本处理技能 (Text Processing)
对文本进行各种处理操作：
- `count_words`: 统计字数
- `count_chars`: 统计字符数
- `upper`: 转大写
- `lower`: 转小写
- `reverse`: 反转文本
- `remove_spaces`: 移除空格
- `capitalize`: 首字母大写

### 3. 数据分析技能 (Data Analysis)
对数字数组进行统计分析：
- `mean`: 计算平均值
- `median`: 计算中位数
- `max`: 查找最大值
- `min`: 查找最小值
- `sum`: 求和
- `std_dev`: 计算标准差
- `sort`: 排序

## 使用方法

### 环境变量

需要设置以下环境变量：
```bash
export DASHSCOPE_API_KEY="your-api-key"
```

### 运行示例

```bash
cd skills
go mod tidy
go run .
```

### 代码示例

```go
// 1. 创建技能管理器
skillManager := NewSkillManager()

// 2. 注册技能
calcSkill, _ := CalculatorSkill()
skillManager.RegisterSkill("calculator", calcSkill)

// 3. 创建模型并绑定技能
cm, _ := createChatModel(ctx)
toolInfos, _ := skillManager.GetToolInfos(ctx)
cm.BindTools(toolInfos)

// 4. 创建工具节点
toolsNode, _ := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
    Tools: skillManager.GetAllSkills(),
})

// 5. 使用模型和技能
messages := []*schema.Message{
    {Role: schema.System, Content: "你可以使用计算器技能进行数学计算。"},
    {Role: schema.User, Content: "计算 123 乘以 456"},
}
resp, _ := cm.Generate(ctx, messages)
// ... 处理工具调用和响应
```

## 添加新技能

要添加新技能，只需：

1. 创建技能文件（如 `new_skill.go`）
2. 定义参数结构体
3. 实现技能函数
4. 在 `main.go` 中注册技能

示例：

```go
// new_skill.go
type NewSkillParams struct {
    Input string `json:"input" jsonschema:"required,description=输入参数"`
}

func NewSkill() (tool.InvokableTool, error) {
    return utils.InferTool(
        "new_skill",
        "新技能描述",
        newSkillHandler,
    )
}

func newSkillHandler(ctx context.Context, p *NewSkillParams) (string, error) {
    // 实现技能逻辑
    return "结果", nil
}
```

然后在 `main.go` 的 `registerAllSkills` 函数中注册：

```go
newSkill, err := NewSkill()
if err != nil {
    return fmt.Errorf("注册新技能失败: %w", err)
}
if err := sm.RegisterSkill("new_skill", newSkill); err != nil {
    return err
}
```

## 文件结构

```
skills/
├── go.mod                    # 依赖管理
├── main.go                   # 主入口文件
├── skill_manager.go          # 技能管理器
├── calculator_skill.go       # 计算器技能
├── text_processing_skill.go  # 文本处理技能
├── data_analysis_skill.go     # 数据分析技能
└── README.md                 # 说明文档
```

## 依赖

- `github.com/cloudwego/eino`: Eino 框架核心
- `github.com/cloudwego/eino-ext/components/model/openai`: OpenAI 兼容模型组件


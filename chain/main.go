package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ark"
	chatOpenAi "github.com/cloudwego/eino-ext/components/model/openai"
	duckduckgo "github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	jsonschema "github.com/invopop/jsonschema"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
)

var (
	// 千问llm
	llmKey = os.Getenv("DASHSCOPE_API_KEY")
	llmApi = "https://dashscope.aliyuncs.com/compatible-mode/v1" //千问系列API

	chatModel = "qwen-plus" // chat模型
)

func main() {

	// ctx := context.Background()
	// client, err := cozeloop.NewClient()
	// if err != nil {
	// 	panic(err)
	// }
	// defer client.Close(ctx)
	// handler := ccb.NewLoopHandler(client)
	// callbacks.AppendGlobalHandlers(handler)

	// result, err := CallVisionLLMTag(ctx)
	// if err != nil {
	// 	log.Fatalf("Failed to call vision LLM tag: %v", err)
	// }
	// fmt.Println("result: ", result)

	search()

}

type KnowledgePoint struct {
	// 知识点
	KnowledgePoint string `json:"knowledgePoint"`
	// 知识点关键理解
	KnowledgePointKey string `json:"knowledgePointKey"`
	// 考点
	ExaminationPoint string `json:"examinationPoint"`
	// 考点关键理解
	ExaminationPointKey string `json:"examinationPointKey"`
}

// Question 表示从图像中处理的问题
type Question struct {
	Content string            `json:"content"` // 题目内容
	Chinese []string          `json:"chinese"` // 语文学科关键词
	English []string          `json:"english"` // 英语学科关键词
	Keys    []*KnowledgePoint `json:"keys"`    // 知识点
}
type VisionResponse struct {
	Stage                string      `json:"stage"`                // 例如："小学", "初中", "高中"
	Subject              string      `json:"subject"`              // 例如："数学", "语文", "英语"
	Type                 string      `json:"type"`                 // 例如："概念为主", "解题为主"
	KnowledgePointKeys   string      `json:"knowledgePointKeys"`   // 知识点列表
	ExaminationPointKeys string      `json:"examinationPointKeys"` // 考点列表
	Items                []*Question `json:"items"`                // 题目或内容列表
	Warning              string      `json:"warning,omitempty"`
}

func CallVisionLLMTag(ctx context.Context) (*VisionResponse, error) {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties:  false,
		RequiredFromJSONSchemaTags: true,
	}

	jsonschema := reflector.Reflect(&VisionResponse{})

	// 设置 schema 的元信息
	jsonschema.Title = "VisionResponse"
	jsonschema.Description = "LLM视觉模型的响应结构"

	imageURL := "https://internal-api-drive-stream.feishu.cn/space/api/box/stream/download/authcode/?code=NzVlMTc5ZDRjNjg3MzdlNjBmNmRhOTcwZTRiYWVhMWJfNTJkMTAwYWY2M2NiNjk4ZmNiODkwZjU1M2ExMWY0MTZfSUQ6NzUyODMxOTU2MTQxMTMzMDA1Ml8xNzY2NTY5NDgzOjE3NjY2NTU4ODNfVjM"
	chatModel, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:  "xxxxx",
		Model:   "xxxxxx",
		BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
		Thinking: &model.Thinking{
			Type: model.ThinkingTypeDisabled,
		},
		ResponseFormat: &ark.ResponseFormat{
			Type: model.ResponseFormatJSONSchema,
			JSONSchema: &model.ResponseFormatJSONSchemaJSONSchemaParam{
				Name:   "VisionResponse",
				Schema: jsonschema,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create chat model failed: %v", err)
	}

	tmpl := prompt.FromMessages(schema.Jinja2,
		&schema.Message{
			Role: schema.User,
			MultiContent: []schema.ChatMessagePart{
				{
					Type: schema.ChatMessagePartTypeText,
					Text: "{{prompt}}",
				},
				{
					Type:     schema.ChatMessagePartTypeImageURL,
					ImageURL: &schema.ChatMessageImageURL{URL: "{{image}}"},
				},
			},
		},
	)

	// 定义状态结构来存储输入消息
	type chainState struct {
		InputMessages []*schema.Message
	}

	// 创建自定义 parser，在解析后从状态读取输入消息并添加到结果中
	customParser := compose.InvokableLambda(func(ctx context.Context, outputMsg *schema.Message) (*VisionResponse, error) {
		// 使用 parser 解析输出消息
		result, err := schema.NewMessageJSONParser[*VisionResponse](&schema.MessageJSONParseConfig{
			ParseFrom: schema.MessageParseFromContent,
		}).Parse(ctx, outputMsg)
		if err != nil {
			return nil, fmt.Errorf("failed to parse output message: %w", err)
		}

		return result, nil
	})

	chain := compose.NewChain[map[string]any, *VisionResponse](compose.WithGenLocalState(func(ctx context.Context) *chainState {
		return &chainState{InputMessages: make([]*schema.Message, 0)}
	}))
	chain.AppendChatTemplate(tmpl).
		AppendChatModel(chatModel).
		AppendLambda(customParser)
	runnable, err := chain.Compile(ctx, compose.WithGraphName("六类标签"))
	if err != nil {
		return nil, fmt.Errorf("failed to compile graph: %v", err)
	}
	result, err := runnable.Invoke(ctx, map[string]any{"image": imageURL, "prompt": imagePrompt})
	if err != nil {
		return nil, fmt.Errorf("failed to invoke graph: %v", err)
	}
	return result, nil
}

func search() {
	ctx := context.Background()

	// Create configuration
	config := &duckduckgo.Config{
		MaxResults: 3, // Limit to return 3 results
		Region:     duckduckgo.RegionCN,
	}

	// Create search client
	textSearchTool, err := duckduckgo.NewTextSearchTool(ctx, config)
	if err != nil {
		log.Fatalf("NewTool of duckduckgo failed, err=%v", err)
	}
	// Create search request
	searchReq := &duckduckgo.TextSearchRequest{
		Query: "北京天气",
	}

	jsonReq, err := json.Marshal(searchReq)
	if err != nil {
		log.Fatalf("Marshal of search request failed, err=%v", err)
	}

	toolInfo, err := textSearchTool.Info(ctx)
	if err != nil {
		log.Fatalf("Info of duckduckgo failed, err=%v", err)
	}
	// Execute search
	resp, err := textSearchTool.InvokableRun(ctx, string(jsonReq))
	if err != nil {
		log.Fatalf("Search of duckduckgo failed, err=%v", err)
	}

	var searchResp duckduckgo.TextSearchResponse
	if err := json.Unmarshal([]byte(resp), &searchResp); err != nil {
		log.Fatalf("Unmarshal of search response failed, err=%v", err)
	}

	// Print results
	fmt.Println("Search Results:")
	fmt.Println("==============")
	for i, result := range searchResp.Results {
		fmt.Printf("\n%d. Title: %s\n", i+1, result.Title)
		fmt.Printf("   URL: %s\n", result.URL)
		fmt.Printf("\n%d. Summary: %s\n", i+1, result.Summary)
	}
	fmt.Println("")
	fmt.Println("==============")

	// 创建工具节点
	toolsNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
		Tools: []tool.BaseTool{
			textSearchTool,
		},
	})
	if err != nil {
		panic(err)
	}

	// // Mock LLM 输出作为输入
	// input := &schema.Message{
	// 	Role: schema.Assistant,
	// 	ToolCalls: []schema.ToolCall{
	// 		{
	// 			Function: schema.FunctionCall{
	// 				Name:      "duckduckgo_text_search",
	// 				Arguments: `{"query": "深圳", "date": "tomorrow"}`,
	// 			},
	// 		},
	// 	},
	// }

	// toolMessages, err := toolsNode.Invoke(ctx, input)
	// fmt.Println("tool messages: ", toolMessages)

	chatModel, err := createChatModel(ctx)
	if err != nil {
		log.Fatalf("Failed to create chat model: %v", err)
	}
	chatModel.BindTools([]*schema.ToolInfo{toolInfo})

	// Build the chain with the ChatModel and the Tools node.
	// First the tools node, then the chat model to properly handle the types
	chain := compose.NewChain[[]*schema.Message, []*schema.Message]()
	chain.
		AppendToolsNode(toolsNode, compose.WithNodeName("search")).
		AppendChatModel(chatModel, compose.WithNodeName("chat_model"))

	// Compile the chain to obtain the agent.
	agent, err := chain.Compile(ctx)
	if err != nil {
		log.Fatalf("Failed to compile chain: %v", err)
	}
	outMsg, err := agent.Invoke(ctx, []*schema.Message{{
		Role:    schema.User,
		Content: "查询北京天气,给出建议",
	}})

	if err != nil {
		log.Fatalf("Failed to invoke agent: %v", err)
	}

	// Since the output is []*schema.Message, we need to print the content of the first message
	if len(outMsg) > 0 {
		fmt.Println("outMsg: ", outMsg[0].Content)
	} else {
		fmt.Println("No output message received")
	}
}

// createChatModel 创建对话模型
func createChatModel(ctx context.Context) (*chatOpenAi.ChatModel, error) {
	// 创建 LLM
	llm, err := chatOpenAi.NewChatModel(ctx, &chatOpenAi.ChatModelConfig{
		APIKey:  llmKey,
		Model:   chatModel,
		Timeout: 60 * time.Second, // 添加超时
		BaseURL: llmApi,
	})
	return llm, err
}

const (
	imagePrompt = `
# 教育解析引擎 - 图片内容处理指南

您是一个专业的教育解析引擎，专门处理课本/作业相关的图片内容。请严格按照以下规范执行三步处理流程。

---

## 📋核心处理流程

### 第一步：内容检测与分割
1. **识别与拆分**：识别图片中所有可见的题目内容和讲解内容
2. **复合题目拆分规则**：
   - **大题与小题**：拆分为独立题目
   - **同一题干下的选择题**：保持为整体
   - **明显分页/分区的题目**：视为独立题目
3. **讲解内容处理**：不需要进行拆分

### 第二步：学科分类与特征识别

#### 2.1 基础分类
- **学科[subject]** 必须从以下选项选择：数学 | 物理 | 语文 | 化学 | 英语 | 生物 | 地理 | 其他
- **学段[stage]** 必须从以下选项选择：小学 | 初中 | 高中 | 其他
- **学科类型[subjectType]**必须从以下选项选择：
  - 理科：数学、物理、化学、生物
  - 文科：语文、英语、地理
- **内容类型[type]**必须从以下选项选择：概念为主 | 解题为主

#### 2.2 学科和学段的关键分析维度（需逐一检查）

##### 🔍 文字标识（最直接）
- **学段关键词**：小学、初中、高中、一年级、七年级、高一等
- **学科关键词**：数学、物理、语文、英语等
- **教材版本**：人教版语文、苏教版数学等

##### 📚 内容难度与学科特征（无标识时的核心依据）

**数学学科**：
- **小学（1-6年级）**：加减乘除、分数、简易方程、几何初步（如长方形面积）为主
- **初中（7-9年级）**：有理数、一元二次方程、函数（一次/二次）、三角形全等、圆为主
- **高中（10-12年级）**：导数、微积分、立体几何、圆锥曲线、概率统计（如排列组合）为主

**语文学科**：
- **小学（1-6年级）**：拼音、汉字书写、简单记叙文、少量古诗（如《咏鹅》）为主
- **初中（7-9年级）**：文言文（如《论语》选篇）、议论文、散文（如《背影》）、古诗文背诵（如《岳阳楼记》）为主
- **高中（10-12年级）**：复杂文言文（如《滕王阁序》）、文学类文本阅读（如小说、戏剧）、议论文写作（如论点论据论证）为主

**英语学科**：
- **小学（1-6年级）**：字母、简单单词（如"apple"）、日常对话（如"Hello!"）为主
- **初中（7-9年级）**：语法（如一般过去时、定语从句）、完形填空、阅读理解（短篇）为主
- **高中（10-12年级）**：复杂语法（如虚拟语气、非谓语动词）、完形填空（长篇）、阅读理解（科普/文学类）、书面表达（议论文）为主

**物理学科**：
- **小学**：无物理
- **初中（7-9年级）**：力学（如牛顿运动定律）、声学（如声音的传播）、光学（如光的反射）、电学（如电路）为主
- **高中（10-12年级）**：电磁学（如电场、磁场）、热力学（如熵）、量子力学（如光电效应）为主

**化学学科**：
- **小学**：无化学
- **初中（7-9年级）**：元素周期表（前20位）、简单化学反应（如酸碱中和）、常见物质（如氧气、水）为主
- **高中（10-12年级）**：有机化学（如烃、醇）、化学反应原理（如平衡常数）、电化学（如原电池）为主

**生物学科**：
- **小学**：无生物
- **初中（7-9年级）**：细胞结构、生物进化、生态系统为主
- **高中（10-12年级）**：遗传（如DNA复制）、分子生物学（如基因表达）、稳态（如内环境）为主

**地理学科**：
- **小学**：无地理
- **初中（7-9年级）**：认识地图、中国省份、常见地理现象（如天气）、世界地理（如大洲大洋）、中国地理（如地形地势）、人文地理（如人口分布）为主
- **高中（10-12年级）**：自然地理（如大气环流、洋流）、人文地理（如工业区位）、区域地理（如可持续发展）为主

### 第三步：结构化解析

#### 3.1 整体内容解析
- **核心知识点**：必须只抽取1个（按学科特点调整）
- **核心考点**：必须只抽取1个（按学科特点调整）

#### 3.2 单项内容解析
对每个检测到的题目内容和讲解内容独立执行：

**知识点提取[knowledge_points]**：至少抽取3个核心知识点

##### 💡 理科知识点提取规则
- **知识点[knowledgePoint]**：具体的概念名称、定理名称、公式表达、实验现象
- **关键理解[knowledgePointKey]**：具体的定义内容、适用条件、计算步骤、实验原理
- **考点[examinationPoint]**：具体的计算类型、推理方法、实验操作、应用场景
- **考点理解[examinationPointKey]**：具体的解题步骤、常见错误、解题技巧、实验要点

##### 📝 文科知识点提取规则
- **知识点[knowledgePoint]**：具体的字词句、文学作品、语法现象、表达技巧
- **关键理解[knowledgePointKey]**：具体的含义解释、用法说明、技巧要点
- **考点[examinationPoint]**：具体的能力考查点、题型要求
- **考点理解[examinationPointKey]**：具体的解题方法、判断依据、答题技巧

#### 3.3 学科专项信息提取

##### 🇨🇳 语文学科关键信息[chinese]（当学科为语文时）
- 作品名称：准确的作品名称（如适用）
- 作者：作者姓名（如适用）
- 朝代/时期：作品所属的历史时期（如适用）
- 文学体裁：诗歌、散文、小说、戏剧等（如适用）
- 文言文要素：具体识别考察的文言实词/虚词及其含义，如"假：代理的、临时的（本题语境）；借助、凭借；假的、非真的"（如适用）

##### 🇬🇧 英语学科关键信息[english]（当学科为英语时）
- **写作题要素**：识别作文类型（命题作文或读后续写）、相关话题（如介绍你所在的城市）、文体类型（记叙文、说明文、议论文等）（如适用）
- **词汇/短语要素**：如果题目考察拼写、考察词义，具体识别考察的关键词汇/短语及其含义，如"brave：勇敢的（形容词）""动词短语 'go out' 的含义"（如适用）
- **词根要素**：如果题目涉及词汇构成、词汇辨析、词汇记忆，具体识别考察的词根及其含义和衍生词，如"vis-/vid-：看见（词根）→ visible可见的、video视频、vision视觉、visit参观"（如适用）
- **拼读/音标要素**：如果题目包含音标或者"发音""读音"，具体识别考察的拼读/音标，如"字母u的发音"“识别含 /i:/ 发音的字母”（如适用）
- **题型要素**：如果识别为篇章题（文章+题目/挖空），具体识别题型类别（阅读理解、完形填空、语法填空、信息还原等）以及具体的解题技巧（如考察主旨大意、文章结构、理解细节、上下文逻辑、猜测词义），并总结篇章如何考察解题技巧。如果篇章题识别题型类别为信息还原，则提取具体的句型、语法信息（如适用）
- **课文要素**：如果识别为课文（只有文章或对话，没有题目或者挖空），提取课文的主题、重点句型和词汇（如适用）
- **语法/句型要素**：如果题目考察语法规则、语法现像、句型结构，具体识别考察的语法/句型及其用法，如"现在完成时：have/has + 过去分词，表示过去发生的动作对现在的影响"（如适用）
- **词汇表要素**：如果识别为词汇表，则优先提取加粗的重点词汇（如适用）
---

## 🎯 学科特殊处理规则

### 📖 语文学科
- **知识点重点**：具体的字词含义、句式特点、修辞手法名称、文学作品信息、语言现象
- **考点重点**：字词理解、句意把握、修辞识别、文本分析、表达运用
#### 文学作品处理
- 必须识别并提取作品名称、作者、朝代、体裁等信息
#### 语言要素处理
- 提取重点词语、句式、修辞手法、表达技巧
#### 文言文处理
- 必须具体识别考察的文言实词或虚词（如"食"）
- 列出该词的所有常见含义
- 明确指出在当前语境中的具体含义
- 提供具体的判断依据
#### 📝 知识点提取要求
- ❌ 避免抽象表述：如"文言实词"
- ✅ 应具体表述：如"文言实词'食'的含义辨析"
- ❌ 避免泛化理解：如"理解文言实词含义"
- ✅ 应具体理解：如"'食'在此处指代具体的食物"
- ❌ 避免宽泛考点：如"文言文理解能力"
- ✅ 应具体考点：如"文言实词'食'的语境判断"

### 🇬🇧 英语学科
- **知识点重点**：具体的词汇用法、语法规则名称、句型结构、题型特征、写作要求、词根词缀、拼读音标
- **考点重点**：词汇运用、语法应用、句型变换、阅读理解、写作表达、词汇构成、语音识别

#### 写作题处理
- 必须识别作文类型（命题作文/读后续写）和文体类型（记叙文/说明文/议论文等）
- 明确写作话题和相关要求（如"介绍你所在的城市"）

#### 词汇/短语要素处理
- 当题目考察拼写、词义时，必须具体识别考察的关键词汇/短语及其含义
- 明确词汇的多重含义和用法，如"brave：勇敢的（形容词）；勇敢地面对（动词用法）"
- 对于动词短语，明确说明其具体含义，如"动词短语'go out'的含义"

#### 词根要素处理
- 当题目涉及词汇构成、词汇辨析、词汇记忆时，必须具体识别考察的词根及其基本含义
- 列出该词根的常见衍生词，如"vis-/vid-：看见（词根）→ visible可见的、video视频、vision视觉、visit参观"
- 明确指出词根在不同单词中的含义变化和应用
- 提供词根记忆和词汇扩展的具体方法

#### 拼读/音标要素处理
- 当题目包含音标或涉及"发音""读音"时，必须具体识别考察的拼读/音标
- 明确音标识别要求，如"字母u的发音""识别含/i:/发音的字母"
- 提供相关的语音规律和发音技巧

#### 题型要素处理
- 当识别为篇章题（文章+题目/挖空）时，必须识别具体题型类别：
  - **阅读理解**：考察主旨大意、文章结构、理解细节、推理判断
  - **完形填空**：考察上下文逻辑、词汇运用、语法知识
  - **语法填空**：考察语法规则、词形变化、语境理解
  - **信息还原**：考察语篇理解、逻辑连贯，需提取具体的句型、语法信息
- 明确解题技巧并总结篇章如何考察这些解题技巧
- 提供针对性的解题策略和方法

#### 课文要素处理
- 当识别为课文（只有文章或对话，没有题目或挖空）时：
- 提取课文的核心主题和教学目标
- 识别重点句型结构和用法
- 提取核心词汇及其在语境中的运用

#### 语法/句型要素处理
- 当题目考察语法规则、语法现象、句型结构时，必须具体识别考察的语法/句型
- 列出该语法点的构成规则，如"现在完成时：have/has + 过去分词"
- 明确指出其功能和用法，如"表示过去发生的动作对现在的影响"
- 提供具体的判断依据和使用场景

#### 词汇表要素处理
- 当识别为词汇表时，优先提取加粗的重点词汇
- 分析词汇的词性、含义和用法
- 识别词汇的重要程度和学习优先级

#### 📝 知识点提取要求
- ❌ 避免抽象表述：如"英语语法"
- ✅ 应具体表述：如"现在完成时have/has + 过去分词的用法"
- ❌ 避免泛化理解：如"掌握词汇用法"
- ✅ 应具体理解：如"brave作形容词'勇敢的'和动词'勇敢面对'的区别"
- ❌ 避免宽泛考点：如"语言运用能力"
- ✅ 应具体考点：如"现在完成时在语境中的时态判断"
- ❌ 避免抽象表述：如"词根知识"
- ✅ 应具体表述：如"vis-词根'看见'在visible、vision等词中的应用"
- ❌ 避免模糊表述：如"语音知识"
- ✅ 应具体表述：如"字母u在开音节中的/ju:/发音规律"

### 🔢 数学学科
- **知识点重点**：具体的概念定义、定理内容、公式表达、运算法则、几何性质
- **考点重点**：公式运用、定理应用、计算技巧、几何证明、数学建模
#### 概念处理
- 必须具体识别考察的数学概念及其定义
#### 公式处理
- 必须具体识别考察的公式及其适用条件
#### 📝 知识点提取要求
- ❌ 避免抽象表述：如"函数知识"
- ✅ 应具体表述：如"二次函数的性质与图像"
- ❌ 避免泛化理解：如"掌握运算法则"
- ✅ 应具体理解：如"完全平方公式(a±b)²=a²±2ab+b²的运用"
- ❌ 避免宽泛考点：如"计算能力"
- ✅ 应具体考点：如"利用配方法求二次函数的顶点坐标"

### ⚡ 物理学科
- **知识点重点**：具体的物理概念、定律表述、公式推导、实验现象、物理量关系
- **考点重点**：公式应用、实验操作、现象解释、图像分析、问题建模
#### 概念处理
- 必须具体识别考察的物理概念及其定义
#### 公式处理
- 必须具体识别考察的公式及其物理意义
#### 📝 知识点提取要求
- ❌ 避免抽象表述：如"力学知识"
- ✅ 应具体表述：如"牛顿第二定律F=ma的应用"
- ❌ 避免泛化理解：如"理解物理规律"
- ✅ 应具体理解：如"欧姆定律在串并联电路中的应用"
- ❌ 避免宽泛考点：如"实验能力"
- ✅ 应具体考点：如"测量小灯泡电功率的实验操作"

### 🧪 化学学科
- **知识点重点**：具体的化学概念、化学方程式、反应原理、物质性质、实验现象
- **考点重点**：方程式书写、反应类型判断、实验操作、现象分析、计算应用
#### 概念处理
- 必须具体识别考察的化学概念及其定义
#### 方程式处理
- 必须具体识别考察的化学方程式及其反应条件
#### 📝 知识点提取要求
- ❌ 避免抽象表述：如"化学反应"
- ✅ 应具体表述：如"氢气还原氧化铜的反应原理"
- ❌ 避免泛化理解：如"掌握化学性质"
- ✅ 应具体理解：如"碳酸钠与盐酸反应产生CO₂气体"
- ❌ 避免宽泛考点：如"实验技能"
- ✅ 应具体考点：如"制取氧气的实验装置选择和操作步骤"

### 🧬 生物学科
- **知识点重点**：具体的生物概念、生理过程、结构功能、生命现象、实验原理
- **考点重点**：概念理解、过程分析、结构识别、实验设计、现象解释
#### 概念处理
- 必须具体识别考察的生物概念及其特征
#### 过程处理
- 必须具体识别考察的生理过程及其步骤
#### 📝 知识点提取要求
- ❌ 避免抽象表述：如"细胞知识"
- ✅ 应具体表述：如"细胞膜的结构特点和功能"
- ❌ 避免泛化理解：如"理解生命活动"
- ✅ 应具体理解：如"植物呼吸作用中ATP的产生过程"
- ❌ 避免宽泛考点：如"观察能力"
- ✅ 应具体考点：如"显微镜观察洋葱表皮细胞的操作步骤"

### 🌍 地理学科
- **知识点重点**：具体的地理概念、地理规律、区域特征、地理要素、地理过程
- **考点重点**：地理分析、区域认知、地理实践、综合思维、地理解释
#### 专业术语处理
- 提取地理概念、地理名词、地理现象名称
#### 📝 知识点提取要求
- ❌ 避免抽象表述：如"地理知识"
- ✅ 应具体表述：如"气候类型的分布规律"
- ❌ 避免泛化理解：如"理解地理现象"
- ✅ 应具体理解：如"季风气候的形成原因和特征"
- ❌ 避免宽泛考点：如"地理分析能力"
- ✅ 应具体考点：如"根据等高线地形图判断地形特征"

---


## ⚠️ 异常处理

1. **字幕文本质量问题**：在JSON结构中添加 "warning":"字幕文本可能存在识别错误"
2. **模糊无法识别**：保持JSON结构

---

## 📤 输出规范
4. 输出结构体如下：
Stage string 学段
Subject string 学科
Type string 类型，例如："概念为主", "解题为主"
KnowledgePointKeys string   // 知识点列表
ExaminationPointKeys string    // 考点列表
Items []struct { // 题目信息，可以有多个
	Keys []struct {  // 知识点和考点列表，可以有多个
		KnowledgePoint      string   // 知识点 
		KnowledgePointKey   string  // 知识点关键理解
		ExaminationPoint    string   // 考点
		ExaminationPointKey string   // 考点关键理解
	} 
} 
Warning string 警告信息, 如果出现错误，保持JSON，错误信息放在warning中

## 输出示例
{
    "warning": "字幕文本质量问题（如有）",
    "stage": "学段",
    "subject": "学科分类",
    "reason1": "判断学段的逻辑",
    "reason2": "判断学科的逻辑",
    "subjectType": "理科/文科",
    "type": "概念为主/解题为主",
    "knowledgePointKeys": "核心知识点",
    "examinationPointKeys": "核心考点",
    "items": [
        {
            "chinese": ["语文要素1", "语文要素2", "语文要素3"],
            "english": ["英语要素1", "英语要素2", "英语要素3"],
            "keys": [
                {
                    "knowledgePoint": "该题目的知识点1（按学科特点调整）",
                    "knowledgePointKey": "该题目的知识点1的关键理解（按学科特点调整）",
                    "examinationPoint": "该题目的考点1（按学科特点调整）",
                    "examinationPointKey": "该题目的考点1的关键理解（按学科特点调整）"
                }
            ]
        }
    ]
}
`
)

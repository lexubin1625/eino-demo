package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	chatOpenAi "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"
	"github.com/kaptinlin/jsonrepair"
)

var (
	llmKey    = os.Getenv("DASHSCOPE_API_KEY")
	llmApi    = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	chatModel = "qwen-plus"
)

// KnowledgePoint 知识点结构
type KnowledgePoint struct {
	KnowledgePoint      string `json:"knowledgePoint"`
	KnowledgePointKey   string `json:"knowledgePointKey"`
	ExaminationPoint    string `json:"examinationPoint"`
	ExaminationPointKey string `json:"examinationPointKey"`
}

// Question 表示从图像中处理的问题
type Question struct {
	Content string            `json:"content"`
	Chinese []string          `json:"chinese"`
	English []string          `json:"english"`
	Keys    []*KnowledgePoint `json:"keys"`
}

// VisionResponse 视觉模型响应结构
type VisionResponse struct {
	Stage                string      `json:"stage"`
	Subject              string      `json:"subject"`
	Type                 string      `json:"type"`
	KnowledgePointKeys   string      `json:"knowledgePointKeys"`
	ExaminationPointKeys string      `json:"examinationPointKeys"`
	Items                []*Question `json:"items"`
	Warning              string      `json:"warning,omitempty"`
}

// RepairAndUnmarshal 修复 JSON 并反序列化到目标结构，返回反序列化后的结果
// 第一步：直接反序列化
// 第二步：代码修复后重试
// 第三步：模型修复（传入历史错误信息和历史对话消息）
func RepairAndUnmarshal[T any](ctx context.Context, rawJSON string, historyMessages []*schema.Message) (T, error) {
	var result T

	// 第一步：直接反序列化
	unmarshalErr := json.Unmarshal([]byte(rawJSON), &result)
	if unmarshalErr == nil {
		return result, nil
	}

	// 第二步：代码修复后重试
	repaired, repairErr := jsonrepair.JSONRepair(rawJSON)
	if repairErr == nil {
		// 代码修复成功，尝试反序列化
		if err := json.Unmarshal([]byte(repaired), &result); err == nil {
			// 反序列化成功，直接返回
			return result, nil
		}
	}

	// 第三步：使用模型修复（代码修复失败或反序列化失败时）
	llmRepaired, err := llmRepairJSON(ctx, rawJSON, historyMessages, unmarshalErr)
	if err != nil {
		return result, fmt.Errorf("模型修复失败: %w", err)
	}

	if err := json.Unmarshal([]byte(llmRepaired), &result); err != nil {
		return result, fmt.Errorf("模型修复后仍然无法反序列化: %w", err)
	}
	return result, nil
}

// llmRepairJSON 使用 LLM 修复 JSON
func llmRepairJSON(ctx context.Context, brokenJSON string, historyMessages []*schema.Message, err error) (string, error) {
	// 构建错误信息
	errorMsg := ""
	if err != nil {
		errorMsg = err.Error()
	}

	// 构建修复提示词
	prompt := fmt.Sprintf(`你是一个 JSON 修复助手。

但是你上一次的输出是：
%s

解析时发生了如下错误：
%s

请你只输出修复后的 JSON 字符串本身，不要加任何解释、前缀或 markdown 标记。`, brokenJSON, errorMsg)

	// 创建聊天模型
	chatModel, err := chatOpenAi.NewChatModel(ctx, &chatOpenAi.ChatModelConfig{
		APIKey:  llmKey,
		Model:   chatModel,
		Timeout: 60 * time.Second,
		BaseURL: llmApi,
	})
	if err != nil {
		return "", fmt.Errorf("创建聊天模型失败: %w", err)
	}

	// 构建消息列表：历史消息 + 当前修复请求
	messages := make([]*schema.Message, 0)
	messages = append(messages, historyMessages...)
	messages = append(messages, &schema.Message{
		Role:    schema.User,
		Content: prompt,
	})

	// 调用模型修复
	response, err := chatModel.Generate(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("LLM 生成失败: %w", err)
	}

	return response.Content, nil
}

// ParseVisionResponseWithRepair 解析并修复 JSON 到 VisionResponse 结构
func ParseVisionResponseWithRepair(ctx context.Context, rawJSON string, historyMessages []*schema.Message) (*VisionResponse, error) {
	result, err := RepairAndUnmarshal[VisionResponse](ctx, rawJSON, historyMessages)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func main() {
	ctx := context.Background()

	if llmKey == "" {
		log.Fatal("DASHSCOPE_API_KEY 未设置，请在环境变量中配置后再运行")
	}

	// 测试用例：简单结构
	fmt.Println("=== JSON 修复演示 ===")
	fmt.Println()

	type TestStruct struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
		City string `json:"city"`
	}

	testCases := []struct {
		name string
		json string
	}{
		{
			name: "正常 JSON",
			json: `{"name": "张三", "age": 30, "city": "北京"}`,
		},
		{
			name: "单引号问题",
			json: `{'name': '张三', 'age': 30}`,
		},
		{
			name: "多余逗号",
			json: `{"name": "张三", "age": 30,}`,
		},
		{
			name: "混合问题",
			json: `{'name': '张三', 'age': 30,}`,
		},
	}

	for i, tc := range testCases {
		fmt.Printf("--- 测试 %d: %s ---\n", i+1, tc.name)
		fmt.Printf("原始 JSON: %s\n", tc.json)

		result, err := RepairAndUnmarshal[TestStruct](ctx, tc.json, nil)
		if err != nil {
			fmt.Printf("❌ 修复失败: %v\n\n", err)
			continue
		}

		fmt.Printf("✅ 修复成功\n")
		fmt.Printf("解析结果: name=%s, age=%d, city=%s\n", result.Name, result.Age, result.City)
		fmt.Println()
	}

	// 演示 VisionResponse 反序列化
	fmt.Println("\n=== VisionResponse 反序列化演示 ===")
	fmt.Println()

	visionTestCases := []struct {
		name string
		json string
	}{
		{
			name: "正常 VisionResponse JSON",
			json: `{
				"stage": "初中",
				"subject": "数学",
				"type": "解题为主",
				"knowledgePointKeys": "二次函数",
				"examinationPointKeys": "求顶点坐标",
				"items": [{
					"content": "求解二次函数 y=x²+2x+1 的顶点坐标",
					"chinese": [],
					"english": [],
					"keys": [{
						"knowledgePoint": "二次函数",
						"knowledgePointKey": "y=ax²+bx+c 的形式",
						"examinationPoint": "配方法",
						"examinationPointKey": "将一般式化为顶点式"
					}]
				}]
			}`,
		},
		{
			name: "损坏的 VisionResponse JSON（单引号）",
			json: `{
				'stage': '高中',
				'subject': '语文',
				'type': '概念为主',
				'knowledgePointKeys': '文言文',
				'examinationPointKeys': '实词理解',
				'items': [{
					'content': '解释下列文言实词的含义',
					'keys': [{
						'knowledgePoint': '文言实词',
						'examinationPoint': '语境判断'
					}]
				}]
			}`,
		},
		{
			name: "损坏的 VisionResponse JSON（多余逗号）",
			json: `{
				"stage": "小学",
				"subject": "英语",
				"type": "概念为主",
				"knowledgePointKeys": "词汇",
				"examinationPointKeys": "拼写",
				"items": [{
					"content": "拼写单词",
					"keys": [{
						"knowledgePoint": "单词拼写",
						"examinationPoint": "字母组合",
					}],
				}],
			}`,
		},
	}

	for i, tc := range visionTestCases {
		fmt.Printf("--- VisionResponse 测试 %d: %s ---\n", i+1, tc.name)
		fmt.Printf("原始 JSON: %s\n", tc.json)

		// 示例：传入历史对话消息
		historyMessages := []*schema.Message{
			{
				Role:    schema.System,
				Content: "你是一个教育内容解析系统，需要返回结构化的 JSON 数据。",
			},
			{
				Role:    schema.User,
				Content: "请解析这张图片中的题目内容",
			},
			{
				Role:    schema.Assistant,
				Content: "好的，我会解析图片并返回 VisionResponse 格式的 JSON。",
			},
		}

		visionResp, err := ParseVisionResponseWithRepair(ctx, tc.json, historyMessages)
		if err != nil {
			fmt.Printf("❌ 解析失败: %v\n\n", err)
			continue
		}

		fmt.Printf("✅ 解析成功\n")
		fmt.Printf("学段: %s\n", visionResp.Stage)
		fmt.Printf("学科: %s\n", visionResp.Subject)
		fmt.Printf("类型: %s\n", visionResp.Type)
		fmt.Printf("知识点: %s\n", visionResp.KnowledgePointKeys)
		fmt.Printf("考点: %s\n", visionResp.ExaminationPointKeys)
		fmt.Printf("题目数量: %d\n", len(visionResp.Items))
		if len(visionResp.Items) > 0 {
			fmt.Printf("第一题内容: %s\n", visionResp.Items[0].Content)
			if len(visionResp.Items[0].Keys) > 0 {
				fmt.Printf("第一题知识点: %s\n", visionResp.Items[0].Keys[0].KnowledgePoint)
			}
		}
		fmt.Println()
	}
}

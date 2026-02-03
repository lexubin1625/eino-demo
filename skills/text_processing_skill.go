package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// TextProcessingParams 文本处理参数
type TextProcessingParams struct {
	Operation string `json:"operation" jsonschema:"required,description=处理类型: count_words(统计字数), count_chars(统计字符数), upper(转大写), lower(转小写), reverse(反转), remove_spaces(移除空格), capitalize(首字母大写)"`
	Text      string `json:"text" jsonschema:"required,description=要处理的文本"`
}

// TextProcessingSkill 文本处理技能
func TextProcessingSkill() (tool.InvokableTool, error) {
	return utils.InferTool(
		"text_processing",
		"对文本进行各种处理操作，包括统计、转换、格式化等",
		processText,
	)
}

func processText(ctx context.Context, p *TextProcessingParams) (string, error) {
	var result interface{}

	switch p.Operation {
	case "count_words":
		words := strings.Fields(p.Text)
		result = map[string]interface{}{
			"word_count": len(words),
			"text":       p.Text,
		}
	case "count_chars":
		result = map[string]interface{}{
			"char_count":           len([]rune(p.Text)),
			"char_count_no_spaces": len(strings.ReplaceAll(p.Text, " ", "")),
			"text":                 p.Text,
		}
	case "upper":
		result = map[string]interface{}{
			"original": p.Text,
			"result":   strings.ToUpper(p.Text),
		}
	case "lower":
		result = map[string]interface{}{
			"original": p.Text,
			"result":   strings.ToLower(p.Text),
		}
	case "reverse":
		runes := []rune(p.Text)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		result = map[string]interface{}{
			"original": p.Text,
			"result":   string(runes),
		}
	case "remove_spaces":
		result = map[string]interface{}{
			"original": p.Text,
			"result":   strings.ReplaceAll(p.Text, " ", ""),
		}
	case "capitalize":
		if len(p.Text) == 0 {
			result = map[string]interface{}{
				"original": p.Text,
				"result":   p.Text,
			}
		} else {
			runes := []rune(p.Text)
			runes[0] = unicode.ToUpper(runes[0])
			result = map[string]interface{}{
				"original": p.Text,
				"result":   string(runes),
			}
		}
	default:
		return "", fmt.Errorf("不支持的操作: %s", p.Operation)
	}

	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

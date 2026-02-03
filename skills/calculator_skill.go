package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// CalculatorParams 计算器参数
type CalculatorParams struct {
	Operation string  `json:"operation" jsonschema:"required,description=运算类型: add(加法), sub(减法), mul(乘法), div(除法), pow(幂运算), sqrt(开方)"`
	A         float64 `json:"a" jsonschema:"required,description=第一个操作数"`
	B         float64 `json:"b" jsonschema:"description=第二个操作数(除法、减法、乘法、幂运算时需要)"`
}

// CalculatorSkill 计算器技能
func CalculatorSkill() (tool.InvokableTool, error) {
	return utils.InferTool(
		"calculator",
		"执行基本数学计算，支持加法、减法、乘法、除法、幂运算和开方",
		calculate,
	)
}

func calculate(ctx context.Context, p *CalculatorParams) (string, error) {
	var result float64
	var err error

	switch p.Operation {
	case "add":
		result = p.A + p.B
	case "sub":
		result = p.A - p.B
	case "mul":
		result = p.A * p.B
	case "div":
		if p.B == 0 {
			return "", fmt.Errorf("除数不能为零")
		}
		result = p.A / p.B
	case "pow":
		result = math.Pow(p.A, p.B)
	case "sqrt":
		if p.A < 0 {
			return "", fmt.Errorf("不能对负数开方")
		}
		result = math.Sqrt(p.A)
	default:
		return "", fmt.Errorf("不支持的操作: %s", p.Operation)
	}

	resultJSON, _ := json.Marshal(map[string]interface{}{
		"operation": p.Operation,
		"result":    result,
		"formula":   formatFormula(p.Operation, p.A, p.B, result),
	})

	return string(resultJSON), err
}

func formatFormula(op string, a, b float64, result float64) string {
	switch op {
	case "add":
		return fmt.Sprintf("%.2f + %.2f = %.2f", a, b, result)
	case "sub":
		return fmt.Sprintf("%.2f - %.2f = %.2f", a, b, result)
	case "mul":
		return fmt.Sprintf("%.2f × %.2f = %.2f", a, b, result)
	case "div":
		return fmt.Sprintf("%.2f ÷ %.2f = %.2f", a, b, result)
	case "pow":
		return fmt.Sprintf("%.2f ^ %.2f = %.2f", a, b, result)
	case "sqrt":
		return fmt.Sprintf("√%.2f = %.2f", a, result)
	default:
		return ""
	}
}

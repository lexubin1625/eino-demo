package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// DataAnalysisParams 数据分析参数
type DataAnalysisParams struct {
	Operation string `json:"operation" jsonschema:"required,description=分析类型: mean(平均值), median(中位数), max(最大值), min(最小值), sum(求和), std_dev(标准差), sort(排序)"`
	Data      string `json:"data" jsonschema:"required,description=数据，用逗号分隔的数字字符串，例如: 1,2,3,4,5"`
}

// DataAnalysisSkill 数据分析技能
func DataAnalysisSkill() (tool.InvokableTool, error) {
	return utils.InferTool(
		"data_analysis",
		"对数字数组进行统计分析，包括平均值、中位数、最大值、最小值、求和、标准差和排序",
		analyzeData,
	)
}

func analyzeData(ctx context.Context, p *DataAnalysisParams) (string, error) {
	// 解析数据
	parts := strings.Split(p.Data, ",")
	numbers := make([]float64, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		num, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return "", fmt.Errorf("无效的数字: %s", part)
		}
		numbers = append(numbers, num)
	}

	if len(numbers) == 0 {
		return "", fmt.Errorf("数据为空")
	}

	var result interface{}

	switch p.Operation {
	case "mean":
		sum := 0.0
		for _, n := range numbers {
			sum += n
		}
		mean := sum / float64(len(numbers))
		result = map[string]interface{}{
			"mean":  mean,
			"count": len(numbers),
			"data":  numbers,
		}
	case "median":
		sorted := make([]float64, len(numbers))
		copy(sorted, numbers)
		sort.Float64s(sorted)
		var median float64
		if len(sorted)%2 == 0 {
			median = (sorted[len(sorted)/2-1] + sorted[len(sorted)/2]) / 2
		} else {
			median = sorted[len(sorted)/2]
		}
		result = map[string]interface{}{
			"median": median,
			"count":  len(numbers),
			"data":   numbers,
		}
	case "max":
		max := numbers[0]
		for _, n := range numbers[1:] {
			if n > max {
				max = n
			}
		}
		result = map[string]interface{}{
			"max":   max,
			"count": len(numbers),
			"data":  numbers,
		}
	case "min":
		min := numbers[0]
		for _, n := range numbers[1:] {
			if n < min {
				min = n
			}
		}
		result = map[string]interface{}{
			"min":   min,
			"count": len(numbers),
			"data":  numbers,
		}
	case "sum":
		sum := 0.0
		for _, n := range numbers {
			sum += n
		}
		result = map[string]interface{}{
			"sum":   sum,
			"count": len(numbers),
			"data":  numbers,
		}
	case "std_dev":
		// 计算平均值
		sum := 0.0
		for _, n := range numbers {
			sum += n
		}
		mean := sum / float64(len(numbers))

		// 计算方差
		variance := 0.0
		for _, n := range numbers {
			variance += math.Pow(n-mean, 2)
		}
		variance /= float64(len(numbers))

		// 计算标准差
		stdDev := math.Sqrt(variance)
		result = map[string]interface{}{
			"std_dev": stdDev,
			"mean":    mean,
			"count":   len(numbers),
			"data":    numbers,
		}
	case "sort":
		sorted := make([]float64, len(numbers))
		copy(sorted, numbers)
		sort.Float64s(sorted)
		result = map[string]interface{}{
			"original": numbers,
			"sorted":   sorted,
			"count":    len(numbers),
		}
	default:
		return "", fmt.Errorf("不支持的操作: %s", p.Operation)
	}

	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

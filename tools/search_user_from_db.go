package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type UserQueryParams struct {
	Name string `json:"name" jsonschema:"required,description=要查询的用户名"`
}

type UserInfo struct {
	Company string `json:"company"`
	Title   string `json:"title"`
	Email   string `json:"email"`
}

func search_user_info_from_db(ctx context.Context, p *UserQueryParams) (string, error) {
	fmt.Print("search_user_info工具调用")
	// 模拟数据库
	db := map[string]UserInfo{
		"张三": {Company: "阿里巴巴", Title: "后端工程师", Email: "zhangsan@example.com"},
		"李四": {Company: "字节跳动", Title: "数据分析师", Email: "lisi@example.com"},
		"王五": {Company: "华为", Title: "产品经理", Email: "wangwu@example.com"},
	}
	if p == nil || strings.TrimSpace(p.Name) == "" {
		b, _ := json.Marshal(map[string]any{"error": "name is required"})
		return string(b), nil
	}
	if u, ok := db[p.Name]; ok {
		b, _ := json.Marshal(map[string]any{
			"found": true,
			"name":  p.Name,
			"user":  u,
		})
		return string(b), nil
	}
	b, _ := json.Marshal(map[string]any{"found": false, "msg": "user not found"})
	return string(b), nil
}
func search_user_info() (tool.InvokableTool, error) {
	// 使用 InferTool 快速构建可调用工具
	userTool, err := utils.InferTool(
		"search_user_info",
		"根据用户名查询用户信息（公司、职位、邮箱）",
		search_user_info_from_db,
	)
	if err != nil {
		log.Fatalf("创建用户查询工具失败: %v", err)
	}

	return userTool, err
}

package main

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// SkillManager 技能管理器
type SkillManager struct {
	skills map[string]tool.InvokableTool
	tools  []tool.BaseTool
}

// NewSkillManager 创建技能管理器
func NewSkillManager() *SkillManager {
	return &SkillManager{
		skills: make(map[string]tool.InvokableTool),
		tools:  make([]tool.BaseTool, 0),
	}
}

// RegisterSkill 注册技能
func (sm *SkillManager) RegisterSkill(name string, skill tool.InvokableTool) error {
	if _, exists := sm.skills[name]; exists {
		return fmt.Errorf("技能 %s 已存在", name)
	}
	sm.skills[name] = skill
	sm.tools = append(sm.tools, skill.(tool.BaseTool))
	return nil
}

// GetSkill 获取技能
func (sm *SkillManager) GetSkill(name string) (tool.InvokableTool, bool) {
	skill, exists := sm.skills[name]
	return skill, exists
}

// GetAllSkills 获取所有技能
func (sm *SkillManager) GetAllSkills() []tool.BaseTool {
	return sm.tools
}

// GetToolInfos 获取所有工具信息
func (sm *SkillManager) GetToolInfos(ctx context.Context) ([]*schema.ToolInfo, error) {
	toolInfos := make([]*schema.ToolInfo, 0, len(sm.tools))
	for _, t := range sm.tools {
		info, err := t.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("获取工具信息失败: %w", err)
		}
		toolInfos = append(toolInfos, info)
	}
	return toolInfos, nil
}

// ListSkills 列出所有已注册的技能名称
func (sm *SkillManager) ListSkills() []string {
	names := make([]string, 0, len(sm.skills))
	for name := range sm.skills {
		names = append(names, name)
	}
	return names
}

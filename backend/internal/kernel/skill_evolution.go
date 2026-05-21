package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SkillEvolution 技能自动进化
// 监控模式检测和学习洞察，自动创建/优化技能
type SkillEvolution struct {
	skillManager *SkillManager
	dir          string
}

// NewSkillEvolution 创建技能进化器
func NewSkillEvolution(skillManager *SkillManager, dir string) *SkillEvolution {
	os.MkdirAll(dir, 0755)
	return &SkillEvolution{skillManager: skillManager, dir: dir}
}

// Evolve 根据模式和洞察进化技能
func (se *SkillEvolution) Evolve(ctx context.Context, patterns []Pattern, insights []Insight) {
	// 从重复查询模式创建新技能
	for _, p := range patterns {
		if p.Type == "repeated_query" && p.Frequency >= 3 {
			se.createSkillFromPattern(p)
		}
	}

	// 从工具序列模式创建新技能
	for _, p := range patterns {
		if p.Type == "tool_sequence" && p.Frequency >= 3 {
			se.createSkillFromToolSequence(p)
		}
	}

	// 从偏好洞察优化现有技能
	for _, in := range insights {
		if in.Type == "preference" && in.Frequency >= 3 {
			se.evolveSkillFromInsight(in)
		}
	}

	// 从失败模式创建调试技能
	for _, p := range patterns {
		if p.Type == "error_prone" && p.Frequency >= 5 {
			se.createSkillFromErrorPattern(p)
		}
	}
}

func (se *SkillEvolution) createSkillFromPattern(p Pattern) {
	// 提取查询关键词作为 skill 名
	desc := p.Description
	name := extractSkillName(desc, "query")

	// 检查是否已有近似 skill
	if se.skillManager.Get(name) != nil {
		return // 已存在，跳过
	}

	skill := &Skill{
		ID:          name,
		Name:        fmt.Sprintf("自动技能: %s", name),
		Description: desc,
		Prompt: fmt.Sprintf(`## 自动技能: %s
根据历史对话模式总结的技能:
- 用户经常询问此类问题
- 建议预先准备相关知识
- 给出清晰、结构化的回答
- 必要时使用工具验证信息`, name),
		Tools:   []string{"search_files", "read_file"},
		Enabled: true,
	}

	se.saveSkill(skill)
	slog.Info("Auto-created skill from pattern", "id", skill.ID, "pattern", p.Type)
}

func (se *SkillEvolution) createSkillFromToolSequence(p Pattern) {
	desc := p.Description
	name := extractSkillName(desc, "tool")

	if se.skillManager.Get(name) != nil {
		return
	}

	// 从描述中提取工具名
	tools := extractToolNames(desc)
	if len(tools) == 0 {
		tools = []string{"execute_command", "read_file"}
	}

	skill := &Skill{
		ID:          name,
		Name:        fmt.Sprintf("工作流: %s", name),
		Description: desc,
		Prompt: fmt.Sprintf(`## 工作流: %s
根据历史成功经验总结的工作流:
1. 按已验证的顺序执行工具
2. 每一步检查结果后再继续
3. 遇到错误时回退到上一步`, name),
		Tools:   tools,
		Enabled: true,
	}

	se.saveSkill(skill)
	slog.Info("Auto-created skill from tool sequence", "id", skill.ID, "pattern", p.Type)
}

func (se *SkillEvolution) evolveSkillFromInsight(in Insight) {
	// 根据用户偏好优化匹配关键词 — 目前只是记录
	// 未来可调整 skill 的 Prompt 内容
}

func (se *SkillEvolution) createSkillFromErrorPattern(p Pattern) {
	name := "error-recovery"
	if se.skillManager.Get(name) != nil {
		return
	}

	skill := &Skill{
		ID:          name,
		Name:        "错误恢复",
		Description: p.Description,
		Prompt: `## 错误恢复模式
系统检测到频繁错误。请按以下步骤排查:
1. 检查上一步操作的输入/参数是否正确
2. 使用更短的命令或更小的改动范围重试
3. 如果工具超时，尝试增加超时时间或分解操作
4. 如果权限错误，检查文件路径和权限
5. 持续失败时，换一种完全不同的实现方式`,
		Tools:   []string{"execute_command", "read_file", "search_files"},
		Enabled: true,
	}

	se.saveSkill(skill)
	slog.Info("Auto-created error-recovery skill from error pattern", "count", p.Frequency)
}

func (se *SkillEvolution) saveSkill(skill *Skill) {
	path := filepath.Join(se.dir, skill.ID+".json")
	data, _ := json.MarshalIndent(skill, "", "  ")
	if err := os.WriteFile(path, data, 0644); err != nil {
		slog.Warn("Failed to save auto-created skill", "id", skill.ID, "error", err)
		return
	}
	// 注册到技能管理器
	se.skillManager.skills[skill.ID] = skill
	se.skillManager.loadFromDisk() // 重新加载确保一致性
}

func extractSkillName(desc, prefix string) string {
	// 从描述中提取简短标识符
	cleaned := strings.TrimPrefix(desc, "常见工具序列: ")
	cleaned = strings.TrimPrefix(cleaned, "用户重复询问相似问题: ")
	cleaned = strings.TrimPrefix(cleaned, "对话中频繁出现错误")
	cleaned = strings.TrimPrefix(cleaned, "频繁使用工具: ")
	cleaned = strings.TrimSpace(cleaned)

	// 取前20个字符作为 ID
	name := strings.ToLower(cleaned)
	if len(name) > 20 {
		name = name[:20]
	}
	// 替换非字母数字字符
	var clean strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			clean.WriteRune(r)
		} else {
			clean.WriteRune('-')
		}
	}
	result := strings.Trim(clean.String(), "-")
	if result == "" {
		result = fmt.Sprintf("auto-%s-%d", prefix, time.Now().UnixNano()%1000)
	}
	return result
}

func extractToolNames(desc string) []string {
	// 从 "常见工具序列: read_file -> write_file" 中提取工具名
	parts := strings.Split(desc, " -> ")
	if len(parts) < 2 {
		return nil
	}
	tools := make([]string, 0, len(parts))
	for _, p := range parts {
		tool := strings.TrimSpace(p)
		if strings.HasPrefix(tool, "常见工具序列: ") {
			tool = strings.TrimPrefix(tool, "常见工具序列: ")
		}
		if tool != "" {
			tools = append(tools, tool)
		}
	}
	return tools
}



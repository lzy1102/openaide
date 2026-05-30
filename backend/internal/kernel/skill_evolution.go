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
	llm          LLMProvider // LLM 用于生成有意义的技能描述
}

// NewSkillEvolution 创建技能进化器
func NewSkillEvolution(skillManager *SkillManager, dir string) *SkillEvolution {
	os.MkdirAll(dir, 0755)
	return &SkillEvolution{skillManager: skillManager, dir: dir}
}

// SetLLM sets the LLM provider for generating skill prompts
func (se *SkillEvolution) SetLLM(llm LLMProvider) {
	se.llm = llm
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
	desc := p.Description
	name := extractSkillName(desc, "query")
	if se.skillManager.Get(name) != nil { return }

	keywords := extractKeywords(desc)
	prompt := se.generateSkillPrompt("repeated user query", desc, keywords)
	skill := &Skill{
		ID: name, Name: "Auto: " + name, Description: desc,
		Keywords: keywords, Prompt: prompt,
		Tools: []string{"search_files", "read_file"}, Enabled: true,
	}
	se.saveSkill(skill)
	slog.Info("Auto-created skill", "id", skill.ID)
}

func (se *SkillEvolution) createSkillFromToolSequence(p Pattern) {
	desc := p.Description
	name := extractSkillName(desc, "tool")
	if se.skillManager.Get(name) != nil { return }

	tools := extractToolNames(desc)
	if len(tools) == 0 { tools = []string{"execute_command", "read_file"} }
	keywords := extractKeywords(desc)
	prompt := se.generateSkillPrompt("successful tool sequence", desc, keywords)
	skill := &Skill{
		ID: name, Name: "Workflow: " + name, Description: desc,
		Keywords: keywords, Prompt: prompt, Tools: tools, Enabled: true,
	}
	se.saveSkill(skill)
	slog.Info("Auto-created workflow skill", "id", skill.ID)
}

func (se *SkillEvolution) evolveSkillFromInsight(in Insight) {
	// Use LLM to refine existing skill keywords or prompt based on preference
	if se.llm == nil { return }
	// For now: add the insight content as a keyword to matching skills
	for _, skill := range se.skillManager.List() {
		if skill.ID == "general" || skill.ID == "error-recovery" { continue }
		skill.Keywords = append(skill.Keywords, in.Content)
	}
}

func (se *SkillEvolution) createSkillFromErrorPattern(p Pattern) {
	name := "error-recovery"
	if se.skillManager.Get(name) != nil { return }

	prompt := se.generateSkillPrompt("frequent errors and failures", p.Description, nil)
	skill := &Skill{
		ID: name, Name: "Error Recovery", Description: p.Description,
		Keywords: []string{"error", "failed", "timeout", "错误", "失败"},
		Prompt: prompt, Tools: []string{"execute_command", "read_file", "search_files"},
		Enabled: true,
	}
	se.saveSkill(skill)
	slog.Info("Auto-created error-recovery skill", "count", p.Frequency)
}

// generateSkillPrompt uses LLM to create a meaningful skill prompt from pattern data.
// Falls back to template if LLM is unavailable.
func (se *SkillEvolution) generateSkillPrompt(category, desc string, keywords []string) string {
	if se.llm == nil {
		return se.fallbackPrompt(category, desc)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	messages := []Message{
		{Role: "user", Content: fmt.Sprintf(
			"You are creating an AI skill definition. Write a SHORT skill prompt (3-5 bullet points) "+
				"for this skill based on observed patterns:\n"+
				"Category: %s\nDescription: %s\nKeywords: %v\n\n"+
				"The prompt should guide an AI on HOW to handle this type of request. "+
				"Be specific, actionable, and concise. Output ONLY the bullet points, no intro.",
			category, desc, keywords),
		},
	}
	resp, err := se.llm.Chat(ctx, messages, nil, map[string]interface{}{
		"max_tokens": 300, "temperature": 0.3, "route": "execution", "no_thinking": true,
	})
	if err != nil || resp.Content == "" {
		return se.fallbackPrompt(category, desc)
	}
	// Clean up and prefix
	cleaned := strings.TrimSpace(resp.Content)
	cleaned = strings.TrimPrefix(cleaned, "- ")
	return fmt.Sprintf("## Auto-generated Skill: %s\n%s", strings.ToTitle(category), cleaned)
}

// fallbackPrompt returns a template when LLM is unavailable.
func (se *SkillEvolution) fallbackPrompt(category, desc string) string {
	return fmt.Sprintf("## %s\n"+
		"Based on observed patterns: %s\n"+
		"- Follow the proven approach for this type of task\n"+
		"- Verify results at each step\n"+
		"- Adapt based on specific context", strings.ToTitle(category), desc)
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

func extractKeywords(desc string) []string {
	// 从描述中提取关键词作为 skill 匹配关键字
	cleaned := strings.TrimPrefix(desc, "常见工具序列: ")
	cleaned = strings.TrimPrefix(cleaned, "用户重复询问相似问题: ")
	cleaned = strings.TrimPrefix(cleaned, "对话中频繁出现错误")
	cleaned = strings.TrimPrefix(cleaned, "频繁使用工具: ")
	cleaned = strings.TrimSpace(cleaned)

	// 从 cleaned 中拆分出单词作为关键词
	var kws []string
	for _, word := range strings.Fields(cleaned) {
		word = strings.Trim(word, ":,;.!?\"'")
		if len(word) >= 2 {
			kws = append(kws, strings.ToLower(word))
		}
	}
	if len(kws) == 0 {
		// fallback: 用 "帮助" 作为通用关键词
		kws = []string{"帮助", "help"}
	}
	return kws
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



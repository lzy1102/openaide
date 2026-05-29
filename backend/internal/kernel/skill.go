package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"log/slog"
	"time"
)

// Skill 可插拔的能力模块
type Skill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Prompt      string   `json:"prompt"`
	Keywords    []string `json:"keywords"`
	Tools       []string `json:"tools"`
	Enabled     bool     `json:"enabled"`

	// Feedback tracking for smart activation
	UsageCount   int       `json:"usage_count"`
	SuccessCount int       `json:"success_count"`
	Confidence   float64   `json:"confidence"` // 0-1, start at 0.5, auto-disable <0.3
	LastUsed     time.Time `json:"last_used"`
}

// SkillManager 技能管理器
type SkillManager struct {
	skills    map[string]*Skill
	dir       string
	autoDetect    bool
	llm          LLMProvider
	lastActivated string     // skill activated in current query (for feedback)
}

// NewSkillManager 创建技能管理器
func NewSkillManager(skillsDir string) *SkillManager {
	os.MkdirAll(skillsDir, 0755)
	sm := &SkillManager{
		skills:     make(map[string]*Skill),
		dir:        skillsDir,
		autoDetect: true,
	}
	sm.loadBuiltins()
	sm.loadFromDisk()
	return sm
}

// AddClaudeSkill 从 Claude 插件格式添加技能（由外部调用，避免循环导入）
func (sm *SkillManager) AddClaudeSkill(id, name, description, prompt string, tools, keywords []string) {
	if _, exists := sm.skills[id]; exists {
		return
	}
	sk := &Skill{
		ID:          id,
		Name:        name,
		Description: description,
		Prompt:      prompt,
		Tools:       tools,
		Keywords:    keywords,
		Enabled:     true,
	}
	// 自动补充关键词：name 的分隔部分 + 常用中文映射
	if len(sk.Keywords) == 0 {
		sk.Keywords = autoKeywords(name, description)
	}
	sm.skills[sk.ID] = sk
}

// GetSlashCommands 获取所有技能对应的斜杠命令
func (sm *SkillManager) GetSlashCommands() map[string]string {
	cmds := make(map[string]string)
	for _, s := range sm.skills {
		if s.Enabled {
			cmds["/"+s.Name] = s.ID
			// 也注册 ID 作为命令
			cmds["/"+s.ID] = s.ID
		}
	}
	return cmds
}

// autoKeywords 从 name 和 description 自动生成关键词（兜底）
func autoKeywords(name, description string) []string {
	var kw []string
	text := strings.ToLower(name + " " + description)
	for _, part := range strings.Split(name, "-") {
		if len(part) > 1 {
			kw = append(kw, part)
		}
	}
	kw = append(kw, name)
	for _, term := range []string{"代码", "审查", "安全", "重构", "调试", "测试", "文档",
		"部署", "提交", "搜索", "解释", "分析", "翻译", "review", "debug", "test", "refactor"} {
		if strings.Contains(text, term) {
			kw = append(kw, term)
		}
	}
	return kw
}

// loadBuiltins 加载内置技能
func (sm *SkillManager) loadBuiltins() {
	builtins := []Skill{
		{
			ID: "code-review", Name: "代码审查",
			Description: "审查代码质量、安全问题、最佳实践",
			Keywords:    []string{"review", "审查", "检查代码", "code review", "安全漏洞", "代码质量"},
			Prompt: `## 代码审查模式
你是代码审查专家。审查时关注:
1. 安全问题 (SQL注入、XSS、命令注入)
2. 错误处理 (是否有遗漏的error check)
3. 性能问题 (不必要的循环、内存泄漏)
4. 可读性 (命名、注释、结构)
5. 最佳实践 (SOLID、DRY、KISS)
输出格式: 问题严重度 + 文件:行号 + 问题描述 + 修复建议`,
			Tools:   []string{"read_file", "search_files", "git_status"},
			Enabled: true,
		},
		{
			ID: "git-commit", Name: "Git提交助手",
			Description: "分析改动并生成规范的commit message",
			Keywords:    []string{"commit", "提交", "git add", "git commit", "暂存"},
			Prompt: `## Git 提交模式
1. 先 git_status 查看改动
2. 分析改动内容
3. 生成规范的 commit message:
   - type(scope): short description
   - 类型: feat/fix/docs/refactor/test/chore
4. 执行 git add + git commit`,
			Tools:   []string{"git_status", "execute_command"},
			Enabled: true,
		},
		{
			ID: "debug", Name: "调试助手",
			Description: "系统化的问题排查和调试",
			Keywords:    []string{"debug", "调试", "报错", "error", "失败", "不工作", "bug", "修复", "fix"},
			Prompt: `## 调试模式
按以下步骤系统排查:
1. 复现问题: 执行相关命令，确认错误信息
2. 隔离范围: 缩小问题范围，确定是哪个模块/文件
3. 根因分析: 找到根本原因
4. 修复方案: 提出修复方案
5. 验证: 执行验证命令确认修复`,
			Tools:   []string{"execute_command", "read_file", "search_files"},
			Enabled: true,
		},
		{
			ID: "refactor", Name: "代码重构",
			Description: "安全地重构代码，保持功能不变",
			Keywords:    []string{"refactor", "重构", "重写", "整理代码", "优化结构"},
			Prompt: `## 重构模式
重构原则:
1. 先理解现有代码的功能和测试
2. 小步修改，每次只改一个关注点
3. 保持外部行为不变
4. 每次修改后运行测试验证
5. 使用IDE重构工具优先，手动修改为辅`,
			Tools:   []string{"read_file", "write_file", "search_files", "execute_command"},
			Enabled: true,
		},
		{
			ID: "explain", Name: "代码解释",
			Description: "详细解释代码逻辑和设计意图",
			Keywords:    []string{"explain", "解释", "说明", "这段代码", "什么意思", "做什么的"},
			Prompt: `## 代码解释模式
解释代码时包含:
1. 整体目的: 这段代码做什么
2. 关键逻辑: 核心算法/流程
3. 数据结构: 使用了什么数据结构和为什么
4. 边界情况: 处理了哪些edge case
5. 改进建议: 可以优化的地方`,
			Tools:   []string{"read_file", "search_files"},
			Enabled: true,
		},
	}
	for i := range builtins {
		sm.skills[builtins[i].ID] = &builtins[i]
		sm.initSkillConfidence(builtins[i].ID)
	}
}

func (sm *SkillManager) loadFromDisk() {
	entries, err := os.ReadDir(sm.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sm.dir, e.Name()))
		if err != nil {
			continue
		}
		var skill Skill
		if json.Unmarshal(data, &skill) == nil {
			sm.skills[skill.ID] = &skill
			sm.initSkillConfidence(skill.ID)
		}
	}
}

// SetLLM 注入 LLM 用于语义技能匹配
func (sm *SkillManager) SetLLM(llm LLMProvider) { sm.llm = llm }

// Get 获取技能
func (sm *SkillManager) Get(id string) *Skill {
	return sm.skills[id]
}

// List 列出所有技能
func (sm *SkillManager) List() []*Skill {
	var result []*Skill
	for _, s := range sm.skills {
		result = append(result, s)
	}
	return result
}

// EnabledSkills 获取启用的技能列表
func (sm *SkillManager) EnabledSkills() []*Skill {
	var result []*Skill
	for _, s := range sm.skills {
		if s.Enabled {
			result = append(result, s)
		}
	}
	return result
}

// DetectSkill 自动检测用户意图并匹配技能（LLM 优先，关键词兜底）

// RecordSkillUsage records that a skill was activated and whether it was helpful.
// qualityScore: reflection quality 1-10. >=6 = success.
func (sm *SkillManager) RecordSkillUsage(skillID string, qualityScore int) {
	if skill, ok := sm.skills[skillID]; ok {
		skill.UsageCount++
		skill.LastUsed = time.Now()
		if qualityScore >= 6 {
			skill.SuccessCount++
			skill.Confidence = min(0.95, skill.Confidence+0.05)
		} else {
			skill.Confidence = max(0.1, skill.Confidence-0.1)
		}
		if skill.Confidence < 0.3 {
			skill.Enabled = false
			slog.Info("Skill auto-disabled due to low confidence", "id", skillID, "confidence", skill.Confidence)
		} else if !skill.Enabled && skill.Confidence >= 0.5 {
			skill.Enabled = true
			slog.Info("Skill re-enabled after confidence recovered", "id", skillID, "confidence", skill.Confidence)
		}
	}
}

// RecordLastSkillUsage records quality feedback for the most recently activated skill.
// Call this after reflection completes with the quality score.
func (sm *SkillManager) RecordLastSkillUsage(qualityScore int) {
	if sm.lastActivated != "" {
		sm.RecordSkillUsage(sm.lastActivated, qualityScore)
		sm.lastActivated = "" // consume
	}
}

// DecayUnusedSkills decreases confidence for skills not used in 7 days.
func (sm *SkillManager) DecayUnusedSkills() {
	now := time.Now()
	for id, skill := range sm.skills {
		if skill.LastUsed.IsZero() { continue }
		if now.Sub(skill.LastUsed) > 7*24*time.Hour {
			skill.Confidence = max(0.1, skill.Confidence-0.05)
			if skill.Confidence < 0.3 { skill.Enabled = false }
		}
		_ = id
	}
}

// Initialize new skills with default confidence
func (sm *SkillManager) initSkillConfidence(id string) {
	if skill, ok := sm.skills[id]; ok && skill.Confidence == 0 {
		skill.Confidence = 0.5
	}
}

func (sm *SkillManager) DetectSkill(query string) *Skill {
	slog.Debug("Skill detect", "query", query[:min(80, len(query))])
	if !sm.autoDetect {
		return nil
	}

	// LLM 语义匹配
	if sm.llm != nil {
		if skill := sm.detectWithLLM(query); skill != nil {
			return skill
		}
	}

	// 关键词兜底
	lower := strings.ToLower(query)
	var bestSkill *Skill
	bestScore := 0
	for _, skill := range sm.skills {
		if !skill.Enabled || len(skill.Keywords) == 0 {
			continue
		}
		score := 0
		for _, kw := range skill.Keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestSkill = skill
		}
	}
	return bestSkill
}

func (sm *SkillManager) detectWithLLM(query string) *Skill {
	var skillList strings.Builder
	for _, s := range sm.skills {
		if s.Enabled {
			skillList.WriteString(fmt.Sprintf("- %s: %s\n", s.ID, s.Description))
		}
	}
	if skillList.Len() == 0 {
		return nil
	}

	resp, err := sm.llm.Chat(context.Background(), []Message{
		{Role: "user", Content: fmt.Sprintf("Which skill best matches this query? Reply with the skill ID or 'none'.\n\nSkills:\n%s\nQuery: %s", skillList.String(), query)},
	}, nil, map[string]interface{}{"max_tokens": 30, "temperature": 0, "route": "execution"})
	if err != nil {
		return nil
	}

	choice := strings.TrimSpace(resp.Content)
	if choice == "" || choice == "none" {
		return nil
	}
	return sm.skills[choice]
}

// InjectPrompt 将技能提示词注入系统消息
func (sm *SkillManager) InjectPrompt(query string, basePrompt string) string {
	skill := sm.DetectSkill(query)
	if skill == nil {
		return basePrompt
	}

	var builder strings.Builder
	builder.WriteString(basePrompt)
	builder.WriteString(fmt.Sprintf("\n\n## 当前激活技能: %s\n", skill.Name))
	builder.WriteString(skill.Prompt)
	return builder.String()
}

// GetTools 获取技能需要的工具列表
func (sm *SkillManager) GetTools(query string) []string {
	skill := sm.DetectSkill(query)
	if skill == nil {
		return nil
	}
	return skill.Tools
}

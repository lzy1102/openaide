package kernel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Skill 可插拔的能力模块
type Skill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`  // 注入 system prompt 的内容
	Tools       []string `json:"tools"` // 该 skill 需要启用的额外工具
	Enabled     bool   `json:"enabled"`
}

// SkillManager 技能管理器
type SkillManager struct {
	skills    map[string]*Skill
	dir       string
	autoDetect bool
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

// loadBuiltins 加载内置技能
func (sm *SkillManager) loadBuiltins() {
	builtins := []Skill{
		{
			ID: "code-review", Name: "代码审查",
			Description: "审查代码质量、安全问题、最佳实践",
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
		}
	}
}

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

// DetectSkill 自动检测用户意图并匹配技能
func (sm *SkillManager) DetectSkill(query string) *Skill {
	if !sm.autoDetect {
		return nil
	}

	lower := strings.ToLower(query)
	keywords := map[string]string{
		"code-review": "review,审查,检查代码,code review,安全漏洞,代码质量",
		"git-commit":  "commit,提交,git add,git commit,暂存",
		"debug":       "debug,调试,报错,error,失败,不工作,bug,修复,fix",
		"refactor":    "refactor,重构,重写,整理代码,优化结构",
		"explain":     "explain,解释,说明,这段代码,什么意思,做什么的",
	}

	bestMatch := ""
	bestScore := 0
	for skillID, kwStr := range keywords {
		score := 0
		for _, kw := range strings.Split(kwStr, ",") {
			if strings.Contains(lower, strings.TrimSpace(kw)) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestMatch = skillID
		}
	}

	if bestScore > 0 {
		return sm.Get(bestMatch)
	}
	return nil
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

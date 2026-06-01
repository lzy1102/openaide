package kernel

import (
	"os"
	"path/filepath"
	"strings"
)

// ── Layered Prompt System ──────────────────────────────────
// Layers are assembled dynamically based on task context.
// Simple queries get L0+L1 (~300 tokens). Complex tasks get all layers.
//
//   L0: Identity + Safety       (always, ~200 tok)
//   L1: Project Context         (OPENAIDE.md etc., ~100 tok)
//   L2: Skill Prompt            (skill match, ~100-500 tok)
//   L3: Task Adapter            (coding/research/review, ~200 tok)
//   L4: Learned Experience      (learner insights, ~100 tok)
//   L5: Reflection Improvement  (last reflection, ~100 tok)

// ── L0: Identity + Safety (always loaded) ─────────────────

func promptL0_EN() string {
	return `You are OpenAIDE, a versatile AI coding assistant.

## Core
- Adapt to the task: programmer, reviewer, researcher, teacher.
- If unsure: look it up. Don't guess.
- If beyond capability: say so and suggest alternatives.
- Complex tasks: plan first. Simple questions: answer directly.

## Output Quality
- When analyzing code: always provide concrete improvement suggestions with specific file names and line numbers. Don't just point out problems — propose solutions.
- When asked "what can be improved": give a prioritized list with estimated effort for each item.
- When reviewing: say what to change, which file to change it in, and why. Never leave the user wondering "ok, so what do I do about it?"
- Analysis without actionable recommendations is incomplete analysis.

## Safety
- Never execute destructive commands without explicit confirmation.
- Never modify files outside the project directory.
- File writes and command execution require approval.`
}

func promptL0_ZH() string {
	return `你是 OpenAIDE，一个多功能的 AI 编程助手。

## 核心原则
- 根据任务切换角色：程序员、审查者、研究者、教师
- 不确定时查阅，不要猜测
- 超出能力时承认并建议替代方案
- 复杂任务先规划，简单问题直接回答

## 输出质量
- 分析代码时必须给出具体改进建议，包含文件名和行号。不要只指出问题——要提出解决方案。
- 被问"有哪些改进空间"时，列出优先级排序的 action items，每项标注预估工作量。
- 审查代码时说清楚：改什么、在哪个文件改、为什么改。不要让用户猜"然后呢？"
- 没有可执行建议的分析是不完整的分析。

## 安全
- 执行破坏性命令前必须确认
- 不要修改项目目录外的文件
- 文件写入和命令执行需要审批`
}

// ── L1: Project Context ────────────────────────────────────

func promptL1() string {
	cwd, _ := os.Getwd()
	var sb strings.Builder
	sb.WriteString("\n[WorkingDir] ")
	sb.WriteString(cwd)

	// Git branch
	if out, err := runGitCmd("rev-parse", "--abbrev-ref", "HEAD"); err == nil && out != "" {
		sb.WriteString("\n[Git] branch: ")
		sb.WriteString(out)
	}

	// Project rules (OPENAIDE.md, CLAUDE.md, etc.)
	ruleFiles := []string{"CLAUDE.md", "OPENAIDE.md", "CODEBUDDY.md", "CONVENTIONS.md"}
	for _, f := range ruleFiles {
		if data, err := os.ReadFile(filepath.Join(cwd, f)); err == nil && len(data) > 0 {
			content := string(data)
			if len(content) > 2000 {
				content = content[:2000] + "..."
			}
			sb.WriteString("\n[Rules] ")
			sb.WriteString(f)
			sb.WriteString(":\n")
			sb.WriteString(content)
			break // Only load the first found rule file
		}
	}

	// RepoMap symbol map
	if rm := GenerateRepoMap(cwd); rm != "" {
		sb.WriteString("\n[RepoMap]\n")
		sb.WriteString(rm)
	}
	return sb.String()
}

// ── L2: Skill Prompt ───────────────────────────────────────

// L2 is injected by SkillActor.InjectPrompt. Defined here for completeness.

// ── L3: Task Adapter ────────────────────────────────────────

func promptL3_EN(task string) string {
	switch {
	case containsAny(task, "code", "fix", "refactor", "implement", "write", "bug", "test",
		"代码", "修复", "重构", "实现", "写", "测试", "改", "build", "add", "create", "function",
		"api", "endpoint", "handler", "route", "component", "module", "class", "struct", "interface"):
		return `
## Coding Mode
- Write clean, idiomatic code following existing project conventions.
- Handle errors explicitly. Never swallow errors silently.
- Add tests for new functionality when appropriate.
- Use the project's existing patterns — don't introduce new styles.`

	case containsAny(task, "review", "audit", "check", "security", "vulnerability",
		"审查", "审计", "检查", "安全", "漏洞", "review", "pr", "diff"):
		return `
## Review Mode
- Check for: correctness, security, performance, readability, edge cases.
- Flag potential issues with concrete suggestions.
- Look for: SQL injection, XSS, race conditions, nil pointers, resource leaks.
- Verify error handling is complete and appropriate.`

	case containsAny(task, "explain", "how", "what", "why", "document", "tutorial",
		"解释", "怎么", "为什么", "文档", "教程", "介绍", "describe"):
		return `
## Teaching Mode
- Lead with a clear, simple explanation.
- Use examples and analogies where helpful.
- Answer the "why" — not just the "how".
- Structure: concept → example → deeper details (if needed).`

	case containsAny(task, "research", "investigate", "analyze", "compare", "options",
		"研究", "调查", "分析", "比较", "方案", "design", "architecture"):
		return `
## Research Mode
- Gather information before forming conclusions.
- When claiming "X is missing" or "X is unsafe": verify FIRST. Use search_files or grep to check if X exists elsewhere in the codebase. Many things (timeouts, locks, validation) are handled in callers, not in the function you're reading.
- Never report "missing error handling" without checking the call chain.
- Present findings with clear pros/cons and your reasoning.
- Cite specific files and code where relevant.
- End with a concrete action plan: what to do first, estimated effort, which files to touch.`

	default:
		return ""
	}
}

func promptL3_ZH(task string) string {
	switch {
	case containsAny(task, "code", "fix", "refactor", "implement", "write", "bug", "test",
		"代码", "修复", "重构", "实现", "写", "测试", "改", "build", "add", "create"):
		return `
## 编码模式
- 遵循项目已有的编码规范和模式。
- 显式处理错误，永远不要静默吞掉异常。
- 适当时为新功能添加测试。
- 不引入与项目风格不一致的新写法。`

	case containsAny(task, "review", "audit", "check", "security", "vulnerability",
		"审查", "审计", "检查", "安全", "漏洞", "review", "pr", "diff"):
		return `
## 审查模式
- 检查：正确性、安全性、性能、可读性、边界条件。
- 发现潜在问题时给出具体改进建议。
- 关注：SQL注入、XSS、竞态条件、空指针、资源泄漏。
- 验证错误处理是否完整和恰当。`

	case containsAny(task, "explain", "how", "what", "why", "document", "tutorial",
		"解释", "怎么", "为什么", "文档", "教程", "介绍", "describe"):
		return `
## 教学模式
- 先用清晰简洁的语言解释核心概念。
- 使用示例和类比来帮助理解。
- 解释"为什么"而不仅仅是"怎么做"。
- 结构：概念 → 示例 → 深入细节（如需）。`

	case containsAny(task, "research", "investigate", "analyze", "compare", "options",
		"研究", "调查", "分析", "比较", "方案", "design", "architecture"):
		return `
## 研究模式
- 先收集信息，再形成结论。
- 声称"缺少 X"或"X 不安全"之前，先用 search_files 或 grep 验证 X 是否在代码库其他地方已经存在。很多处理（超时、锁、校验）是在调用方而非当前函数中完成的。
- 不要不检查调用链就报告"缺少错误处理"。
- 列出清晰的优劣分析和推理过程。
- 引用具体的文件和代码行。
- 结尾必须给出可执行的行动计划：先做什么、预估工作量、涉及哪些文件。`

	default:
		return ""
	}
}

// ── L4: Learned Experience ─────────────────────────────────

func promptL4(insights []string) string {
	if len(insights) == 0 {
		return ""
	}
	sb := new(strings.Builder)
	sb.WriteString("\n## Project Knowledge\n")
	for _, insight := range insights {
		sb.WriteString("- ")
		sb.WriteString(insight)
		sb.WriteString("\n")
	}
	return sb.String()
}

// ── L5: Reflection Improvement ─────────────────────────────

func promptL5(reflection *ReflectionResult) string {
	if reflection == nil || reflection.Quality == 0 {
		return ""
	}
	sb := new(strings.Builder)
	sb.WriteString("\n## Lessons from Last Execution")
	if len(reflection.Issues) > 0 {
		sb.WriteString("\nIssues to avoid:")
		for _, issue := range reflection.Issues {
			sb.WriteString("\n- ")
			sb.WriteString(issue)
		}
	}
	if len(reflection.Suggestions) > 0 {
		sb.WriteString("\nImprovements to apply:")
		for _, s := range reflection.Suggestions {
			sb.WriteString("\n- ")
			sb.WriteString(s)
		}
	}
	if reflection.Learned != "" {
		sb.WriteString("\nKey lesson: ")
		sb.WriteString(reflection.Learned)
	}
	return sb.String()
}

// ── Builders ────────────────────────────────────────────────

// buildSystemPrompt assembles the stable prompt prefix (L0+L1+L3).
// L0 and L1 are always present. L3 adapts to the task type.
// L2 (skill), L4 (learner), L5 (reflection) are injected as dynamic tail
// in buildMessages for prompt cache efficiency.
func (k *AgentKernel) buildSystemPrompt(query *Query) string {
	zh := isZhEnv()
	var sb strings.Builder

	// L0: Identity + Safety (always)
	if zh {
		sb.WriteString(promptL0_ZH())
	} else {
		sb.WriteString(promptL0_EN())
	}

	// L1: Project context
	if l1 := promptL1(); l1 != "" {
		sb.WriteString(l1)
	}

	// L3: Task adapter (based on query content)
	if zh {
		if l3 := promptL3_ZH(query.Content); l3 != "" {
			sb.WriteString(l3)
		}
	} else {
		if l3 := promptL3_EN(query.Content); l3 != "" {
			sb.WriteString(l3)
		}
	}

	return sb.String()
}

// ── Helpers ─────────────────────────────────────────────────

func containsAny(s string, keywords ...string) bool {
	lower := strings.ToLower(s)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// defaultSystemPrompt returns the default system prompt based on locale.
func defaultSystemPrompt() string {
	if isZhEnv() {
		return promptL0_ZH()
	}
	return promptL0_EN()
}

// LoadSystemPrompt loads a custom system prompt from a directory.
// Looks for system.{lang}.md first, then system.md, then falls back to default.
func LoadSystemPrompt(dir string) string {
	// Try language-specific file
	suffix := "en"
	if isZhEnv() {
		suffix = "zh"
	}
	if data, err := os.ReadFile(filepath.Join(dir, "system."+suffix+".md")); err == nil && len(data) > 0 {
		return string(data)
	}
	// Try generic file
	if data, err := os.ReadFile(filepath.Join(dir, "system.md")); err == nil && len(data) > 0 {
		return string(data)
	}
	return ""
}

// WriteSystemPrompt writes a custom system prompt to disk.
func WriteSystemPrompt(dir, prompt string) error {
	os.MkdirAll(dir, 0755)
	suffix := "en"
	if isZhEnv() {
		suffix = "zh"
	}
	return os.WriteFile(filepath.Join(dir, "system."+suffix+".md"), []byte(prompt), 0644)
}

func isZhEnv() bool {
	lang := os.Getenv("LANG")
	if lang == "" {
		lang = os.Getenv("LC_ALL")
	}
	return strings.HasPrefix(lang, "zh")
}


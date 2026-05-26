package kernel

import (
	"os"
	"path/filepath"
	"strings"
)

func defaultSystemPromptEN() string {
	return `You are OpenAIDE, an AI assistant.

## Core Principles
- Your role adapts to the task — programmer, writer, analyst, teacher… switch as needed
- When uncertain about facts: look them up, don't guess
- When beyond your capability: say "I'm not sure" and suggest alternatives
- Complex tasks: plan first, then execute. Simple questions: answer directly

## Thinking Process
1. Understand → 2. Choose perspective → 3. Gather info → 4. Plan → 5. Execute & Verify → 6. Deliver

## Capabilities
- Can: read files, write code, search web, query knowledge base, run commands, Git
- Can: spawn sub-agent teams (/analyst /coder /reviewer /executor /team)
- Can: deep-plan complex tasks (research→propose→plan→execute→verify)
- Limit: no destructive commands, irreversible ops need confirmation
- Limit: RepoMap is auto-injected — no need to explore directory structure

## Response Style
1. Lead with the conclusion, support with bullet points
2. Label code blocks with language, bold key info
3. Never regurgitate raw tool output — extract and summarize
4. Simple questions: 1-3 sentences. Complex: structured sections
5. Match the user's language

## When to Stop
1. Enough info → answer immediately, stop searching
2. 3-5 key files → deliver conclusion
3. Same operation fails twice → explain why, suggest alternative
4. After round 4 → prioritize output over more tools

## Examples

❌ Bad:
"Let me check the file…[pastes 100 lines of code]…based on the code, the answer is X"

✅ Good:
"The answer is X. Evidence: 1) … 2) … See file.go:42."`
}



func defaultSystemPromptZH() string {
	return `你是 OpenAIDE，一个 AI 智能助手。

## 核心原则
- 用户需求决定你的角色——程序员、作家、分析师、教师…任何身份按需切换
- 遇到不确定的事实：用工具查，不要猜
- 超出能力范围：诚实说"我不确定"，给出替代建议
- 复杂任务先规划再执行，简单问题直接回答

## 思考方式
1. 理解意图 → 2. 选择视角 → 3. 收集信息 → 4. 制定方案 → 5. 执行验证 → 6. 交付结果

## 能力边界
- 可以：读文件、写代码、搜索网页、查知识库、执行命令、Git 操作
- 可以：调用子 Agent 团队协作（/analyst /coder /reviewer /executor /team）
- 可以：深度规划复杂任务（研究→方案对比→计划→执行→验证）
- 限制：不执行破坏性命令，不可逆操作需确认
- 限制：项目文件地图（RepoMap）已自动注入，无需重复探索目录结构

## 回复风格
1. 结论先行，用要点展开
2. 代码标注语言，关键信息粗体
3. 禁止复述工具输出原文——提炼后用自己话总结
4. 简单问题 1-3 句，复杂问题分段展开
5. 中文环境用中文回复，代码/术语保留原文

## 何时停止
1. 信息足够 → 立即给答案，不再搜索
2. 3-5 个关键文件后 → 给出结论
3. 同一操作失败 2 次 → 说明原因，换方案
4. 第 4 轮以后 → 优先输出结论

## 示例

❌ 差的回答：
"让我查看文件…[粘贴100行代码]…根据代码，答案是X"

✅ 好的回答：
"答案是X，依据是：1) … 2) … 相关代码在 file.go:42 行。"`
}



// defaultSystemPrompt returns the default prompt in the detected language.
func defaultSystemPrompt() string {
	if IsZhEnv() {
		return defaultSystemPromptZH()
	}
	return defaultSystemPromptEN()
}

// IsZhEnv reports whether the system locale is Chinese.
func IsZhEnv() bool {
	for _, env := range []string{"LANG", "LC_MESSAGES", "LC_ALL"} {
		if strings.Contains(strings.ToLower(os.Getenv(env)), "zh") {
			return true
		}
	}
	return false
}

// IsFirstRun 检查是否首次启动（system.md 和 system.{lang}.md 都不存在）
func IsFirstRun(promptsDir string) bool {
	if _, err := os.Stat(filepath.Join(promptsDir, fileName(""))); err == nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(promptsDir, fileName(langSuffix()))); err != nil {
		return true
	}
	return false
}

// WriteSystemPrompt 写入自定义系统提示词
func WriteSystemPrompt(promptsDir string, content string) error {
	os.MkdirAll(promptsDir, 0755)
	return os.WriteFile(filepath.Join(promptsDir, fileName("")), []byte(content), 0644)
}

// LoadSystemPrompt 从文件加载系统提示词，文件不存在时写入默认值
// 优先级：system.{lang}.md > system.md > 硬编码默认值
func LoadSystemPrompt(promptsDir string) string {
	// 1. 语言特定文件
	path := filepath.Join(promptsDir, fileName(langSuffix()))
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		return string(data)
	}
	// 2. 通用文件（向后兼容）
	genericPath := filepath.Join(promptsDir, fileName(""))
	if data, err := os.ReadFile(genericPath); err == nil && len(data) > 0 {
		return string(data)
	}
	// 3. 写入默认文件
	os.MkdirAll(promptsDir, 0755)
	os.WriteFile(path, []byte(defaultSystemPrompt()), 0644)
	return defaultSystemPrompt()
}

func langSuffix() string {
	if IsZhEnv() {
		return ".zh"
	}
	return ".en"
}

func fileName(suffix string) string {
	return "system" + suffix + ".md"
}

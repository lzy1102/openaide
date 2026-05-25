package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
)

// ── ANSI Colors ───────────────────────────────────────────

// 语义色板 — 每个场景有明确归属
const (
	cReset  = "\033[0m"
	cBold   = "\033[1m"
	cDim    = "\033[2m"
	cItalic = "\033[3m"

	cRed    = "\033[31m"
	cGreen  = "\033[32m"
	cYellow = "\033[33m"
	cBlue   = "\033[34m"
	cMagenta = "\033[35m"
	cCyan   = "\033[36m"

	cGray   = "\033[90m"  // 辅助信息
	cWhite  = "\033[97m"  // 高亮
)

// 语义别名
var (
	cLogo      = cCyan + cBold    // 品牌
	cHeading   = cCyan + cBold    // 标题
	cUser      = cBlue + cBold    // 用户查询回显
	cAI        = ""               // AI 回答（无前缀，glamour 渲染）
	cToolName  = cYellow + cBold  // 工具名称
	cToolOK    = cGreen           // 工具成功
	cToolErr   = cRed + cBold     // 工具失败
	cThink     = cDim + cItalic    // 思考内容
	cStatusBar = cDim             // 状态栏
	cPrompt    = cGreen + cBold   // 提示符
	cError     = cRed + cBold     // 错误
	cWarn      = cYellow          // 警告
	cSuccess   = cGreen           // 成功确认
	cHint      = cDim             // 提示文字
	cInfo      = cGray            // 辅助信息
	cSep       = cDim             // 分隔线
)

// ── Markdown Renderer ─────────────────────────────────────

var mdRenderer *glamour.TermRenderer

func initMarkdown() {
	if mdRenderer != nil { return }
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	if err != nil { return }
	mdRenderer = r
}

func RenderMarkdown(text string) string {
	if mdRenderer == nil { initMarkdown() }
	if mdRenderer == nil { return text }
	out, err := mdRenderer.Render(text)
	if err != nil { return text }
	return out
}

// ── Output Helpers ────────────────────────────────────────

// PrintUserEcho echoes the user's query in blue
func PrintUserEcho(query string) {
	fmt.Printf("\n  %s▸ %s%s\n", cUser, query, cReset)
}

// PrintThinking shows dimmed thinking (one line per round)
func PrintThinking(text string) {
	firstLine := strings.SplitN(text, "\n", 2)[0]
	if len(firstLine) > 140 { firstLine = firstLine[:137] + "..." }
	fmt.Printf("  %s[think] %s%s\n", cThink, firstLine, cReset)
}

// PrintToolCall shows a tool starting execution
func PrintToolCall(name string) {
	fmt.Printf("\n  %s⚙ %s%s ", cToolName, name, cReset)
}

// PrintToolDone shows completed tool with result
func PrintToolDone(summary string) {
	fmt.Printf("%s✓%s", cToolOK, cReset)
	if summary != "" {
		fmt.Printf(" %s%s%s", cInfo, trunc(summary, 80), cReset)
	}
	fmt.Println()
}

// PrintToolError shows a failed tool
func PrintToolError(name, err string) {
	fmt.Printf("\n  %s✗ %s%s %s%s%s\n", cToolErr, name, cReset, cInfo, err, cReset)
}

// PrintStatusLine prints the footer after completion
func PrintStatusLine(tokens, tools int, elapsed time.Duration) {
	sep := strings.Repeat("─", 60)
	fmt.Printf("\n%s%s%s\n", cSep, sep, cReset)

	var parts []string
	if tokens > 0 {
		parts = append(parts, fmt.Sprintf("%s⚡ %d tokens%s", cInfo, tokens, cReset))
	}
	if tools > 0 {
		parts = append(parts, fmt.Sprintf("%s🔧 %d tools%s", cInfo, tools, cReset))
	}
	parts = append(parts, fmt.Sprintf("%s⏱ %v%s", cInfo, elapsed.Round(100*time.Millisecond), cReset))
	fmt.Printf("  %s\n\n", strings.Join(parts, "  "+cSep+"│"+cReset+"  "))
}

// PrintError shows a red error
func PrintError(err string) {
	fmt.Printf("\n  %s✗ %s%s\n", cError, err, cReset)
}

// PrintWarning shows a yellow warning
func PrintWarning(msg string) {
	fmt.Printf("  %s⚠ %s%s\n", cWarn, msg, cReset)
}

// PrintSuccess shows a green success message
func PrintSuccess(msg string) {
	fmt.Printf("  %s✓ %s%s\n", cSuccess, msg, cReset)
}

// PrintInfo shows a gray info message
func PrintInfo(msg string) {
	fmt.Printf("  %s%s%s\n", cInfo, msg, cReset)
}

// ── Prompt ────────────────────────────────────────────────

func PromptStyle(sessionID, modelName string) string {
	prompt := "❯ "
	if sessionID != "" {
		prompt = fmt.Sprintf("%s%s%s │ %s%s", cDim, sessionID[:8], cReset, cPrompt, prompt)
	}
	return prompt + cReset
}

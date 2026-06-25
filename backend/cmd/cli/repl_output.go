package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/pterm/pterm"
)

// ── ANSI Colors (minimal, for inline formatting) ──────────

const (
	cReset   = "\033[0m"
	cBold    = "\033[1m"
	cDim     = "\033[2m"
	cItalic  = "\033[3m"
	cRed     = "\033[31m"
	cGreen   = "\033[32m"
	cYellow  = "\033[33m"
	cBlue    = "\033[34m"
	cCyan    = "\033[36m"
	cGray    = "\033[90m"
)

var (
	cLogo      = cCyan + cBold
	cUser      = cBlue + cBold
	cToolOK    = cGreen
	cToolErr   = cRed + cBold
	cThink     = cDim + cItalic
	cPrompt    = cGreen + cBold
	cPromptBusy = cYellow + cBold
	cError     = cRed + cBold
	cWarn      = cYellow
	cSuccess   = cGreen
	cInfo      = cGray
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

// ── Prompt ────────────────────────────────────────────────

func PromptStyle(sessionID, modelName string, busy bool, extra ...string) string {
	dot := cGreen + "●" + cReset
	if busy { dot = cYellow + "◉" + cReset }
	// Session display: title > first msg > hash
	sid := ""
	if len(extra) > 0 && extra[0] != "" {
		sid = trunc(extra[0], 24) // use session title or first query
	} else {
		sid = sessionID
		if len(sid) > 8 { sid = sid[:8] }
	}
	// Git dirty indicator
	suffix := ""
	if len(extra) > 1 { suffix = extra[1] }
	return fmt.Sprintf("%s %s%s %s❯%s %s", dot, cDim, sid, cPrompt, cReset, suffix)
}

// ── Tool Section ──────────────────────────────────────────

type toolSection struct {
	names   []string
	results []string
	errors  []string
}
var currentToolSection toolSection

func BeginToolSection()  { currentToolSection = toolSection{} }
func AddToolCall(n string) { currentToolSection.names = append(currentToolSection.names, n) }
func AddToolResult(name, summary, errStr string) {
	currentToolSection.results = append(currentToolSection.results, summary)
	currentToolSection.errors = append(currentToolSection.errors, errStr)
}

func EndToolSection() {
	if len(currentToolSection.names) == 0 { return }
	errors := 0
	for _, e := range currentToolSection.errors {
		if e != "" { errors++ }
	}

	// Compact: always show one-line summary
	seen := make(map[string]bool)
	var names []string
	for _, n := range currentToolSection.names {
		if !seen[n] { seen[n] = true; names = append(names, n) }
	}
	icon := "📁"
	if errors > 0 { icon = "⚠️" }
	fmt.Printf("  %s %s%s%s\n", icon, pterm.Cyan(strings.Join(names, ", ")), cReset, cReset)
}

// ── Thinking ──────────────────────────────────────────────

var lastThink string

func PrintThinking(text string) {
	firstLine := strings.SplitN(text, "\n", 2)[0]
	if len(firstLine) > 80 { firstLine = firstLine[:77] + "..." }
	// Dedup: skip identical consecutive think lines
	if firstLine == lastThink { return }
	lastThink = firstLine
	fmt.Printf("\r\033[K  %s[think]%s %s%s%s\n", cThink, cReset, cDim, firstLine, cReset)
}

// ── Progress (pterm Spinner + Progressbar) ────────────────

var currentSpinner *pterm.SpinnerPrinter

func ShowSpinner(text string) {
	if currentSpinner != nil { currentSpinner.Stop() }
	s, _ := pterm.DefaultSpinner.WithRemoveWhenDone(true).Start(text)
	currentSpinner = s
}

func UpdateSpinner(text string) {
	if currentSpinner != nil { currentSpinner.UpdateText(text) }
}

func StopSpinner() {
	if currentSpinner != nil { currentSpinner.Stop(); currentSpinner = nil }
}

func ShowProgress(total int, title string) *pterm.ProgressbarPrinter {
	p, _ := pterm.DefaultProgressbar.
		WithTotal(total).
		WithTitle(title).
		WithShowCount(true).
		WithShowPercentage(true).
		WithRemoveWhenDone(false).
		Start()
	return p
}

// ── Status Bar ────────────────────────────────────────────

var sessionTokens int // 会话累计 token

func PrintStatusBar(tokens, tools int, elapsed time.Duration, model string, cacheHit, cacheMiss int) {
	sessionTokens += tokens
	var parts []string
	if model != "" {
		parts = append(parts, pterm.Gray(model))
	}
	if tokens > 0 {
		parts = append(parts, fmt.Sprintf("%s⚡ %dk%s", cInfo, tokens/1000, cReset))
	}
	if tools > 0 {
		parts = append(parts, fmt.Sprintf("%s🔧 %d%s", cInfo, tools, cReset))
	}
	parts = append(parts, fmt.Sprintf("%s⏱ %v%s", cInfo, elapsed.Round(100*time.Millisecond), cReset))
	fmt.Printf("  %s  %s\n", pterm.Gray("│"), strings.Join(parts, "  │  "))
}

// ── Message Helpers ───────────────────────────────────────

func PrintError(err string)   { pterm.Error.Println(err) }
func PrintWarning(msg string) { pterm.Warning.Println(msg) }
func PrintSuccess(msg string) { pterm.Success.Println(msg) }
func PrintInfo(msg string)    { pterm.Info.Println(msg) }
// ── Diff Highlighting ────────────────────────────────────

// RenderDiff 渲染 diff 格式：绿 +、红 -
func RenderDiff(text string) string {
	var sb strings.Builder
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- "):
			sb.WriteString(cBold + line + cReset + "\n")
		case strings.HasPrefix(line, "@@ "):
			sb.WriteString(cCyan + line + cReset + "\n")
		case strings.HasPrefix(line, "+"):
			sb.WriteString(cGreen + cBold + line + cReset + "\n")
		case strings.HasPrefix(line, "-"):
			sb.WriteString(cRed + cBold + line + cReset + "\n")
		case strings.HasPrefix(line, "diff --git "):
			sb.WriteString(cBold + line + cReset + "\n")
		default:
			sb.WriteString(line + "\n")
		}
	}
	return sb.String()
}

// IsDiff 检测文本是否为 diff 格式
func IsDiff(text string) bool {
	return strings.Contains(text, "\n--- ") || strings.Contains(text, "\n+++ ") ||
		strings.HasPrefix(text, "diff --git ") || strings.Contains(text, "\n@@ ")
}

// RenderToolOutput 智能渲染工具输出（diff 格式用高亮，其他用纯文本）
func RenderToolOutput(raw string) string {
	if IsDiff(raw) {
		return "\n" + RenderDiff(raw)
	}
	firstLine := strings.SplitN(raw, "\n", 2)[0]
	return strings.TrimPrefix(firstLine, "// ")
}

// ── Rich Output ──────────────────────────────────────────

// PrintTable 渲染简单表格
func PrintTable(headers []string, rows [][]string) {
	tbl := pterm.DefaultTable.WithHasHeader().WithData(pterm.TableData{
		headers,
	})
	for _, row := range rows {
		tbl.Data = append(tbl.Data, row)
	}
	tbl.Render()
}

// PrintQuote 渲染引用块
func PrintQuote(text, source string) {
	fmt.Printf("  %s│%s %s\n", cDim, cReset, text)
	if source != "" {
		fmt.Printf("  %s│%s %s─ %s%s\n", cDim, cReset, cDim, source, cReset)
	}
}

// PrintList 渲染清单
func PrintList(items []string, ordered bool) {
	for i, item := range items {
		if ordered {
			fmt.Printf("  %s%d.%s %s\n", cDim, i+1, cReset, item)
		} else {
			fmt.Printf("  %s•%s %s\n", cDim, cReset, item)
		}
	}
}

// PrintPanel 渲染面板
func PrintPanel(title, content string) {
	pterm.DefaultBox.WithTitle(title).Print(content)
}

func Println() { fmt.Println() }

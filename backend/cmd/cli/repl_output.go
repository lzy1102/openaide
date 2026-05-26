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
	name := "openaide"
	if modelName != "" {
		name = strings.SplitN(modelName, " ", 2)[0]
	}
	sid := sessionID
	if len(sid) > 8 { sid = sid[:8] }
	suffix := ""
	if len(extra) > 0 { suffix = extra[0] }
	return fmt.Sprintf("%s %s%s %s %s❯%s %s", dot, cDim, sid, cPrompt, name, cReset, suffix)
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
	total := len(currentToolSection.names)
	errors := 0
	for _, e := range currentToolSection.errors {
		if e != "" { errors++ }
	}

	if errors == 0 && total > 1 {
		seen := make(map[string]bool)
		var names []string
		for _, n := range currentToolSection.names {
			if !seen[n] { seen[n] = true; names = append(names, n) }
		}
		pterm.Info.Printfln("📁 %d tools (%s)", total, strings.Join(names, ", "))
		return
	}

	for i, name := range currentToolSection.names {
		marker := cToolOK + "✓" + cReset
		if currentToolSection.errors[i] != "" {
			marker = cToolErr + "✗" + cReset
		}
		info := ""
		if currentToolSection.results[i] != "" {
			info = " " + pterm.Gray(currentToolSection.results[i])
		}
		fmt.Printf("  %s %s%s%s\n", marker, pterm.Cyan(name), info, cReset)
	}
	fmt.Println()
}

// ── Thinking ──────────────────────────────────────────────

func PrintThinking(text string) {
	firstLine := strings.SplitN(text, "\n", 2)[0]
	if len(firstLine) > 100 { firstLine = firstLine[:97] + "..." }
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

func PrintStatusBar(tokens, tools int, elapsed time.Duration, model string) {
	sessionTokens += tokens
	var parts []string
	if model != "" {
		parts = append(parts, pterm.Gray(model))
	}
	if tokens > 0 {
		parts = append(parts, fmt.Sprintf("%s⚡ %d tokens%s", cInfo, tokens, cReset))
	}
	if sessionTokens > 0 && sessionTokens != tokens {
		parts = append(parts, fmt.Sprintf("%s累计 %dk%s", cInfo, sessionTokens/1000, cReset))
	}
	if tools > 0 {
		parts = append(parts, fmt.Sprintf("%s🔧 %d tools%s", cInfo, tools, cReset))
	}
	parts = append(parts, fmt.Sprintf("%s⏱ %v%s", cInfo, elapsed.Round(100*time.Millisecond), cReset))
	fmt.Printf("\n  %s\n\n", strings.Join(parts, "  │  "))
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
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			sb.WriteString(cBold + line + cReset + "\n")
		case strings.HasPrefix(line, "@@"):
			sb.WriteString(cCyan + line + cReset + "\n")
		case strings.HasPrefix(line, "+"):
			sb.WriteString(cGreen + line + cReset + "\n")
		case strings.HasPrefix(line, "-"):
			sb.WriteString(cRed + line + cReset + "\n")
		default:
			sb.WriteString(line + "\n")
		}
	}
	return sb.String()
}

func Println() { fmt.Println() }

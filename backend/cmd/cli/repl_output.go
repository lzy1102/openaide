package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
)

// ── ANSI Colors ───────────────────────────────────────────

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
	cWhite  = "\033[97m"
	cGray   = "\033[90m"
)

// 语义别名
var (
	cLogo      = cCyan + cBold
	cHeading   = cCyan + cBold
	cUser      = cBlue + cBold
	cToolName  = cYellow + cBold
	cToolOK    = cGreen
	cToolErr   = cRed + cBold
	cThink     = cDim + cItalic
	cStatusBar = cDim
	cPrompt    = cGreen + cBold
	cPromptBusy= cYellow + cBold
	cError     = cRed + cBold
	cWarn      = cYellow
	cSuccess   = cGreen
	cHint      = cDim
	cInfo      = cGray
	cSep       = cDim
	cBorder    = cGray
)

// ── Box Drawing ───────────────────────────────────────────

const (
	boxH  = "─"
	boxV  = "│"
	boxTL = "╭"
	boxTR = "╮"
	boxBL = "╰"
	boxBR = "╯"
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

func PromptStyle(sessionID, modelName string, busy bool) string {
	dot := cGreen + "●" + cReset
	if busy { dot = cYellow + "◉" + cReset }

	name := "openaide"
	if modelName != "" {
		name = strings.SplitN(modelName, " ", 2)[0]
	}
	return fmt.Sprintf("%s %s%s %s %s❯%s ", dot, cDim, sessionID[:8], cPrompt, name, cReset)
}

// ── Tool Call Section ─────────────────────────────────────

type toolSection struct {
	names   []string
	results []string // summary per tool
	errors  []string // error per tool
}

var currentToolSection toolSection

func BeginToolSection() {
	currentToolSection = toolSection{}
}

func AddToolCall(name string) {
	currentToolSection.names = append(currentToolSection.names, name)
}

func AddToolResult(name, summary, errStr string) {
	currentToolSection.results = append(currentToolSection.results, summary)
	currentToolSection.errors = append(currentToolSection.errors, errStr)
}

func EndToolSection() {
	if len(currentToolSection.names) == 0 { return }

	total := len(currentToolSection.names)
	ok := 0
	for _, e := range currentToolSection.errors {
		if e == "" { ok++ }
	}

	// Header
	width := 60
	fmt.Printf("\n  %s%s %s工具 %s(%d)%s %s", cBorder, boxTL, cReset, cDim, total, cReset, strings.Repeat(boxH, width-10-len(fmt.Sprintf("%d", total))))
	fmt.Printf("%s%s\n", cBorder, boxTR)

	// Each tool
	for i, name := range currentToolSection.names {
		marker := cToolOK + "✓" + cReset
		if currentToolSection.errors[i] != "" {
			marker = cToolErr + "✗" + cReset
		}
		summary := currentToolSection.results[i]
		if summary == "" { summary = "ok" }
		if len(summary) > 40 { summary = summary[:37] + "..." }
		fmt.Printf("  %s%s %s %s%-30s%s %s%s%s\n",
			cBorder, boxV, marker, cToolName, name, cReset, cInfo, summary, cReset)
	}

	// Footer
	fmt.Printf("  %s%s% s %s\n", cBorder, boxBL, strings.Repeat(boxH, width-4), cBorder, boxBR)
	Println()
}

// ── Thinking Display ──────────────────────────────────────

func PrintThinking(text string) {
	firstLine := strings.SplitN(text, "\n", 2)[0]
	if len(firstLine) > 120 { firstLine = firstLine[:117] + "..." }
	fmt.Printf("\r\033[K  %s[think]%s %s%s%s\n", cThink, cReset, cDim, firstLine, cReset)
}

// ── Progress Bar ──────────────────────────────────────────

var progressLineActive bool

func ShowProgress(phase string, current, total int) {
	progressLineActive = true
	bar := progressBar(current, total, 30)
	fmt.Printf("\r\033[K  %s%-12s%s %s %s%d/%d%s\n",
		cDim, phase, cReset, bar, cInfo, current, total, cReset)
	if progressLineActive {
		fmt.Print("\033[1A") // cursor up
	}
}

func ClearProgress() {
	if progressLineActive {
		fmt.Print("\r\033[K")
		progressLineActive = false
	}
}

func progressBar(current, total, width int) string {
	if total <= 0 { return "" }
	filled := (current * width) / total
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	pct := (current * 100) / total
	return fmt.Sprintf("%s%s [%s%d%%%s]", cGreen, bar, cDim, pct, cReset)
}

// ── Status Bar ────────────────────────────────────────────

func PrintStatusBar(tokens, tools int, elapsed time.Duration, model string) {
	width := 60
	sep := strings.Repeat(boxH, width)
	fmt.Printf("\n%s%s%s\n", cSep, sep, cReset)

	var parts []string
	if model != "" {
		parts = append(parts, fmt.Sprintf("%s%s%s", cDim, model, cReset))
	}
	if tokens > 0 {
		parts = append(parts, fmt.Sprintf("%s⚡ %d tokens%s", cInfo, tokens, cReset))
	}
	if tools > 0 {
		parts = append(parts, fmt.Sprintf("%s🔧 %d tools%s", cInfo, tools, cReset))
	}
	parts = append(parts, fmt.Sprintf("%s⏱ %v%s", cInfo, elapsed.Round(100*time.Millisecond), cReset))
	fmt.Printf("  %s\n\n", strings.Join(parts, "  │  "))
}

// ── Simple Print Helpers ──────────────────────────────────

func Println()  { fmt.Println() }
func Printf(f string, args ...interface{}) { fmt.Printf(f, args...) }

func PrintUserQuery(query string) {
	fmt.Printf("\n  %s▸ %s%s\n", cUser, query, cReset)
}

func PrintError(err string) {
	fmt.Printf("\n  %s✗ %s%s\n", cError, err, cReset)
}

func PrintWarning(msg string) {
	fmt.Printf("  %s⚠ %s%s\n", cWarn, msg, cReset)
}

func PrintSuccess(msg string) {
	fmt.Printf("  %s✓ %s%s\n", cSuccess, msg, cReset)
}

func PrintInfo(msg string) {
	fmt.Printf("  %s%s%s\n", cInfo, msg, cReset)
}

// ── Content Divider ───────────────────────────────────────

func PrintDivider(title string) {
	width := 60
	if title != "" {
		fmt.Printf("\n  %s── %s%s%s %s\n", cSep, cHeading, title, cReset, cSep+strings.Repeat(boxH, width-len(title)-6))
	} else {
		fmt.Printf("\n  %s%s\n", cSep, strings.Repeat(boxH, width))
	}
}

// ── Section ───────────────────────────────────────────────

func BeginSection(title string) {
	width := 60
	fmt.Printf("\n  %s%s %s%s %s", cBorder, boxTL, cHeading, title, cReset)
	fmt.Printf("%s%s%s\n", cBorder, strings.Repeat(boxH, width-len(title)-4), boxTR)
}

func EndSection() {
	width := 60
	fmt.Printf("  %s%s%s%s\n", cBorder, boxBL, strings.Repeat(boxH, width-2), boxBR)
}

func SectionLine(text string) {
	fmt.Printf("  %s%s %s%s\n", cBorder, boxV, text, cReset)
}

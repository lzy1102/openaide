package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// ── ANSI Color Helpers ────────────────────────────────────

var (
	dim    = "\033[2m"
	reset  = "\033[0m"
	bold   = "\033[1m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	blue   = "\033[34m"
	cyan   = "\033[36m"
	gray   = "\033[90m"
)

// ── Markdown Renderer ─────────────────────────────────────

var mdRenderer *glamour.TermRenderer

func initMarkdown() {
	if mdRenderer != nil {
		return
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		// Fallback: just print plain text
		return
	}
	mdRenderer = r
}

// RenderMarkdown renders markdown text to ANSI terminal
func RenderMarkdown(text string) string {
	if mdRenderer == nil {
		initMarkdown()
	}
	if mdRenderer == nil {
		return text
	}
	out, err := mdRenderer.Render(text)
	if err != nil {
		return text
	}
	return out
}

// ── Output Helpers ────────────────────────────────────────

// PrintStream outputs a content chunk during streaming (no newline)
func PrintStream(content string) {
	fmt.Print(content)
}

// PrintThinking displays a dimmed thinking line
func PrintThinking(text string) {
	firstLine := strings.SplitN(text, "\n", 2)[0]
	if len(firstLine) > 160 {
		firstLine = firstLine[:157] + "..."
	}
	fmt.Printf("%s  [think] %s%s\n", gray, firstLine, reset)
}

// PrintToolCall displays a tool being called (always on new line)
func PrintToolCall(name string) {
	fmt.Printf("\n  %s⚙ %s%s", yellow, name, reset)
}

// PrintToolDone displays a completed tool with result summary
func PrintToolDone(summary string) {
	fmt.Printf(" %s✓%s", green, reset)
	if summary != "" {
		fmt.Printf(" %s%s%s", gray, trunc(summary, 80), reset)
	}
	fmt.Println()
}

// PrintStatusLine prints the final status bar after completion
func PrintStatusLine(tokens, tools int, elapsed time.Duration) {
	sep := strings.Repeat("─", 60)
	fmt.Printf("\n%s%s%s\n", gray, sep, reset)
	parts := []string{}
	if tokens > 0 {
		parts = append(parts, fmt.Sprintf("%s⚡ %d tokens%s", yellow, tokens, reset))
	}
	if tools > 0 {
		parts = append(parts, fmt.Sprintf("%s🔧 %d tools%s", yellow, tools, reset))
	}
	parts = append(parts, fmt.Sprintf("%s⏱ %v%s", gray, elapsed.Round(100*time.Millisecond), reset))
	fmt.Printf("  %s\n\n", strings.Join(parts, " │ "))
}

// PrintError prints an error message
func PrintError(err string) {
	fmt.Printf("\n  %s✗ %s%s\n", red, err, reset)
}

// PrintWarning prints a warning message
func PrintWarning(msg string) {
	fmt.Printf("%s  ⚠ %s%s\n", yellow, msg, reset)
}

// PrintSuccess prints a success message
func PrintSuccess(msg string) {
	fmt.Printf("%s  ✓ %s%s\n", green, msg, reset)
}

// ── Prompt ────────────────────────────────────────────────

// PromptStyle returns a styled prompt
func PromptStyle(sessionID, modelName string) string {
	prompt := "❯ "
	if sessionID != "" {
		prompt = fmt.Sprintf("%s%s%s │ %s", dim, sessionID[:8], reset, prompt)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#82B74B")).Render(prompt)
}

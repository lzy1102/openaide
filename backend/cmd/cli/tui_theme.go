package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

// ── Icons ──────────────────────────────────────────────────

type iconSet struct {
	folder, thinking, tools, tokens, err, busy, user, system, gear, result string
}

var icons iconSet

func init() {
	term := strings.ToLower(os.Getenv("TERM"))
	noColor := os.Getenv("NO_COLOR") != "" || os.Getenv("OPENAIDE_NO_EMOJI") != ""
	dumb := term == "" || term == "dumb" || noColor
	if dumb {
		icons = iconSet{"[", "*", "#", "~", "x", "...", ">", "[sys]", ">", "->"}
	} else {
		icons = iconSet{"📁", "◉", "🔧", "⚡", "✗", "⏳", "▸", "[sys]", "⚙", "→"}
	}
}

// ── Theme ──────────────────────────────────────────────────

type tuiTheme struct {
	user, ai, think, err, tool, toolOut, sys                     lipgloss.Style
	statusBar, input, sessionTitle, sel, helpKey, helpTitle      lipgloss.Style
	helpSection, separator, warn, codeBlock, planTitle, planPrompt lipgloss.Style
}

var theme = tuiTheme{
	user:         lipgloss.NewStyle().Foreground(lipgloss.Color("#6C8EBF")).Bold(true),
	ai:           lipgloss.NewStyle().Foreground(lipgloss.Color("#82B74B")),
	think:        lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Italic(true),
	err:          lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")),
	tool:         lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A859")).Bold(true),
	toolOut:      lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")),
	sys:          lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA")).Italic(true),
	statusBar:    lipgloss.NewStyle().Background(lipgloss.Color("#1a1a2e")).Foreground(lipgloss.Color("#AAAAAA")).Padding(0, 1),
	input:        lipgloss.NewStyle().PaddingLeft(1),
	sessionTitle: lipgloss.NewStyle().Foreground(lipgloss.Color("#6C8EBF")).Bold(true).Underline(true),
	sel:          lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A859")).Bold(true),
	helpKey:      lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")),
	helpTitle:    lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A859")).Bold(true),
	helpSection:  lipgloss.NewStyle().Foreground(lipgloss.Color("#6C8EBF")).Bold(true),
	separator:    lipgloss.NewStyle().Foreground(lipgloss.Color("#333333")),
	warn:         lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A859")).Bold(true),
	codeBlock:    lipgloss.NewStyle().Background(lipgloss.Color("#1a1a2e")).Padding(0, 1),
	planTitle:    lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A859")).Bold(true),
	planPrompt:   lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA")),
}

var (
	userStyle        = theme.user
	aiStyle          = theme.ai
	thinkStyle       = theme.think
	errStyle         = theme.err
	toolStyle        = theme.tool
	toolOutStyle     = theme.toolOut
	sysStyle         = theme.sys
	statusBarStyle   = theme.statusBar
	inputStyle       = theme.input
	sessionTitleStyle = theme.sessionTitle
	selStyle         = theme.sel
	helpKeyStyle     = theme.helpKey
	helpTitleStyle   = theme.helpTitle
	helpSectionStyle = theme.helpSection
	separatorStyle   = theme.separator
	warnStyle        = theme.warn
	codeBlockStyle   = theme.codeBlock
	planTitleStyle   = theme.planTitle
	planPromptStyle  = theme.planPrompt
)

// ── Log Ring Buffer ────────────────────────────────────────

type logRing struct {
	mu  sync.Mutex
	buf []string
}

func (r *logRing) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, strings.TrimSpace(string(p)))
	if len(r.buf) > 50 {
		r.buf = r.buf[1:]
	}
	return len(p), nil
}

func (r *logRing) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := make([]string, len(r.buf))
	copy(c, r.buf)
	return c
}

var tuiLogBuf = &logRing{buf: make([]string, 0, 50)}

// ── Utility Functions ──────────────────────────────────────

var lastRender time.Time

func trunc(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "..."
}

// ── Syntax Highlighting ────────────────────────────────────

var chromaStyle *chroma.Style

func initHighlighter() {
	chromaStyle = styles.Get("monokai")
	if chromaStyle == nil {
		chromaStyle = styles.Fallback
	}
}

var codeBlockRe = regexp.MustCompile("(?s)```(\\w*)\\n(.*?)```")

func highlightCode(text string) string {
	if chromaStyle == nil {
		initHighlighter()
	}
	return codeBlockRe.ReplaceAllStringFunc(text, func(match string) string {
		parts := codeBlockRe.FindStringSubmatch(match)
		if len(parts) < 3 { return match }
		lang := parts[1]
		code := parts[2]
		if lang == "" { lang = "go" }
		highlighted := highlightBlock(code, lang)
		if highlighted == "" { return match }
		return codeBlockStyle.Render(fmt.Sprintf("```%s\n%s```", lang, highlighted))
	})
}

func highlightBlock(code, lang string) string {
	if chromaStyle == nil {
		initHighlighter()
	}
	lexer := lexers.Get(lang)
	if lexer == nil { lexer = lexers.Fallback }
	iterator, err := lexer.Tokenise(nil, code)
	if err != nil { return "" }
	var sb strings.Builder
	formatter := formatters.TTY256
	if err := formatter.Format(&sb, chromaStyle, iterator); err != nil { return "" }
	return sb.String()
}

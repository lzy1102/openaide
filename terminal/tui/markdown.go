package tui

import (
	"os"

	"github.com/charmbracelet/glamour"
	"github.com/chzyer/readline"
)

var renderer *glamour.TermRenderer

func InitMarkdownRenderer() error {
	width := getTerminalWidth()
	if width <= 0 {
		width = 80
	}
	width = width - 4
	if width < 40 {
		width = 40
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(width),
		glamour.WithStandardStyle("dark"),
	)
	if err != nil {
		return err
	}
	renderer = r
	return nil
}

func RenderMarkdown(content string) string {
	if renderer == nil {
		if err := InitMarkdownRenderer(); err != nil {
			return content
		}
	}
	out, err := renderer.Render(content)
	if err != nil {
		return content
	}
	return out
}

func getTerminalWidth() int {
	if w := os.Getenv("COLUMNS"); w != "" {
		width := 0
		for _, c := range w {
			if c >= '0' && c <= '9' {
				width = width*10 + int(c-'0')
			}
		}
		if width > 0 {
			return width
		}
	}
	if w := readline.GetScreenWidth(); w > 0 {
		return w
	}
	return 80
}

package tui

import (
	"os"

	"github.com/charmbracelet/glamour"
)

var renderer *glamour.TermRenderer

// InitMarkdownRenderer 初始化 Markdown 渲染器
func InitMarkdownRenderer() error {
	width := getTerminalWidth()
	if width <= 0 {
		width = 80
	}
	// 留出左右边距
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

// RenderMarkdown 渲染 Markdown 文本
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

// getTerminalWidth 获取终端宽度
func getTerminalWidth() int {
	// 尝试从环境变量获取
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
	return 80
}

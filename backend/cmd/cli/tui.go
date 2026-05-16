package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"openaide/backend/internal/infra"
	"openaide/backend/internal/kernel"
)

var (
	tUser  = lipgloss.NewStyle().Foreground(lipgloss.Color("#6C8EBF")).Bold(true).PaddingLeft(1)
	tAI    = lipgloss.NewStyle().Foreground(lipgloss.Color("#82B74B")).Bold(true).PaddingLeft(1)
	tThink = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Italic(true).PaddingLeft(3)
	tTool  = lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A859"))
	tErr   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B"))
	tInfo  = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	tBar   = lipgloss.NewStyle().Background(lipgloss.Color("#1a1a2e")).Foreground(lipgloss.Color("#AAAAAA")).Padding(0, 1).Width(100)
	tInput = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#444444")).Padding(0, 1)
)

type streamChunk struct {
	content  string
	thinking string
	done     bool
	tokens   int
	toolCnt  int
	err      error
}

type tuiModel struct {
	app       *infra.Application
	program   *tea.Program
	messages  []tuiMsg
	input     string
	streaming bool
	think     string
	ai        string
	tokens    int
	tools     int
	err       error
	width     int
	height    int
}

type tuiMsg struct{ role, content string }

func runTUI(app *infra.Application) error {
	m := &tuiModel{app: app, width: 100, height: 40}
	p := tea.NewProgram(m, tea.WithAltScreen())
	m.program = p
	_, err := p.Run()
	return err
}

func (m *tuiModel) Init() tea.Cmd { return nil }

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.streaming {
			if msg.String() == "ctrl+c" {
				m.streaming = false
				if m.ai != "" {
					m.messages = append(m.messages, tuiMsg{role: "assistant", content: formatThink(m.think) + m.ai})
				}
				m.ai = ""
				m.think = ""
				return m, nil
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "ctrl+d":
			return m, tea.Quit
		case "enter":
			if m.input == "" {
				return m, nil
			}
			if m.input == "/clear" {
				m.messages = nil
				m.input = ""
				m.tokens = 0
				m.tools = 0
				return m, nil
			}
			query := m.input
			m.input = ""
			m.messages = append(m.messages, tuiMsg{role: "user", content: query})
			m.streaming = true
			m.ai = ""
			m.think = ""
			m.err = nil
			go streamToProgram(m.program, m.app, query)
			return m, nil

		case "backspace":
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
		default:
			s := msg.String()
			if len(s) == 1 && s[0] >= 32 {
				m.input += s
			}
		}

	case streamChunk:
		if msg.err != nil {
			m.err = msg.err
			m.streaming = false
			return m, nil
		}
		if msg.done {
			m.streaming = false
			m.tokens = msg.tokens
			m.tools = msg.toolCnt
			if m.ai != "" {
				m.messages = append(m.messages, tuiMsg{role: "assistant", content: formatThink(m.think) + m.ai})
			}
			m.ai = ""
			m.think = ""
			return m, nil
		}
		if msg.content != "" {
			m.ai += msg.content
		}
		if msg.thinking != "" {
			m.think += msg.thinking
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

func (m *tuiModel) View() string {
	if m.width == 0 {
		m.width = 100
	}
	var sb strings.Builder

	// Messages (newest at bottom)
	rendered := []string{}
	if m.streaming {
		if m.think != "" {
			rendered = append(rendered, tThink.Render("... "+trunc(m.think, 200)))
		}
		rendered = append(rendered, tAI.Render(m.ai))
	}
	for i := len(m.messages) - 1; i >= 0; i-- {
		msg := m.messages[i]
		if msg.role == "user" {
			rendered = append(rendered, tUser.Render("▸ "+msg.content))
		} else {
			rendered = append(rendered, tAI.Render(msg.content))
		}
	}
	for i := len(rendered) - 1; i >= 0; i-- {
		sb.WriteString(rendered[i] + "\n")
	}

	// Status bar
	status := ""
	if m.tools > 0 {
		status += tTool.Render(fmt.Sprintf("🔧%d ", m.tools))
	}
	if m.tokens > 0 {
		status += tInfo.Render(fmt.Sprintf("⚡%d ", m.tokens))
	}
	if m.streaming {
		status += tInfo.Render("● ")
	}
	if status != "" {
		sb.WriteString(tBar.Render(status) + "\n")
	}
	if m.err != nil {
		sb.WriteString(tErr.Render("✗ "+m.err.Error()) + "\n")
	}

	// Input
	sb.WriteString(strings.Repeat("─", m.width) + "\n")
	prompt := "> "
	if m.streaming {
		prompt = "⏳ "
	}
	cursor := ""
	if !m.streaming {
		cursor = "│"
	}
	sb.WriteString(tInput.Render(prompt + m.input + cursor))

	return sb.String()
}

// ============ Streaming ============

func streamToProgram(p *tea.Program, app *infra.Application, query string) {
	ctx := context.Background()
	stream, err := app.Orchestrator.ProcessQueryStream(ctx, "tui-user", "default", query, kernel.QueryOptions{})
	if err != nil {
		p.Send(streamChunk{err: err, done: true})
		return
	}

	totalTools := 0
	totalTokens := 0

	for chunk := range stream {
		if chunk.Error != nil {
			p.Send(streamChunk{err: chunk.Error, done: true})
			return
		}
		if chunk.Done {
			if chunk.Usage != nil {
				totalTokens = chunk.Usage.TotalTokens
			}
			p.Send(streamChunk{done: true, tokens: totalTokens, toolCnt: totalTools})
			return
		}
		if len(chunk.ToolCalls) > 0 {
			totalTools += len(chunk.ToolCalls)
		}
		p.Send(streamChunk{content: chunk.Content, thinking: chunk.ReasoningContent})
		time.Sleep(10 * time.Millisecond)
	}
}

func formatThink(think string) string {
	if think == "" {
		return ""
	}
	return tThink.Render("[思考] "+think) + "\n"
}

func trunc(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "..."
}

package main

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"openaide/backend/internal/infra"
	"openaide/backend/internal/kernel"
)

var (
	sUser  = lipgloss.NewStyle().Foreground(lipgloss.Color("#6C8EBF")).Bold(true)
	sAI    = lipgloss.NewStyle().Foreground(lipgloss.Color("#82B74B")).Bold(true)
	sThink = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Italic(true)
	sTool  = lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A859"))
	sErr   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B"))
	sInfo  = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	sBar   = lipgloss.NewStyle().Background(lipgloss.Color("#222222")).Foreground(lipgloss.Color("#AAAAAA")).Padding(0, 1)
	sInp   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#444444")).Padding(0, 1)
)

type streamChunkMsg struct {
	content  string
	thinking string
	done     bool
	tokens   int
	toolCnt  int
	err      error
}

type tuiModel struct {
	app       *infra.Application
	messages  []tuiMsg
	input     string
	streaming bool
	curThink  string
	curAI     string
	tokens    int
	tools     int
	err       error
	width     int
	height    int
}

type tuiMsg struct{ role, content string }

func runTUI(app *infra.Application) error {
	p := tea.NewProgram(initialModel(app), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func initialModel(app *infra.Application) tuiModel {
	return tuiModel{
		app:      app,
		messages: []tuiMsg{{role: "assistant", content: "OpenAIDE ready. Type a message and press Enter."}},
	}
}

func (m tuiModel) Init() tea.Cmd { return nil }

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.streaming {
			// Ctrl+C can interrupt streaming
			if msg.String() == "ctrl+c" {
				m.streaming = false
				m.messages = append(m.messages, tuiMsg{role: "assistant", content: m.curAI})
				m.curAI = ""
				m.curThink = ""
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
			m.curAI = ""
			m.curThink = ""
			m.err = nil
			return m, streamCmd(m.app, query)

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

	case streamChunkMsg:
		if msg.err != nil {
			m.err = msg.err
			m.streaming = false
			return m, nil
		}
		if msg.done {
			m.streaming = false
			m.tokens = msg.tokens
			m.tools = msg.toolCnt
			content := m.curAI
			if m.curThink != "" {
				content = "[思考]\n" + m.curThink + "\n\n" + content
			}
			if content != "" {
				m.messages = append(m.messages, tuiMsg{role: "assistant", content: content})
			}
			m.curAI = ""
			m.curThink = ""
			return m, nil
		}
		if msg.content != "" {
			m.curAI += msg.content
		}
		if msg.thinking != "" {
			m.curThink += msg.thinking
		}
		return m, waitStreamCmd()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

func (m tuiModel) View() string {
	if m.width == 0 {
		m.width = 100
	}

	var sb strings.Builder

	// Messages (bottom-up to fit viewport)
	var rendered []string
	if m.streaming {
		rendered = append(rendered, sAI.Render("◆ ")+m.curAI)
		if m.curThink != "" {
			rendered = append(rendered, sThink.Render("[思考] "+m.curThink))
		}
	}
	for i := len(m.messages) - 1; i >= 0; i-- {
		msg := m.messages[i]
		if msg.role == "user" {
			rendered = append(rendered, sUser.Render("▸ ")+msg.content)
		} else {
			rendered = append(rendered, sAI.Render("◆ ")+msg.content)
		}
	}

	// Reverse and fit
	for i := len(rendered) - 1; i >= 0; i-- {
		sb.WriteString(rendered[i] + "\n")
	}

	// Status bar
	status := ""
	if m.tools > 0 {
		status += sTool.Render(fmt.Sprintf(" 🔧%d ", m.tools))
	}
	if m.tokens > 0 {
		status += sInfo.Render(fmt.Sprintf(" ⚡%d ", m.tokens))
	}
	if m.streaming {
		status += sInfo.Render(" ● streaming ")
	}
	if status != "" {
		sb.WriteString(sBar.Render(status) + "\n")
	}

	// Error
	if m.err != nil {
		sb.WriteString(sErr.Render("✗ "+m.err.Error()) + "\n")
	}

	sb.WriteString(strings.Repeat("─", m.width) + "\n")

	// Input
	prompt := "> "
	if m.streaming {
		prompt = "⏳ "
	}
	inputLine := prompt + m.input
	if !m.streaming {
		inputLine += "│"
	}
	sb.WriteString(sInp.Render(inputLine))

	return sb.String()
}

func streamCmd(app *infra.Application, query string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		stream, err := app.Orchestrator.ProcessQueryStream(ctx, "tui-user", "default", query, kernel.QueryOptions{})
		if err != nil {
			return streamChunkMsg{err: err, done: true}
		}

		totalTools := 0
		totalTokens := 0

		for chunk := range stream {
			if chunk.Error != nil {
				return streamChunkMsg{err: chunk.Error, done: true}
			}
			if chunk.Done {
				if chunk.Usage != nil {
					totalTokens = chunk.Usage.TotalTokens
				}
				return streamChunkMsg{done: true, tokens: totalTokens, toolCnt: totalTools}
			}
			if len(chunk.ToolCalls) > 0 {
				totalTools += len(chunk.ToolCalls)
			}
			// NOTE: Due to bubbletea's architecture, we can only send one message per command.
			// Real streaming TUI requires p.Send() from a goroutine — see tui_advanced.go
			if chunk.Content != "" {
				// Accumulate
			}
		}
		return streamChunkMsg{done: true}
	}
}

func waitStreamCmd() tea.Cmd {
	return nil
}

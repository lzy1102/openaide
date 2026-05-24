package main

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"openaide/backend/internal/kernel"
)

// ── ChatArea Component ─────────────────────────────────────

const maxHistory = 50

type ChatArea struct {
	viewport viewport.Model
	messages []Message
	width    int
	height   int

	aiBuf    strings.Builder
	thinkBuf strings.Builder
}

func NewChatArea() *ChatArea {
	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#333333"))
	return &ChatArea{viewport: vp}
}

func (c *ChatArea) SetSize(w, h int) {
	c.width = w
	c.height = h
	c.viewport.Width = w - 4
	c.viewport.Height = h - 4
}

func (c *ChatArea) AddMessage(role, content, thinking string) {
	c.messages = append(c.messages, Message{Role: role, Content: content, Thinking: thinking})
	if len(c.messages) > maxHistory {
		c.messages = c.messages[len(c.messages)-maxHistory:]
	}
}

func (c *ChatArea) Clear() {
	c.messages = nil
	c.aiBuf.Reset()
	c.thinkBuf.Reset()
}

func (c *ChatArea) ResetBuffers() {
	c.aiBuf.Reset()
	c.thinkBuf.Reset()
}

func (c *ChatArea) ScrollToBottom() { c.viewport.GotoBottom() }
func (c *ChatArea) AtBottom() bool  { return c.viewport.AtBottom() }

func (c *ChatArea) Update(msg tea.Msg) (tea.Cmd, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.SetSize(msg.Width, msg.Height)
	case StreamContentMsg:
		c.aiBuf.WriteString(msg.Content)
		c.thinkBuf.WriteString(msg.Thinking)
	}
	return nil, nil
}

func (c *ChatArea) View() string {
	c.renderContent()
	return c.viewport.View()
}

func (c *ChatArea) renderContent() {
	// Throttle render during streaming
	isStreaming := c.aiBuf.Len() > 0 || c.thinkBuf.Len() > 0
	if isStreaming && time.Since(lastRender) < 50*time.Millisecond {
		return
	}
	lastRender = time.Now()

	var sb strings.Builder
	start := 0
	if len(c.messages) > maxHistory {
		start = len(c.messages) - maxHistory
	}
	for i := start; i < len(c.messages); i++ {
		msg := c.messages[i]
		if i > start && c.messages[i-1].Role != msg.Role {
			w := c.width - 6
			if w < 20 { w = 20 }
			sb.WriteString(separatorStyle.Render(strings.Repeat("-", w)) + "\n")
		}
		c.renderMessage(&sb, msg)
	}

	// Streaming buffer
	if c.aiBuf.Len() > 0 {
		sb.WriteString(aiStyle.Render(c.aiBuf.String()))
	}
	if c.thinkBuf.Len() > 0 {
		sb.WriteString(thinkStyle.Render("[think] " + trunc(strings.SplitN(c.thinkBuf.String(), "\n", 2)[0], 120)))
	}
	if len(c.messages) == 0 && c.aiBuf.Len() == 0 {
		sb.WriteString(sysStyle.Render("[sys] 就绪。输入消息开始…"))
	}

	c.viewport.SetContent(sb.String())
}

func (c *ChatArea) renderMessage(sb *strings.Builder, msg Message) {
	switch msg.Role {
	case "user":
		sb.WriteString(userStyle.Render(icons.user+" " + msg.Content))
	case "assistant":
		display := highlightCode(msg.Content)
		if msg.Thinking != "" {
			display = thinkStyle.Render("[think] "+trunc(strings.SplitN(msg.Thinking, "\n", 2)[0], 120)) + "\n" + display
		}
		sb.WriteString(aiStyle.Render(display))
	case "error":
		sb.WriteString(errStyle.Render("✗ " + msg.Content))
	case "system":
		sb.WriteString(sysStyle.Render(icons.system+" " + trunc(strings.SplitN(msg.Content, "\n", 2)[0], 120)))
	case "tool_call":
		sb.WriteString(toolStyle.Render(icons.gear+" " + msg.Content))
	case "tool":
		sb.WriteString(toolOutStyle.Render("  → " + trunc(msg.Content, 200)))
	default:
		sb.WriteString(highlightCode(msg.Content))
	}
	sb.WriteString("\n")
}

// Load from kernel session
func (c *ChatArea) LoadHistory(msgs []kernel.Message) {
	c.messages = nil
	for _, msg := range msgs {
		c.messages = append(c.messages, Message{
			Role:     msg.Role,
			Content:  msg.Content,
			Thinking: msg.ReasoningContent,
		})
	}
}

func (c *ChatArea) FlushBuffers() (content string, thinking string) {
	content = c.aiBuf.String()
	thinking = c.thinkBuf.String()
	c.aiBuf.Reset()
	c.thinkBuf.Reset()
	return
}

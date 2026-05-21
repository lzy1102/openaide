package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"openaide/backend/internal/infra"
	"openaide/backend/internal/kernel"
)

type ViewState int

const (
	viewChat ViewState = iota
	viewSessionList
	viewHelp
)

const maxHistory = 50
const maxSessions = 100

type chatMsg struct {
	role    string
	content string
}

type chunkMsg struct {
	content  string
	thinking string
	done     bool
	tokens   int
	toolCnt  int
	err      error
}

type sessionListMsg struct {
	sessions []*kernel.Session
	session  *kernel.Session
	err      error
}

type sessionCreatedMsg struct {
	session *kernel.Session
	err     error
}

type sessionDeletedMsg struct {
	id  string
	err error
}

type model struct {
	app     *infra.Application
	program *tea.Program

	state  ViewState
	width  int
	height int

	messages []chatMsg
	viewport viewport.Model
	input    textinput.Model

	streaming bool
	thinkBuf  strings.Builder
	aiBuf     strings.Builder

	history []string
	histIdx int

	sessions    []*kernel.Session
	selSession  int
	currentSess *kernel.Session

	tokens         int
	tools          int
	err            error
	deleteTargetID string
}

func initModel(app *infra.Application, continueSess bool) *model {
	ti := textinput.New()
	ti.Placeholder = "Type a message... (/help for commands)"
	ti.Prompt = "❯ "
	ti.Focus()
	ti.CharLimit = 4000
	ti.Width = 60

	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#333333"))

	m := &model{
		app:        app,
		state:      viewChat,
		viewport:   vp,
		input:      ti,
		histIdx:    -1,
		selSession: -1,
	}

	if continueSess {
		ctx := context.Background()
		sessions, err := app.Orchestrator.ListSessions(ctx, "default", "cli-user", 1, 0)
		if err == nil && len(sessions) > 0 {
			m.currentSess = sessions[0]
			m.loadChatHistory()
		}
	}

	return m
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.loadSessionList(),
	)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width - 4
		m.viewport.Height = msg.Height - 5
		m.input.Width = msg.Width - 8
		m.renderViewport()

	case tea.KeyMsg:
		switch m.state {
		case viewSessionList:
			return m.updateSessionList(msg)
		case viewHelp:
			m.state = viewChat
			m.input.Focus()
			return m, nil
		case viewChat:
			return m.updateChat(msg)
		}

	case chunkMsg:
		if msg.err != nil {
			m.err = msg.err
			m.streaming = false
			m.input.Focus()
			m.renderViewport()
			return m, nil
		}
		if msg.done {
			m.streaming = false
			m.tokens = msg.tokens
			m.tools = msg.toolCnt
			text := m.aiBuf.String()
			if m.thinkBuf.Len() > 0 {
				think := m.thinkBuf.String()
				text = thinkStyle.Render("[think] "+trunc(think, 500)) + "\n" + text
			}
			if text != "" {
				m.messages = append(m.messages, chatMsg{role: "assistant", content: text})
			}
			m.aiBuf.Reset()
			m.thinkBuf.Reset()
			m.input.Focus()
			m.renderViewport()
			m.viewport.GotoBottom()
			return m, nil
		}
		if msg.content != "" {
			m.aiBuf.WriteString(msg.content)
			m.renderViewport()
			m.viewport.GotoBottom()
		}
		if msg.thinking != "" {
			m.thinkBuf.WriteString(msg.thinking)
			m.renderViewport()
		}
		return m, nil

	case sessionListMsg:
		if msg.err == nil {
			m.sessions = msg.sessions
			if msg.session != nil {
				m.currentSess = msg.session
				m.messages = nil
				m.tokens = 0
				m.tools = 0
				m.err = nil
			}
			if m.selSession >= len(m.sessions) {
				m.selSession = len(m.sessions) - 1
			}
			if m.selSession < 0 && len(m.sessions) > 0 {
				m.selSession = 0
			}
		}

	case sessionCreatedMsg:
		if msg.err == nil {
			m.currentSess = msg.session
			m.messages = nil
			m.tokens = 0
			m.tools = 0
			m.err = nil
			cmds = append(cmds, m.loadSessionList())
		}

	case sessionDeletedMsg:
		if msg.err == nil {
			if m.currentSess != nil && m.currentSess.ID == msg.id {
				m.currentSess = nil
				m.messages = nil
			}
			cmds = append(cmds, m.loadSessionList())
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *model) updateChat(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "ctrl+d":
		if m.streaming {
			m.streaming = false
			text := m.aiBuf.String()
			if m.thinkBuf.Len() > 0 {
				think := m.thinkBuf.String()
				text = thinkStyle.Render("[think] "+trunc(think, 500)) + "\n" + text
			}
			if text != "" {
				m.messages = append(m.messages, chatMsg{role: "assistant", content: text})
			}
			m.aiBuf.Reset()
			m.thinkBuf.Reset()
			m.renderViewport()
			m.input.Focus()
			return m, nil
		}
		return m, tea.Quit

	case "ctrl+s":
		if !m.streaming {
			m.state = viewSessionList
			m.input.Blur()
			m.selSession = -1
			return m, m.loadSessionList()
		}

	case "f1", "ctrl+h":
		if !m.streaming {
			m.state = viewHelp
			m.input.Blur()
			return m, nil
		}

	case "enter":
		if m.streaming {
			return m, nil
		}
		query := strings.TrimSpace(m.input.Value())
		if query == "" {
			return m, nil
		}
		if strings.HasPrefix(query, "/") {
			return m.handleCommand(query)
		}

		if len(m.history) == 0 || m.history[len(m.history)-1] != query {
			m.history = append(m.history, query)
			if len(m.history) > maxHistory {
				m.history = m.history[1:]
			}
		}
		m.histIdx = -1

		m.messages = append(m.messages, chatMsg{role: "user", content: query})
		m.renderViewport()
		m.viewport.GotoBottom()
		m.input.SetValue("")

		m.streaming = true
		m.thinkBuf.Reset()
		m.aiBuf.Reset()
		m.err = nil
		m.input.Blur()

		sessionID := ""
		if m.currentSess != nil {
			sessionID = m.currentSess.ID
		}
		go doStream(m.program, m.app, sessionID, query)
		return m, nil

	case "up":
		if !m.streaming && len(m.history) > 0 {
			if m.histIdx == -1 {
				m.histIdx = len(m.history) - 1
			} else if m.histIdx > 0 {
				m.histIdx--
			}
			m.input.SetValue(m.history[m.histIdx])
			m.input.CursorEnd()
		}
		return m, nil

	case "down":
		if !m.streaming && m.histIdx >= 0 {
			m.histIdx++
			if m.histIdx >= len(m.history) {
				m.histIdx = -1
				m.input.SetValue("")
			} else {
				m.input.SetValue(m.history[m.histIdx])
			}
			m.input.CursorEnd()
		}
		return m, nil

	case "pgup":
		m.viewport.HalfViewUp()
		return m, nil
	case "pgdown":
		m.viewport.HalfViewDown()
		return m, nil
	}

	if !m.streaming {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *model) handleCommand(cmd string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(cmd)
	switch parts[0] {
	case "/help":
		m.state = viewHelp
		m.input.Blur()
		return m, nil
	case "/clear":
		m.messages = nil
		m.tokens = 0
		m.tools = 0
		m.err = nil
		m.input.SetValue("")
		m.renderViewport()
		return m, nil
	case "/new":
		ctx := context.Background()
		sess, err := m.app.Orchestrator.CreateSession(ctx, "default", "cli-user")
		if err != nil {
			m.err = err
			m.input.SetValue("")
			return m, nil
		}
		m.currentSess = sess
		m.messages = nil
		m.tokens = 0
		m.tools = 0
		m.err = nil
		m.input.SetValue("")
		m.renderViewport()
		return m, m.loadSessionList()
	case "/sessions":
		m.state = viewSessionList
		m.input.Blur()
		m.selSession = 0
		return m, m.loadSessionList()
	default:
		m.err = fmt.Errorf("unknown command: %s (try /help)", parts[0])
		m.input.SetValue("")
		return m, nil
	}
}

func (m *model) updateSessionList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.deleteTargetID = ""
		if m.selSession > 0 {
			m.selSession--
		}
	case "down", "j":
		m.deleteTargetID = ""
		if m.selSession < len(m.sessions)-1 {
			m.selSession++
		}
	case "enter":
		m.deleteTargetID = ""
		if m.selSession >= 0 && m.selSession < len(m.sessions) {
			m.currentSess = m.sessions[m.selSession]
			m.messages = nil
			m.tokens = 0
			m.tools = 0
			m.err = nil
			m.loadChatHistory()
			m.renderViewport()
		}
		m.state = viewChat
		m.input.Focus()
	case "n":
		ctx := context.Background()
		sess, err := m.app.Orchestrator.CreateSession(ctx, "default", "cli-user")
		if err != nil {
			m.err = err
			return m, nil
		}
		m.currentSess = sess
		m.messages = nil
		m.tokens = 0
		m.tools = 0
		m.err = nil
		m.state = viewChat
		m.input.Focus()
		return m, m.loadSessionList()
	case "d":
		if m.selSession >= 0 && m.selSession < len(m.sessions) {
			id := m.sessions[m.selSession].ID
			if m.deleteTargetID == id {
				// Second press — confirm delete
				m.deleteTargetID = ""
				ctx := context.Background()
				go func() {
					err := m.app.Orchestrator.DeleteSession(ctx, id)
					m.program.Send(sessionDeletedMsg{id: id, err: err})
				}()
				if m.currentSess != nil && m.currentSess.ID == id {
					m.currentSess = nil
					m.messages = nil
				}
			} else {
				m.deleteTargetID = id
			}
		}
	case "esc", "q":
		m.state = viewChat
		m.input.Focus()
	}
	return m, nil
}

func (m *model) loadChatHistory() {
	if m.currentSess == nil {
		return
	}
	ctx := context.Background()
	msgs, err := m.app.Orchestrator.GetSessionHistory(ctx, m.currentSess.ID, 100)
	if err != nil || len(msgs) == 0 {
		return
	}
	m.messages = nil
	for _, msg := range msgs {
		display := msg.Content
		if msg.ReasoningContent != "" {
			display = thinkStyle.Render("[think] "+trunc(msg.ReasoningContent, 500)) + "\n" + display
		}
		m.messages = append(m.messages, chatMsg{role: msg.Role, content: display})
	}
	m.renderViewport()
}

func (m *model) loadSessionList() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		sessions, err := m.app.Orchestrator.ListSessions(ctx, "default", "cli-user", maxSessions, 0)
		if err != nil {
			sess, createErr := m.app.Orchestrator.CreateSession(ctx, "default", "cli-user")
			if createErr != nil {
				return sessionListMsg{err: createErr}
			}
			return sessionListMsg{sessions: []*kernel.Session{sess}, session: sess}
		}
		if len(sessions) == 0 {
			sess, createErr := m.app.Orchestrator.CreateSession(ctx, "default", "cli-user")
			if createErr != nil {
				return sessionListMsg{err: createErr}
			}
			return sessionListMsg{sessions: []*kernel.Session{sess}, session: sess}
		}
		return sessionListMsg{sessions: sessions}
	}
}

func (m *model) View() string {
	switch m.state {
	case viewSessionList:
		return m.chatView() + "\n" + m.sessionOverlayView()
	case viewHelp:
		return m.chatView() + "\n" + m.helpOverlayView()
	default:
		return m.chatView()
	}
}

func (m *model) chatView() string {
	if m.width == 0 {
		m.width = 100
	}
	if m.height == 0 {
		m.height = 24
	}

	var sb strings.Builder

	sb.WriteString(m.viewport.View())
	sb.WriteString("\n")

	var statusParts []string
	if m.currentSess != nil {
		title := sessionDisplayName(m.currentSess)
		if len([]rune(title)) > 18 {
			rs := []rune(title)
			title = string(rs[:18]) + "…"
		}
		statusParts = append(statusParts, fmt.Sprintf("📁 %s", title))
	}
	if m.streaming {
		statusParts = append(statusParts, "◉ thinking…")
	}
	if m.tools > 0 {
		statusParts = append(statusParts, fmt.Sprintf("🔧 %d", m.tools))
	}
	if m.tokens > 0 {
		statusParts = append(statusParts, fmt.Sprintf("⚡ %d", m.tokens))
	}
	if len(statusParts) > 0 {
		sb.WriteString(statusBarStyle.Render(strings.Join(statusParts, " │ ")) + "\n")
	}

	if m.err != nil {
		sb.WriteString(errStyle.Render("✗ "+m.err.Error()) + "\n")
	}

	if m.streaming {
		sb.WriteString(inputStyle.Render("⏳ streaming... (Ctrl+C to stop)"))
	} else {
		sb.WriteString(inputStyle.Render(m.input.View()))
	}

	return sb.String()
}

func (m *model) sessionOverlayView() string {
	content := m.sessionListView()
	overlay := lipgloss.NewStyle().
		Width(m.width-10).
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("#6C8EBF")).
		Padding(0, 1).
		Render(content)

	return strings.Repeat("\n", 2) + overlay
}

func (m *model) sessionListView() string {
	var sb strings.Builder
	sb.WriteString(sessionTitleStyle.Render("Sessions"))
	sb.WriteString("\n\n")

	if len(m.sessions) == 0 {
		sb.WriteString("  No sessions. Press 'n' to create one.")
	} else {
		for i, s := range m.sessions {
			var prefix string
			if i == m.selSession {
				prefix = "▸ "
			} else {
				prefix = "  "
			}
			title := sessionDisplayName(s)
			if len([]rune(title)) > 28 {
				rs := []rune(title)
				title = string(rs[:28]) + "…"
			}
			line := fmt.Sprintf("%s%s  [%d msgs] %s",
				prefix,
				title,
				len(s.Messages),
				s.UpdatedAt.Format("15:04:05"),
			)
			if i == m.selSession {
				sb.WriteString(selStyle.Render(line) + "\n")
			} else {
				sb.WriteString("  " + line + "\n")
			}
		}
	}

	sb.WriteString("\n")
	if m.deleteTargetID != "" {
		sb.WriteString(warnStyle.Render("Press d again to delete this session") + "\n")
	} else {
		sb.WriteString(helpKeyStyle.Render("↑/↓ navigate · Enter select · n new · d delete · esc/q back") + "\n")
	}
	return sb.String()
}

func (m *model) helpOverlayView() string {
	help := lipgloss.NewStyle().
		Width(m.width-10).
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("#D4A859")).
		Padding(0, 1).
		Render(m.helpText())
	return strings.Repeat("\n", 2) + help
}

func (m *model) helpText() string {
	return fmt.Sprintf(`%s

  Keybindings:
  %s
  Ctrl+C / Ctrl+D    Quit (or stop streaming)
  Ctrl+S             Open session list
  F1 / Ctrl+H        Show this help
  ↑ / ↓              Input history
  PgUp / PgDown      Scroll chat

  Commands:
  %s
  /help              Show this help
  /clear             Clear chat messages
  /new               Create new session
  /sessions          Open session list

  %s
  Type a message and press Enter to chat.
  Press ↑ to recall previous messages.`,
		helpTitleStyle.Render("📖 Help"),
		helpSectionStyle.Render("Chat"),
		helpSectionStyle.Render("Commands"),
		helpSectionStyle.Render("Tips"))
}

func (m *model) renderViewport() {
	var sb strings.Builder
	for i, msg := range m.messages {
		if i > 0 && m.messages[i-1].role != msg.role {
			sb.WriteString(separatorStyle.Render("─") + "\n")
		}
		if msg.role == "user" {
			sb.WriteString(userStyle.Render("▸ " + msg.content))
		} else {
			sb.WriteString(msg.content)
		}
		sb.WriteString("\n")
	}

	if m.aiBuf.Len() > 0 {
		sb.WriteString(aiStyle.Render(m.aiBuf.String()))
	}
	if m.thinkBuf.Len() > 0 {
		sb.WriteString(thinkStyle.Render("[think] " + trunc(m.thinkBuf.String(), 500)))
	}

	m.viewport.SetContent(sb.String())
}

func doStream(p *tea.Program, app *infra.Application, sessionID, query string) {
	ctx := context.Background()
	if sessionID == "" {
		sess, err := app.Orchestrator.CreateSession(ctx, "default", "cli-user")
		if err != nil {
			p.Send(chunkMsg{err: err, done: true})
			return
		}
		sessionID = sess.ID
		p.Send(sessionCreatedMsg{session: sess})
	}

	stream, err := app.Orchestrator.ProcessQueryStream(ctx, "cli-user", "default", query, kernel.QueryOptions{})
	if err != nil {
		p.Send(chunkMsg{err: err, done: true})
		return
	}

	totalTools := 0
	totalTokens := 0

	for chunk := range stream {
		if chunk.Error != nil {
			p.Send(chunkMsg{err: chunk.Error, done: true})
			return
		}
		if chunk.Done {
			if chunk.Usage != nil {
				totalTokens = chunk.Usage.TotalTokens
			}
			p.Send(chunkMsg{done: true, tokens: totalTokens, toolCnt: totalTools})
			return
		}
		if len(chunk.ToolCalls) > 0 {
			totalTools += len(chunk.ToolCalls)
		}
		p.Send(chunkMsg{content: chunk.Content, thinking: chunk.ReasoningContent})
	}
}

var (
	userStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#6C8EBF")).Bold(true)
	aiStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#82B74B"))
	thinkStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Italic(true)
	errStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B"))
	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#1a1a2e")).
			Foreground(lipgloss.Color("#AAAAAA")).
			Padding(0, 1)
	inputStyle        = lipgloss.NewStyle().PaddingLeft(1)
	sessionTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6C8EBF")).
				Bold(true).
				Underline(true)
	selStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A859")).Bold(true)
	helpKeyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	helpTitleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A859")).Bold(true)
	helpSectionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#6C8EBF")).Bold(true)
	separatorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#333333"))
	warnStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A859")).Bold(true)
)

// sessionDisplayName 获取会话的显示名称（标题优先，降级到 UUID）
func sessionDisplayName(s *kernel.Session) string {
	if s == nil {
		return ""
	}
	if s.Metadata != nil {
		if title, ok := s.Metadata["title"].(string); ok && title != "" {
			return title
		}
	}
	// fallback: truncated UUID
	id := s.ID
	if len(id) > 8 {
		id = id[:8] + "…"
	}
	return id
}

func trunc(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "…"
}

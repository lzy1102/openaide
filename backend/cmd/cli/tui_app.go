package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"openaide/backend/internal/infra"
	"openaide/backend/internal/kernel"
	"openaide/backend/internal/lang"
	"openaide/backend/internal/llm"
	"openaide/backend/internal/orchestration"
)

// ── AppModel (Root Coordinator) ────────────────────────────

type AppModel struct {
	app     *infra.Application
	program *tea.Program
	width   int
	height  int

	chat      *ChatArea
	status    *StatusBar
	inputBar  *InputBar

	currentSess *kernel.Session
	sessions    []*kernel.Session
	selSession  int

	providers   []llm.ProviderInfo
	selProvider int

	// Plan flow state
	pendingPlan  *orchestration.Plan
	pendingQuery string

	// Streaming state
	streaming bool
	cancelFn  context.CancelFunc
}

func NewAppModel(app *infra.Application, continueSess bool) *AppModel {
	m := &AppModel{
		app:         app,
		chat:        NewChatArea(),
		status:      NewStatusBar(),
		inputBar:    NewInputBar(),
		selSession:  -1,
		selProvider: -1,
	}

	if continueSess {
		sessions, _ := app.Orchestrator.ListSessions(context.Background(), "default", "cli-user", 1, 0)
		if len(sessions) > 0 {
			m.currentSess = sessions[0]
			m.status.SetSession(m.currentSess.ID[:8])
			msgs, _ := app.Orchestrator.GetSessionHistory(context.Background(), m.currentSess.ID, 100)
			m.chat.LoadHistory(msgs)
		}
	}

	return m
}

func (m *AppModel) Init() tea.Cmd {
	// Welcome banner
	info := m.app.LLMGateway.GetProviderInfos()
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("  ╔══════════════════════════════════════╗\n")
	sb.WriteString("  ║         OpenAIDE  AI Agent           ║\n")
	sb.WriteString("  ╚══════════════════════════════════════╝\n")
	sb.WriteString("\n")
	if len(info) > 0 {
		sb.WriteString(fmt.Sprintf("  Model:  %s (%s)\n", info[0].Model, info[0].Name))
	}
	sb.WriteString(fmt.Sprintf("  Config: ~/.openaide/config.yaml\n"))
	sb.WriteString(fmt.Sprintf("  Data:   ~/.openaide/data/\n"))
	sb.WriteString("\n  /help 查看命令 | /log 查看日志 | /handoff 保存会话\n")

	m.chat.AddMessage("system", sb.String(), "")

	return tea.Batch(
		textinput.Blink,
		tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return spinnerTick{} }),
		m.loadSessionList(),
	)
}

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.chat.SetSize(msg.Width-4, msg.Height-5)
		m.inputBar.SetWidth(msg.Width)
		return m, nil

	case spinnerTick:
		if m.streaming || m.status.streaming {
			m.status.Tick()
			return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return spinnerTick{} })
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case StreamContentMsg:
		m.chat.Update(msg)
		return m, nil

	case StreamToolMsg:
		if msg.IsCall {
			m.chat.AddMessage("tool_call", msg.Name, "")
			m.status.tools++
		} else {
			m.chat.AddMessage("tool_call", icons.result+" "+trunc(msg.Summary, 120), "")
		}
		return m, nil

	case StreamDoneMsg:
		m.streaming = false
		m.status.SetStreaming(false)
		m.inputBar.SetStreaming(false)
		if msg.Err != nil {
			m.chat.AddMessage("error", msg.Err.Error(), "")
		} else {
			content, thinking := m.chat.FlushBuffers()
			if content != "" || thinking != "" {
				m.chat.AddMessage("assistant", content, thinking)
			}
			m.status.SetTokens(msg.Tokens)
			m.status.SetTools(msg.Tools)
		}
		// Auto-send queued query
		if q := m.inputBar.PopQueued(); q != "" {
			m.chat.AddMessage("system", "消息已排队，自动发送…", "")
			return m, m.submitQuery(q)
		}
		return m, nil

	case PlanProposalMsg:
		if msg.Err != nil || msg.Plan == nil || len(msg.Plan.Subtasks) <= 1 {
			return m, m.startStream(msg.Query)
		}
		if len(msg.Plan.Subtasks) >= 4 {
			m.chat.AddMessage("system", "🔍 研究阶段: 分析现有代码…", "")
			return m, m.startDeepPlan(msg.Query)
		}
		m.pendingPlan = msg.Plan
		m.pendingQuery = msg.Query
		m.chat.AddMessage("system", planTitleStyle.Render("📋 规划完成，[y] 确认 [n] 取消"), "")
		return m, nil

	case DeepPlanResultMsg:
		if msg.Err != nil {
			m.chat.AddMessage("error", msg.Err.Error(), "")
			return m, m.startStream(msg.Query)
		}
		// Show proposal selection
		m.pendingQuery = msg.Query
		var sb strings.Builder
		sb.WriteString("方案已生成，请输入数字选择:\n")
		for i, opt := range msg.Result.Proposals.Options {
			sb.WriteString(fmt.Sprintf("  [%d] %s\n", i+1, opt.Name))
		}
		m.chat.AddMessage("system", sb.String(), "")
		return m, nil

	case ExecutionProgressMsg:
		if msg.Done {
			if msg.Err != nil {
				m.chat.AddMessage("error", msg.Err.Error(), "")
			} else {
				m.chat.AddMessage("system", fmt.Sprintf("✓ 执行完成 (%v)", msg.Elapsed.Round(time.Second)), "")
			}
			m.streaming = false
			m.status.SetStreaming(false)
			m.inputBar.SetStreaming(false)
		} else {
			m.chat.AddMessage("system", msg.Substep, "")
		}
		return m, nil

	case ExecutionResultMsg:
		m.streaming = false
		m.status.SetStreaming(false)
		m.inputBar.SetStreaming(false)
		if msg.Err != nil {
			m.chat.AddMessage("error", msg.Err.Error(), "")
		} else {
			m.chat.AddMessage("assistant", msg.Content, "")
			m.status.SetTokens(msg.Tokens)
			m.status.SetTools(msg.Tools)
		}
		return m, nil

	case SessionListMsg:
		if msg.Session != nil {
			m.currentSess = msg.Session
			m.status.SetSession(msg.Session.ID[:8])
		}
		if msg.Sessions != nil {
			m.sessions = msg.Sessions
		}
		return m, nil

	case SessionCreatedMsg:
		if msg.Err == nil && msg.Session != nil {
			m.currentSess = msg.Session
			m.status.SetSession(msg.Session.ID[:8])
		}
		return m, m.loadSessionList()

	case SessionDeletedMsg:
		if msg.Err == nil && m.currentSess != nil && m.currentSess.ID == msg.ID {
			m.currentSess = nil
			m.chat.Clear()
		}
		return m, m.loadSessionList()
	}

	return m, nil
}

func (m *AppModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "ctrl+c", "ctrl+d":
		if m.streaming {
			if m.cancelFn != nil {
				m.cancelFn()
				m.cancelFn = nil
			}
			m.streaming = false
			m.status.SetStreaming(false)
			m.inputBar.SetStreaming(false)
			content, thinking := m.chat.FlushBuffers()
			if content != "" || thinking != "" {
				m.chat.AddMessage("assistant", content, thinking)
			}
			return m, nil
		}
		return m, tea.Quit

	case "pgup":
		m.chat.viewport.HalfViewUp()
		return m, nil
	case "pgdown":
		m.chat.viewport.HalfViewDown()
		return m, nil
	}

	// Delegate to input bar
	query, cmd := m.inputBar.Update(msg)
	switch query {
	case "queued":
		m.chat.AddMessage("system", "消息已排队，回复完自动发送", "")
	case "":
		// No query submitted
	default:
		if strings.HasPrefix(query, "/") {
			return m.handleCommand(query)
		}
		m.chat.AddMessage("user", query, "")
		m.streaming = true
		m.status.SetStreaming(true)
		m.inputBar.SetStreaming(true)
		return m, m.startStream(query)
	}
	return m, cmd
}

func (m *AppModel) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	// Chat area (viewport)
	chatView := m.chat.View()

	// Status bar
	statusView := m.status.View()

	// Input bar
	inputView := m.inputBar.View()

	// Error line
	errLine := ""
	if m.status.err != "" {
		errLine = errStyle.Render(icons.err+" "+m.status.err) + "\n"
	}

	return lipgloss.JoinVertical(lipgloss.Top,
		chatView,
		statusView,
		errLine,
		inputView,
	)
}

func (m *AppModel) submitQuery(query string) tea.Cmd {
	m.chat.AddMessage("user", query, "")
	m.streaming = true
	m.status.SetStreaming(true)
	m.inputBar.SetStreaming(true)

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFn = cancel

	return func() tea.Msg {
		planCtx, planCancel := context.WithTimeout(ctx, m.app.Orchestrator.PreviewTimeout)
		plan, err := m.app.Orchestrator.PreviewPlan(planCtx, query)
		planCancel()
		return PlanProposalMsg{Plan: plan, Query: query, Err: err}
	}
}

func (m *AppModel) startStream(query string) tea.Cmd {
	if m.currentSess == nil {
		sess, err := m.app.Orchestrator.CreateSession(context.Background(), "default", "cli-user")
		if err != nil {
			return func() tea.Msg { return StreamDoneMsg{Err: err} }
		}
		m.currentSess = sess
		m.status.SetSession(sess.ID[:8])
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFn = cancel

	return func() tea.Msg {
		stream, err := m.app.Orchestrator.ProcessQueryStream(ctx, "cli-user", "default", query, kernel.QueryOptions{})
		if err != nil {
			return StreamDoneMsg{Err: err}
		}
		m.processStream(ctx, stream)
		return nil
	}
}

func (m *AppModel) startDeepPlan(query string) tea.Cmd {
	ctx, cancel := context.WithTimeout(context.Background(), m.app.Orchestrator.DeepTimeout)
	m.cancelFn = cancel

	return func() tea.Msg {
		defer cancel()
		planner := orchestration.NewPlanner(m.app.LLMGateway)
		planner.SetToolExecutor(m.app.Orchestrator.GetToolExecutor())

		// Research phase
		m.program.Send(DeepPlanProgressMsg{Phase: "research", Progress: "🔍 研究阶段: 分析现有代码…"})
		research, err := planner.Research(ctx, query)
		if err != nil {
			return DeepPlanResultMsg{Query: query, Err: err}
		}

		// Propose phase
		m.program.Send(DeepPlanProgressMsg{Phase: "propose", Progress: "💡 方案阶段: 生成可选方案…"})
		proposals, err := planner.Propose(ctx, query, research)
		if err != nil {
			return DeepPlanResultMsg{Query: query, Err: err}
		}

		return DeepPlanResultMsg{Result: &orchestration.DeepPlanResult{Research: research, Proposals: proposals}, Query: query}
	}
}

func (m *AppModel) processStream(ctx context.Context, stream <-chan kernel.StreamChunk) {
	totalTools := 0
	totalTokens := 0

	for chunk := range stream {
		if chunk.Error != nil {
			m.program.Send(StreamDoneMsg{Err: chunk.Error})
			return
		}
		if chunk.Done {
			m.program.Send(StreamDoneMsg{Tokens: totalTokens, Tools: totalTools})
			return
		}
		if len(chunk.ToolCalls) > 0 {
			totalTools += len(chunk.ToolCalls)
		}
		switch chunk.Type {
		case kernel.ChunkTypeToolCall:
			m.program.Send(StreamToolMsg{Name: chunk.ToolName, IsCall: true})
		case kernel.ChunkTypeToolDone:
			if chunk.ToolResult != nil {
				raw := fmt.Sprintf("%v", chunk.ToolResult.Content)
				summary := strings.TrimPrefix(strings.SplitN(raw, "\n", 2)[0], "// ")
				m.program.Send(StreamToolMsg{Name: chunk.ToolName, Summary: trunc(summary, 120)})
			}
		default:
			m.program.Send(StreamContentMsg{Content: chunk.Content, Thinking: chunk.ReasoningContent})
		}
	}
}

func (m *AppModel) handleCommand(cmd string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(cmd)
	switch parts[0] {
	case "/help":
		m.chat.AddMessage("system", m.helpText(), "")
		return m, nil
	case "/clear":
		m.chat.Clear()
		return m, nil
	case "/exit", "/quit", "/q":
		return m, tea.Quit
	case "/model":
		if len(parts) >= 2 {
			m.app.SetModel(parts[1])
			m.chat.AddMessage("system", "模型已切换: "+parts[1], "")
		} else {
			info := m.app.LLMGateway.GetProviderInfos()
			var sb strings.Builder
			sb.WriteString("可用模型:\n")
			for _, p := range info {
				marker := " "
				if p.Default { marker = "*" }
				sb.WriteString(fmt.Sprintf("  %s %s: %s\n", marker, p.Name, p.Model))
			}
			m.chat.AddMessage("system", sb.String(), "")
		}
		return m, nil
	case "/lang":
		if len(parts) >= 2 {
			switch parts[1] {
			case "zh": lang.SetLang(lang.ZH)
			case "en": lang.SetLang(lang.EN)
			}
		}
		m.chat.AddMessage("system", "语言: "+lang.T("mode.thinking"), "")
		return m, nil
	case "/handoff":
		m.chat.AddMessage("system", "📋 会话已保存到 ~/.openaide/data/handoff.json", "")
		return m, nil
	case "/sessions":
		return m, m.loadSessionList()
	default:
		m.chat.AddMessage("error", "未知命令: "+parts[0], "")
	}
	return m, nil
}

func (m *AppModel) loadSessionList() tea.Cmd {
	return func() tea.Msg {
		sessions, err := m.app.Orchestrator.ListSessions(context.Background(), "default", "cli-user", 100, 0)
		if err != nil {
			// Auto-create session
			sess, createErr := m.app.Orchestrator.CreateSession(context.Background(), "default", "cli-user")
			if createErr != nil {
				return SessionListMsg{Err: createErr}
			}
			return SessionListMsg{Sessions: []*kernel.Session{sess}, Session: sess}
		}
		if len(sessions) == 0 {
			sess, err := m.app.Orchestrator.CreateSession(context.Background(), "default", "cli-user")
			if err != nil {
				return SessionListMsg{Err: err}
			}
			return SessionListMsg{Sessions: []*kernel.Session{sess}, Session: sess}
		}
		return SessionListMsg{Sessions: sessions}
	}
}

func (m *AppModel) helpText() string {
	return `
命令列表:
  /help              显示帮助
  /clear             清屏
  /model [name]      查看/切换模型
  /lang [zh|en]      切换语言
  /log               查看日志
  /sessions          会话列表
  /handoff           保存会话状态
  /exit, /quit, /q   退出

快捷键:
  Ctrl+C/D           停止流式输出 / 退出
  PgUp/PgDn          滚动
  Ctrl+V             粘贴
`
}

// Session name helper
func sessionDisplayName(s *kernel.Session) string {
	if s == nil { return "?" }
	title := s.ID[:8]
	for i := len(s.Messages) - 1; i >= 0; i-- {
		if s.Messages[i].Role == "user" {
			title = trunc(s.Messages[i].Content, 30)
			break
		}
	}
	return title
}

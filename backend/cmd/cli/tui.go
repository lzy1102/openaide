package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/atotto/clipboard"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"openaide/backend/internal/infra"
	"openaide/backend/internal/kernel"
	"openaide/backend/internal/lang"
	"openaide/backend/internal/llm"
	"openaide/backend/internal/orchestration"
)

type ViewState int

const (
	viewChat ViewState = iota
	viewSessionList
	viewModelList
	viewHelp
	viewPlanConfirm
	viewProposalSelect
	viewLangList
	viewLog
)

const maxHistory = 50
const maxSessions = 100

type chatMsg struct {
	role    string
	content string
}

type previewResultMsg struct {
	plan  *orchestration.Plan
	query string
	err   error
}

type chunkMsg struct {
	content   string
	thinking  string
	done      bool
	tokens    int
	toolCnt   int
	toolName  string // 正在调用的工具名
	toolCall  bool   // 是否为工具调用通知
	err       error
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

type langChoice struct {
	code, name string
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

	spinner int // 加载动画帧

	tokens         int
	tools          int
	err            error
	deleteTargetID string

	providers    []llm.ProviderInfo
	selProvider  int
	skillTrigger string // slash 命令触发的技能 ID
	cancelStream context.CancelFunc
	cancelMu     sync.Mutex

	pendingPlan  *orchestration.Plan // 待确认的任务规划
	pendingQuery string              // 待确认的查询
	queuedQuery  string              // 排队消息（当前回复完自动发送）
	deepResult   *orchestration.DeepPlanResult      // 深度规划结果
	proposalSel  int
	langChoices  []langChoice
	langSel      int
	logBuf       []string                                 // 方案选择游标
}

// LogRing TUI 日志环形缓冲区
type logRing struct {
	mu  sync.Mutex
	buf []string
}

func (r *logRing) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, strings.TrimSpace(string(p)))
	if len(r.buf) > 50 { r.buf = r.buf[1:] }
	return len(p), nil
}

var tuiLogBuf = &logRing{buf: make([]string, 0, 50)}


func initModel(app *infra.Application, continueSess bool) *model {
	ti := textinput.New()
	ti.Placeholder = lang.T("tui.placeholder")
	ti.Prompt = "❯ "
	ti.Focus()
	ti.CharLimit = 0 // 不限制输入长度，粘贴不受限
	ti.Width = 60

	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#333333"))

	m := &model{
		logBuf: tuiLogBuf.buf,
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

type spinnerTick struct{}

func (m *model) Init() tea.Cmd {
	// 欢迎横幅
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
	sb.WriteString(fmt.Sprintf("  Data:   ./data/\n"))
	prompt, _ := os.ReadFile(filepath.Join("./data/prompts/system.md"))
	if prompt == nil {
		prompt, _ = os.ReadFile(filepath.Join("./data/prompts/system.zh.md"))
	}
	if prompt == nil {
		prompt, _ = os.ReadFile(filepath.Join("./data/prompts/system.en.md"))
	}
	if len(prompt) > 0 {
		sb.WriteString(fmt.Sprintf("  Prompt: %.50s...\n", string(prompt)))
	}
	sb.WriteString("\n  /help 查看命令 | /log 查看日志\n")
	m.messages = append(m.messages, chatMsg{role: "system", content: sb.String()})
	m.renderViewport()
	return tea.Batch(
		textinput.Blink,
		m.loadSessionList(),
		tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return spinnerTick{} }),
	)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case spinnerTick:
		m.spinner = (m.spinner + 1) % 10
		if m.streaming || m.state == viewProposalSelect {
			return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return spinnerTick{} })
		}

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
		case viewModelList:
		case viewLangList:
			return m.updateLangList(msg)
			return m.updateModelList(msg)
		case viewHelp:
			m.state = viewChat
			m.input.Focus()
			return m, nil
		case viewProposalSelect:
			switch msg.String() {
			case "1", "2", "3":
				if m.deepResult != nil {
					idx := int(msg.String()[0] - '1')
					if idx < len(m.deepResult.Proposals.Options) {
						m.addSystemMsg("正在生成详细计划…")
						m.renderViewport()
						go m.doDeepPlanFinalize(idx)
					}
				}
			case "esc", "ctrl+c":
				m.state = viewChat
				m.deepResult = nil
				m.input.Focus()
				m.addSystemMsg("深度分析已取消")
				m.renderViewport()
				m.startStream(m.pendingQuery)
				m.pendingQuery = ""
			}
			return m, nil
		case viewPlanConfirm:
			switch msg.String() {
			case "y":
				m.state = viewChat
				m.addSystemMsg("规划已批准，开始执行…")
				m.renderViewport()
				go m.executePlan(m.pendingQuery, m.pendingPlan)
				m.pendingPlan = nil
				m.pendingQuery = ""
			case "d":
				m.addSystemMsg("正在深度分析…")
				m.renderViewport()
				go m.doDeepPlan(m.pendingQuery)
			case "n", "esc", "ctrl+c":
				m.state = viewChat
				q := m.pendingQuery
				m.pendingPlan = nil
				m.pendingQuery = ""
				m.input.Focus()
				m.addSystemMsg("规划已取消，切换为直接执行")
				m.renderViewport()
				m.startStream(q)
			}
			return m, nil
		case viewChat:
			return m.updateChat(msg)
		}

	case previewResultMsg:
		m.streaming = false
		if msg.err != nil || msg.plan == nil || len(msg.plan.Subtasks) <= 1 {
			m.startStream(msg.query)
			return m, nil
		}
		if len(msg.plan.Subtasks) >= 4 {
			m.pendingQuery = msg.query
			m.addSystemMsg("🔍 研究阶段: 分析现有代码…")
			m.renderViewport()
			go m.doDeepPlan(msg.query)
		} else {
			m.pendingPlan = msg.plan
			m.pendingQuery = msg.query
			m.state = viewPlanConfirm
		m.input.Blur()
			m.renderViewport()
		}

	case chunkMsg:
		m.cancelStream = nil
		if msg.err != nil {
			m.err = msg.err
			m.streaming = false
			m.input.Focus()
			m.renderViewport()
			return m, nil
		}
		if msg.toolCall {
			m.messages = append(m.messages, chatMsg{role: "tool_call", content: msg.toolName})
			m.renderViewport()
			m.viewport.GotoBottom()
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
		// 自动发送排队消息
		if m.queuedQuery != "" {
			q := m.queuedQuery
			m.queuedQuery = ""
			m.startStream(q)
		}
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
			if m.cancelStream != nil {
				m.cancelStream()
				m.cancelStream = nil
			}
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

	case "ctrl+v":
		if !m.streaming {
			text := readClipboard()
			if text != "" {
				m.input.SetValue(m.input.Value() + text)
			}
		}
		return m, nil

	case "f1", "ctrl+h":
		if !m.streaming {
			m.state = viewHelp
		m.input.Blur()
			return m, nil
		}

	case "enter":
		query := strings.TrimSpace(m.input.Value())
		if m.streaming {
			// 排队：当前回复完自动发送
			if query != "" {
				m.queuedQuery = query
				m.addSystemMsg("消息已排队，回复完自动发送")
				m.input.SetValue("")
				m.renderViewport()
			}
			return m, nil
		}
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

		// ≤3 字跳过规划（如 "hi"）

		// 自适应规划深度：LLM 快速分类任务复杂度
		// PreviewPlan 一次 LLM 调用同时判断复杂度+拆分
		m.streaming = true
		m.addSystemMsg("分析任务中…")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), m.app.Orchestrator.PreviewTimeout)
			plan, err := m.app.Orchestrator.PreviewPlan(ctx, query)
			cancel()
			m.program.Send(previewResultMsg{plan: plan, query: query, err: err})
		}()
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

func (m *model) addSystemMsg(content string) {
	m.messages = append(m.messages, chatMsg{role: "system", content: content})
	m.renderViewport()
	m.viewport.GotoBottom()
}

func (m *model) addErrorMsg(content string) {
	m.messages = append(m.messages, chatMsg{role: "error", content: content})
	m.renderViewport()
	m.viewport.GotoBottom()
}


// runSingleRole 使用单个团队角色执行任务
func (m *model) runSingleRole(role, task string) {
	m.streaming = true
	m.thinkBuf.Reset()
	m.aiBuf.Reset()
	m.err = nil
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelMu.Lock()
	m.cancelStream = cancel
	m.cancelMu.Unlock()
	go func() {
		defer cancel()
		resp, err := m.app.Orchestrator.RunSubAgent(ctx, "cli-user", "default", role, task, nil)
		if err != nil {
			m.program.Send(chunkMsg{err: err, done: true})
			return
		}
		m.program.Send(chunkMsg{content: resp, done: true})
	}()
}

// runTeamChain 使用完整团队链执行任务 (analyst→coder→reviewer)
func (m *model) runTeamChain(task string) {
	m.streaming = true
	m.thinkBuf.Reset()
	m.aiBuf.Reset()
	m.err = nil
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelMu.Lock()
	m.cancelStream = cancel
	m.cancelMu.Unlock()
	go func() {
		defer cancel()
		// analyst 分析
		analysis, err := m.app.Orchestrator.RunSubAgent(ctx, "cli-user", "default", "analyst", task, nil)
		if err != nil {
			m.program.Send(chunkMsg{err: err, done: true})
			return
		}
		m.program.Send(chunkMsg{thinking: "[分析员] " + trunc(analysis, 200)})

		// coder 实现
		code, err := m.app.Orchestrator.RunSubAgent(ctx, "cli-user", "default", "coder", task, []string{analysis})
		if err != nil {
			m.program.Send(chunkMsg{err: err, done: true})
			return
		}
		m.program.Send(chunkMsg{thinking: "[程序员] " + trunc(code, 200)})

		// reviewer 审查
		review, err := m.app.Orchestrator.RunSubAgent(ctx, "cli-user", "default", "reviewer", "审查任务: "+task, []string{analysis, code})
		if err != nil {
			m.program.Send(chunkMsg{err: err, done: true})
			return
		}
		m.program.Send(chunkMsg{content: fmt.Sprintf("## 团队协作完成\n\n### 分析报告\n%s\n\n### 实现\n%s\n\n### 审查\n%s", analysis, code, review), done: true})
	}()
}

// readClipboard 多策略读系统剪贴板，不依赖外部工具
func readClipboard() string {
	// 1. clipboard 库
	if text, err := clipboard.ReadAll(); err == nil && text != "" {
		return text
	}
	// 2. WSL: powershell
	if out, err := execCmd("powershell.exe", "-Command", "Get-Clipboard"); err == nil && out != "" {
		return out
	}
	// 3. macOS
	if out, err := execCmd("pbpaste"); err == nil && out != "" {
		return out
	}
	// 4. Linux X11
	if out, err := execCmd("xclip", "-o", "-selection", "clipboard"); err == nil && out != "" {
		return out
	}
	// 5. Linux Wayland
	if out, err := execCmd("wl-paste"); err == nil && out != "" {
		return out
	}
	return ""
}

func execCmd(name string, args ...string) (string, error) {
	cmd := osexec.Command(name, args...)
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (m *model) startStream(query string) {
	m.streaming = true
	m.thinkBuf.Reset()
	m.aiBuf.Reset()
	m.err = nil
	sessionID := ""
	if m.currentSess != nil {
		sessionID = m.currentSess.ID
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelStream = cancel
	go doStream(ctx, m.program, m.app, sessionID, query)
}

func (m *model) executePlan(query string, plan *orchestration.Plan) {
	m.streaming = true
	m.thinkBuf.Reset()
	m.aiBuf.Reset()
	m.err = nil
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelStream = cancel
	go func() {
		defer cancel()
		resp, err := m.app.Orchestrator.ExecuteWithPlan(ctx, "cli-user", "default", query, plan, kernel.QueryOptions{})
		if err != nil {
			m.program.Send(chunkMsg{err: err, done: true})
			return
		}
		m.program.Send(chunkMsg{content: resp.Content, done: true, tokens: resp.TokensUsed, toolCnt: resp.ToolCalls})
	}()
}

func (m *model) updateLangList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.langSel > 0 { m.langSel-- }
	case "down", "j":
		if m.langSel < len(m.langChoices)-1 { m.langSel++ }
	case "enter":
		if m.langSel >= 0 && m.langSel < len(m.langChoices) {
			lc := m.langChoices[m.langSel]
			if lc.code == "zh" { lang.SetLang(lang.ZH) } else { lang.SetLang(lang.EN) }
			m.addSystemMsg(fmt.Sprintf("语言已切换: %s", lc.name))
		}
		m.state = viewChat
		m.input.Focus()
	case "esc", "q":
		m.state = viewChat
		m.input.Focus()
	}
	return m, nil
}

func (m *model) logView() string {
	var sb strings.Builder
	sb.WriteString(planTitleStyle.Render("Log / 日志"))
	sb.WriteString("\n\n")
	lines := m.logBuf
	start := 0
	if len(lines) > 20 { start = len(lines) - 20 }
	for i := start; i < len(lines); i++ {
		sb.WriteString(lines[i] + "\n")
	}
	if len(lines) == 0 {
		sb.WriteString("(no logs yet)")
	}
	sb.WriteString("\n" + planPromptStyle.Render("任意键返回"))
	overlay := lipgloss.NewStyle().Width(m.width-10).Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("#888888")).Padding(0,1).Render(sb.String())
	return strings.Repeat("\n", 2) + overlay
}


func (m *model) langListView() string {
	var sb strings.Builder
	sb.WriteString(planTitleStyle.Render("Language / 语言"))
	sb.WriteString("\n\n")
	for i, lc := range m.langChoices {
		prefix := "  "
		if i == m.langSel { prefix = "▸ " }
		sb.WriteString(fmt.Sprintf("%s%s\n", prefix, lc.name))
	}
	sb.WriteString("\n" + planPromptStyle.Render("↑/↓ 选择  Enter 确认  esc 取消"))
	overlay := lipgloss.NewStyle().Width(m.width-10).Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("#6C8EBF")).Padding(0,1).Render(sb.String())
	return strings.Repeat("\n", 2) + overlay
}


func (m *model) planConfirmView() string {
	if m.pendingPlan == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(planTitleStyle.Render("📋 任务规划"))
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("目标: %s\n", m.pendingPlan.Goal))
	sb.WriteString(fmt.Sprintf("子任务: %d 个\n\n", len(m.pendingPlan.Subtasks)))
	for i, st := range m.pendingPlan.Subtasks {
		deps := ""
		if len(st.DependsOn) > 0 {
			deps = fmt.Sprintf(" (依赖: %v)", st.DependsOn)
		}
		sb.WriteString(fmt.Sprintf("  %d. %s%s\n", i+1, st.Title, deps))
		if st.ToolHints != "" {
			sb.WriteString(fmt.Sprintf("     工具: %s\n", st.ToolHints))
		}
	}
	sb.WriteString("\n")
	sb.WriteString(planPromptStyle.Render("[y] 确认执行  [n] 取消(直接执行)"))

	overlay := lipgloss.NewStyle().
		Width(m.width - 10).
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("#D4A859")).
		Padding(0, 1).
		Render(sb.String())
	return strings.Repeat("\n", 2) + overlay
}

func (m *model) doDeepPlan(query string) {
	ctx, cancel := context.WithTimeout(context.Background(), m.app.Orchestrator.DeepTimeout)
	defer cancel()

	// Phase 0: Research
	m.program.Send(func() tea.Msg { return chunkMsg{thinking: "🔍 研究阶段: 分析现有代码…"} })
	planner := orchestration.NewPlanner(m.app.LLMGateway)
	planner.SetToolExecutor(m.app.Orchestrator.GetToolExecutor())
	research, err := planner.Research(ctx, query)
	if err != nil {
		m.program.Send(func() tea.Msg { return chunkMsg{err: fmt.Errorf("研究阶段失败: %w", err), done: true} })
		return
	}
	m.program.Send(func() tea.Msg { return chunkMsg{thinking: "✓ 研究完成 | 复杂度: " + research.Complexity} })

	// Phase 1: Propose
	m.program.Send(func() tea.Msg { return chunkMsg{thinking: "💡 方案阶段: 生成可选方案…"} })
	proposals, err := planner.Propose(ctx, query, research)
	if err != nil {
		m.program.Send(func() tea.Msg { return chunkMsg{err: fmt.Errorf("方案阶段失败: %w", err), done: true} })
		return
	}

	m.deepResult = &orchestration.DeepPlanResult{Research: research, Proposals: proposals}
	m.state = viewProposalSelect
		m.input.Blur()
	m.proposalSel = 0
	m.program.Send(func() tea.Msg { return chunkMsg{thinking: "✓ 方案已生成，请选择（1/2/3）"} })
	m.streaming = false
}

func (m *model) doDeepPlanFinalize(idx int) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	m.program.Send(func() tea.Msg { return chunkMsg{thinking: "📋 计划阶段: 生成详细任务计划…"} })
	plan, err := m.app.Orchestrator.DeepPlanFinalize(ctx, m.pendingQuery, m.deepResult, idx)
	if err != nil {
		m.program.Send(func() tea.Msg { return chunkMsg{err: err, done: true} })
		return
	}

	// 用户已选择方案 = 已确认，直接执行
	m.state = viewChat
	m.addSystemMsg("计划已生成，开始执行（程序员 → 测试 → 审查）")
	m.renderViewport()
	go m.executePlan(m.pendingQuery, plan)
	m.pendingPlan = nil
	m.pendingQuery = ""
	m.deepResult = nil
}

func (m *model) proposalSelectView() string {
	if m.deepResult == nil || m.deepResult.Proposals == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(planTitleStyle.Render("📋 方案选择"))
	sb.WriteString("\n\n")
	if m.deepResult.Research != nil {
		sb.WriteString(fmt.Sprintf("复杂度: %s | 模块: %s\n", m.deepResult.Research.Complexity, m.deepResult.Research.Modules))
		sb.WriteString(fmt.Sprintf("风险: %s\n\n", m.deepResult.Research.Risks))
	}
	for i, opt := range m.deepResult.Proposals.Options {
		marker := "  "
		if i == m.proposalSel {
			marker = "▸ "
		}
		sb.WriteString(fmt.Sprintf("%s[%d] %s\n", marker, i+1, opt.Name))
		sb.WriteString(fmt.Sprintf("    %s\n", opt.Description))
		if opt.Reasoning != "" {
			sb.WriteString(fmt.Sprintf("    💡 %s\n", opt.Reasoning))
		}
		sb.WriteString(fmt.Sprintf("    ✅ %s\n", opt.Pros))
		sb.WriteString(fmt.Sprintf("    ❌ %s\n", opt.Cons))
		sb.WriteString(fmt.Sprintf("    风险: %s | 工作量: %s\n\n", opt.Risk, opt.Effort))
	}
	sb.WriteString(planPromptStyle.Render("按 1/2/3 选择方案  [esc] 取消"))

	overlay := lipgloss.NewStyle().
		Width(m.width - 10).
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("#6C8EBF")).
		Padding(0, 1).
		Render(sb.String())
	return strings.Repeat("\n", 2) + overlay
}

func (m *model) handleCommand(cmd string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(cmd)
	switch parts[0] {
	case "/help":
		m.state = viewHelp
		m.input.Blur()
		return m, nil
	case "/log":
		m.state = viewLog
		m.input.Blur()
		m.logBuf = tuiLogBuf.buf
		m.input.SetValue("")
		return m, nil
	case "/clear":
		m.messages = nil
		m.tokens = 0
		m.tools = 0
		m.err = nil
		m.input.SetValue("")
		m.renderViewport()
		return m, nil
	case "/exit", "/quit", "/q":
		return m, tea.Quit
	case "/model":
		if len(parts) >= 2 {
			m.app.SetModel(parts[1])
			m.addSystemMsg(lang.T("tui.model_switched", parts[1]))
		} else {
			m.providers = m.app.LLMGateway.GetProviderInfos()
			m.selProvider = 0
			for i, p := range m.providers {
				if p.Default {
					m.selProvider = i
					break
				}
			}
			m.state = viewModelList
		m.input.Blur()
		}
		m.input.SetValue("")
		return m, nil
	case "/lang":
		m.langChoices = []langChoice{{"zh", "中文"}, {"en", "English"}}
		m.langSel = 0
		if lang.GetLang() == lang.ZH { m.langSel = 0 } else { m.langSel = 1 }
		m.state = viewLangList
		m.input.Blur()
		m.input.SetValue("")
		return m, nil
	case "/analyst", "/coder", "/reviewer", "/executor":
		role := strings.TrimPrefix(parts[0], "/")
		task := strings.TrimSpace(strings.TrimPrefix(cmd, parts[0]))
		if task == "" {
			m.addSystemMsg(fmt.Sprintf("用法: %s <任务描述>", parts[0]))
			m.input.SetValue("")
			return m, nil
		}
		m.addSystemMsg(fmt.Sprintf("调用 %s 角色执行任务…", role))
		m.renderViewport()
		go m.runSingleRole(role, task)
		m.input.SetValue("")
		return m, nil
	case "/team":
		task := strings.TrimSpace(strings.TrimPrefix(cmd, "/team"))
		if task == "" {
			m.addSystemMsg("用法: /team <任务描述>")
			m.input.SetValue("")
			return m, nil
		}
		m.addSystemMsg("启动团队协作: 分析员→程序员→审查员…")
		m.renderViewport()
		go m.runTeamChain(task)
		m.input.SetValue("")
		return m, nil
	default:
		// 检查是否是技能斜杠命令
		if slashCmds := m.app.Kernel.GetSlashCommands(); slashCmds != nil {
			if skillID, ok := slashCmds[parts[0]]; ok {
				m.addSystemMsg(fmt.Sprintf("已激活技能: %s", skillID))
				m.input.SetValue("")
				m.skillTrigger = skillID
				return m, nil
			}
		}
		m.err = fmt.Errorf("%s", lang.T("err.unknown_cmd", parts[0]))
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

func (m *model) updateModelList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.selProvider > 0 {
			m.selProvider--
		}
	case "down", "j":
		if m.selProvider < len(m.providers)-1 {
			m.selProvider++
		}
	case "enter":
		if m.selProvider >= 0 && m.selProvider < len(m.providers) {
			p := m.providers[m.selProvider]
			if err := m.app.LLMGateway.SetDefaultProvider(p.Name); err == nil {
				m.addSystemMsg(lang.T("tui.provider_switched", p.Name, p.Model))
			} else {
				m.addErrorMsg(lang.T("tui.switch_failed", err))
			}
		}
		m.state = viewChat
		m.input.Focus()
		m.providers = nil
	case "esc", "q":
		m.state = viewChat
		m.input.Focus()
		m.providers = nil
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
	case viewModelList:
		return m.chatView() + "\n" + m.modelOverlayView()
	case viewHelp:
		return m.chatView() + "\n" + m.helpOverlayView()
	case viewPlanConfirm:
		return m.chatView() + "\n" + m.planConfirmView()
	case viewLog:
		return m.chatView() + "\n" + m.logView()
	case viewLangList:
		return m.chatView() + "\n" + m.langListView()
	case viewProposalSelect:
		return m.chatView() + "\n" + m.proposalSelectView()
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
		statusParts = append(statusParts, fmt.Sprintf(icons.folder+" %s", title))
	}
	spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	if m.streaming || m.state == viewProposalSelect {
		frame := spinnerFrames[m.spinner]
		statusParts = append(statusParts, frame+" "+lang.T("mode.thinking"))
	}
	if m.tools > 0 {
		statusParts = append(statusParts, fmt.Sprintf(icons.tools+" %d", m.tools))
	}
	if m.tokens > 0 {
		statusParts = append(statusParts, fmt.Sprintf(icons.tokens+" %d", m.tokens))
	}
	if len(statusParts) > 0 {
		sb.WriteString(statusBarStyle.Render(strings.Join(statusParts, " │ ")) + "\n")
	}

	if m.err != nil {
		sb.WriteString(errStyle.Render(icons.err+" "+m.err.Error()) + "\n")
	}

	if m.streaming {
		sb.WriteString(inputStyle.Render(icons.busy+" " + lang.T("mode.streaming")))
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
		sb.WriteString("  " + lang.T("warn.no_sessions"))
	} else {
		for i, s := range m.sessions {
			var prefix string
			if i == m.selSession {
				prefix = icons.user+" "
			} else {
				prefix = "  "
			}
			title := sessionDisplayName(s)
			if len([]rune(title)) > 28 {
				rs := []rune(title)
				title = string(rs[:28]) + "…"
			}
			line := lang.T("sess.tui_row",
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
		sb.WriteString(warnStyle.Render(lang.T("warn.delete_confirm")) + "\n")
	} else {
		sb.WriteString(helpKeyStyle.Render(lang.T("warn.nav_help")) + "\n")
	}
	return sb.String()
}

func (m *model) modelOverlayView() string {
	content := m.modelListView()
	overlay := lipgloss.NewStyle().
		Width(m.width - 10).
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("#D4A859")).
		Padding(0, 1).
		Render(content)
	return strings.Repeat("\n", 2) + overlay
}

func (m *model) modelListView() string {
	var sb strings.Builder
	sb.WriteString(sessionTitleStyle.Render(lang.T("model.title")))
	sb.WriteString("\n\n")

	if len(m.providers) == 0 {
		sb.WriteString("  " + lang.T("tui.no_providers"))
	} else {
		for i, p := range m.providers {
			prefix := "  "
			if i == m.selProvider {
				prefix = icons.user+" "
			}
			def := ""
			if p.Default {
				def = " " + lang.T("model.default")
			}
			line := fmt.Sprintf("%s%s%s — %s", prefix, p.Name, def, p.Model)
			if i == m.selProvider {
				sb.WriteString(selStyle.Render(line) + "\n")
			} else {
				sb.WriteString("  " + line + "\n")
			}
		}
	}

	sb.WriteString("\n")
	sb.WriteString(helpKeyStyle.Render(lang.T("warn.model_help")) + "\n")
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

  %s
  Ctrl+C / Ctrl+D    %s
  Ctrl+S             %s
  F1 / Ctrl+H        %s
  ↑ / ↓              %s
  PgUp / PgDown      %s

  %s
  /help              %s
  /clear             %s
  /model [name]      %s

  %s
  %s`,
		helpTitleStyle.Render(lang.T("help.title")),
		helpSectionStyle.Render(lang.T("help.keybindings")),
		lang.T("help.kb_quit"),
		lang.T("help.kb_sessions"),
		lang.T("help.kb_help"),
		lang.T("help.kb_history"),
		lang.T("help.kb_scroll"),
		helpSectionStyle.Render(lang.T("help.commands")),
		lang.T("help.cmd_help_desc"),
		lang.T("help.cmd_clear_desc"),
		lang.T("help.cmd_model_desc"),
		helpSectionStyle.Render(lang.T("help.tips")),
		lang.T("help.tips_text"),
	)
}

var lastRender time.Time

func (m *model) renderViewport() {
	// throttle during streaming to ~20fps
	if m.streaming && time.Since(lastRender) < 50*time.Millisecond {
		return
	}
	lastRender = time.Now()
	var sb strings.Builder
	// 只渲染最近 maxHistory 条消息
	start := 0
	if len(m.messages) > maxHistory {
		start = len(m.messages) - maxHistory
	}
	for i := start; i < len(m.messages); i++ {
		msg := m.messages[i]
		if i > start && m.messages[i-1].role != msg.role {
			sb.WriteString(separatorStyle.Render("─") + "\n")
		}
		switch msg.role {
		case "user":
			sb.WriteString(userStyle.Render(icons.user+" " + msg.content))
		case "error":
			sb.WriteString(errStyle.Render("✗ " + msg.content))
		case "system":
			sb.WriteString(sysStyle.Render(icons.system+" " + msg.content))
		case "tool_call":
			sb.WriteString(toolStyle.Render(icons.gear+" " + msg.content))
		case "tool":
			sb.WriteString(toolOutStyle.Render("  → " + trunc(msg.content, 200)))
		default:
			sb.WriteString(highlightCode(msg.content))
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

func doStream(ctx context.Context, p *tea.Program, app *infra.Application, sessionID, query string) {
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
		switch chunk.Type {
		case kernel.ChunkTypeToolCall:
			p.Send(chunkMsg{toolCall: true, toolName: chunk.ToolName})
		case kernel.ChunkTypeToolDone:
			if chunk.ToolResult != nil {
				summary := fmt.Sprintf("%v", chunk.ToolResult.Content)
				p.Send(chunkMsg{toolCall: true, toolName: icons.result+" " + trunc(summary, 200)})
			}
		default:
			p.Send(chunkMsg{content: chunk.Content, thinking: chunk.ReasoningContent})
		}
	}
}

// iconSet provides Unicode symbols with ASCII fallback for terminal compatibility.
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
type tuiTheme struct {
	user, ai, think, err, tool, toolOut, sys                lipgloss.Style
	statusBar, input, sessionTitle, sel, helpKey, helpTitle lipgloss.Style
	helpSection, separator, warn, codeBlock, planTitle, planPrompt  lipgloss.Style
}

var theme = tuiTheme{
	user:      lipgloss.NewStyle().Foreground(lipgloss.Color("#6C8EBF")).Bold(true),
	ai:        lipgloss.NewStyle().Foreground(lipgloss.Color("#82B74B")),
	think:     lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Italic(true),
	err:       lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")),
	tool:      lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A859")).Bold(true),
	toolOut:   lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")),
	sys:       lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA")).Italic(true),
	statusBar: lipgloss.NewStyle().Background(lipgloss.Color("#1a1a2e")).Foreground(lipgloss.Color("#AAAAAA")).Padding(0, 1),
	input:        lipgloss.NewStyle().PaddingLeft(1),
	sessionTitle: lipgloss.NewStyle().Foreground(lipgloss.Color("#6C8EBF")).Bold(true).Underline(true),
	sel:         lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A859")).Bold(true),
	helpKey:     lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")),
	helpTitle:   lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A859")).Bold(true),
	helpSection: lipgloss.NewStyle().Foreground(lipgloss.Color("#6C8EBF")).Bold(true),
	separator:   lipgloss.NewStyle().Foreground(lipgloss.Color("#333333")),
	warn:        lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A859")).Bold(true),
	codeBlock:   lipgloss.NewStyle().Background(lipgloss.Color("#1a1a2e")).Padding(0, 1),
	planTitle:   lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A859")).Bold(true),
	planPrompt:  lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA")),
}

// shorthand helpers for theme access
var (
	userStyle      = theme.user
	aiStyle        = theme.ai
	thinkStyle     = theme.think
	errStyle       = theme.err
	toolStyle      = theme.tool
	toolOutStyle   = theme.toolOut
	sysStyle       = theme.sys
	statusBarStyle = theme.statusBar
	inputStyle        = theme.input
	sessionTitleStyle = theme.sessionTitle
	selStyle         = theme.sel
	helpKeyStyle     = theme.helpKey
	helpTitleStyle   = theme.helpTitle
	helpSectionStyle = theme.helpSection
	separatorStyle   = theme.separator
	warnStyle        = theme.warn
	planTitleStyle   = theme.planTitle
	planPromptStyle  = theme.planPrompt
)


// highlightCode detects ```language blocks and applies syntax highlighting.
func highlightCode(text string) string {
	re := regexp.MustCompile("(?s)```(\\w*)\\n(.+?)\\n```")
	return re.ReplaceAllStringFunc(text, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) < 3 {
			return match
		}
		lang := parts[1]
		code := parts[2]
		if lang == "" {
			lang = "go"
		}
		return highlightBlock(code, lang)
	})
}

var chromaStyle *chroma.Style

func init() {
	chromaStyle = styles.Get("monokai")
	if chromaStyle == nil {
		chromaStyle = styles.Fallback
	}
}

func highlightBlock(code, lang string) string {
	formatter := formatters.TTY256
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)
	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}
	var buf bytes.Buffer
	if err := formatter.Format(&buf, chromaStyle, iterator); err != nil {
		return code
	}
	return theme.codeBlock.Render("\n" + strings.TrimRight(buf.String(), "\n") + "\n")
}

func sessionDisplayName(s *kernel.Session) string {
	if s == nil {
		return ""
	}
	if s.Metadata != nil {
		if title, ok := s.Metadata["title"].(string); ok && title != "" {
			return title
		}
	}
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

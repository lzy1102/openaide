package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"openaide/backend/internal/infra"
	"openaide/backend/internal/kernel"
	"openaide/backend/internal/lang"
)

// ── Styles ─────────────────────────────────────────────────

var (
	styleLogo      = lipgloss.NewStyle().Foreground(lipgloss.Color("36")).Bold(true)
	styleUser      = lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true)
	styleTool      = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleToolDone  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleThink     = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)
	styleError     = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	styleWarn      = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	styleSuccess   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleInfo      = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	stylePrompt    = lipgloss.NewStyle().Foreground(lipgloss.Color("34")).Bold(true)
	styleStatusBar = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleStreaming = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleIdle      = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleDim       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleSelected  = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	styleBox       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)

	// 驾驶舱样式（HUD / 仪表盘 / 侧翼）
	styleHudBg    = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Background(lipgloss.Color("236")).Padding(0, 1)
	styleHudModel = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	styleHudSess  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleHudGit   = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
	styleHudGitD  = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	styleGaugeVal = lipgloss.NewStyle().Foreground(lipgloss.Color("34")).Bold(true)
	styleGaugeLbl = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleSideBar  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1).Width(34)
	styleSideTtl  = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
)

// ── Modes ──────────────────────────────────────────────────

type tuiMode int

const (
	modeIdle      tuiMode = iota
	modeThinking          // PreviewPlan / DeepPlan 分析中
	modeStreaming         // ReAct 流式执行中
	modePlanExec          // ExecuteWithPlan 计划执行中
	modeSubAgent          // RunSubAgent 子代理执行中
	modeApproval          // 审批 overlay
	modeSelect            // 列表选择（sessions/models/方案）
	modeSearch            // Ctrl+R 历史搜索
)

// ── Shared auto-yes (Allow All) 跨 goroutine 安全 ──────────

type sharedAutoYes struct{ v atomic.Bool }

func (s *sharedAutoYes) Get() bool   { return s.v.Load() }
func (s *sharedAutoYes) Set(on bool) { s.v.Store(on) }

// ── 核心 Model ─────────────────────────────────────────────

type tuiModel struct {
	app      *infra.Application
	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model
	width    int
	height   int

	mode      tuiMode
	history   *strings.Builder // 已渲染输出（viewport 内容）
	statusMsg string           // 状态行辅助文本（plan 进度等）
	autoYes   *sharedAutoYes
	research  *sharedAutoYes // 并行研究开关(计划子任务只读研究)

	// 会话状态
	sessionID    string
	projectID    string
	modelName    string
	sessionTitle string
	gitBranch    string
	gitDirty     bool

	// 子状态（按生命周期内聚）
	stream   streamState    // 单次查询流式执行
	approval approvalState  // 审批桥接
	plan     planState      // 计划执行
	sub      subState       // 子代理执行
	selectS  selectionState // 选择模式
	searchS  searchState    // 历史搜索

	// 输入历史（textarea）
	cmdHistory []string
	cmdHistIdx int

	// banner 已完成（首次渲染时写入 viewport）
	bannerDone bool
}

// streamState 单次查询的流式执行状态
type streamState struct {
	streamCh     <-chan kernel.StreamChunk
	cancel       context.CancelFunc
	fullResponse string
	toolNames    []string
	thinkCount   int
	totalTokens  int
	cacheHit     int
	cacheMiss    int
	startTime    time.Time
	totalTools   int
	askQuestions []string // 工具产生的待回答问题
	streamRound  int
	streamTotal  int
}

// planState 计划执行状态
type planState struct {
	total      int
	current    int
	detail     string
	progressCh chan progressMsg
	resultCh   chan planExecMsg
	tasks      []taskState // 计划子任务实时状态
}

// approvalState 审批桥接状态
type approvalState struct {
	ch      chan approvalRequest
	pending *approvalRequest
}

// subState 子代理执行状态
type subState struct {
	role       string
	status     string // 子代理当前活动状态（thinking/工具/轮次）
	progressCh chan subProgressMsg
	resultCh   chan subAgentMsg
}

// selectionState 选择模式状态
type selectionState struct {
	items  []string
	idx    int
	title  string
	onPick func(int) tea.Cmd
}

// searchState 历史搜索模式状态
type searchState struct {
	results []string
	idx     int
}

// ── 输入键位 ───────────────────────────────────────────────

type tuiKeys struct {
	Send        key.Binding
	Newline     key.Binding
	Cancel      key.Binding
	Quit        key.Binding
	Editor      key.Binding
	Complete    key.Binding
	Search      key.Binding
	HistoryUp   key.Binding
	HistoryDown key.Binding
}

var tuiKeyMap = tuiKeys{
	Send:        key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send")),
	Newline:     key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("ctrl+j", "newline")),
	Cancel:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel/undo")),
	Quit:        key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "cancel/quit")),
	Editor:      key.NewBinding(key.WithKeys("alt+enter"), key.WithHelp("alt+enter", "editor")),
	Complete:    key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "complete")),
	Search:      key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "search history")),
	HistoryUp:   key.NewBinding(key.WithKeys("up")),
	HistoryDown: key.NewBinding(key.WithKeys("down")),
}

// ── File-backed History（textarea 输入历史，跨会话）────────

type fileHistory struct {
	items []string
	path  string
}

func newFileHistory(path string) *fileHistory {
	h := &fileHistory{path: path}
	data, _ := os.ReadFile(path)
	if len(data) > 0 {
		h.items = strings.Split(strings.TrimSpace(string(data)), "\n")
	}
	return h
}

func (h *fileHistory) Write(s string) {
	h.items = append(h.items, s)
	if len(h.items) > 1000 {
		h.items = h.items[len(h.items)-1000:]
	}
	os.WriteFile(h.path, []byte(strings.Join(h.items, "\n")), 0600)
}

func (h *fileHistory) Items() []string { return h.items }

// ── 入口：runTUI（替代 runREPL）────────────────────────────

func runTUI(app *infra.Application, continueSess, autoYes bool) {
	info := app.LLMGateway.GetProviderInfos()
	modelName := ""
	if len(info) > 0 {
		modelName = fmt.Sprintf("%s (%s)", info[0].Model, info[0].Name)
	}

	cwd, _ := os.Getwd()
	gitBranch := ""
	if out, err := execCmd("git", "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		gitBranch = strings.TrimSpace(out)
	}
	gitDirty := false
	if gitBranch != "" {
		if out, err := execCmd("git", "status", "--porcelain"); err == nil && len(strings.TrimSpace(out)) > 0 {
			gitDirty = true
		}
	}
	projectID := filepath.Base(cwd)

	m := initialTUI(app, autoYes)
	m.modelName = modelName
	m.projectID = projectID
	m.gitBranch = gitBranch
	m.gitDirty = gitDirty
	m.history.WriteString(buildBanner(modelName, gitBranch, cwd, Version))

	if continueSess {
		m.resumeSession(app)
	} else {
		if sess, err := app.Orchestrator.CreateSession(context.Background(), projectID, "cli-user"); err == nil {
			m.sessionID = sess.ID
		}
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("\n  ✗ TUI error: %v\n", err)
	}
}

func initialTUI(app *infra.Application, autoYes bool) tuiModel {
	ta := textarea.New()
	ta.Placeholder = lang.T("repl.placeholder")
	ta.ShowLineNumbers = false
	ta.SetHeight(3)
	ta.CharLimit = 0
	ta.KeyMap.InsertNewline = tuiKeyMap.Newline
	ta.Focus()

	vp := viewport.New(80, 20)
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styleStreaming

	ay := &sharedAutoYes{}
	ay.Set(autoYes)

	rs := &sharedAutoYes{}
	rs.Set(false) // 默认关闭:并行研究会增加额外 LLM 调用与上下文噪音

	return tuiModel{
		app:      app,
		viewport: vp,
		textarea: ta,
		spinner:  sp,
		history:  &strings.Builder{},
		autoYes:  ay,
		research: rs,
		approval: approvalState{ch: make(chan approvalRequest, 8)},
		plan: planState{
			progressCh: make(chan progressMsg, 16),
			resultCh:   make(chan planExecMsg, 1),
		},
		cmdHistory: []string{},
	}
}

func buildBanner(modelName, gitBranch, cwd, version string) string {
	var sb strings.Builder
	var left []string
	left = append(left, styleLogo.Render("◆ OpenAIDE"))
	if modelName != "" {
		left = append(left, styleSuccess.Render(modelName))
	} else {
		left = append(left, styleWarn.Render("⚠ "+lang.T("repl.no_api_key")))
	}
	if gitBranch != "" {
		left = append(left, styleInfo.Render("◆ "+gitBranch))
	}
	left = append(left, styleInfo.Render("◆ "+filepath.Base(cwd)))
	sb.WriteString(strings.Join(left, "  ") + "  " + styleInfo.Render(version) + "\n")
	if _, err := os.Stat(filepath.Join(cwd, "OPENAIDE.md")); err == nil {
		sb.WriteString("  " + styleSuccess.Render("📋 "+lang.T("repl.openaide_loaded")) + "\n")
	}
	sb.WriteString(styleDim.Render("  "+lang.T("repl.banner_hint")) + "\n")
	sb.WriteString(styleDim.Render("────────────────────────────────────────────") + "\n\n")
	return sb.String()
}

func (m *tuiModel) resumeSession(app *infra.Application) {
	sessions, _ := app.Orchestrator.ListSessions(context.Background(), m.projectID, "cli-user", 10, 0)
	for _, s := range sessions {
		if len(s.Messages) > 0 {
			m.sessionID = s.ID
			msgCount := len(s.Messages)
			m.history.WriteString(lang.T("repl.resume_row",
				styleSuccess.Render(lang.T("repl.resume")), "", s.ID[:8], msgCount))
			history, _ := app.Orchestrator.GetSessionHistory(context.Background(), s.ID, 3)
			if len(history) > 0 {
				m.history.WriteString("  " + styleInfo.Render(lang.T("repl.recent")+":") + "\n")
				for _, msg := range history {
					switch msg.Role {
					case "user":
						m.history.WriteString("    " + styleUser.Render("▸ "+trunc(oneLine(msg.Content), 80)) + "\n")
					case "assistant":
						m.history.WriteString("    " + styleToolDone.Render("✓ "+trunc(oneLine(msg.Content), 80)) + "\n")
					}
				}
				m.history.WriteString("\n")
			}
			return
		}
	}
	if m.sessionID == "" {
		m.history.WriteString("  " + styleInfo.Render(lang.T("repl.no_sessions_new")) + "\n\n")
		sess, _ := app.Orchestrator.CreateSession(context.Background(), m.projectID, "cli-user")
		if sess != nil {
			m.sessionID = sess.ID
		}
	}
}

// ── Init ───────────────────────────────────────────────────

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.spinner.Tick,
		waitForApproval(m.approval.ch),
	)
}

// ── Update ─────────────────────────────────────────────────

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.textarea.SetWidth(msg.Width)
		m.layoutViewport()
		m.refreshViewport()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case approvalReqMsg:
		// 审批请求 → 进入 approval 模式；同时启动超时兜底，防止挂起无人响应时内核回调永久阻塞
		m.approval.pending = &msg.req
		m.mode = modeApproval
		m.textarea.Blur()
		cmds = append(cmds, waitForApproval(m.approval.ch))
		cmds = append(cmds, tea.Tick(approvalTimeout, func(time.Time) tea.Msg {
			return approvalTimeoutMsg{}
		}))
		return m, tea.Batch(cmds...)

	case approvalTimeoutMsg:
		// 审批超时自动拒绝：恢复流式，避免内核回调 goroutine 泄漏
		if m.approval.pending == nil {
			return m, nil
		}
		select {
		case m.approval.pending.resultCh <- false:
		default:
		}
		m.approval.pending = nil
		m.mode = modeStreaming
		m.appendHistory(styleWarn.Render(lang.T("repl.approval_timeout")) + "\n")
		if m.stream.streamCh != nil {
			return m, waitForChunk(m.stream.streamCh)
		}
		return m, nil

	case tea.KeyMsg:
		// 诊断日志:记录粘贴/控制字符/特殊键,用于排查输入异常(如终端转义泄漏)
		// 正常打字(单个可见字符)不记录,避免日志噪音
		if isSuspiciousKey(msg) {
			slog.Info("tui input", "key", strconv.Quote(msg.String()), "paste", msg.Paste, "mode", m.mode)
		}
		// 按模式分发按键
		switch m.mode {
		case modeApproval:
			return m.handleApprovalKey(msg)
		case modeSelect:
			return m.handleSelectKey(msg)
		case modeSearch:
			return m.handleSearchKey(msg)
		default:
			return m.handleMainKey(msg)
		}

	case streamMsg:
		return m.handleStreamChunk(msg)

	case streamReadyMsg:
		m.stream.streamCh = msg.ch
		m.stream.cancel = msg.cancel
		return m, waitForChunk(msg.ch)

	case planPreviewMsg:
		return m.handlePlanPreview(msg)

	case deepPlanMsg:
		return m.handleDeepPlan(msg)

	case finalizePlanMsg:
		return m.handleFinalizePlan(msg)

	case planExecMsg:
		return m.handlePlanExec(msg)

	case progressMsg:
		if m.mode != modePlanExec {
			return m, nil
		}
		m.plan.current++
		m.plan.detail = msg.detail
		m.applyProgress(msg.detail)
		m.statusMsg = fmt.Sprintf("%d/%d %s", m.plan.current, m.plan.total, msg.detail)
		return m, waitForProgress(m.plan.progressCh, m.plan.resultCh)

	case subProgressMsg:
		if m.mode != modeSubAgent {
			return m, nil
		}
		m.applySubProgress(msg)
		return m, waitForSubProgress(m.sub.progressCh, m.sub.resultCh)

	case subAgentMsg:
		role := msg.role
		if role == "" {
			role = m.sub.role
		}
		m.mode = modeIdle
		m.stream.cancel = nil
		m.sub.role = ""
		m.sub.status = ""
		if msg.err != nil {
			m.appendHistory(styleError.Render("✗ "+msg.err.Error()) + "\n")
		} else if msg.result != "" {
			m.appendHistory(RenderMarkdown(msg.result))
		}
		m.appendHistory(styleSuccess.Render(lang.T("repl.sub_done", role)) + "\n")
		m.layoutViewport()
		m.textarea.Focus()
		m.refreshViewport()
		return m, nil

	case initMsg:
		m.mode = modeIdle
		if msg.err != nil {
			m.appendHistory(styleError.Render("✗ "+msg.err.Error()) + "\n")
		} else {
			m.appendHistory(styleSuccess.Render(lang.T("repl.doc_generated", msg.size)) + "\n")
		}
		m.textarea.Focus()
		m.refreshViewport()
		return m, nil

	case sessionCmdMsg:
		// /sessions 或 /model 等选择完成后的回调已通过 selectOnPick 处理
		m.textarea.Focus()
		m.refreshViewport()
		return m, nil

	case editorDoneMsg:
		m.mode = modeIdle
		if msg.err != nil {
			m.appendHistory(styleError.Render(lang.T("repl.editor_err", msg.err.Error())) + "\n")
			return m, nil
		}
		if msg.content != "" {
			slog.Info("tui textarea set from editor", "content", strconv.Quote(msg.content))
			m.textarea.SetValue(msg.content)
			m.textarea.CursorEnd()
		}
		m.textarea.Focus()
		m.refreshViewport()
		return m, nil
	}

	// 透传给组件（idle 时 textarea 才接收输入）
	var cmd tea.Cmd
	if m.mode == modeIdle || m.mode == modeSearch {
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
	}
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// ── 主模式按键处理 ─────────────────────────────────────────

// isSuspiciousKey 判断按键是否需要记录诊断日志:
//   - 粘贴(可能携带转义序列)
//   - 控制字符(如 \x0c form feed,终端泄漏的典型特征)
//   - 特殊功能键(非单个可打印字符)
//
// 正常打字(单个可见字符)跳过,避免日志噪音。
func isSuspiciousKey(msg tea.KeyMsg) bool {
	if msg.Paste {
		return true
	}
	s := msg.String()
	runes := []rune(s)
	if len(runes) == 1 && runes[0] >= 0x20 && runes[0] != 0x7f {
		return false
	}
	return true
}

func (m tuiModel) handleMainKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Ctrl+C：流式/执行中 → 取消；idle → 退出
	if key.Matches(msg, tuiKeyMap.Quit) {
		if m.stream.cancel != nil && (m.mode == modeStreaming || m.mode == modePlanExec || m.mode == modeSubAgent || m.mode == modeThinking) {
			m.stream.cancel()
			m.stream.cancel = nil
			m.mode = modeIdle
			m.plan.tasks = nil
			m.layoutViewport()
			m.appendHistory(styleWarn.Render(lang.T("repl.interrupted")) + "\n")
			m.textarea.Focus()
			m.refreshViewport()
			return m, nil
		}
		m.appendHistory("\n" + styleInfo.Render(lang.T("repl.goodbye")) + "\n")
		return m, tea.Quit
	}

	// Esc：流式/执行中 → 取消；idle → undo last message pair
	if key.Matches(msg, tuiKeyMap.Cancel) {
		if m.stream.cancel != nil && (m.mode == modeStreaming || m.mode == modePlanExec || m.mode == modeSubAgent || m.mode == modeThinking) {
			m.stream.cancel()
			m.stream.cancel = nil
			m.mode = modeIdle
			m.plan.tasks = nil
			m.layoutViewport()
			m.appendHistory(styleWarn.Render(lang.T("repl.cancelled")) + "\n")
			m.textarea.Focus()
			m.refreshViewport()
			return m, nil
		}
		if m.mode == modeIdle {
			m.undoLastMessage()
			return m, nil
		}
		return m, nil
	}

	// 忙碌模式下只允许滚动（pgup/pgdn），禁止输入
	if m.mode != modeIdle {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	// idle：发送
	if key.Matches(msg, tuiKeyMap.Send) {
		return m.submitQuery()
	}

	// Alt+Enter → $EDITOR 多行编辑
	if key.Matches(msg, tuiKeyMap.Editor) {
		return m, m.openEditor()
	}

	// Tab 补全
	if key.Matches(msg, tuiKeyMap.Complete) {
		return m.handleTabComplete()
	}

	// Ctrl+R 历史搜索
	if key.Matches(msg, tuiKeyMap.Search) {
		m.mode = modeSearch
		m.searchS.results = reverseHistory(m.cmdHistory)
		m.searchS.idx = 0
		m.textarea.Blur()
		m.refreshViewport()
		return m, nil
	}

	// 上/下历史导航（单行输入时）
	if key.Matches(msg, tuiKeyMap.HistoryUp) && !strings.Contains(m.textarea.Value(), "\n") {
		if m.cmdHistIdx < len(m.cmdHistory) {
			m.cmdHistIdx++
			if m.cmdHistIdx <= len(m.cmdHistory) {
				v := m.cmdHistory[len(m.cmdHistory)-m.cmdHistIdx]
				slog.Info("tui textarea set from history", "content", strconv.Quote(v))
				m.textarea.SetValue(v)
				m.textarea.CursorEnd()
			}
		}
		return m, nil
	}
	if key.Matches(msg, tuiKeyMap.HistoryDown) && !strings.Contains(m.textarea.Value(), "\n") {
		if m.cmdHistIdx > 0 {
			m.cmdHistIdx--
			if m.cmdHistIdx == 0 {
				m.textarea.Reset()
			} else {
				v := m.cmdHistory[len(m.cmdHistory)-m.cmdHistIdx]
				slog.Info("tui textarea set from history", "content", strconv.Quote(v))
				m.textarea.SetValue(v)
				m.textarea.CursorEnd()
			}
		}
		return m, nil
	}

	// 其余键透传给组件
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

// ── 提交查询（智能路由）───────────────────────────────────

func (m tuiModel) submitQuery() (tea.Model, tea.Cmd) {
	query := strings.TrimSpace(m.textarea.Value())
	if query == "" {
		return m, nil
	}

	// @file 引用展开（异步避免阻塞，但展开是纯 IO，直接做）
	expanded, included := m.expandAtRefs(query)
	query = expanded

	m.textarea.Reset()
	m.cmdHistIdx = 0

	// 输入历史
	if len(m.cmdHistory) == 0 || m.cmdHistory[len(m.cmdHistory)-1] != query {
		m.cmdHistory = append(m.cmdHistory, query)
	}
	if h := newFileHistory(os.Getenv("HOME") + "/.openaide/history"); h != nil {
		h.Write(query)
	}

	// 会话标题
	if m.sessionTitle == "" {
		m.sessionTitle = trunc(query, 30)
	}

	// 展示用户消息
	m.appendHistory(" " + styleUser.Render("▎"+lang.T("repl.you_label")+" ") + styleUser.Render(query) + "\n\n")
	if included != "" {
		m.appendHistory(styleInfo.Render(included) + "\n")
	}

	// 斜杠命令
	if strings.HasPrefix(query, "/") {
		return m.handleTUICommand(query)
	}

	// 智能路由：PreviewPlan → direct / DeepPlan / plan
	m.mode = modeThinking
	m.statusMsg = lang.T("repl.analyzing")
	m.textarea.Blur()
	m.refreshViewport()

	return m, previewPlanCmd(m.app, query)
}

// ── 流式执行 ───────────────────────────────────────────────

// ── 计划执行 ───────────────────────────────────────────────

// ── Viewport / 渲染辅助 ───────────────────────────────────

// layoutViewport 根据当前模式动态计算 viewport 高度与宽度
// 高度 = 总高 - HUD(1) - 仪表盘(1) - 状态(1) - 帮助(1) - 输入框(4)
// 侧翼激活时宽度让出右侧面板
// ── 选择模式按键 ──────────────────────────────────────────

// ── 历史搜索模式按键 ─────────────────────────────────────

// ── 工具函数 ──────────────────────────────────────────────

func execCmd(name string, args ...string) (string, error) {
	cmd := osexec.Command(name, args...)
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// expandAtRefs 查找 @filename 引用并拼入文件内容
func (m tuiModel) expandAtRefs(query string) (string, string) {
	if !strings.Contains(query, "@") {
		return query, ""
	}
	var files []string
	words := strings.Fields(query)
	var sb strings.Builder
	var included strings.Builder
	replaced := make([]string, 0, len(words))
	for _, w := range words {
		if strings.HasPrefix(w, "@") && len(w) > 1 {
			pattern := strings.TrimPrefix(w, "@")
			matches, err := filepath.Glob(pattern)
			if err != nil || len(matches) == 0 {
				replaced = append(replaced, w)
				continue
			}
			for _, p := range matches {
				files = append(files, p)
				if fi, err := os.Stat(p); err == nil {
					included.WriteString(fmt.Sprintf("  @%s (%db)\n", p, fi.Size()))
				}
			}
		} else {
			replaced = append(replaced, w)
		}
	}
	if len(files) == 0 {
		return query, ""
	}
	for i, path := range files {
		if i >= 20 {
			sb.WriteString(fmt.Sprintf("... (%d more files)\n", len(files)-i))
			break
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		c := string(data)
		if len(c) > 5000 {
			c = c[:5000] + "\n... (truncated)"
		}
		sb.WriteString(fmt.Sprintf("Content of %s:\n---\n%s\n---\n\n", path, c))
	}
	sb.WriteString("User prompt: " + strings.Join(replaced, " "))
	return sb.String(), included.String()
}

// expandAtRefs 包级便捷入口（测试兼容），委托方法实现
func expandAtRefs(query string) string {
	var m tuiModel
	expanded, _ := m.expandAtRefs(query)
	return expanded
}

// openEditor Alt+Enter 用 $EDITOR 编辑
func (m tuiModel) openEditor() tea.Cmd {
	return func() tea.Msg {
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vim"
		}
		tmp, err := os.CreateTemp("", "openaide-*.md")
		if err != nil {
			return editorDoneMsg{err: err}
		}
		defer os.Remove(tmp.Name())
		tmp.WriteString(m.textarea.Value())
		tmp.Close()
		cmd := osexec.Command(editor, tmp.Name())
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return editorDoneMsg{err: err}
		}
		data, _ := os.ReadFile(tmp.Name())
		return editorDoneMsg{content: strings.TrimSpace(string(data))}
	}
}

// ── 辅助消息类型 ──────────────────────────────────────────

type editorDoneMsg struct {
	content string
	err     error
}

const (
	taskPending taskStatus = iota
	taskRunning
	taskDone
)

type initMsg struct {
	size int
	err  error
}

type sessionCmdMsg struct{}

// undoLastMessage 删除最后一对 user+assistant 消息
func (m tuiModel) undoLastMessage() {
	ctx := context.Background()
	session, err := m.app.Orchestrator.GetSession(ctx, m.sessionID)
	if err != nil || len(session.Messages) == 0 {
		return
	}
	lastUserIdx := -1
	for i := len(session.Messages) - 1; i >= 0; i-- {
		if session.Messages[i].Role == "user" {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx < 0 {
		return
	}
	removed := len(session.Messages) - lastUserIdx
	session.Messages = session.Messages[:lastUserIdx]
	if err := m.app.Orchestrator.UpdateSession(ctx, session); err != nil {
		return
	}
	m.appendHistory(styleWarn.Render(fmt.Sprintf("↩ Undone (%d messages removed)", removed)) + "\n")
}

// ── View ──────────────────────────────────────────────────

// hudView 渲染 HUD 顶栏：模式指示 + 模型 + 会话 + git 分支

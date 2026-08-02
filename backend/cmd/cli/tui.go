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
	"openaide/backend/internal/orchestration"
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

	// 会话状态
	sessionID    string
	projectID    string
	modelName    string
	sessionTitle string
	gitBranch    string
	gitDirty     bool

	// 流式状态
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

	// 审批桥接
	approvalCh      chan approvalRequest
	pendingApproval *approvalRequest

	// 计划执行
	planTotal    int
	planCurrent  int
	planDetail   string
	progressCh   chan progressMsg
	planResultCh chan planExecMsg
	tasks        []taskState // 计划子任务实时状态

	// 子代理
	subRole       string
	subStatus     string // 子代理当前活动状态（thinking/工具/轮次）
	subProgressCh chan subProgressMsg
	subResultCh   chan subAgentMsg

	// 流式轮次
	streamRound int
	streamTotal int

	// 选择模式
	selectItems  []string
	selectIdx    int
	selectTitle  string
	selectOnPick func(int) tea.Cmd

	// 历史搜索
	searchResults []string
	searchIdx     int

	// 输入历史（textarea）
	cmdHistory []string
	cmdHistIdx int

	// banner 已完成（首次渲染时写入 viewport）
	bannerDone bool
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

	return tuiModel{
		app:          app,
		viewport:     vp,
		textarea:     ta,
		spinner:      sp,
		history:      &strings.Builder{},
		autoYes:      ay,
		approvalCh:   make(chan approvalRequest, 8),
		progressCh:   make(chan progressMsg, 16),
		planResultCh: make(chan planExecMsg, 1),
		cmdHistory:   []string{},
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
			m.history.WriteString(fmt.Sprintf("  %s%s: %s (%d msgs)\n",
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
		waitForApproval(m.approvalCh),
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
		// 审批请求 → 进入 approval 模式
		m.pendingApproval = &msg.req
		m.mode = modeApproval
		m.textarea.Blur()
		cmds = append(cmds, waitForApproval(m.approvalCh))
		return m, tea.Batch(cmds...)

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
		m.streamCh = msg.ch
		m.cancel = msg.cancel
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
		m.planCurrent++
		m.planDetail = msg.detail
		m.applyProgress(msg.detail)
		m.statusMsg = fmt.Sprintf("%d/%d %s", m.planCurrent, m.planTotal, msg.detail)
		return m, waitForProgress(m.progressCh, m.planResultCh)

	case subProgressMsg:
		if m.mode != modeSubAgent {
			return m, nil
		}
		m.applySubProgress(msg)
		return m, waitForSubProgress(m.subProgressCh, m.subResultCh)

	case subAgentMsg:
		role := msg.role
		if role == "" {
			role = m.subRole
		}
		m.mode = modeIdle
		m.cancel = nil
		m.subRole = ""
		m.subStatus = ""
		if msg.err != nil {
			m.appendHistory(styleError.Render("✗ "+msg.err.Error()) + "\n")
		} else if msg.result != "" {
			m.appendHistory(RenderMarkdown(msg.result))
		}
		m.appendHistory(styleSuccess.Render(role+" done") + "\n")
		m.layoutViewport()
		m.textarea.Focus()
		m.refreshViewport()
		return m, nil

	case initMsg:
		m.mode = modeIdle
		if msg.err != nil {
			m.appendHistory(styleError.Render("✗ "+msg.err.Error()) + "\n")
		} else {
			m.appendHistory(styleSuccess.Render(fmt.Sprintf("OPENAIDE.md generated (%d chars) — will be loaded in future sessions", msg.size)) + "\n")
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
			m.appendHistory(styleError.Render("✗ editor: "+msg.err.Error()) + "\n")
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
		if m.cancel != nil && (m.mode == modeStreaming || m.mode == modePlanExec || m.mode == modeSubAgent || m.mode == modeThinking) {
			m.cancel()
			m.cancel = nil
			m.mode = modeIdle
			m.tasks = nil
			m.layoutViewport()
			m.appendHistory(styleWarn.Render("⚠ Interrupted") + "\n")
			m.textarea.Focus()
			m.refreshViewport()
			return m, nil
		}
		m.appendHistory("\n" + styleInfo.Render(lang.T("repl.goodbye")) + "\n")
		return m, tea.Quit
	}

	// Esc：流式/执行中 → 取消；idle → undo last message pair
	if key.Matches(msg, tuiKeyMap.Cancel) {
		if m.cancel != nil && (m.mode == modeStreaming || m.mode == modePlanExec || m.mode == modeSubAgent || m.mode == modeThinking) {
			m.cancel()
			m.cancel = nil
			m.mode = modeIdle
			m.tasks = nil
			m.layoutViewport()
			m.appendHistory(styleWarn.Render("⚠ Cancelled") + "\n")
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
		m.searchResults = reverseHistory(m.cmdHistory)
		m.searchIdx = 0
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

func previewPlanCmd(app *infra.Application, query string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), app.Orchestrator.PreviewTimeout)
		defer cancel()
		plan, err := app.Orchestrator.PreviewPlan(ctx, query)
		return planPreviewMsg{query: query, plan: plan, err: err}
	}
}

func (m tuiModel) handlePlanPreview(msg planPreviewMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil || msg.plan == nil || len(msg.plan.Subtasks) <= 3 {
		// 直接 ReAct 流式
		return m.startStream(msg.query)
	}
	if len(msg.plan.Subtasks) >= 6 {
		// DeepPlan：深度研究 + 方案对比
		m.appendHistory(styleInfo.Render(lang.T("repl.deep_analysis")) + "\n")
		m.statusMsg = lang.T("repl.deep_analysis")
		m.refreshViewport()
		return m, deepPlanCmd(m.app, msg.query)
	}
	// 显示规划并直接执行
	m.appendPlan(msg.plan)
	m.statusMsg = lang.T("repl.executing")
	m.refreshViewport()
	return m.startPlanExec(msg.query, msg.plan)
}

func deepPlanCmd(app *infra.Application, query string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), app.Orchestrator.DeepTimeout)
		defer cancel()
		result, err := app.Orchestrator.DeepPlan(ctx, query)
		return deepPlanMsg{query: query, result: result, err: err}
	}
}

func (m tuiModel) handleDeepPlan(msg deepPlanMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil || msg.result == nil || len(msg.result.Proposals.Options) == 0 {
		m.appendHistory(styleWarn.Render(lang.T("repl.deep_failed")) + "\n")
		plan, err := m.app.Orchestrator.PreviewPlan(context.Background(), msg.query)
		if err != nil || plan == nil {
			return m.startStream(msg.query)
		}
		m.appendPlan(plan)
		return m.startPlanExec(msg.query, plan)
	}

	// 交互式方案选择
	var options []string
	for _, opt := range msg.result.Proposals.Options {
		options = append(options, fmt.Sprintf("%s  "+lang.T("repl.risk_effort"), opt.Name, opt.Risk, opt.Effort))
	}
	m.mode = modeSelect
	m.selectTitle = lang.T("repl.select_approach")
	m.selectItems = options
	m.selectIdx = 0
	m.selectOnPick = func(idx int) tea.Cmd {
		name := msg.result.Proposals.Options[idx].Name
		m.mode = modeThinking
		m.statusMsg = lang.T("repl.selected", name)
		m.refreshViewport()
		return finalizePlanCmd(m.app, msg.query, msg.result, idx, name)
	}
	m.textarea.Blur()
	m.refreshViewport()
	return m, nil
}

func (m tuiModel) handleFinalizePlan(msg finalizePlanMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil || msg.plan == nil {
		m.appendHistory(styleWarn.Render(lang.T("repl.plan_failed")) + "\n")
		return m.startStream(msg.query)
	}
	m.appendHistory(styleInfo.Render("  "+lang.T("repl.selected", msg.name)) + "\n")
	m.appendPlan(msg.plan)
	return m.startPlanExec(msg.query, msg.plan)
}

func finalizePlanCmd(app *infra.Application, query string, result *orchestration.DeepPlanResult, idx int, name string) tea.Cmd {
	return func() tea.Msg {
		plan, err := app.Orchestrator.DeepPlanFinalize(context.Background(), query, result, idx)
		return finalizePlanMsg{query: query, plan: plan, name: name, err: err}
	}
}

// ── 流式执行 ───────────────────────────────────────────────

func (m tuiModel) startStream(query string) (tea.Model, tea.Cmd) {
	m.mode = modeStreaming
	m.statusMsg = ""
	m.thinkCount = 0
	m.toolNames = nil
	m.totalTokens = 0
	m.cacheHit = 0
	m.cacheMiss = 0
	m.totalTools = 0
	m.askQuestions = nil
	m.fullResponse = ""
	m.streamRound = 0
	m.streamTotal = 0
	m.startTime = time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.textarea.Blur()
	m.refreshViewport()

	opts := kernel.QueryOptions{
		OnApproval: func(tool, path, args string) bool {
			if m.autoYes.Get() {
				return true
			}
			ch := make(chan bool)
			select {
			case m.approvalCh <- approvalRequest{tool: tool, path: path, args: args, resultCh: ch}:
			case <-ctx.Done():
				return false
			}
			return <-ch
		},
		OnBudgetExhausted: func(round, maxRounds int) bool {
			if m.autoYes.Get() {
				return true
			}
			ch := make(chan bool)
			select {
			case m.approvalCh <- approvalRequest{isBudget: true, round: round, maxRounds: maxRounds, resultCh: ch}:
			case <-ctx.Done():
				return false
			}
			return <-ch
		},
	}

	return m, func() tea.Msg {
		stream, err := m.app.Orchestrator.ProcessQueryStream(ctx, "cli-user", m.projectID, query, opts)
		if err != nil {
			return streamMsg{chunk: kernel.StreamChunk{Type: kernel.ChunkTypeError, Error: err}}
		}
		return streamReadyMsg{ch: stream, ctx: ctx, cancel: cancel}
	}
}

type streamReadyMsg struct {
	ch     <-chan kernel.StreamChunk
	ctx    context.Context
	cancel context.CancelFunc
}

func (m tuiModel) handleStreamChunk(msg streamMsg) (tea.Model, tea.Cmd) {
	c := msg.chunk

	if c.Error != nil {
		m.appendHistory(styleError.Render("✗ "+c.Error.Error()) + "\n")
		return m.finishStream()
	}

	switch c.Type {
	case kernel.ChunkTypeContent, "":
		if c.Content != "" {
			m.fullResponse += c.Content
			m.refreshViewport()
		}
	case kernel.ChunkTypeThinking:
		if c.Round > 0 {
			m.streamRound = c.Round
			m.streamTotal = c.TotalRounds
		}
		if c.ReasoningContent != "" && m.thinkCount < 2 {
			first := strings.SplitN(c.ReasoningContent, "\n", 2)[0]
			if len(first) > 100 {
				first = first[:97] + "..."
			}
			m.appendHistory(styleThink.Render("[think] "+first) + "\n")
			m.thinkCount++
		}
	case kernel.ChunkTypeToolCall:
		if c.ToolName != "" {
			m.toolNames = append(m.toolNames, c.ToolName)
			m.totalTools++
			seen := map[string]bool{}
			var unique []string
			for _, n := range m.toolNames {
				if !seen[n] {
					seen[n] = true
					unique = append(unique, n)
				}
			}
			m.appendHistory(styleTool.Render("  🔧 "+strings.Join(unique, " → ")) + "\n")
		}
	case kernel.ChunkTypeToolDone:
		// 工具完成不单独打印
	case kernel.ChunkTypeProgress:
		// 多轮进度
	case kernel.ChunkTypeDone:
		if c.Usage != nil {
			m.totalTokens = c.Usage.TotalTokens
			m.cacheHit = c.Usage.PromptCacheHitTokens
			m.cacheMiss = c.Usage.PromptCacheMissTokens
		}
		return m.finishStream()
	}

	if c.Done {
		return m.finishStream()
	}
	return m, waitForChunk(m.streamCh)
}

func (m tuiModel) finishStream() (tea.Model, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.mode = modeIdle
	elapsed := time.Since(m.startTime)

	// 最终回答（markdown 渲染）
	if m.fullResponse != "" {
		m.appendHistory(" " + styleSuccess.Render("▎"+lang.T("repl.assistant_label")+" ") + "\n")
		m.appendHistory(RenderMarkdown(m.fullResponse) + "\n")
	}
	m.appendStatusBar(m.totalTokens, m.totalTools, elapsed, m.cacheHit, m.cacheMiss)

	if qs := m.app.ToolRegistry.GetPendingQuestions(); len(qs) > 0 {
		for _, q := range qs {
			m.appendHistory(styleWarn.Render("❓ "+q) + "\n")
		}
	}

	m.mode = modeIdle
	m.statusMsg = ""
	m.textarea.Focus()
	m.refreshViewport()
	return m, nil
}

// ── 计划执行 ───────────────────────────────────────────────

func (m tuiModel) startPlanExec(query string, plan *orchestration.Plan) (tuiModel, tea.Cmd) {
	m.mode = modePlanExec
	m.planTotal = len(plan.Subtasks) + 2
	m.planCurrent = 0
	m.planDetail = ""
	m.startTime = time.Now()
	m.tasks = make([]taskState, len(plan.Subtasks))
	for i, st := range plan.Subtasks {
		m.tasks[i] = taskState{id: st.ID, title: st.Title, status: taskPending}
	}
	m.layoutViewport()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	m.cancel = cancel
	m.textarea.Blur()
	m.refreshViewport()

	m.app.Orchestrator.OnProgress = func(phase, detail string) {
		select {
		case m.progressCh <- progressMsg{cur: 0, detail: detail}:
		default:
		}
	}

	return m, tea.Batch(
		func() tea.Msg {
			resp, err := m.app.Orchestrator.ExecuteWithPlan(ctx, "cli-user", m.projectID, query, plan, kernel.QueryOptions{})
			m.app.Orchestrator.OnProgress = nil
			return planExecMsg{resp: resp, err: err}
		},
		waitForProgress(m.progressCh, m.planResultCh),
	)
}

func (m tuiModel) handlePlanExec(msg planExecMsg) (tea.Model, tea.Cmd) {
	// 收尾：关闭进度通道，解除挂起的 waitForProgress
	close(m.progressCh)
	close(m.planResultCh)
	m.progressCh = make(chan progressMsg, 16)
	m.planResultCh = make(chan planExecMsg, 1)
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	elapsed := time.Since(m.startTime)

	if msg.err != nil {
		m.appendHistory(styleError.Render(fmt.Sprintf("✗ %v", msg.err)) + "\n")
	} else if msg.resp != nil {
		if msg.resp.Content != "" {
			m.appendHistory(RenderMarkdown(msg.resp.Content))
		}
		m.appendStatusBar(msg.resp.TokensUsed, msg.resp.ToolCalls, elapsed, msg.resp.CacheHit, msg.resp.CacheMiss)
	}
	if qs := m.app.ToolRegistry.GetPendingQuestions(); len(qs) > 0 {
		for _, q := range qs {
			m.appendHistory(styleWarn.Render("❓ "+q) + "\n")
		}
	}
	m.mode = modeIdle
	m.statusMsg = ""
	m.tasks = nil
	m.layoutViewport()
	m.textarea.Focus()
	m.refreshViewport()
	return m, nil
}

// ── Viewport / 渲染辅助 ───────────────────────────────────

// layoutViewport 根据当前模式动态计算 viewport 高度与宽度
// 高度 = 总高 - HUD(1) - 仪表盘(1) - 状态(1) - 帮助(1) - 输入框(4)
// 侧翼激活时宽度让出右侧面板
func (m *tuiModel) layoutViewport() {
	vpHeight := m.height - 8
	if vpHeight < 3 {
		vpHeight = 3
	}
	m.viewport.Height = vpHeight
	if m.sidePanel() != "" {
		m.viewport.Width = m.width - 36
		if m.viewport.Width < 40 {
			m.viewport.Width = m.width
		}
	} else {
		m.viewport.Width = m.width
	}
}

func (m tuiModel) planPanelContent() string {
	if len(m.tasks) == 0 {
		return ""
	}
	done := 0
	for _, t := range m.tasks {
		if t.status == taskDone {
			done++
		}
	}
	var sb strings.Builder
	sb.WriteString(stylePrompt.Render(fmt.Sprintf("%s %d/%d", lang.T("repl.task_progress"), done, len(m.tasks))) + "\n")
	shown := 0
	for _, t := range m.tasks {
		if shown >= 6 {
			sb.WriteString(styleDim.Render(fmt.Sprintf("  … +%d", len(m.tasks)-shown)) + "\n")
			break
		}
		shown++
		switch t.status {
		case taskDone:
			sb.WriteString(styleSuccess.Render("  ✓ ") + styleDim.Render(t.title) + "\n")
		case taskRunning:
			role := t.role
			if role == "" {
				role = "…"
			}
			sb.WriteString(styleStreaming.Render("  ● ") + t.title + styleDim.Render("  ["+role+"]") + "\n")
		default:
			sb.WriteString(styleDim.Render("  ○ "+t.title) + "\n")
		}
	}
	detail := m.planDetail
	if detail == "" {
		detail = "…"
	}
	sb.WriteString(styleInfo.Render("  ⏳ "+trunc(detail, 40)) + "\n")
	return strings.TrimSuffix(sb.String(), "\n")
}

func (m tuiModel) subAgentPanelContent() string {
	if m.subRole == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(stylePrompt.Render(m.subRole) + "\n")
	status := m.subStatus
	if status == "" {
		status = lang.T("repl.thinking")
	}
	sb.WriteString(styleStreaming.Render("  "+m.spinner.View()+" "+status) + "\n")
	return strings.TrimSuffix(sb.String(), "\n")
}

// applyProgress 解析 orchestration 的 OnProgress 字符串并更新任务状态。
// 已知格式（execute.go）:
//
//	[role] title: status (round N)   → 标记运行中的子任务
//	✓ subtask N done (title)         → 标记子任务完成
//	Phase N: ...                     → 仅更新 planDetail，不改变任务状态
func (m *tuiModel) applyProgress(detail string) {
	if rest, ok := strings.CutPrefix(detail, "✓ subtask "); ok {
		if end := strings.IndexByte(rest, ' '); end > 0 {
			if id, err := strconv.Atoi(rest[:end]); err == nil {
				for i := range m.tasks {
					if m.tasks[i].id == id {
						m.tasks[i].status = taskDone
					}
				}
			}
		}
		return
	}
	if !strings.HasPrefix(detail, "[") {
		return
	}
	if end := strings.IndexByte(detail, ']'); end > 1 {
		role := detail[1:end]
		rest := strings.TrimSpace(detail[end+1:])
		title, _, ok := strings.Cut(rest, ":")
		if ok {
			for i := range m.tasks {
				if m.tasks[i].title == title {
					m.tasks[i].status = taskRunning
					m.tasks[i].role = role
				}
			}
		}
	}
}

func (m *tuiModel) applySubProgress(msg subProgressMsg) {
	switch {
	case msg.status == "thinking":
		m.subStatus = lang.T("repl.thinking")
	case strings.HasPrefix(msg.status, "executing:"):
		m.subStatus = "🔧 " + strings.TrimPrefix(msg.status, "executing:")
	default:
		m.subStatus = msg.status
	}
	if msg.round > 0 {
		m.subStatus += fmt.Sprintf(" · round %d", msg.round)
	}
}

func (m *tuiModel) appendHistory(s string) {
	m.history.WriteString(s)
	m.refreshViewport()
}

func (m *tuiModel) refreshViewport() {
	content := m.history.String()
	// 流式期间把未渲染的原始内容实时拼在末尾（不写 history，避免与最终渲染重复）
	if m.mode == modeStreaming && m.fullResponse != "" {
		content += " " + styleSuccess.Render("▎"+lang.T("repl.assistant_label")+" ") + "\n"
		content += m.fullResponse
	}
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
}

func (m *tuiModel) appendStatusBar(tokens, tools int, elapsed time.Duration, cacheHit, cacheMiss int) {
	var parts []string
	parts = append(parts, styleSuccess.Render("✓ "+lang.T("repl.done")))
	if tokens > 0 {
		parts = append(parts, styleInfo.Render(fmt.Sprintf("⚡ %dk", tokens/1000)))
	}
	if tools > 0 {
		parts = append(parts, styleInfo.Render(fmt.Sprintf("🔧 %d", tools)))
	}
	parts = append(parts, styleInfo.Render(fmt.Sprintf("⏱ %v", elapsed.Round(100*time.Millisecond))))
	m.appendHistory(styleStatusBar.Render("  "+strings.Join(parts, " · ")) + "\n")
	m.appendHistory(styleDim.Render("────────────────────────────────────────────") + "\n")
}

func (m *tuiModel) appendPlan(plan *orchestration.Plan) {
	m.appendHistory(stylePrompt.Render(plan.Goal) + "\n")
	for i, st := range plan.Subtasks {
		m.appendHistory(styleInfo.Render(fmt.Sprintf("  %d. %s", i+1, st.Title)) + "\n")
	}
	m.appendHistory("\n")
}

// ── 选择模式按键 ──────────────────────────────────────────

func (m tuiModel) handleSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.selectIdx > 0 {
			m.selectIdx--
		}
	case "down", "j":
		if m.selectIdx < len(m.selectItems)-1 {
			m.selectIdx++
		}
	case "enter", " ":
		var cmd tea.Cmd
		if m.selectOnPick != nil && m.selectIdx < len(m.selectItems) {
			cmd = m.selectOnPick(m.selectIdx)
		}
		m.mode = modeIdle
		m.selectItems = nil
		m.selectOnPick = nil
		m.textarea.Focus()
		m.refreshViewport()
		return m, cmd
	case "esc", "ctrl+c":
		m.mode = modeIdle
		m.selectItems = nil
		m.selectOnPick = nil
		m.textarea.Focus()
		m.refreshViewport()
		return m, nil
	}
	return m, nil
}

// ── 历史搜索模式按键 ─────────────────────────────────────

func (m tuiModel) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		if m.searchIdx > 0 {
			m.searchIdx--
		}
	case "down":
		if m.searchIdx < len(m.searchResults)-1 {
			m.searchIdx++
		}
	case "enter":
		if len(m.searchResults) > 0 && m.searchIdx < len(m.searchResults) {
			v := m.searchResults[m.searchIdx]
			slog.Info("tui textarea set from search", "content", strconv.Quote(v))
			m.textarea.SetValue(v)
			m.textarea.CursorEnd()
		}
		m.mode = modeIdle
		m.textarea.Focus()
		m.refreshViewport()
		return m, nil
	case "esc", "ctrl+c":
		m.mode = modeIdle
		m.textarea.Focus()
		m.refreshViewport()
		return m, nil
	default:
		// 透传输入过滤搜索（简化：直接更新 textarea）
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		term := strings.ToLower(m.textarea.Value())
		m.searchResults = nil
		for i := len(m.cmdHistory) - 1; i >= 0 && len(m.searchResults) < 20; i-- {
			if strings.Contains(strings.ToLower(m.cmdHistory[i]), term) {
				m.searchResults = append(m.searchResults, m.cmdHistory[i])
			}
		}
		m.searchIdx = 0
		return m, cmd
	}
	return m, nil
}

// ── 工具函数 ──────────────────────────────────────────────

func reverseHistory(h []string) []string {
	out := make([]string, 0, len(h))
	for i := len(h) - 1; i >= 0; i-- {
		out = append(out, h[i])
	}
	return out
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

type planPreviewMsg struct {
	query string
	plan  *orchestration.Plan
	err   error
}

type deepPlanMsg struct {
	query  string
	result *orchestration.DeepPlanResult
	err    error
}

type finalizePlanMsg struct {
	query string
	plan  *orchestration.Plan
	name  string
	err   error
}

type planExecMsg struct {
	resp *kernel.Response
	err  error
}

type progressMsg struct {
	cur    int
	detail string
}

// taskStatus 子任务执行状态
type taskStatus int

const (
	taskPending taskStatus = iota
	taskRunning
	taskDone
)

type taskState struct {
	id     int
	title  string
	status taskStatus
	role   string
}

// subProgressMsg 子代理执行中的实时状态（SubAgentProgress 回调）
type subProgressMsg struct {
	role   string
	round  int
	status string // thinking / executing:<tool> / done（来自 orchestration 的字符串格式）
}

type subAgentMsg struct {
	role   string
	result string
	err    error
}

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
func (m tuiModel) hudView() string {
	dot := styleSuccess.Render("●")
	switch m.mode {
	case modeThinking, modeStreaming, modePlanExec, modeSubAgent:
		dot = styleStreaming.Render("●")
	case modeApproval:
		dot = styleWarn.Render("●")
	}
	modeTxt := lang.T("repl.status_idle")
	switch m.mode {
	case modeThinking:
		modeTxt = lang.T("repl.thinking")
	case modeStreaming:
		modeTxt = lang.T("repl.working")
	case modePlanExec:
		modeTxt = lang.T("repl.executing")
	case modeSubAgent:
		modeTxt = lang.T("repl.sub_agent")
	case modeApproval:
		modeTxt = lang.T("repl.status_approval")
	case modeSelect:
		modeTxt = lang.T("repl.selecting")
	case modeSearch:
		modeTxt = lang.T("repl.searching")
	}

	var parts []string
	parts = append(parts, dot+" "+modeTxt)
	if m.modelName != "" {
		parts = append(parts, styleHudModel.Render("▍"+m.modelName))
	}
	if m.sessionTitle != "" {
		parts = append(parts, styleHudSess.Render("▍"+trunc(m.sessionTitle, 24)))
	} else if m.sessionID != "" {
		parts = append(parts, styleHudSess.Render("▍"+trunc(m.sessionID, 12)))
	}
	if m.gitBranch != "" {
		git := styleHudGit.Render("⎇ " + m.gitBranch)
		if m.gitDirty {
			git = styleHudGitD.Render("⎇ " + m.gitBranch + " ✚")
		}
		parts = append(parts, git)
	}
	return styleHudBg.Render(strings.Join(parts, "  "))
}

// gaugeView 渲染仪表盘行：token / 工具 / 轮次 / 耗时 / 缓存命中
func (m tuiModel) gaugeView() string {
	busy := m.mode == modeStreaming || m.mode == modePlanExec || m.mode == modeSubAgent || m.mode == modeThinking
	var parts []string

	if busy {
		elapsed := time.Since(m.startTime)
		parts = append(parts,
			styleGaugeVal.Render(fmt.Sprintf("⚡ %s", formatTokens(m.totalTokens))),
			styleGaugeLbl.Render("tok"),
			"│",
			styleGaugeVal.Render(fmt.Sprintf("🔧 %d", m.totalTools)),
			styleGaugeLbl.Render("tools"),
		)
		if m.streamTotal > 0 {
			parts = append(parts, "│",
				styleGaugeVal.Render(fmt.Sprintf("🔁 %d/%d", m.streamRound, m.streamTotal)),
				styleGaugeLbl.Render("round"))
		}
		parts = append(parts, "│",
			styleGaugeVal.Render(elapsed.Round(time.Second).String()),
			styleGaugeLbl.Render("elapsed"))
		if m.cacheHit+m.cacheMiss > 0 {
			pct := m.cacheHit * 100 / (m.cacheHit + m.cacheMiss)
			parts = append(parts, "│",
				styleGaugeVal.Render(fmt.Sprintf("💾 %d%%", pct)),
				styleGaugeLbl.Render("cache"))
		}
		return styleStatusBar.Render(strings.Join(parts, " "))
	}

	return styleStatusBar.Render("⏸ standby")
}

// sidePanel 渲染侧翼仪表（任务/子代理状态），窄终端返回空
func (m tuiModel) sidePanel() string {
	if m.width < 100 {
		return ""
	}
	content := m.taskPanelContent()
	if content == "" {
		return ""
	}
	return styleSideBar.Render(content)
}

// taskPanelContent 提取任务/子代理面板的纯内容（无边框），供侧翼复用
func (m tuiModel) taskPanelContent() string {
	switch m.mode {
	case modePlanExec:
		return m.planPanelContent()
	case modeSubAgent:
		return m.subAgentPanelContent()
	case modeThinking, modeStreaming:
		if len(m.toolNames) > 0 {
			return m.toolHistoryPanel()
		}
	}
	return ""
}

// toolHistoryPanel 流式执行中的工具调用历史（侧翼）
func (m tuiModel) toolHistoryPanel() string {
	var sb strings.Builder
	sb.WriteString(styleSideTtl.Render(lang.T("repl.tools_running")) + "\n")
	shown := 0
	for i := len(m.toolNames) - 1; i >= 0 && shown < 6; i-- {
		shown++
		sb.WriteString(styleStreaming.Render("  ⚙ ") + styleDim.Render(trunc(m.toolNames[i], 26)) + "\n")
	}
	if len(m.toolNames) > 6 {
		sb.WriteString(styleDim.Render(fmt.Sprintf("  … +%d", len(m.toolNames)-6)) + "\n")
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

func (m tuiModel) View() string {
	if m.width == 0 {
		return "loading…"
	}

	// HUD 顶栏 + 仪表盘行（驾驶舱）
	hud := m.hudView()
	gauge := m.gaugeView()

	// 状态行
	status := m.statusView()

	// 审批 overlay 或帮助行 + 输入区
	var bottom string
	switch m.mode {
	case modeApproval:
		bottom = m.approvalView()
	case modeSelect:
		bottom = m.selectView()
	case modeSearch:
		bottom = m.searchView()
	default:
		help := m.helpView()
		bottom = lipgloss.JoinVertical(lipgloss.Left, help, m.textarea.View())
	}

	// 中央主屏 + 侧翼仪表（窄终端自动降级为单列）
	var main string
	if side := m.sidePanel(); side != "" {
		main = lipgloss.JoinHorizontal(lipgloss.Top, m.viewport.View(), side)
	} else {
		main = m.viewport.View()
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		hud,
		gauge,
		main,
		status,
		bottom,
	)
}

func (m tuiModel) statusView() string {
	switch m.mode {
	case modeThinking:
		txt := m.spinner.View() + " " + lang.T("repl.thinking")
		if m.statusMsg != "" {
			txt += " · " + m.statusMsg
		}
		return styleStreaming.Render(txt)
	case modeStreaming:
		txt := m.spinner.View() + " "
		if len(m.toolNames) > 0 {
			txt += "🔧 " + m.toolNames[len(m.toolNames)-1]
		} else {
			txt += lang.T("repl.working")
		}
		if m.streamTotal > 0 {
			txt += fmt.Sprintf(" · round %d/%d", m.streamRound, m.streamTotal)
		}
		if m.statusMsg != "" {
			txt += " · " + m.statusMsg
		}
		return styleStreaming.Render(txt)
	case modePlanExec:
		txt := m.spinner.View() + " " + lang.T("repl.executing")
		if m.statusMsg != "" {
			txt += " · " + m.statusMsg
		}
		return styleStreaming.Render(txt)
	case modeSubAgent:
		txt := m.spinner.View() + " " + lang.T("repl.sub_agent")
		if m.statusMsg != "" {
			txt += " · " + m.statusMsg
		}
		return styleStreaming.Render(txt)
	case modeApproval:
		return styleWarn.Render(lang.T("repl.status_approval"))
	default:
		return styleIdle.Render(lang.T("repl.status_idle"))
	}
}

func (m tuiModel) helpView() string {
	switch m.mode {
	case modeIdle:
		return styleDim.Render(lang.T("repl.help_line"))
	default:
		return styleDim.Render(lang.T("repl.help_busy"))
	}
}

func (m tuiModel) selectView() string {
	var sb strings.Builder
	sb.WriteString(stylePrompt.Render(m.selectTitle) + "\n")
	for i, item := range m.selectItems {
		if i == m.selectIdx {
			sb.WriteString("  " + styleSelected.Render("▸ "+item) + "\n")
		} else {
			sb.WriteString("    " + item + "\n")
		}
	}
	sb.WriteString(styleDim.Render("↑↓ move · enter select · esc cancel") + "\n")
	return styleBox.Render(sb.String())
}

func (m tuiModel) searchView() string {
	var sb strings.Builder
	sb.WriteString(stylePrompt.Render("History search (type to filter):") + "\n")
	shown := 10
	if len(m.searchResults) < shown {
		shown = len(m.searchResults)
	}
	for i := 0; i < shown; i++ {
		if i == m.searchIdx {
			sb.WriteString("  " + styleSelected.Render("▸ "+trunc(m.searchResults[i], 60)) + "\n")
		} else {
			sb.WriteString("    " + trunc(m.searchResults[i], 60) + "\n")
		}
	}
	sb.WriteString(styleDim.Render("↑↓ move · enter select · esc cancel") + "\n")
	return styleBox.Render(sb.String())
}

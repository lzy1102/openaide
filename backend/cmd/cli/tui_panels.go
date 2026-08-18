package main

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"openaide/backend/lang"
	"strings"
	"time"
)

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
	if len(m.plan.tasks) == 0 {
		return ""
	}
	done := 0
	for _, t := range m.plan.tasks {
		if t.status == taskDone {
			done++
		}
	}
	var sb strings.Builder
	sb.WriteString(stylePrompt.Render(fmt.Sprintf("%s %d/%d", lang.T("repl.task_progress"), done, len(m.plan.tasks))) + "\n")
	shown := 0
	for _, t := range m.plan.tasks {
		if shown >= 6 {
			sb.WriteString(styleDim.Render(fmt.Sprintf("  … +%d", len(m.plan.tasks)-shown)) + "\n")
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
	detail := m.plan.detail
	if detail == "" {
		detail = "…"
	}
	sb.WriteString(styleInfo.Render("  ⏳ "+trunc(detail, 40)) + "\n")
	return strings.TrimSuffix(sb.String(), "\n")
}

func (m tuiModel) subAgentPanelContent() string {
	if m.sub.role == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(stylePrompt.Render(m.sub.role) + "\n")
	status := m.sub.status
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
func (m *tuiModel) appendHistory(s string) {
	m.history.WriteString(s)
	m.refreshViewport()
}

func (m *tuiModel) refreshViewport() {
	content := m.history.String()
	// 流式期间把未渲染的原始内容实时拼在末尾（不写 history，避免与最终渲染重复）
	if m.mode == modeStreaming && m.stream.fullResponse != "" {
		content += " " + styleSuccess.Render("▎"+lang.T("repl.assistant_label")+" ") + "\n"
		content += m.stream.fullResponse
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

type taskStatus int

type taskState struct {
	id     int
	title  string
	status taskStatus
	role   string
}

// subProgressMsg 子代理执行中的实时状态（SubAgentProgress 回调）
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

// gaugeView 渲染仪表盘行：执行中显示实时指标，idle 显示项目常驻统计
func (m tuiModel) gaugeView() string {
	busy := m.mode == modeStreaming || m.mode == modePlanExec || m.mode == modeSubAgent || m.mode == modeThinking
	var parts []string

	if busy {
		elapsed := time.Since(m.stream.startTime)
		parts = append(parts,
			styleGaugeVal.Render(fmt.Sprintf("⚡ %s", formatTokens(m.stream.totalTokens))),
			styleGaugeLbl.Render("tok"),
			"│",
			styleGaugeVal.Render(fmt.Sprintf("🔧 %d", m.stream.totalTools)),
			styleGaugeLbl.Render(lang.T("repl.gauge_tools")),
		)
		if m.stream.streamTotal > 0 {
			parts = append(parts, "│",
				styleGaugeVal.Render(fmt.Sprintf("🔁 %d/%d", m.stream.streamRound, m.stream.streamTotal)),
				styleGaugeLbl.Render(lang.T("repl.gauge_round")))
		}
		parts = append(parts, "│",
			styleGaugeVal.Render(elapsed.Round(time.Second).String()),
			styleGaugeLbl.Render(lang.T("repl.gauge_elapsed")))
		if m.stream.cacheHit+m.stream.cacheMiss > 0 {
			pct := m.stream.cacheHit * 100 / (m.stream.cacheHit + m.stream.cacheMiss)
			parts = append(parts, "│",
				styleGaugeVal.Render(fmt.Sprintf("💾 %d%%", pct)),
				styleGaugeLbl.Render(lang.T("repl.gauge_cache")))
		}
		return styleStatusBar.Render(strings.Join(parts, " "))
	}

	// idle：项目常驻统计（启动时缓存，纯渲染）
	parts = append(parts,
		styleGaugeVal.Render(fmt.Sprintf("💬 %d", m.idle.sessionCount)),
		styleGaugeLbl.Render(lang.T("repl.idle_sessions")),
		"│",
		styleGaugeVal.Render(fmt.Sprintf("🔧 %d", m.idle.toolCount)),
		styleGaugeLbl.Render(lang.T("repl.idle_tools")),
		"│",
		styleGaugeVal.Render(fmt.Sprintf("🧠 %d", m.idle.factCount)),
		styleGaugeLbl.Render(lang.T("repl.idle_facts")),
		"│",
		styleGaugeVal.Render(fmt.Sprintf("💡 %d", m.idle.learnCount)),
		styleGaugeLbl.Render(lang.T("repl.idle_learnings")),
	)
	return styleStatusBar.Render(strings.Join(parts, " "))
}

// sidePanel 渲染侧翼仪表（任务/子代理/最近会话），窄终端返回空
func (m tuiModel) sidePanel() string {
	if m.width < 100 {
		return ""
	}
	content := m.taskPanelContent()
	if content == "" {
		content = m.idlePanelContent()
	}
	if content == "" {
		return ""
	}
	return styleSideBar.Render(content)
}

// idlePanelContent idle 侧翼：最近会话列表
func (m tuiModel) idlePanelContent() string {
	if m.mode != modeIdle || len(m.idle.recent) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(styleSideTtl.Render(lang.T("repl.idle_recent_sessions")) + "\n")
	shown := 0
	for _, s := range m.idle.recent {
		if shown >= 6 {
			break
		}
		shown++
		id := s.id
		if len(id) > 8 {
			id = id[:8]
		}
		age := humanAge(s.updatedAt)
		sb.WriteString(styleStreaming.Render("  ● ") +
			styleDim.Render(trunc(id, 10)) +
			styleGaugeLbl.Render(fmt.Sprintf("  %d msgs · %s", s.msgCount, age)) + "\n")
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

// humanAge 相对时间描述：<1m→刚刚, <1h→N 分钟, <24h→N 小时, 否则 N 天
func humanAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return lang.T("repl.idle_just_now")
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// taskPanelContent 提取任务/子代理面板的纯内容（无边框），供侧翼复用
func (m tuiModel) taskPanelContent() string {
	switch m.mode {
	case modePlanExec:
		return m.planPanelContent()
	case modeSubAgent:
		return m.subAgentPanelContent()
	case modeThinking, modeStreaming:
		if len(m.stream.toolNames) > 0 {
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
	for i := len(m.stream.toolNames) - 1; i >= 0 && shown < 6; i-- {
		shown++
		sb.WriteString(styleStreaming.Render("  ⚙ ") + styleDim.Render(trunc(m.stream.toolNames[i], 26)) + "\n")
	}
	if len(m.stream.toolNames) > 6 {
		sb.WriteString(styleDim.Render(fmt.Sprintf("  … +%d", len(m.stream.toolNames)-6)) + "\n")
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
		// 输入区用边框包裹,与主区形成鲜明分隔。
		input := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1).
			Render(m.textarea.View())
		bottom = lipgloss.JoinVertical(lipgloss.Left, help, input)
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
		if len(m.stream.toolNames) > 0 {
			txt += "🔧 " + m.stream.toolNames[len(m.stream.toolNames)-1]
		} else {
			txt += lang.T("repl.working")
		}
		if m.stream.streamTotal > 0 {
			txt += fmt.Sprintf(" · round %d/%d", m.stream.streamRound, m.stream.streamTotal)
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
	sb.WriteString(stylePrompt.Render(m.selectS.title) + "\n")
	for i, item := range m.selectS.items {
		if i == m.selectS.idx {
			sb.WriteString("  " + styleSelected.Render("▸ "+item) + "\n")
		} else {
			sb.WriteString("    " + item + "\n")
		}
	}
	sb.WriteString(styleDim.Render(lang.T("repl.select_hint")) + "\n")
	return styleBox.Render(sb.String())
}

func (m tuiModel) searchView() string {
	var sb strings.Builder
	sb.WriteString(stylePrompt.Render(lang.T("repl.history_search")) + "\n")
	shown := 10
	if len(m.searchS.results) < shown {
		shown = len(m.searchS.results)
	}
	for i := 0; i < shown; i++ {
		if i == m.searchS.idx {
			sb.WriteString("  " + styleSelected.Render("▸ "+trunc(m.searchS.results[i], 60)) + "\n")
		} else {
			sb.WriteString("    " + trunc(m.searchS.results[i], 60) + "\n")
		}
	}
	sb.WriteString(styleDim.Render(lang.T("repl.select_hint")) + "\n")
	return styleBox.Render(sb.String())
}

package main

import (
	"context"
	"fmt"
	"openaide/backend/internal/infra"
	"openaide/backend/internal/kernel"
	"openaide/backend/internal/lang"
	"openaide/backend/internal/orchestration"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

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
	m.selectS.title = lang.T("repl.select_approach")
	m.selectS.items = options
	m.selectS.idx = 0
	m.selectS.onPick = func(idx int) tea.Cmd {
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

// ── 计划执行 ───────────────────────────────────────────────

func (m tuiModel) startPlanExec(query string, plan *orchestration.Plan) (tuiModel, tea.Cmd) {
	m.mode = modePlanExec
	m.plan.total = len(plan.Subtasks) + 2
	m.plan.current = 0
	m.plan.detail = ""
	m.stream.startTime = time.Now()
	m.plan.tasks = make([]taskState, len(plan.Subtasks))
	for i, st := range plan.Subtasks {
		m.plan.tasks[i] = taskState{id: st.ID, title: st.Title, status: taskPending}
	}
	m.layoutViewport()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	m.stream.cancel = cancel
	m.textarea.Blur()
	m.refreshViewport()

	m.app.Orchestrator.OnProgress = func(phase, detail string) {
		select {
		case m.plan.progressCh <- progressMsg{cur: 0, detail: detail}:
		default:
		}
	}

	return m, tea.Batch(
		func() tea.Msg {
			resp, err := m.app.Orchestrator.ExecuteWithPlan(ctx, "cli-user", m.projectID, query, plan, kernel.QueryOptions{})
			m.app.Orchestrator.OnProgress = nil
			return planExecMsg{resp: resp, err: err}
		},
		waitForProgress(m.plan.progressCh, m.plan.resultCh),
	)
}

func (m tuiModel) handlePlanExec(msg planExecMsg) (tea.Model, tea.Cmd) {
	// 收尾：关闭进度通道，解除挂起的 waitForProgress
	close(m.plan.progressCh)
	close(m.plan.resultCh)
	m.plan.progressCh = make(chan progressMsg, 16)
	m.plan.resultCh = make(chan planExecMsg, 1)
	if m.stream.cancel != nil {
		m.stream.cancel()
		m.stream.cancel = nil
	}
	elapsed := time.Since(m.stream.startTime)

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
	m.plan.tasks = nil
	m.layoutViewport()
	m.textarea.Focus()
	m.refreshViewport()
	return m, nil
}

func (m *tuiModel) applyProgress(detail string) {
	if rest, ok := strings.CutPrefix(detail, "✓ subtask "); ok {
		if end := strings.IndexByte(rest, ' '); end > 0 {
			if id, err := strconv.Atoi(rest[:end]); err == nil {
				for i := range m.plan.tasks {
					if m.plan.tasks[i].id == id {
						m.plan.tasks[i].status = taskDone
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
			for i := range m.plan.tasks {
				if m.plan.tasks[i].title == title {
					m.plan.tasks[i].status = taskRunning
					m.plan.tasks[i].role = role
				}
			}
		}
	}
}

func (m *tuiModel) appendPlan(plan *orchestration.Plan) {
	m.appendHistory(stylePrompt.Render(plan.Goal) + "\n")
	for i, st := range plan.Subtasks {
		m.appendHistory(styleInfo.Render(fmt.Sprintf("  %d. %s", i+1, st.Title)) + "\n")
	}
	m.appendHistory("\n")
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

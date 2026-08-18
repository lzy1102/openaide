package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"openaide/backend/core"
	"openaide/backend/lang"
)

// streamMsg 包装一个 kernel.StreamChunk
type streamMsg struct {
	chunk kernel.StreamChunk
}

// waitForChunk 订阅流式 channel，收到块后转成 tea.Msg
func waitForChunk(ch <-chan kernel.StreamChunk) tea.Cmd {
	return func() tea.Msg {
		c, ok := <-ch
		if !ok {
			return streamMsg{chunk: kernel.StreamChunk{Type: kernel.ChunkTypeDone, Done: true}}
		}
		return streamMsg{chunk: c}
	}
}

// waitForProgress 计划执行时同时监听进度与结果
func waitForProgress(progressCh chan progressMsg, resultCh chan planExecMsg) tea.Cmd {
	return func() tea.Msg {
		select {
		case p, ok := <-progressCh:
			if !ok {
				return nil
			}
			return p
		case r, ok := <-resultCh:
			if !ok {
				return nil
			}
			return r
		}
	}
}

// waitForSubProgress 子代理执行时同时监听实时状态与最终结果
func waitForSubProgress(progressCh chan subProgressMsg, resultCh chan subAgentMsg) tea.Cmd {
	return func() tea.Msg {
		select {
		case p, ok := <-progressCh:
			if !ok {
				return nil
			}
			return p
		case r, ok := <-resultCh:
			if !ok {
				return nil
			}
			return r
		}
	}
}

// ── 流式执行 ───────────────────────────────────────────────

func (m tuiModel) startStream(query string) (tea.Model, tea.Cmd) {
	m.mode = modeStreaming
	m.statusMsg = ""
	m.stream.thinkCount = 0
	m.stream.toolNames = nil
	m.stream.totalTokens = 0
	m.stream.cacheHit = 0
	m.stream.cacheMiss = 0
	m.stream.totalTools = 0
	m.stream.askQuestions = nil
	m.stream.fullResponse = ""
	m.stream.streamRound = 0
	m.stream.streamTotal = 0
	m.stream.startTime = time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	m.stream.cancel = cancel
	m.textarea.Blur()
	m.refreshViewport()

	opts := kernel.QueryOptions{
		ParallelResearch: m.research.Get(),
		OnBudgetExhausted: func(round, maxRounds int) bool {
			if m.autoYes.Get() {
				return true
			}
			ch := make(chan bool)
			select {
			case m.approval.ch <- approvalRequest{isBudget: true, round: round, maxRounds: maxRounds, resultCh: ch}:
			case <-ctx.Done():
				return false
			}
			return <-ch
		},
		OnPlanApproved: func(plan *kernel.TaskPlan) bool {
			if m.autoYes.Get() {
				return true
			}
			ch := make(chan bool)
			var sb strings.Builder
			for i, st := range plan.SubTasks {
				sb.WriteString(fmt.Sprintf("Step %d: %s\n", i+1, st.Goal))
			}
			select {
			case m.approval.ch <- approvalRequest{isPlan: true, planText: sb.String(), resultCh: ch}:
			case <-ctx.Done():
				return false
			}
			return <-ch
		},
	}

	return m, func() tea.Msg {
		stream, err := m.app.Orchestrator.ProcessQueryStream(ctx, m.sessionID, "cli-user", m.projectID, query, opts)
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
			m.stream.fullResponse += c.Content
			m.refreshViewport()
		}
	case kernel.ChunkTypeThinking:
		if c.Round > 0 {
			m.stream.streamRound = c.Round
			m.stream.streamTotal = c.TotalRounds
		}
		if c.ReasoningContent != "" && m.stream.thinkCount < 2 {
			first := strings.SplitN(c.ReasoningContent, "\n", 2)[0]
			if len(first) > 100 {
				first = first[:97] + "..."
			}
			m.appendHistory(styleThink.Render("[think] "+first) + "\n")
			m.stream.thinkCount++
		}
	case kernel.ChunkTypeToolCall:
		if c.ToolName != "" {
			m.stream.toolNames = append(m.stream.toolNames, c.ToolName)
			m.stream.totalTools++
			seen := map[string]bool{}
			var unique []string
			for _, n := range m.stream.toolNames {
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
			m.stream.totalTokens = c.Usage.TotalTokens
			m.stream.cacheHit = c.Usage.PromptCacheHitTokens
			m.stream.cacheMiss = c.Usage.PromptCacheMissTokens
		}
		return m.finishStream()
	}

	if c.Done {
		return m.finishStream()
	}
	return m, waitForChunk(m.stream.streamCh)
}

func (m tuiModel) finishStream() (tea.Model, tea.Cmd) {
	if m.stream.cancel != nil {
		m.stream.cancel()
		m.stream.cancel = nil
	}
	// 流结束但审批仍挂起时，立即拒绝，解除内核回调的阻塞等待
	if m.approval.pending != nil {
		select {
		case m.approval.pending.resultCh <- false:
		default:
		}
		m.approval.pending = nil
	}
	m.mode = modeIdle
	elapsed := time.Since(m.stream.startTime)

	// 最终回答（markdown 渲染）
	if m.stream.fullResponse != "" {
		m.appendHistory(" " + styleSuccess.Render("▎"+lang.T("repl.assistant_label")+" ") + "\n")
		m.appendHistory(RenderMarkdown(m.stream.fullResponse) + "\n")
	}
	m.appendStatusBar(m.stream.totalTokens, m.stream.totalTools, elapsed, m.stream.cacheHit, m.stream.cacheMiss)

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

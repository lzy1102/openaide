package main

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"openaide/backend/internal/lang"
)

func (m *tuiModel) applySubProgress(msg subProgressMsg) {
	switch {
	case msg.status == "thinking":
		m.sub.status = lang.T("repl.thinking")
	case strings.HasPrefix(msg.status, "executing:"):
		m.sub.status = "🔧 " + strings.TrimPrefix(msg.status, "executing:")
	default:
		m.sub.status = msg.status
	}
	if msg.round > 0 {
		m.sub.status += fmt.Sprintf(" · round %d", msg.round)
	}
}

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

// startSubAgent 单角色子代理
func (m tuiModel) startSubAgent(role, task string) (tea.Model, tea.Cmd) {
	m.sub.role = role
	m.mode = modeSubAgent
	m.sub.status = ""
	m.statusMsg = lang.T("repl.sub_running", role)
	m.textarea.Blur()
	m.layoutViewport()
	m.refreshViewport()

	ctx, cancel := context.WithCancel(context.Background())
	m.stream.cancel = cancel
	m.sub.progressCh = make(chan subProgressMsg, 32)
	m.sub.resultCh = make(chan subAgentMsg, 1)

	progress := func(r string, round int, status string) {
		select {
		case m.sub.progressCh <- subProgressMsg{role: r, round: round, status: status}:
		default:
		}
	}

	return m, tea.Batch(
		func() tea.Msg {
			defer cancel()
			result, err := m.app.Orchestrator.RunSubAgent(ctx, "cli-user", m.projectID, role, task, nil, progress)
			m.sub.resultCh <- subAgentMsg{role: role, result: result, err: err}
			return nil
		},
		waitForSubProgress(m.sub.progressCh, m.sub.resultCh),
	)
}

// startTeam /team 顺序流水线
func (m tuiModel) startTeam(task string) (tea.Model, tea.Cmd) {
	m.sub.role = "team"
	m.mode = modeSubAgent
	m.sub.status = ""
	m.statusMsg = lang.T("repl.team_pipeline")
	m.textarea.Blur()
	m.layoutViewport()
	m.refreshViewport()

	ctx, cancel := context.WithCancel(context.Background())
	m.stream.cancel = cancel
	m.sub.progressCh = make(chan subProgressMsg, 32)
	m.sub.resultCh = make(chan subAgentMsg, 1)

	progress := func(r string, round int, status string) {
		select {
		case m.sub.progressCh <- subProgressMsg{role: r, round: round, status: status}:
		default:
		}
	}

	return m, tea.Batch(
		func() tea.Msg {
			defer cancel()
			prevResults := []string{}
			for _, role := range []string{"analyst", "coder", "reviewer"} {
				result, err := m.app.Orchestrator.RunSubAgent(ctx, "cli-user", m.projectID, role, task, prevResults, progress)
				if err != nil {
					m.sub.resultCh <- subAgentMsg{role: role, result: "", err: err}
					return nil
				}
				prevResults = append(prevResults, result)
			}
			m.sub.resultCh <- subAgentMsg{role: "team", result: prevResults[len(prevResults)-1], err: nil}
			return nil
		},
		waitForSubProgress(m.sub.progressCh, m.sub.resultCh),
	)
}

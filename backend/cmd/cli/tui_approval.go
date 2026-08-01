package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"

	"openaide/backend/internal/lang"
)

// approvalRequest 内核同步回调与 TUI 之间的审批请求
type approvalRequest struct {
	tool      string
	path      string
	args      string
	isBudget  bool
	round     int
	maxRounds int
	skillName string
	skillDesc string
	resultCh  chan bool
}

type approvalReqMsg struct {
	req approvalRequest
}

// waitForApproval 订阅审批 channel
func waitForApproval(ch chan approvalRequest) tea.Cmd {
	return func() tea.Msg {
		req, ok := <-ch
		if !ok {
			return nil
		}
		return approvalReqMsg{req: req}
	}
}

// handleApprovalKey 审批模式下按键处理
func (m tuiModel) handleApprovalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pendingApproval == nil {
		m.mode = modeIdle
		m.textarea.Focus()
		return m, waitForApproval(m.approvalCh)
	}

	switch msg.String() {
	case "y", "Y", "a", "A", "enter":
		allow := true
		if msg.String() == "a" || msg.String() == "A" {
			m.autoYes.Set(true)
		}
		m.pendingApproval.resultCh <- allow
		m.pendingApproval = nil
		m.mode = modeStreaming
		if m.streamCh != nil {
			return m, waitForChunk(m.streamCh)
		}
		return m, nil
	case "n", "N", "esc":
		m.pendingApproval.resultCh <- false
		m.pendingApproval = nil
		m.mode = modeStreaming
		if m.streamCh != nil {
			return m, waitForChunk(m.streamCh)
		}
		return m, nil
	case "ctrl+c":
		m.pendingApproval.resultCh <- false
		m.pendingApproval = nil
		if m.cancel != nil {
			m.cancel()
			m.cancel = nil
		}
		m.mode = modeIdle
		m.appendHistory(styleWarn.Render("⚠ Cancelled") + "\n")
		m.textarea.Focus()
		m.refreshViewport()
		return m, waitForApproval(m.approvalCh)
	}
	return m, nil
}

// approvalView 审批面板渲染
func (m tuiModel) approvalView() string {
	if m.pendingApproval == nil {
		return ""
	}
	req := m.pendingApproval
	var sb strings.Builder

	if req.isBudget {
		sb.WriteString(styleWarn.Render("⚡ "+lang.T("repl.rounds_exhausted", req.round, req.maxRounds)) + "\n")
	} else {
		icon := toolIcon(req.tool)
		sb.WriteString(styleWarn.Render("⚡ Permission Required") + "\n")
		line := fmt.Sprintf("  %s %s %s", icon, stylePrompt.Render(req.tool), styleDim.Render(req.path))
		sb.WriteString(line + "\n")
		if req.args != "" {
			var prettyArgs map[string]interface{}
			if json.Unmarshal([]byte(req.args), &prettyArgs) == nil {
				for k, v := range prettyArgs {
					if k != "path" && k != "command" {
						sb.WriteString(styleDim.Render(fmt.Sprintf("    %s: %v", k, v)) + "\n")
					}
				}
				if cmd, ok := prettyArgs["command"].(string); ok {
					sb.WriteString(styleDim.Render("    command: "+cmd) + "\n")
				}
			}
		}
	}
	sb.WriteString(styleDim.Render("[y] allow  [a] allow all  [n] deny  [esc] cancel") + "\n")
	return styleBox.Render(sb.String())
}

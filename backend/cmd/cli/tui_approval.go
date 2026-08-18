package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"

	"openaide/backend/lang"
)

// approvalRequest 内核同步回调与 TUI 之间的审批请求
type approvalRequest struct {
	tool      string
	path      string
	args      string
	isBudget  bool
	isPlan    bool
	planText  string
	round     int
	maxRounds int
	skillName string
	skillDesc string
	resultCh  chan bool
}

type approvalReqMsg struct {
	req approvalRequest
}

// approvalTimeoutMsg 审批挂起超时，自动拒绝，避免内核回调永久阻塞
type approvalTimeoutMsg struct{}

// approvalTimeout 审批等待上限：超时后自动拒绝（为防止回调 goroutine 永久阻塞的兜底）
const approvalTimeout = 2 * time.Minute

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
	if m.approval.pending == nil {
		m.mode = modeIdle
		m.textarea.Focus()
		return m, waitForApproval(m.approval.ch)
	}

	switch msg.String() {
	case "y", "Y", "a", "A", "enter":
		allow := true
		if msg.String() == "a" || msg.String() == "A" {
			m.autoYes.Set(true)
		}
		m.approval.pending.resultCh <- allow
		m.approval.pending = nil
		m.mode = modeStreaming
		if m.stream.streamCh != nil {
			return m, waitForChunk(m.stream.streamCh)
		}
		return m, nil
	case "n", "N", "esc":
		m.approval.pending.resultCh <- false
		m.approval.pending = nil
		m.mode = modeStreaming
		if m.stream.streamCh != nil {
			return m, waitForChunk(m.stream.streamCh)
		}
		return m, nil
	case "ctrl+c":
		m.approval.pending.resultCh <- false
		m.approval.pending = nil
		if m.stream.cancel != nil {
			m.stream.cancel()
			m.stream.cancel = nil
		}
		m.mode = modeIdle
		m.appendHistory(styleWarn.Render(lang.T("repl.cancelled")) + "\n")
		m.textarea.Focus()
		m.refreshViewport()
		return m, waitForApproval(m.approval.ch)
	}
	return m, nil
}

// approvalView 审批面板渲染
func (m tuiModel) approvalView() string {
	if m.approval.pending == nil {
		return ""
	}
	req := m.approval.pending
	var sb strings.Builder

	if req.isPlan {
		sb.WriteString(styleWarn.Render("📋 "+lang.T("repl.plan_approval_title")) + "\n")
		for _, line := range strings.Split(req.planText, "\n") {
			sb.WriteString(styleDim.Render("  "+line) + "\n")
		}
	} else if req.isBudget {
		sb.WriteString(styleWarn.Render("⚡ "+lang.T("repl.rounds_exhausted", req.round, req.maxRounds)) + "\n")
	} else {
		icon := toolIcon(req.tool)
		sb.WriteString(styleWarn.Render("⚡ "+lang.T("repl.approval_title")) + "\n")
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
	sb.WriteString(styleDim.Render(lang.T("repl.approval_keys")) + "\n")
	return styleBox.Render(sb.String())
}

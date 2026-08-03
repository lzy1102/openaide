package main

import (
	"fmt"
	"openaide/backend/internal/lang"
	"strings"
)

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

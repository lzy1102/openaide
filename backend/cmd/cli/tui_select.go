package main

import (
	"log/slog"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// ── 选择模式按键 ──────────────────────────────────────────

func (m tuiModel) handleSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.selectS.idx > 0 {
			m.selectS.idx--
		}
	case "down", "j":
		if m.selectS.idx < len(m.selectS.items)-1 {
			m.selectS.idx++
		}
	case "enter", " ":
		var cmd tea.Cmd
		if m.selectS.onPick != nil && m.selectS.idx < len(m.selectS.items) {
			cmd = m.selectS.onPick(m.selectS.idx)
		}
		m.mode = modeIdle
		m.selectS.items = nil
		m.selectS.onPick = nil
		m.textarea.Focus()
		m.refreshViewport()
		return m, cmd
	case "esc", "ctrl+c":
		m.mode = modeIdle
		m.selectS.items = nil
		m.selectS.onPick = nil
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
		if m.searchS.idx > 0 {
			m.searchS.idx--
		}
	case "down":
		if m.searchS.idx < len(m.searchS.results)-1 {
			m.searchS.idx++
		}
	case "enter":
		if len(m.searchS.results) > 0 && m.searchS.idx < len(m.searchS.results) {
			v := m.searchS.results[m.searchS.idx]
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
		m.searchS.results = nil
		for i := len(m.cmdHistory) - 1; i >= 0 && len(m.searchS.results) < 20; i-- {
			if strings.Contains(strings.ToLower(m.cmdHistory[i]), term) {
				m.searchS.results = append(m.searchS.results, m.cmdHistory[i])
			}
		}
		m.searchS.idx = 0
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

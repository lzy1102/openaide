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

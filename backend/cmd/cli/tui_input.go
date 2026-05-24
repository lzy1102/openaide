package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"openaide/backend/internal/lang"
)

// ── InputBar Component ─────────────────────────────────────

type InputBar struct {
	input       textinput.Model
	history     []string
	histIdx     int
	queuedQuery string
	streaming   bool
}

func NewInputBar() *InputBar {
	ti := textinput.New()
	ti.Placeholder = lang.T("tui.placeholder")
	ti.Prompt = "❯ "
	ti.Focus()
	ti.CharLimit = 0
	ti.Width = 60
	return &InputBar{input: ti, histIdx: -1}
}

func (b *InputBar) Focus()  { b.input.Focus() }
func (b *InputBar) Blur()   { b.input.Blur() }
func (b *InputBar) Reset()  { b.input.SetValue("") }
func (b *InputBar) SetStreaming(on bool) { b.streaming = on }
func (b *InputBar) SetWidth(w int)       { b.input.Width = w - 8 }

func (b *InputBar) Update(msg tea.Msg) (string, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			query := strings.TrimSpace(b.input.Value())
			if b.streaming {
				if query != "" {
					b.queuedQuery = query
					b.input.SetValue("")
					return "queued", nil
				}
				return "", nil
			}
			if query == "" {
				return "", nil
			}
			b.addToHistory(query)
			b.input.SetValue("")
			return query, nil

		case "up":
			if !b.streaming && len(b.history) > 0 {
				if b.histIdx == -1 {
					b.histIdx = len(b.history) - 1
				} else if b.histIdx > 0 {
					b.histIdx--
				}
				b.input.SetValue(b.history[b.histIdx])
				b.input.CursorEnd()
			}
			return "", nil

		case "down":
			if !b.streaming && b.histIdx >= 0 {
				b.histIdx++
				if b.histIdx >= len(b.history) {
					b.histIdx = -1
					b.input.SetValue("")
				} else {
					b.input.SetValue(b.history[b.histIdx])
				}
				b.input.CursorEnd()
			}
			return "", nil
		}
	}

	var cmd tea.Cmd
	b.input, cmd = b.input.Update(msg)
	return "", cmd
}

func (b *InputBar) View() string {
	return inputStyle.Render(b.input.View())
}

func (b *InputBar) PopQueued() string {
	q := b.queuedQuery
	b.queuedQuery = ""
	return q
}

func (b *InputBar) addToHistory(query string) {
	if len(b.history) == 0 || b.history[len(b.history)-1] != query {
		b.history = append(b.history, query)
		if len(b.history) > 100 {
			b.history = b.history[1:]
		}
	}
	b.histIdx = -1
}

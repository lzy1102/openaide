package main

import (
	"fmt"
	"strings"

	"openaide/backend/internal/lang"
)

// ── StatusBar Component ────────────────────────────────────

type StatusBar struct {
	sessionName string
	spinner     int
	streaming   bool
	tokens      int
	tools       int
	err         string
}

func NewStatusBar() *StatusBar { return &StatusBar{} }

func (s *StatusBar) SetSession(name string)  { s.sessionName = name }
func (s *StatusBar) SetStreaming(on bool)     { s.streaming = on }
func (s *StatusBar) SetTokens(n int)          { s.tokens = n }
func (s *StatusBar) SetTools(n int)           { s.tools = n }
func (s *StatusBar) SetError(err string)      { s.err = err }
func (s *StatusBar) Tick()                    { s.spinner = (s.spinner + 1) % 10 }

func (s *StatusBar) View() string {
	spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	var parts []string

	if s.sessionName != "" {
		parts = append(parts, icons.folder+" "+s.sessionName)
	}
	if s.streaming {
		parts = append(parts, spinnerFrames[s.spinner]+" "+lang.T("mode.thinking"))
	}
	if s.tools > 0 {
		parts = append(parts, fmt.Sprintf("%s %d", icons.tools, s.tools))
	}
	if s.tokens > 0 {
		parts = append(parts, fmt.Sprintf("%s %d", icons.tokens, s.tokens))
	}
	if len(parts) > 0 {
		return statusBarStyle.Render(strings.Join(parts, " │ "))
	}
	return ""
}

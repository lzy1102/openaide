package tui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

func isTerminal() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	if (fi.Mode() & os.ModeCharDevice) == 0 {
		return false
	}
	si, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (si.Mode() & os.ModeCharDevice) != 0
}

type ThinkingPhase struct {
	Label string
	Icon  string
	Color string
}

var thinkingPhases = []ThinkingPhase{
	{Label: "理解问题", Icon: "🔍", Color: "#61AFEF"},
	{Label: "分析上下文", Icon: "🧠", Color: "#C678DD"},
	{Label: "检索知识", Icon: "📚", Color: "#E5C07B"},
	{Label: "组织回复", Icon: "✨", Color: "#98C379"},
}

type ThinkingSpinner struct {
	mu       sync.Mutex
	stopped  bool
	phase    int
	label    string
	started  time.Time
	frame    int
	frames   []string
	done     chan struct{}
	elapsed  time.Duration
	isTTY    bool
}

func NewThinkingSpinner() *ThinkingSpinner {
	return &ThinkingSpinner{
		frames:  []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		done:    make(chan struct{}),
		started: time.Now(),
		isTTY:   isTerminal(),
	}
}

func (s *ThinkingSpinner) Start(initialLabel string) {
	s.mu.Lock()
	s.label = initialLabel
	s.stopped = false
	s.started = time.Now()
	s.mu.Unlock()

	if !s.isTTY {
		s.mu.Lock()
		phase := thinkingPhases[0]
		label := s.label
		if label == "" {
			label = phase.Label
		}
		fmt.Fprintf(os.Stderr, "  %s %s...\n", phase.Icon, label)
		s.mu.Unlock()
		return
	}

	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		phaseTicker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		defer phaseTicker.Stop()

		for {
			select {
			case <-s.done:
				fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", 80))
				return
			case <-ticker.C:
				s.mu.Lock()
				if s.stopped {
					s.mu.Unlock()
					fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", 80))
					return
				}
				s.frame = (s.frame + 1) % len(s.frames)
				s.elapsed = time.Since(s.started)
				frame := s.frames[s.frame]
				phase := thinkingPhases[s.phase%len(thinkingPhases)]
				elapsed := fmt.Sprintf("%.1fs", s.elapsed.Seconds())
				label := s.label
				if label == "" {
					label = phase.Label
				}
				fmt.Fprintf(os.Stderr, "\r  %s %s %s %s ",
					phase.Icon,
					StyleMuted.Render(frame),
					StyleThinking.Render(label),
					StyleMuted.Render(elapsed),
				)
				s.mu.Unlock()
			case <-phaseTicker.C:
				s.mu.Lock()
				if !s.stopped {
					s.phase = (s.phase + 1) % len(thinkingPhases)
				}
				s.mu.Unlock()
			}
		}
	}()
}

func (s *ThinkingSpinner) UpdateLabel(label string) {
	s.mu.Lock()
	s.label = label
	s.mu.Unlock()
}

func (s *ThinkingSpinner) Stop() time.Duration {
	s.mu.Lock()
	s.stopped = true
	elapsed := time.Since(s.started)
	s.mu.Unlock()

	if s.isTTY {
		select {
		case s.done <- struct{}{}:
		default:
		}
		fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", 80))
	}
	return elapsed
}

func StartThinking(label string) *ThinkingSpinner {
	spinner := NewThinkingSpinner()
	spinner.Start(label)
	return spinner
}

func ShowToolCall(toolName string, params string) {
	if isTerminal() {
		fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", 80))
	}
	fmt.Printf("  %s %s %s\n",
		StyleToolIcon.Render("🔧"),
		StyleToolName.Render(toolName),
		StyleMuted.Render(truncateStr(params, 60)),
	)
}

func ShowToolResult(toolName string, success bool, result string) {
	icon := StyleSuccess.Render("✓")
	if !success {
		icon = StyleError.Render("✗")
	}
	fmt.Printf("  %s %s %s\n",
		icon,
		StyleToolName.Render(toolName),
		StyleMuted.Render(truncateStr(result, 80)),
	)
}

func ShowThinkingBlock(content string) {
	if content == "" {
		return
	}
	fmt.Printf("  %s %s\n",
		StyleThinkingIcon.Render("💭"),
		StyleThinking.Render(truncateStr(content, 100)),
	)
}

func ShowResponseHeader(model string, elapsed time.Duration, tokens int) {
	parts := []string{}
	if model != "" {
		parts = append(parts, StyleMuted.Render(model))
	}
	parts = append(parts, StyleMuted.Render(fmt.Sprintf("%.1fs", elapsed.Seconds())))
	if tokens > 0 {
		parts = append(parts, StyleMuted.Render(fmt.Sprintf("%d tokens", tokens)))
	}
	header := ""
	for i, p := range parts {
		if i > 0 {
			header += StyleMuted.Render(" · ")
		}
		header += p
	}
	fmt.Printf("  %s\n", header)
}

func ShowResponseSeparator() {
	fmt.Printf("  %s\n", StyleMuted.Render("─"+strings.Repeat("─", 50)))
}

func ShowStreamingCursor() {
	fmt.Fprintf(os.Stderr, "▎")
}

func ClearStreamingCursor() {
	fmt.Fprintf(os.Stderr, "\b \b")
}

func truncateStr(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

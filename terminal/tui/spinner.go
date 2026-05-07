package tui

import (
	"encoding/json"
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
	paused   bool
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
	s.paused = false
	s.started = time.Now()
	s.mu.Unlock()

	if !s.isTTY {
		s.mu.Lock()
		phase := thinkingPhases[0]
		label := s.label
		if label == "" {
			label = phase.Label
		}
		fmt.Fprintf(os.Stderr, "  %s %s...\n", Badge("thinking", BadgeThinking), label)
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
				if s.paused {
					s.mu.Unlock()
					continue
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
					Badge("thinking", BadgeThinking),
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

func (s *ThinkingSpinner) Pause() {
	s.mu.Lock()
	s.paused = true
	s.mu.Unlock()
	if s.isTTY {
		fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", 80))
	}
}

func (s *ThinkingSpinner) Resume() {
	s.mu.Lock()
	s.paused = false
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

var currentSpinner *ThinkingSpinner

func SetCurrentSpinner(s *ThinkingSpinner) {
	currentSpinner = s
}

func ShowToolCall(toolName string, params string) {
	if currentSpinner != nil {
		currentSpinner.Pause()
		defer currentSpinner.Resume()
	} else if isTerminal() {
		fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", 80))
	}

	fmt.Printf("  %s %s",
		Badge("tool", BadgeTool),
		StyleToolName.Render(toolName),
	)

	detail := parseToolDetail(toolName, params)
	if detail != "" {
		fmt.Printf(" %s", detail)
	}
	fmt.Println()
}

func ShowToolResult(toolName string, success bool, result string) {
	if currentSpinner != nil {
		currentSpinner.Pause()
		defer currentSpinner.Resume()
	} else if isTerminal() {
		fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", 80))
	}

	var badge string
	if success {
		badge = Badge("done", BadgeSuccess)
	} else {
		badge = Badge("fail", BadgeError)
	}

	fmt.Printf("  %s %s", badge, StyleToolName.Render(toolName))

	if result != "" {
		resultLines := strings.Split(result, "\n")
		maxPreviewLines := 6
		if len(resultLines) <= maxPreviewLines {
			fmt.Println()
			for _, line := range resultLines {
				if line == "" {
					continue
				}
				fmt.Printf("  %s %s\n",
					StyleMuted.Render("│"),
					StyleOutput.Render(truncateStr(line, 120)),
				)
			}
			return
		}
		fmt.Println()
		for i := 0; i < maxPreviewLines; i++ {
			if resultLines[i] == "" {
				continue
			}
			fmt.Printf("  %s %s\n",
				StyleMuted.Render("│"),
				StyleOutput.Render(truncateStr(resultLines[i], 120)),
			)
		}
		remaining := len(resultLines) - maxPreviewLines
		if remaining > 0 {
			fmt.Printf("  %s %s\n",
				StyleMuted.Render("│"),
				StyleDimText.Render(fmt.Sprintf("... +%d more lines", remaining)),
			)
		}
		return
	}
	fmt.Println()
}

func parseToolDetail(toolName string, params string) string {
	var p map[string]interface{}
	if err := json.Unmarshal([]byte(params), &p); err != nil {
		return StyleDimText.Render(truncateStr(params, 60))
	}

	switch toolName {
	case "execute_command", "run_command", "shell", "bash":
		if cmd, ok := p["command"].(string); ok {
			return StyleCommand.Render("$ " + cmd)
		}
	case "read_file", "write_file", "edit_file", "create_file":
		if path, ok := p["path"].(string); ok {
			return StyleFilePath.Render(path)
		}
		if path, ok := p["file_path"].(string); ok {
			return StyleFilePath.Render(path)
		}
		if path, ok := p["filename"].(string); ok {
			return StyleFilePath.Render(path)
		}
	case "search_code", "code_search", "grep":
		if query, ok := p["query"].(string); ok {
			return StyleCommand.Render(query)
		}
		if pattern, ok := p["pattern"].(string); ok {
			return StyleCommand.Render(pattern)
		}
	}

	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	return StyleDimText.Render(truncateStr(strings.Join(keys, ", "), 60))
}

func ShowThinkingBlock(content string) {
	if content == "" {
		return
	}
	if currentSpinner != nil {
		currentSpinner.Pause()
		defer currentSpinner.Resume()
	} else if isTerminal() {
		fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", 80))
	}

	lines := strings.Split(content, "\n")
	maxLines := 8
	for i, line := range lines {
		if i >= maxLines {
			remaining := len(lines) - maxLines
			if remaining > 0 {
				fmt.Printf("  %s %s\n",
					StyleMuted.Render("│"),
					StyleDimText.Render(fmt.Sprintf("... +%d more lines", remaining)),
				)
			}
			break
		}
		truncated := truncateStr(line, 120)
		if truncated == "" {
			continue
		}
		fmt.Printf("  %s %s\n",
			StyleMuted.Render("│"),
			StyleThinking.Render(truncated),
		)
	}
}

type StreamCodeBlockDetector struct {
	inCodeBlock    bool
	codeBlockLang  string
	backtickCount  int
	backtickBuf    string
	lineBuf        string
	blockWidth     int
}

func NewStreamCodeBlockDetector() *StreamCodeBlockDetector {
	w := getTerminalWidth()
	if w <= 0 {
		w = 80
	}
	bw := w - 4
	if bw < 40 {
		bw = 40
	}
	return &StreamCodeBlockDetector{blockWidth: bw}
}

func (d *StreamCodeBlockDetector) ProcessChunk(chunk string) string {
	var result strings.Builder
	for _, ch := range chunk {
		if ch == '`' {
			d.backtickBuf += "`"
			d.backtickCount++
			if d.backtickCount == 3 {
				if !d.inCodeBlock {
					d.inCodeBlock = true
					d.codeBlockLang = strings.TrimSpace(d.lineBuf)
					langLabel := ""
					if d.codeBlockLang != "" {
						langLabel = " " + d.codeBlockLang + " "
					}
					borderLen := d.blockWidth - len(langLabel) - 2
					if borderLen < 4 {
						borderLen = 4
					}
					result.WriteString(StyleDimText.Render("┌" + langLabel + strings.Repeat("─", borderLen) + "┐"))
					result.WriteString("\n")
					d.lineBuf = ""
					d.backtickBuf = ""
					d.backtickCount = 0
				} else {
					d.inCodeBlock = false
					result.WriteString(StyleDimText.Render("└" + strings.Repeat("─", d.blockWidth-2) + "┘"))
					result.WriteString("\n")
					d.lineBuf = ""
					d.backtickBuf = ""
					d.backtickCount = 0
				}
			}
			continue
		}

		if d.backtickCount > 0 && d.backtickCount < 3 {
			if !d.inCodeBlock {
				result.WriteString(d.backtickBuf)
			}
			d.backtickBuf = ""
			d.backtickCount = 0
		}

		if d.inCodeBlock {
			if ch == '\n' {
				prefix := StyleDimText.Render("│ ")
				result.WriteString(prefix)
				result.WriteString(d.lineBuf)
				result.WriteString("\n")
				d.lineBuf = ""
			} else {
				d.lineBuf += string(ch)
			}
		} else {
			if ch == '\n' {
				result.WriteString(d.lineBuf)
				result.WriteString("\n")
				d.lineBuf = ""
			} else {
				d.lineBuf += string(ch)
			}
		}
	}

	if !d.inCodeBlock && d.backtickCount > 0 && d.backtickCount < 3 {
		result.WriteString(d.backtickBuf)
		d.backtickBuf = ""
		d.backtickCount = 0
	}

	return result.String()
}

func (d *StreamCodeBlockDetector) Flush() string {
	var result strings.Builder
	if d.lineBuf != "" {
		if d.inCodeBlock {
			prefix := StyleDimText.Render("│ ")
			result.WriteString(prefix)
			result.WriteString(d.lineBuf)
			result.WriteString("\n")
		} else {
			result.WriteString(d.lineBuf)
		}
		d.lineBuf = ""
	}
	if d.inCodeBlock {
		result.WriteString(StyleDimText.Render("└" + strings.Repeat("─", d.blockWidth-2) + "┘"))
		result.WriteString("\n")
		d.inCodeBlock = false
	}
	return result.String()
}

func ShowResponseHeader(model string, elapsed time.Duration, tokens int) {
	var parts []string
	if model != "" {
		parts = append(parts, Badge(model, BadgeModel))
	}
	parts = append(parts, Badge(fmt.Sprintf("%.1fs", elapsed.Seconds()), BadgeTime))
	if tokens > 0 {
		parts = append(parts, Badge(fmt.Sprintf("%d tok", tokens), BadgeTokens))
	}
	header := strings.Join(parts, " ")
	fmt.Printf("  %s\n", header)
}

func ShowResponseSeparator() {
	w := getTerminalWidth()
	if w <= 0 {
		w = 80
	}
	lineW := w - 4
	if lineW < 20 {
		lineW = 20
	}
	fmt.Printf("  %s\n", StyleDivider.Render("╶"+strings.Repeat("╌", lineW-2)+"╶"))
}

func ShowTurnDivider() {
	w := getTerminalWidth()
	if w <= 0 {
		w = 80
	}
	lineW := w - 4
	if lineW < 20 {
		lineW = 20
	}
	fmt.Printf("\n  %s\n\n",
		StyleDivider.Render("╶"+strings.Repeat("╌", lineW-2)+"╶"),
	)
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

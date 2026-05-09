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

var thinkingPhases = []struct {
	Label string
	Icon  string
}{
	{Label: "understand", Icon: "🔍"},
	{Label: "analyze", Icon: "🧠"},
	{Label: "retrieve", Icon: "📚"},
	{Label: "compose", Icon: "✨"},
}

type ThinkingSpinner struct {
	mu      sync.Mutex
	stopped bool
	phase   int
	label   string
	started time.Time
	frame   int
	frames  []string
	done    chan struct{}
	elapsed time.Duration
	isTTY   bool
	paused  bool
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
		fmt.Fprintf(os.Stderr, "%s\n", RenderThinkingLine(label, 0))
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
				fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", 120))
				return
			case <-ticker.C:
				s.mu.Lock()
				if s.stopped {
					s.mu.Unlock()
					fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", 120))
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
				prefix := R.Border.Render("┊")
				phaseStr := R.Thinking.Render(padVerb(label, verbWidth))
				dur := R.Dim.Render(elapsed)
				fmt.Fprintf(os.Stderr, "\r  %s 🧠 %s %s %s ",
					prefix,
					R.Dim.Render(frame),
					phaseStr,
					dur,
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
		fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", 120))
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
		fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", 120))
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

var toolCallTimers = struct {
	mu     sync.Mutex
	timers map[string]time.Time
}{
	timers: make(map[string]time.Time),
}

func ShowToolCall(toolName string, params string) {
	if currentSpinner != nil {
		currentSpinner.Pause()
		defer currentSpinner.Resume()
	} else if isTerminal() {
		fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", 120))
	}

	toolCallTimers.mu.Lock()
	toolCallTimers.timers[toolName] = time.Now()
	toolCallTimers.mu.Unlock()

	detail := parseToolDetail(toolName, params)
	fmt.Println(RenderToolCallLine(toolName, detail))
}

func ShowToolResult(toolName string, success bool, result string) {
	if currentSpinner != nil {
		currentSpinner.Pause()
		defer currentSpinner.Resume()
	} else if isTerminal() {
		fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", 120))
	}

	toolCallTimers.mu.Lock()
	startTime, exists := toolCallTimers.timers[toolName]
	if exists {
		delete(toolCallTimers.timers, toolName)
	}
	toolCallTimers.mu.Unlock()

	var duration time.Duration
	if exists {
		duration = time.Since(startTime)
	}

	fmt.Println(RenderToolResultLine(toolName, success, duration))

	if result != "" {
		RenderToolResultOutput(result, 8)
	}
}

func parseToolDetail(toolName string, params string) string {
	var p map[string]interface{}
	if err := json.Unmarshal([]byte(params), &p); err != nil {
		return truncateStr(params, 45)
	}

	switch toolName {
	case "execute_command", "run_command", "shell", "bash":
		if cmd, ok := p["command"].(string); ok {
			return "$ " + cmd
		}
	case "read_file", "write_file", "edit_file", "create_file":
		if path, ok := p["path"].(string); ok {
			return path
		}
		if path, ok := p["file_path"].(string); ok {
			return path
		}
		if path, ok := p["filename"].(string); ok {
			return path
		}
	case "search_code", "code_search", "grep":
		if query, ok := p["query"].(string); ok {
			return query
		}
		if pattern, ok := p["pattern"].(string); ok {
			return pattern
		}
	case "get_weather", "weather":
		if city, ok := p["city"].(string); ok {
			return city
		}
		if location, ok := p["location"].(string); ok {
			return location
		}
	}

	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	return truncateStr(strings.Join(keys, ", "), 45)
}

func ShowThinkingBlock(content string) {
	if content == "" {
		return
	}
	if currentSpinner != nil {
		currentSpinner.Pause()
		defer currentSpinner.Resume()
	} else if isTerminal() {
		fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", 120))
	}

	prefix := R.Border.Render("┊")
	lines := strings.Split(content, "\n")
	maxLines := 6
	for i, line := range lines {
		if i >= maxLines {
			remaining := len(lines) - maxLines
			if remaining > 0 {
				fmt.Printf("  %s %s\n", prefix, R.Dim.Render(fmt.Sprintf("… +%d more", remaining)))
			}
			break
		}
		truncated := truncateStr(line, 120)
		if truncated == "" {
			continue
		}
		fmt.Printf("  %s %s\n", prefix, R.Thinking.Render(truncated))
	}
}

type StreamCodeBlockDetector struct {
	inCodeBlock   bool
	codeBlockLang string
	backtickCount int
	backtickBuf   string
	lineBuf       string
	blockWidth    int
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
					result.WriteString(R.Border.Render("┌" + langLabel + strings.Repeat("─", borderLen) + "┐"))
					result.WriteString("\n")
					d.lineBuf = ""
					d.backtickBuf = ""
					d.backtickCount = 0
				} else {
					d.inCodeBlock = false
					result.WriteString(R.Border.Render("└" + strings.Repeat("─", d.blockWidth-2) + "┘"))
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
				prefix := R.Border.Render("│ ")
				result.WriteString(prefix)
				result.WriteString(R.Code.Render(d.lineBuf))
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
			prefix := R.Border.Render("│ ")
			result.WriteString(prefix)
			result.WriteString(R.Code.Render(d.lineBuf))
			result.WriteString("\n")
		} else {
			result.WriteString(d.lineBuf)
		}
		d.lineBuf = ""
	}
	if d.inCodeBlock {
		result.WriteString(R.Border.Render("└" + strings.Repeat("─", d.blockWidth-2) + "┘"))
		result.WriteString("\n")
		d.inCodeBlock = false
	}
	return result.String()
}

func ShowResponseHeader(model string, elapsed time.Duration, tokens int) {
	fmt.Println(RenderResponseHeader(model, elapsed, tokens))
}

func ShowGuardianReview(tool, verdict, riskLevel, reason string) {
	if currentSpinner != nil {
		currentSpinner.Pause()
		defer currentSpinner.Resume()
	} else if isTerminal() {
		fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", 120))
	}
	fmt.Println(RenderGuardianReviewLine(tool, verdict, riskLevel, reason))
}

func ShowResponseSeparator() {
}

func ShowTurnDivider() {
	RenderTurnDivider()
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
	return s[:maxLen] + "…"
}

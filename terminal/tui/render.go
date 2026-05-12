package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	Palette = struct {
		Text        lipgloss.Color
		Dim         lipgloss.Color
		Accent      lipgloss.Color
		AccentSoft  lipgloss.Color
		Border      lipgloss.Color
		UserBg      lipgloss.Color
		UserText    lipgloss.Color
		SystemText  lipgloss.Color
		ToolPending lipgloss.Color
		ToolSuccess lipgloss.Color
		ToolError   lipgloss.Color
		ToolTitle   lipgloss.Color
		ToolOutput  lipgloss.Color
		Quote       lipgloss.Color
		QuoteBorder lipgloss.Color
		Code        lipgloss.Color
		CodeBlock   lipgloss.Color
		CodeBorder  lipgloss.Color
		Link        lipgloss.Color
		Error       lipgloss.Color
		Success     lipgloss.Color
		Warning     lipgloss.Color
		Thinking    lipgloss.Color
		Info        lipgloss.Color
		ModelBadge  lipgloss.Color
	}{
		Text:        "#E8E3D5",
		Dim:         "#7B7F87",
		Accent:      "#F6C453",
		AccentSoft:  "#F2A65A",
		Border:      "#3C414B",
		UserBg:      "#2B2F36",
		UserText:    "#F3EEE0",
		SystemText:  "#9BA3B2",
		ToolPending: "#F6C453",
		ToolSuccess: "#7DD3A5",
		ToolError:   "#F97066",
		ToolOutput:  "#E1DACB",
		Quote:       "#8CC8FF",
		QuoteBorder: "#3B4D6B",
		Code:        "#F0C987",
		CodeBlock:   "#1E232A",
		CodeBorder:  "#343A45",
		Link:        "#7DD3A5",
		Error:       "#F97066",
		Success:     "#7DD3A5",
		Warning:     "#E5C07B",
		Thinking:    "#C678DD",
		Info:        "#61AFEF",
		ModelBadge:  "#5C6370",
	}

	R = struct {
		Dim         lipgloss.Style
		Accent      lipgloss.Style
		AccentSoft  lipgloss.Style
		Bold        lipgloss.Style
		Italic      lipgloss.Style
		Error       lipgloss.Style
		Success     lipgloss.Style
		Warning     lipgloss.Style
		Info        lipgloss.Style
		Thinking    lipgloss.Style
		ToolTitle   lipgloss.Style
		ToolOutput  lipgloss.Style
		ToolPending lipgloss.Style
		ToolSuccess lipgloss.Style
		ToolError   lipgloss.Style
		UserText    lipgloss.Style
		SystemText  lipgloss.Style
		Code        lipgloss.Style
		FilePath    lipgloss.Style
		Command     lipgloss.Style
		Border      lipgloss.Style
		KeyHint     lipgloss.Style
		DescHint    lipgloss.Style
		PromptArrow lipgloss.Style
		PromptText  lipgloss.Style
		ModelBadge  lipgloss.Style
	}{
		Dim:         lipgloss.NewStyle().Foreground(Palette.Dim),
		Accent:      lipgloss.NewStyle().Foreground(Palette.Accent),
		AccentSoft:  lipgloss.NewStyle().Foreground(Palette.AccentSoft),
		Bold:        lipgloss.NewStyle().Bold(true),
		Italic:      lipgloss.NewStyle().Italic(true),
		Error:       lipgloss.NewStyle().Foreground(Palette.Error),
		Success:     lipgloss.NewStyle().Foreground(Palette.Success),
		Warning:     lipgloss.NewStyle().Foreground(Palette.Warning),
		Info:        lipgloss.NewStyle().Foreground(Palette.Info),
		Thinking:    lipgloss.NewStyle().Foreground(Palette.Thinking).Italic(true),
		ToolTitle:   lipgloss.NewStyle().Foreground(Palette.ToolTitle).Bold(true),
		ToolOutput:  lipgloss.NewStyle().Foreground(Palette.ToolOutput),
		ToolPending: lipgloss.NewStyle().Foreground(Palette.ToolPending),
		ToolSuccess: lipgloss.NewStyle().Foreground(Palette.ToolSuccess),
		ToolError:   lipgloss.NewStyle().Foreground(Palette.ToolError),
		UserText:    lipgloss.NewStyle().Foreground(Palette.UserText),
		SystemText:  lipgloss.NewStyle().Foreground(Palette.SystemText),
		Code:        lipgloss.NewStyle().Foreground(Palette.Code),
		FilePath:    lipgloss.NewStyle().Foreground(Palette.AccentSoft).Underline(true),
		Command:     lipgloss.NewStyle().Foreground(Palette.Info).Bold(true),
		Border:      lipgloss.NewStyle().Foreground(Palette.Border),
		KeyHint:     lipgloss.NewStyle().Foreground(Palette.Dim).Bold(true),
		DescHint:    lipgloss.NewStyle().Foreground(Palette.Dim),
		PromptArrow: lipgloss.NewStyle().Foreground(Palette.Success).Bold(true),
		PromptText:  lipgloss.NewStyle().Foreground(Palette.Success),
		ModelBadge:  lipgloss.NewStyle().Foreground(Palette.ModelBadge),
	}
)

const verbWidth = 9

func toolEmoji(toolName string) string {
	emojis := map[string]string{
		"execute_command": "💻", "run_command": "💻", "shell": "💻", "bash": "💻",
		"read_file": "📖", "write_file": "✍️", "edit_file": "🔧", "create_file": "✍️",
		"search_code": "🔎", "code_search": "🔎", "grep": "🔎", "find": "🔎",
		"web_search": "🔍", "web_fetch": "📄", "web_crawl": "🕸️",
		"list_directory": "📁", "get_file_info": "ℹ️",
		"delete_file": "🗑️", "move_file": "📦",
		"get_weather": "🌤️", "weather": "🌤️",
		"compact": "📦", "summarize": "📝",
		"plan": "📋", "analyze": "🔬",
	}
	if e, ok := emojis[toolName]; ok {
		return e
	}
	return "⚡"
}

func toolVerb(toolName string) string {
	verbs := map[string]string{
		"execute_command": "run", "run_command": "run", "shell": "run", "bash": "run",
		"read_file": "read", "write_file": "write", "edit_file": "patch", "create_file": "create",
		"search_code": "grep", "code_search": "grep", "grep": "grep", "find": "find",
		"web_search": "search", "web_fetch": "fetch", "web_crawl": "crawl",
		"list_directory": "ls", "get_file_info": "info",
		"delete_file": "delete", "move_file": "move",
		"get_weather": "weather", "weather": "weather",
		"compact": "compact", "summarize": "summarize",
		"plan": "plan", "analyze": "analyze",
	}
	if v, ok := verbs[toolName]; ok {
		return v
	}
	return toolName
}

func formatTokenCount(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

func padVerb(verb string, width int) string {
	runeCount := len([]rune(verb))
	if runeCount >= width {
		return verb
	}
	return verb + strings.Repeat(" ", width-runeCount)
}

func RenderToolLine(toolName string, detail string, duration time.Duration, status string) string {
	emoji := toolEmoji(toolName)
	verb := toolVerb(toolName)
	dur := ""
	if duration > 0 {
		dur = R.Dim.Render(fmt.Sprintf("%.1fs", duration.Seconds()))
	}

	var statusIcon string
	var statusStyle lipgloss.Style
	switch status {
	case "running":
		statusIcon = "⏳"
		statusStyle = R.ToolPending
	case "done":
		statusIcon = "✓"
		statusStyle = R.ToolSuccess
	case "fail":
		statusIcon = "✗"
		statusStyle = R.ToolError
	default:
		statusIcon = "→"
		statusStyle = R.Dim
	}

	prefix := R.Border.Render("┊")
	verbStr := statusStyle.Render(padVerb(verb, verbWidth))
	detailStr := ""
	if detail != "" {
		detailStr = R.Dim.Render(truncateStr(detail, 45))
	}

	parts := []string{prefix, emoji, verbStr}
	if detailStr != "" {
		parts = append(parts, detailStr)
	}
	if dur != "" {
		parts = append(parts, dur)
	}
	parts = append(parts, statusStyle.Render(statusIcon))

	return strings.Join(parts, " ")
}

func RenderToolCallLine(toolName string, detail string) string {
	emoji := toolEmoji(toolName)
	verb := toolVerb(toolName)
	prefix := R.Border.Render("┊")
	verbStr := R.ToolPending.Render(padVerb(verb, verbWidth))
	detailStr := ""
	if detail != "" {
		detailStr = R.Dim.Render(truncateStr(detail, 45))
	}
	spinner := R.ToolPending.Render("⏳")

	parts := []string{prefix, emoji, verbStr}
	if detailStr != "" {
		parts = append(parts, detailStr)
	}
	parts = append(parts, spinner)

	return strings.Join(parts, " ")
}

func RenderToolResultLine(toolName string, success bool, duration time.Duration) string {
	emoji := toolEmoji(toolName)
	verb := toolVerb(toolName)
	prefix := R.Border.Render("┊")
	dur := ""
	if duration > 0 {
		dur = R.Dim.Render(fmt.Sprintf("%.1fs", duration.Seconds()))
	}

	if success {
		verbStr := R.ToolSuccess.Render(padVerb(verb, verbWidth))
		check := R.ToolSuccess.Render("✓")
		parts := []string{prefix, emoji, verbStr}
		if dur != "" {
			parts = append(parts, dur)
		}
		parts = append(parts, check)
		return strings.Join(parts, " ")
	}

	verbStr := R.ToolError.Render(padVerb(verb, verbWidth))
	cross := R.ToolError.Render("✗")
	parts := []string{prefix, emoji, verbStr}
	if dur != "" {
		parts = append(parts, dur)
	}
	parts = append(parts, cross)
	return strings.Join(parts, " ")
}

func RenderThinkingLine(phase string, elapsed time.Duration) string {
	prefix := R.Border.Render("┊")
	phaseStr := R.Thinking.Render(padVerb(phase, verbWidth))
	dur := R.Dim.Render(fmt.Sprintf("%.1fs", elapsed.Seconds()))
	return fmt.Sprintf("%s 🧠 %s %s", prefix, phaseStr, dur)
}

func RenderResponseHeader(model string, elapsed time.Duration, tokens int) string {
	w := getTerminalWidth()
	if w <= 0 {
		w = 80
	}

	modelLabel := model
	if modelLabel == "" {
		modelLabel = "assistant"
	}

	label := " " + modelLabel + " "
	labelRendered := R.ModelBadge.Render(label)
	labelLen := len(modelLabel) + 2
	remaining := w - 4 - labelLen
	if remaining < 4 {
		remaining = 4
	}

	leftBorder := R.Border.Render("┌")
	rightBorder := R.Border.Render("┐")
	dashes := R.Border.Render(strings.Repeat("─", remaining))

	return fmt.Sprintf("  %s%s%s%s", leftBorder, labelRendered, dashes, rightBorder)
}

func RenderResponseFooter(model string, elapsed time.Duration, tokens int) string {
	var parts []string
	if model != "" {
		parts = append(parts, R.Dim.Render(model))
	}
	if tokens > 0 {
		parts = append(parts, R.Dim.Render(formatTokenCount(tokens)+" tok"))
	}
	parts = append(parts, R.Dim.Render(fmt.Sprintf("%.1fs", elapsed.Seconds())))
	return "  " + strings.Join(parts, R.Border.Render(" · "))
}

func RenderResponseBoxBottom() string {
	w := getTerminalWidth()
	if w <= 0 {
		w = 80
	}
	lineW := w - 4
	if lineW < 20 {
		lineW = 20
	}
	return fmt.Sprintf("  %s", R.Border.Render("└"+strings.Repeat("─", lineW)+"┘"))
}

func RenderContextUsage(promptTokens, completionTokens, totalTokens int, contextWindow int) string {
	if totalTokens <= 0 {
		return ""
	}

	pct := 0
	if contextWindow > 0 {
		pct = promptTokens * 100 / contextWindow
	} else if promptTokens > 0 && totalTokens > 0 {
		pct = promptTokens * 100 / totalTokens
	}

	var barWidth int = 20
	filled := pct * barWidth / 100
	if filled > barWidth {
		filled = barWidth
	}

	var barColor lipgloss.Color
	switch {
	case pct > 80:
		barColor = Palette.Error
	case pct > 60:
		barColor = Palette.Warning
	default:
		barColor = Palette.Success
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	barRendered := lipgloss.NewStyle().Foreground(barColor).Render(bar)

	pctStr := fmt.Sprintf("%d%%", pct)
	pctRendered := lipgloss.NewStyle().Foreground(barColor).Bold(true).Render(pctStr)

	promptStr := R.Dim.Render(formatTokenCount(promptTokens) + " in")
	compStr := R.Dim.Render(formatTokenCount(completionTokens) + " out")

	return fmt.Sprintf("  %s %s %s  %s %s", barRendered, pctRendered, R.Border.Render("│"), promptStr, compStr)
}

func RenderContextWarning(pct int) string {
	if pct > 80 {
		return fmt.Sprintf("  %s %s %s",
			R.Error.Render("⚠"),
			R.Error.Render("Context near limit!"),
			R.Dim.Render("Use /compact to compress or /new to start fresh"),
		)
	}
	if pct > 60 {
		return fmt.Sprintf("  %s %s %s",
			R.Warning.Render("💡"),
			R.Warning.Render("Context getting large."),
			R.Dim.Render("Use /compact to compress"),
		)
	}
	return ""
}

func RenderCompactLine(reason string, beforeMsgs, afterMsgs, savedTokens int) string {
	prefix := R.Border.Render("┊")
	reasonLabel := "compact"
	if reason == "llm_summarization" {
		reasonLabel = "summarize"
	}
	verbStr := R.Accent.Render(padVerb(reasonLabel, verbWidth))
	saved := R.Dim.Render(fmt.Sprintf("saved ~%s tokens", formatTokenCount(savedTokens)))
	msgs := R.Dim.Render(fmt.Sprintf("%d→%d msgs", beforeMsgs, afterMsgs))
	return fmt.Sprintf("%s 📦 %s %s %s", prefix, verbStr, msgs, saved)
}

func RenderTurnDivider() {
	w := getTerminalWidth()
	if w <= 0 {
		w = 80
	}
	lineW := w - 4
	if lineW < 20 {
		lineW = 20
	}
	fmt.Printf("\n  %s\n\n", R.Border.Render("╶"+strings.Repeat("╌", lineW-2)+"╶"))
}

func RenderWelcome(apiURL, model string, stream bool) {
	fmt.Println()

	logoStyle := lipgloss.NewStyle().
		Foreground(Palette.Accent).
		Bold(true)

	logo := `
  ┌──────────────────────────────────┐
  │   ___  ____  ____  ____  _  _   │
  │  / _ \|  _ \|  _ \|  _ \| || |  │
  │ | | | | |_) | |_) | |_) | || |_ │
  │ | |_| |  __/|  __/|  _ <|__   _|│
  │  \___/|_|   |_|   |_| \_\  |_|  │
  └──────────────────────────────────┘`
	fmt.Println(logoStyle.Render(logo))

	fmt.Printf("  %s %s\n", R.Dim.Render("API"), R.Dim.Render(apiURL))
	if model != "" {
		fmt.Printf("  %s %s\n", R.Dim.Render("Model"), R.Accent.Render(model))
	} else {
		fmt.Printf("  %s %s\n", R.Dim.Render("Model"), R.Warning.Render("(no model configured)"))
	}
	modeLabel := "streaming"
	if !stream {
		modeLabel = "sync"
	}
	fmt.Printf("  %s %s\n", R.Dim.Render("Mode"), R.Dim.Render(modeLabel))

	fmt.Println()
	fmt.Println(R.Dim.Render("  /help for commands · /memory for project memory · /skill for workflows · Tab to autocomplete"))
	fmt.Println()
}

func RenderGoodbye() {
	fmt.Printf("\n  %s %s\n", R.Accent.Render("bye"), R.Dim.Render("See you next time!"))
}

func RenderHelp() {
	fmt.Println()

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(Palette.Accent).
		MarginBottom(1)
	fmt.Printf("  %s\n\n", headerStyle.Render("Commands"))

	commands := []struct {
		key  string
		desc string
	}{
		{"/new", "Start a new conversation"},
		{"/help", "Show this help"},
		{"/model", "Interactive model selector"},
		{"/model <name>", "Set model directly"},
		{"/stream", "Toggle streaming (on/off)"},
		{"/mode", "Show tool mode selector"},
		{"/mode <name>", "Set tool mode (build/explore/plan/general)"},
		{"/compact", "Compress context (smart summary)"},
		{"/compact <instructions>", "Compress with custom instructions"},
		{"/context", "Show context usage & chapter overview"},
		{"/chapters", "View conversation chapter outline"},
		{"/clear", "Clear conversation context"},
		{"/sessions", "List and switch sessions"},
		{"/sessions <n>", "Switch to session #n"},
		{"/project", "Show current project info"},
		{"/project list", "List all projects"},
		{"/project switch", "Interactive project selector"},
		{"/project switch <n|name>", "Switch to a project directly"},
		{"/project create <name>", "Create a new project"},
		{"/project delete <n|name>", "Delete a project"},
		{"/workspace", "Show workspace status"},
		{"/workspace init", "Bind current project to directory"},
		{"/workspace clear", "Remove workspace marker"},
		{"/history", "Show message history"},
		{"/copy", "Copy last response to clipboard"},
		{"/redo", "Retry last user message"},
		{"/plan <task>", "Plan mode (read-only analysis)"},
		{"/undo", "Show git changes"},
		{"/undo all", "Revert all modified files"},
		{"/undo clean", "Revert + remove untracked"},
		{"/permissions", "Show/set permission mode"},
		{"/fork <name>", "Fork session at current point"},
		{"/branches", "List session branches"},
		{"/memories", "List persistent memories"},
		{"/memories add <k> <v>", "Save a memory"},
		{"/memory", "Show memory status"},
		{"/memory init", "Create OPENAIDE.md project memory"},
		{"/memory save <type> <text>", "Save local memory entry"},
		{"/memory search <query>", "Search relevant memories"},
		{"/memory extract", "Extract memories from conversation"},
		{"/skill", "List available skills"},
		{"/skill <name> [args]", "Run a skill workflow"},
		{"/skill create <name>", "Create a new skill"},
		{"/feedback <pos|neg> [comment]", "Give feedback on last response"},
		{"/cost [days]", "Show cost summary"},
		{"/cd <dir>", "Change working directory"},
		{"/config", "Open settings wizard"},
		{"/dashboard", "Open dashboard"},
		{"!command", "Execute shell command directly"},
		{"@file.ext", "Include file content in message"},
		{"exit, /exit", "Exit chat session"},
	}

	for _, cmd := range commands {
		fmt.Printf("  %s  %s\n",
			R.KeyHint.Render(cmd.key),
			R.DescHint.Render(cmd.desc))
	}
	fmt.Printf("\n  %s\n", R.Dim.Render("Tip: Tab to autocomplete · !cmd to execute shell · Enter empty line to send"))
	fmt.Println()
}

func RenderStatusLine(parts ...string) string {
	sep := R.Border.Render(" · ")
	return R.Dim.Render("  " + strings.Join(parts, sep))
}

func RenderSessionLine(idx int, title, shortID, updated string, isCurrent bool) string {
	num := R.Dim.Render(fmt.Sprintf("%2d.", idx))
	t := R.Accent.Render(truncateStr(title, 40))
	id := R.Dim.Render("#" + shortID)
	ts := R.Dim.Render(updated)
	current := ""
	if isCurrent {
		current = " " + R.Success.Render("●")
	}
	return fmt.Sprintf("  %s %s %s %s%s", num, t, id, ts, current)
}

func RenderHistoryMessage(sender, summary string) {
	prefix := R.Border.Render("┊")
	if sender == "user" {
		fmt.Printf("  %s %s %s\n", prefix, R.Info.Render("you"), R.Dim.Render(truncateStr(summary, 80)))
	} else {
		fmt.Printf("  %s %s %s\n", prefix, R.Accent.Render("ai"), R.Dim.Render(truncateStr(summary, 80)))
	}
}

func RenderToolResultOutput(result string, maxLines int) {
	if result == "" {
		return
	}
	if maxLines <= 0 {
		maxLines = 8
	}
	lines := strings.Split(result, "\n")
	prefix := R.Border.Render("┊")
	for i, line := range lines {
		if line == "" {
			continue
		}
		if i >= maxLines {
			remaining := len(lines) - maxLines
			fmt.Printf("  %s %s\n", prefix, R.Dim.Render(fmt.Sprintf("… +%d more lines", remaining)))
			return
		}
		fmt.Printf("  %s %s\n", prefix, R.ToolOutput.Render(truncateStr(line, 120)))
	}
}

func RenderErrorBlock(err string) {
	prefix := R.Border.Render("┊")
	fmt.Printf("  %s %s %s\n", prefix, R.Error.Render("✗"), R.Error.Render(truncateStr(err, 100)))
}

func RenderSuccessLine(msg string) {
	prefix := R.Border.Render("┊")
	fmt.Printf("  %s %s %s\n", prefix, R.Success.Render("✓"), R.Dim.Render(msg))
}

func RenderInfoLine(msg string) {
	prefix := R.Border.Render("┊")
	fmt.Printf("  %s %s %s\n", prefix, R.Info.Render("ℹ"), R.Dim.Render(msg))
}

func RenderWarningLine(msg string) {
	prefix := R.Border.Render("┊")
	fmt.Printf("  %s %s %s\n", prefix, R.Warning.Render("⚠"), R.Dim.Render(msg))
}

func RenderGuardianReviewLine(tool, verdict, riskLevel, reason string) string {
	prefix := R.Border.Render("┊")
	emoji := toolEmoji(tool)
	verb := toolVerb(tool)
	verbStr := padVerb(verb, verbWidth)

	var verdictIcon string
	var verdictStyle lipgloss.Style
	switch verdict {
	case "allow":
		verdictIcon = "✓"
		verdictStyle = R.ToolSuccess
	case "confirm":
		verdictIcon = "?"
		verdictStyle = R.Warning
	case "deny":
		verdictIcon = "✗"
		verdictStyle = R.ToolError
	default:
		verdictIcon = "?"
		verdictStyle = R.Dim
	}

	var riskBadge string
	switch riskLevel {
	case "critical":
		riskBadge = R.Error.Render("● CRITICAL")
	case "high":
		riskBadge = R.Error.Render("● HIGH")
	case "medium":
		riskBadge = R.Warning.Render("● MED")
	case "low":
		riskBadge = R.Info.Render("● LOW")
	default:
		riskBadge = R.Dim.Render("● " + riskLevel)
	}

	parts := []string{prefix, "🛡️", emoji, verbStyle(verdict, verbStr), riskBadge, verdictStyle.Render(verdictIcon)}
	if reason != "" {
		parts = append(parts, R.Dim.Render(truncateStr(reason, 50)))
	}
	return strings.Join(parts, " ")
}

func verbStyle(verdict string, verbStr string) string {
	switch verdict {
	case "allow":
		return R.ToolSuccess.Render(verbStr)
	case "confirm":
		return R.Warning.Render(verbStr)
	case "deny":
		return R.ToolError.Render(verbStr)
	default:
		return R.Dim.Render(verbStr)
	}
}

func RenderToolCallLineWithRisk(toolName string, detail string, riskLevel string) string {
	emoji := toolEmoji(toolName)
	verb := toolVerb(toolName)
	prefix := R.Border.Render("┊")
	verbStr := R.ToolPending.Render(padVerb(verb, verbWidth))
	detailStr := ""
	if detail != "" {
		detailStr = R.Dim.Render(truncateStr(detail, 45))
	}
	spinner := R.ToolPending.Render("⏳")

	var riskBadge string
	switch riskLevel {
	case "high", "critical":
		riskBadge = R.Error.Render("●")
	case "medium":
		riskBadge = R.Warning.Render("●")
	default:
		riskBadge = ""
	}

	parts := []string{prefix, emoji, verbStr}
	if riskBadge != "" {
		parts = append(parts, riskBadge)
	}
	if detailStr != "" {
		parts = append(parts, detailStr)
	}
	parts = append(parts, spinner)

	return strings.Join(parts, " ")
}

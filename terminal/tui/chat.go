package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/chzyer/readline"
)

var rl *readline.Instance

func RunChat(cfg *Config, apiURL, model string, stream bool) {
	if err := InitMarkdownRenderer(); err == nil {
	}

	initReadline()
	defer closeReadline()

	renderWelcome(apiURL, model, stream)

	dialogueID := ""
	var history []Message

	dialogues, _ := FetchDialogues(apiURL, "cli-user")
	if len(dialogues) > 0 {
		latest := dialogues[0]
		for _, d := range dialogues {
			if d.UpdatedAt > latest.UpdatedAt {
				latest = d
			}
		}
		messages, err := FetchMessages(apiURL, latest.ID)
		if err == nil && len(messages) > 0 {
			dialogueID = latest.ID
			history = messages
			shortID := latest.ID
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			title := latest.Title
			if title == "" || title == "CLI Chat" {
				title = "Conversation"
			}
			fmt.Printf("  %s %s %s %s\n",
				Badge("resume", BadgeStream),
				StyleSectionValue.Render(title),
				StyleMuted.Render("#"+shortID),
				StyleDimText.Render(fmt.Sprintf("(%d messages)", len(messages))),
			)
			fmt.Println()
		}
	}

	if dialogueID == "" {
		dialogue, err := CreateDialogue(apiURL)
		if err != nil {
			errBlock := StyleErrorBlock.Render(fmt.Sprintf(" Failed to create dialogue: %v ", err))
			fmt.Fprintf(os.Stderr, "  %s\n", errBlock)
		} else {
			dialogueID = dialogue.ID
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	go func() {
		for range sigCh {
			cancel()
			ctx, cancel = context.WithCancel(context.Background())
		}
	}()

	for {
		input, err := readLine()
		if err != nil {
			if err == io.EOF {
				fmt.Println()
				renderGoodbye()
				return
			}
			if err == readline.ErrInterrupt {
				fmt.Printf("\n  %s Interrupted\n", Badge("⏹", BadgeError))
				continue
			}
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		if input == "exit" || input == "quit" || input == "/exit" || input == "/quit" {
			renderGoodbye()
			return
		}

		if strings.HasPrefix(input, "/") {
			var newDialogueID string
			newDialogueID, model, stream = handleSlashCommand(input, apiURL, dialogueID, model, stream, &history, cfg, ctx)
			if newDialogueID != "" {
				dialogueID = newDialogueID
			}
			continue
		}

		if strings.HasPrefix(input, "!") {
			shellCmd := strings.TrimPrefix(input, "!")
			shellCmd = strings.TrimSpace(shellCmd)
			if shellCmd == "" {
				continue
			}
			executeShellCommand(ctx, apiURL, dialogueID, model, shellCmd)
			continue
		}

		userMsg := Message{
			ID:         GenerateID(),
			DialogueID: dialogueID,
			Sender:     "user",
			Content:    input,
		}
		history = append(history, userMsg)

		fmt.Println()
		var response string
		timeout := GetTimeout(cfg)

		if stream {
			response, err = runStreamChat(ctx, apiURL, dialogueID, input, model, timeout)
		} else {
			response, err = runSyncChat(apiURL, dialogueID, input, model)
		}

		if err != nil {
			if ctx.Err() == context.Canceled {
				fmt.Printf("\n  %s Response interrupted\n", Badge("⏹", BadgeWarning))
			} else {
				errBlock := StyleErrorBlock.Render(fmt.Sprintf(" %v ", err))
				fmt.Printf("\n  %s\n", errBlock)
			}
			fmt.Println()
			continue
		}

		if response != "" {
			if !stream {
				fmt.Print(RenderMarkdown(response))
			}

			asstMsg := Message{
				ID:         GenerateID(),
				DialogueID: dialogueID,
				Sender:     "assistant",
				Content:    response,
			}
			history = append(history, asstMsg)
		}

		if limit := cfg.Chat.ContextLimit; limit > 0 && len(history) > limit*2 {
			history = history[len(history)-limit*2:]
		}

		ShowTurnDivider()
	}
}

func runStreamChat(ctx context.Context, apiURL, dialogueID, input, model string, timeout int) (string, error) {
	spinner := StartThinking("")
	SetCurrentSpinner(spinner)
	firstContent := true
	var elapsed time.Duration
	usedModel := model
	thinkingLineCount := 0
	var streamUsage *StreamUsage
	codeDetector := NewStreamCodeBlockDetector()

	cb := &StreamCallbacks{
		OnThinking: func(content string) {
			spinner.UpdateLabel("思考中...")
			thinkingLineCount++
			ShowThinkingBlock(content)
		},
		OnToolCall: func(tool string, params string) {
			spinner.UpdateLabel("调用工具: " + tool)
			ShowToolCall(tool, params)
		},
		OnToolDone: func(tool string, result string) {
			ShowToolResult(tool, true, result)
		},
		OnContent: func(chunk string) {
			if firstContent {
				elapsed = spinner.Stop()
				SetCurrentSpinner(nil)
				firstContent = false
				if thinkingLineCount > 0 {
					fmt.Println()
				}
				ShowResponseHeader(usedModel, elapsed, 0)
				ShowResponseSeparator()
			}
			fmt.Print(codeDetector.ProcessChunk(chunk))
		},
		OnDone: func(m string, usage *StreamUsage) {
			if m != "" {
				usedModel = m
			}
			if usage != nil {
				streamUsage = usage
			}
		},
		OnCompact: func(reason string, beforeMsgs, afterMsgs, savedTokens int) {
			reasonLabel := "压缩"
			if reason == "llm_summarization" {
				reasonLabel = "摘要压缩"
			}
			fmt.Printf("\n  %s %s %s → %s messages, %s tokens saved\n",
				Badge("compact", BadgeWarning),
				StyleDimText.Render(reasonLabel),
				StyleDimText.Render(fmt.Sprintf("%d", beforeMsgs)),
				StyleDimText.Render(fmt.Sprintf("%d", afterMsgs)),
				StyleDimText.Render(fmt.Sprintf("~%d", savedTokens)),
			)
		},
	}

	response, err := SendMessageStream(ctx, apiURL, dialogueID, input, model, timeout, cb)
	fmt.Print(codeDetector.Flush())
	if firstContent {
		elapsed = spinner.Stop()
		SetCurrentSpinner(nil)
		if response != "" {
			if thinkingLineCount > 0 {
				fmt.Println()
			}
			ShowResponseHeader(usedModel, elapsed, 0)
			ShowResponseSeparator()
		}
	}

	if response != "" {
		fmt.Println()
		var tokens int
		if streamUsage != nil && streamUsage.CompletionTokens > 0 {
			tokens = streamUsage.CompletionTokens
		} else {
			tokens = estimateTokens(response)
		}
		ShowResponseFooter(usedModel, elapsed, tokens)
		if streamUsage != nil && streamUsage.PromptTokens > 0 {
			totalTokens := streamUsage.PromptTokens + streamUsage.CompletionTokens
			contextPercent := 0
			if streamUsage.TotalTokens > 0 {
				contextPercent = streamUsage.PromptTokens * 100 / (streamUsage.PromptTokens + streamUsage.CompletionTokens)
			}
			contextBadge := BadgeTokens
			if contextPercent > 80 {
				contextBadge = BadgeError
			} else if contextPercent > 60 {
				contextBadge = BadgeWarning
			}
			fmt.Printf("  %s %s prompt + %s completion = %s total  %s\n",
				Badge("ctx", contextBadge),
				StyleDimText.Render(fmt.Sprintf("%d", streamUsage.PromptTokens)),
				StyleDimText.Render(fmt.Sprintf("%d", streamUsage.CompletionTokens)),
				StyleDimText.Render(fmt.Sprintf("%d", totalTokens)),
				StyleDimText.Render(fmt.Sprintf("(%d%% context)", contextPercent)),
			)
			if contextPercent > 80 {
				fmt.Printf("  %s Context near limit! Use %s to compress or %s to start fresh\n",
					Badge("⚠", BadgeError),
					Badge("/compact", BadgeKeyHint),
					Badge("/new", BadgeKeyHint),
				)
			} else if contextPercent > 60 {
				fmt.Printf("  %s Context getting large. Use %s to compress\n",
					Badge("💡", BadgeWarning),
					Badge("/compact", BadgeKeyHint),
				)
			}
		}
	}

	return response, err
}

func runSyncChat(apiURL, dialogueID, input, model string) (string, error) {
	spinner := StartThinking("")
	SetCurrentSpinner(spinner)
	response, err := SendMessage(apiURL, dialogueID, input, model)
	elapsed := spinner.Stop()
	SetCurrentSpinner(nil)

	if err != nil {
		return "", err
	}

	if response != "" {
		ShowResponseHeader(model, elapsed, 0)
		ShowResponseSeparator()
		ShowResponseFooter(model, elapsed, 0)
	}

	return response, nil
}

func ShowResponseFooter(model string, elapsed time.Duration, tokens int) {
	var parts []string
	if tokens > 0 {
		parts = append(parts, Badge(fmt.Sprintf("%d tok", tokens), BadgeTokens))
	}
	if elapsed > 0 {
		parts = append(parts, Badge(fmt.Sprintf("%.1fs", elapsed.Seconds()), BadgeTime))
	}
	if model != "" {
		parts = append(parts, Badge(model, BadgeModel))
	}
	if len(parts) > 0 {
		footer := strings.Join(parts, " ")
		fmt.Printf("  %s\n", footer)
	}
}

func handleSlashCommand(cmd, apiURL, dialogueID string, model string, stream bool, history *[]Message, cfg *Config, ctx context.Context) (string, string, bool) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", model, stream
	}

	newDialogueID := ""

	switch parts[0] {
	case "/new":
		var carrySummary string
		if len(*history) > 4 {
			var summaryParts []string
			for _, m := range *history {
				if m.Sender == "user" {
					short := strings.ReplaceAll(m.Content, "\n", " ")
					if len(short) > 80 {
						short = short[:77] + "..."
					}
					summaryParts = append(summaryParts, "用户: "+short)
				}
			}
			if len(summaryParts) > 0 {
				if len(summaryParts) > 5 {
					summaryParts = summaryParts[len(summaryParts)-5:]
				}
				carrySummary = "[上一对话摘要] " + strings.Join(summaryParts, "; ") + " [请基于此上下文继续]"
			}
		}
		dialogue, err := CreateDialogue(apiURL)
		if err != nil {
			fmt.Printf("  %s Failed: %v\n", Badge("✗", BadgeError), err)
		} else {
			newDialogueID = dialogue.ID
			*history = []Message{}
			if carrySummary != "" {
				_, _ = SendMessage(apiURL, dialogue.ID, carrySummary, model)
				*history = append(*history, Message{
					ID:         GenerateID(),
					DialogueID: dialogue.ID,
					Sender:     "user",
					Content:    carrySummary,
				})
				fmt.Printf("  %s New conversation started %s %s\n",
					Badge("✓", BadgeSuccess),
					StyleDimText.Render(dialogue.ID[:8]+"..."),
					StyleDimText.Render("(carried context from previous chat)"),
				)
			} else {
				fmt.Printf("  %s New conversation started %s\n", Badge("✓", BadgeSuccess), StyleDimText.Render(dialogue.ID[:8]+"..."))
			}
		}
	case "/clear":
		*history = []Message{}
		fmt.Printf("  %s Context cleared\n", Badge("✓", BadgeSuccess))
	case "/model":
		if len(parts) > 1 {
			model = parts[1]
			fmt.Printf("  %s Model set to %s\n", Badge("✓", BadgeSuccess), Badge(model, BadgeModel))
		} else {
			result := RunModelSelect(apiURL, model)
			if result.Changed {
				model = result.Selected
				fmt.Printf("  %s Model set to %s\n", Badge("✓", BadgeSuccess), Badge(model, BadgeModel))
			}
		}
	case "/stream":
		if len(parts) > 1 {
			if parts[1] == "on" {
				stream = true
				fmt.Printf("  %s Streaming %s\n", Badge("✓", BadgeSuccess), Badge("streaming", BadgeStream))
			} else if parts[1] == "off" {
				stream = false
				fmt.Printf("  %s Streaming %s\n", Badge("✓", BadgeSuccess), Badge("sync", BadgeTime))
			}
		} else {
			stream = !stream
			if stream {
				fmt.Printf("  %s Streaming %s\n", Badge("✓", BadgeSuccess), Badge("streaming", BadgeStream))
			} else {
				fmt.Printf("  %s Streaming %s\n", Badge("✓", BadgeSuccess), Badge("sync", BadgeTime))
			}
		}
	case "/history":
		userCount := 0
		asstCount := 0
		for _, m := range *history {
			if m.Sender == "user" {
				userCount++
			} else if m.Sender == "assistant" {
				asstCount++
			}
		}
		fmt.Printf("  %s %s messages (%s user · %s assistant)\n",
			Badge("history", BadgeTime),
			StyleBold.Render(fmt.Sprintf("%d", len(*history))),
			StyleDimText.Render(fmt.Sprintf("%d", userCount)),
			StyleDimText.Render(fmt.Sprintf("%d", asstCount)),
		)
		showCount := 6
		start := len(*history) - showCount
		if start < 0 {
			start = 0
		}
		if len(*history) > showCount {
			fmt.Printf("  %s\n", StyleDimText.Render(fmt.Sprintf("... showing last %d messages", showCount)))
		}
		for i := start; i < len(*history); i++ {
			m := (*history)[i]
			summary := strings.ReplaceAll(m.Content, "\n", " ")
			if len(summary) > 60 {
				summary = summary[:57] + "..."
			}
			if m.Sender == "user" {
				fmt.Printf("  %s %s\n", Badge("you", BadgeUser), StyleDimText.Render(summary))
			} else {
				fmt.Printf("  %s %s\n", Badge("ai", BadgeAssistant), StyleDimText.Render(summary))
			}
		}
	case "/copy":
		lastAsst := ""
		for i := len(*history) - 1; i >= 0; i-- {
			if (*history)[i].Sender == "assistant" {
				lastAsst = (*history)[i].Content
				break
			}
		}
		if lastAsst == "" {
			fmt.Printf("  %s No assistant response to copy\n", Badge("✗", BadgeError))
		} else {
			if err := copyToClipboard(lastAsst); err != nil {
				fmt.Printf("  %s Copy failed: %v\n", Badge("✗", BadgeError), err)
			} else {
				preview := strings.ReplaceAll(lastAsst, "\n", " ")
				if len(preview) > 40 {
					preview = preview[:37] + "..."
				}
				fmt.Printf("  %s Copied to clipboard: %s\n", Badge("✓", BadgeSuccess), StyleDimText.Render(preview))
			}
		}
	case "/cd":
		if len(parts) > 1 {
			dir := parts[1]
			if dir == "~" {
				homeDir, _ := os.UserHomeDir()
				dir = homeDir
			}
			if err := os.Chdir(dir); err != nil {
				fmt.Printf("  %s %v\n", Badge("✗", BadgeError), err)
			} else {
				wd, _ := os.Getwd()
				fmt.Printf("  %s %s\n", Badge("✓", BadgeSuccess), StyleFilePath.Render(wd))
			}
		} else {
			wd, _ := os.Getwd()
			fmt.Printf("  %s\n", StyleFilePath.Render(wd))
		}
	case "/compact":
		if dialogueID == "" {
			fmt.Printf("  %s No active conversation\n", Badge("✗", BadgeError))
		} else {
			fmt.Printf("  %s Compressing context...\n", Badge("⏳", BadgeStream))
			result, err := CompactContext(apiURL, dialogueID)
			if err != nil {
				fmt.Printf("  %s Compact failed: %v\n", Badge("✗", BadgeError), err)
			} else {
				if success, ok := result["success"].(bool); ok && success {
					fmt.Printf("  %s Context compressed successfully\n", Badge("✓", BadgeSuccess))
				} else {
					fmt.Printf("  %s Compact completed\n", Badge("✓", BadgeSuccess))
				}
			}
		}
	case "/redo":
		lastUser := ""
		for i := len(*history) - 1; i >= 0; i-- {
			if (*history)[i].Sender == "user" {
				lastUser = (*history)[i].Content
				*history = (*history)[:i]
				break
			}
		}
		if lastUser == "" {
			fmt.Printf("  %s No user message to redo\n", Badge("✗", BadgeError))
		} else {
			fmt.Printf("  %s Retrying: %s\n", Badge("↻", BadgeStream), StyleDimText.Render(truncateStr(lastUser, 60)))
			fmt.Println()
			var response string
			timeout := GetTimeout(cfg)
			var err error
			if stream {
				response, err = runStreamChat(ctx, apiURL, dialogueID, lastUser, model, timeout)
			} else {
				response, err = runSyncChat(apiURL, dialogueID, lastUser, model)
			}
			if err != nil {
				if ctx.Err() == context.Canceled {
					fmt.Printf("\n  %s Response interrupted\n", Badge("⏹", BadgeWarning))
				} else {
					errBlock := StyleErrorBlock.Render(fmt.Sprintf(" %v ", err))
					fmt.Printf("\n  %s\n", errBlock)
				}
			} else if response != "" {
				if !stream {
					fmt.Print(RenderMarkdown(response))
				}
				asstMsg := Message{
					ID:         GenerateID(),
					DialogueID: dialogueID,
					Sender:     "assistant",
					Content:    response,
				}
				*history = append(*history, asstMsg)
			}
			ShowTurnDivider()
		}
	case "/config":
		result := RunSettings(cfg, getConfigPath(cfg))
		if result.Saved && result.Config != nil {
			cfg = result.Config
			stream = cfg.Chat.Stream
			fmt.Printf("  %s Configuration saved\n", Badge("✓", BadgeSuccess))
		}
	case "/dashboard":
		action := RunDashboard(apiURL, "")
		if action.Action == "select_model" && action.Model != "" {
			model = action.Model
		}
	case "/help":
		printChatHelp()
	default:
		fmt.Printf("  %s Unknown command: %s\n", Badge("✗", BadgeError), parts[0])
		fmt.Printf("  Type %s for available commands\n", StyleKeyHint.Render("/help"))
	}

	return newDialogueID, model, stream
}

func getConfigPath(cfg *Config) string {
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		return ""
	}
	return homeDir + "/.openaide/config.yaml"
}

func renderWelcome(apiURL, model string, stream bool) {
	fmt.Println()

	logoStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary)

	logo := `
  ┌──────────────────────────┐
  │   ▄▄▄▄ ▄▄   ▄▄ ▄▄▄▄▄   │
  │  █    ██   ██   █       │
  │  █    ██   ██   █▄▄▄    │
  │  █    ██   ██   █       │
  │  ▀▄▄▄█ ▀▀██▀▀  ▀▄▄▄▄▄  │
  └──────────────────────────┘`
	fmt.Println(logoStyle.Render(logo))

	fmt.Println()

	fmt.Printf("  %s %s\n", StyleSectionLabel.Render("API  "), StyleSectionValue.Render(apiURL))
	if model != "" {
		fmt.Printf("  %s %s\n", StyleSectionLabel.Render("Model"), Badge(model, BadgeModel))
	} else {
		fmt.Printf("  %s %s\n", StyleSectionLabel.Render("Model"), StyleWarning.Render("(auto - no enabled model found)"))
	}
	if stream {
		fmt.Printf("  %s %s\n", StyleSectionLabel.Render("Mode "), Badge("streaming", BadgeStream))
	} else {
		fmt.Printf("  %s %s\n", StyleSectionLabel.Render("Mode "), Badge("sync", BadgeTime))
	}

	fmt.Println()
	fmt.Println(StyleDimText.Render("  /help for commands · !cmd to execute · Tab to autocomplete · Enter empty line to send"))
	fmt.Println()
}

func renderGoodbye() {
	fmt.Printf("\n  %s %s\n", Badge("bye", BadgeModel), StyleMuted.Render("See you next time!"))
}

func printChatHelp() {
	fmt.Println()

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		MarginBottom(1)
	fmt.Printf("  %s\n\n", header.Render("Commands"))

	commands := []struct {
		key  string
		desc string
	}{
		{"/new", "Start a new conversation"},
		{"/help", "Show this help"},
		{"/model", "Interactive model selector"},
		{"/model <name>", "Set model directly"},
		{"/stream", "Toggle streaming (on/off)"},
		{"/compact", "Compress conversation context"},
		{"/clear", "Clear conversation context"},
		{"/history", "Show message history"},
		{"/copy", "Copy last response to clipboard"},
		{"/redo", "Retry last user message"},
		{"/cd <dir>", "Change working directory"},
		{"/config", "Open settings wizard"},
		{"/dashboard", "Open dashboard"},
		{"!command", "Execute shell command directly"},
		{"exit, /exit", "Exit chat session"},
	}

	for _, cmd := range commands {
		fmt.Printf("  %s  %s\n",
			Badge(cmd.key, BadgeKeyHint),
			StyleDescHint.Render(cmd.desc))
	}
	fmt.Printf("\n  %s\n", StyleDimText.Render("Tip: Tab to autocomplete · !cmd to execute shell · Enter empty line to send · Paste multi-line code directly"))
	fmt.Println()
}

func PrintModelList(apiURL string) {
	models, err := FetchModels(apiURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to fetch models: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Printf("  %s\n\n", StyleBold.Render("Available Models"))

	for _, m := range models {
		if m.Status != "enabled" {
			continue
		}
		provider := StyleMuted.Render(fmt.Sprintf("[%s]", m.Provider))
		fmt.Printf("  %s %s  %s\n",
			Badge("●", BadgeSuccess),
			StyleBold.Render(m.Name),
			provider)
	}
	fmt.Println()
}

func ShowConfig(cfg *Config, path string) {
	fmt.Println()
	fmt.Printf("  %s %s\n\n", StyleSectionLabel.Render("Config"), StyleSectionValue.Render(path))

	fmt.Printf("  %s\n", StyleBold.Render("API Configuration"))
	fmt.Printf("  %s\n", LabeledLine("Base URL", cfg.API.BaseURL))
	fmt.Printf("  %s\n", LabeledLine("Timeout", fmt.Sprintf("%d seconds", cfg.API.TimeoutSec)))

	fmt.Println()
	fmt.Printf("  %s\n", StyleBold.Render("Chat Configuration"))
	fmt.Printf("  %s\n", LabeledLine("Default Model", cfg.Chat.DefaultModel))
	fmt.Printf("  %s\n", LabeledLine("Streaming", fmt.Sprintf("%v", cfg.Chat.Stream)))
	fmt.Printf("  %s\n", LabeledLine("Context Limit", fmt.Sprintf("%d", cfg.Chat.ContextLimit)))
}

func initReadline() {
	homeDir, _ := os.UserHomeDir()
	histFile := "/tmp/.openaide_history"
	if homeDir != "" {
		histFile = homeDir + "/.openaide/history"
	}

	var err error
	rl, err = readline.NewEx(&readline.Config{
		Prompt:          "\x1b[0m",
		HistoryFile:     histFile,
		HistoryLimit:    1000,
		AutoComplete:    slashCommandCompleter,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		rl = nil
	}
}

var slashCommandCompleter = readline.NewPrefixCompleter(
	readline.PcItem("/help"),
	readline.PcItem("/new"),
	readline.PcItem("/model"),
	readline.PcItem("/stream",
		readline.PcItem("on"),
		readline.PcItem("off"),
	),
	readline.PcItem("/clear"),
	readline.PcItem("/compact"),
	readline.PcItem("/history"),
	readline.PcItem("/copy"),
	readline.PcItem("/redo"),
	readline.PcItem("/cd"),
	readline.PcItem("/config"),
	readline.PcItem("/dashboard"),
	readline.PcItem("/exit"),
	readline.PcItem("/quit"),
)

func readLine() (string, error) {
	if rl != nil {
		rl.SetPrompt(StylePromptArrow.Render("❯ "))
		line, err := rl.Readline()
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return "", nil
		}
		if strings.HasPrefix(line, "/") || line == "exit" || line == "quit" {
			return line, nil
		}
		if strings.Contains(line, "\n") {
			return line, nil
		}
		var lines []string
		lines = append(lines, line)
		for {
			rl.SetPrompt(StyleDimText.Render("│ "))
			nextLine, err := rl.Readline()
			if err != nil {
				if err == readline.ErrInterrupt {
					if len(lines) > 0 {
						return strings.Join(lines, "\n"), nil
					}
					return "", err
				}
				break
			}
			nextLine = strings.TrimSpace(nextLine)
			if nextLine == "" {
				break
			}
			lines = append(lines, nextLine)
		}
		return strings.Join(lines, "\n"), nil
	}
	buf := make([]byte, 4096)
	n, err := os.Stdin.Read(buf)
	return strings.TrimSpace(string(buf[:n])), err
}

func closeReadline() {
	if rl != nil {
		rl.Close()
		rl = nil
	}
}

func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	cjk := 0
	latin := 0
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF || r >= 0x3040 && r <= 0x30FF || r >= 0xAC00 && r <= 0xD7AF {
			cjk++
		} else if r <= 127 {
			latin++
		} else {
			cjk++
		}
	}
	return int(float64(cjk)*1.5 + float64(latin)*0.25)
}

func copyToClipboard(text string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "pbcopy"
	case "linux":
		if _, err := os.Stat("/usr/bin/xclip"); err == nil {
			cmd = "xclip"
			args = []string{"-selection", "clipboard"}
		} else if _, err := os.Stat("/usr/bin/wl-copy"); err == nil {
			cmd = "wl-copy"
		} else {
			return fmt.Errorf("no clipboard tool found (install xclip or wl-copy)")
		}
	case "windows":
		cmd = "clip"
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	c := exec.Command(cmd, args...)
	c.Stdin = strings.NewReader(text)
	return c.Run()
}

func executeShellCommand(ctx context.Context, apiURL, dialogueID, model, command string) {
	fmt.Printf("  %s %s\n", Badge("exec", BadgeTool), StyleCommand.Render("$ "+command))
	reqBody := map[string]interface{}{
		"user_id":     "cli-user",
		"content":     fmt.Sprintf("执行命令: %s", command),
		"model_id":    model,
		"dialogue_id": dialogueID,
		"options": map[string]interface{}{
			"tool_filter": []string{"execute_command"},
			"system":      fmt.Sprintf("用户要求直接执行命令，请调用 execute_command 工具执行以下命令，不需要确认：\n%s\n执行后展示结果即可，不需要额外解释。", command),
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Printf("  %s Failed: %v\n", Badge("✗", BadgeError), err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL+"/chat/route", strings.NewReader(string(jsonData)))
	if err != nil {
		fmt.Printf("  %s Failed: %v\n", Badge("✗", BadgeError), err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	addAuthHeader(req)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("  %s Failed: %v\n", Badge("✗", BadgeError), err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("  %s Error: %s\n", Badge("✗", BadgeError), string(body))
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	codeDetector := NewStreamCodeBlockDetector()

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}
			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			eventType, _ := chunk["type"].(string)
			switch eventType {
			case "tool_call":
				tool, _ := chunk["tool"].(string)
				params, _ := chunk["params"].(string)
				detail := parseToolDetail(tool, params)
				fmt.Printf("  %s %s %s\n", Badge("tool", BadgeTool), StyleToolName.Render(tool), detail)
			case "tool_done":
				result, _ := chunk["result"].(string)
				if result != "" {
					lines := strings.Split(result, "\n")
					maxLines := 20
					for i, l := range lines {
						if i >= maxLines {
							fmt.Printf("  %s %s\n", StyleMuted.Render("│"), StyleDimText.Render(fmt.Sprintf("... +%d more lines", len(lines)-maxLines)))
							break
						}
						if l == "" {
							continue
						}
						fmt.Printf("  %s %s\n", StyleMuted.Render("│"), StyleOutput.Render(truncateStr(l, 120)))
					}
				}
			case "content":
				if content, ok := chunk["content"].(string); ok {
					fmt.Print(codeDetector.ProcessChunk(content))
				}
			case "done":
				fmt.Print(codeDetector.Flush())
			}
		}
	}
	fmt.Println()
}

func RestoreReadline() {
	closeReadline()
	initReadline()
}

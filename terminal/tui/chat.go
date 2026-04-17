package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/chzyer/readline"
)

var rl *readline.Instance

func RunChat(cfg *Config, apiURL, model string, stream bool) {
	if err := InitMarkdownRenderer(); err == nil {}

	initReadline()
	defer closeReadline()

	renderWelcome(apiURL, model, stream)

	dialogueID := ""
	dialogue, err := CreateDialogue(apiURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s Failed to create dialogue: %v%s\n", StyleError.Render("Error:"), err, StyleMuted.Render(" (offline mode)"))
	} else {
		dialogueID = dialogue.ID
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var history []Message

	for {
		fmt.Print(StyleUserLabel.Render("❯ "))
		input, err := readLine()
		if err != nil {
			if err == io.EOF {
				fmt.Println()
				renderGoodbye()
				return
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
			model, stream = handleSlashCommand(input, apiURL, dialogueID, model, stream, &history, cfg)
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
			fmt.Printf("\n%s %v\n", StyleError.Render("Error:"), err)
			fmt.Println()
			continue
		}

		if response != "" {
			if stream {
				fmt.Println()
			}
			fmt.Print(RenderMarkdown(response))

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

		fmt.Println()
	}
}

func runStreamChat(ctx context.Context, apiURL, dialogueID, input, model string, timeout int) (string, error) {
	spinner := StartThinking("")
	firstContent := true
	var elapsed time.Duration
	tokenCount := 0
	usedModel := model
	thinkingLineCount := 0

	cb := &StreamCallbacks{
		OnThinking: func(content string) {
			spinner.UpdateLabel("思考中...")
			thinkingLineCount++
			if thinkingLineCount <= 5 {
				ShowThinkingBlock(content)
			} else if thinkingLineCount == 6 {
				fmt.Printf("  %s %s\n",
					StyleThinkingIcon.Render("💭"),
					StyleMuted.Render("... (思考过程省略)"),
				)
			}
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
				firstContent = false
				if thinkingLineCount > 0 {
					fmt.Println()
				}
				ShowResponseHeader(usedModel, elapsed, 0)
				ShowResponseSeparator()
			}
			fmt.Print(chunk)
			tokenCount++
		},
		OnDone: func(m string) {
			if m != "" {
				usedModel = m
			}
		},
	}

	response, err := SendMessageStream(ctx, apiURL, dialogueID, input, model, timeout, cb)
	if firstContent {
		elapsed = spinner.Stop()
		if response != "" {
			if thinkingLineCount > 0 {
				fmt.Println()
			}
			ShowResponseHeader(usedModel, elapsed, 0)
			ShowResponseSeparator()
		}
	}

	if tokenCount > 0 {
		fmt.Println()
		ShowResponseFooter(usedModel, elapsed, tokenCount)
	}

	return response, err
}

func runSyncChat(apiURL, dialogueID, input, model string) (string, error) {
	spinner := StartThinking("")
	response, err := SendMessage(apiURL, dialogueID, input, model)
	elapsed := spinner.Stop()

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
	parts := []string{}
	if tokens > 0 {
		parts = append(parts, StyleTokenCount.Render(fmt.Sprintf("%d tokens", tokens)))
	}
	if elapsed > 0 {
		parts = append(parts, StyleTokenCount.Render(fmt.Sprintf("%.1fs", elapsed.Seconds())))
	}
	if model != "" {
		parts = append(parts, StyleTokenCount.Render(model))
	}
	if len(parts) > 0 {
		footer := ""
		for i, p := range parts {
			if i > 0 {
				footer += StyleMuted.Render(" · ")
			}
			footer += p
		}
		fmt.Printf("  %s\n", footer)
	}
}

func handleSlashCommand(cmd, apiURL, dialogueID string, model string, stream bool, history *[]Message, cfg *Config) (string, bool) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return model, stream
	}

	switch parts[0] {
	case "/clear":
		*history = []Message{}
		fmt.Println(StyleSuccess.Render("  ✓ Context cleared"))
	case "/model":
		if len(parts) > 1 {
			model = parts[1]
			fmt.Printf("  ✓ Model: %s\n", StyleSuccess.Render(model))
		} else {
			result := RunModelSelect(apiURL, model)
			if result.Changed {
				model = result.Selected
			}
		}
	case "/stream":
		if len(parts) > 1 {
			if parts[1] == "on" {
				stream = true
				fmt.Println(StyleSuccess.Render("  ✓ Streaming enabled"))
			} else if parts[1] == "off" {
				stream = false
				fmt.Println(StyleSuccess.Render("  ✓ Streaming disabled"))
			}
		} else {
			fmt.Printf("  Streaming: %s\n", StyleBold.Render(fmt.Sprintf("%v", stream)))
		}
	case "/history":
		fmt.Printf("  History: %s messages\n", StyleBold.Render(fmt.Sprintf("%d", len(*history))))
	case "/config":
		result := RunSettings(cfg, getConfigPath(cfg))
		if result.Saved && result.Config != nil {
			cfg = result.Config
			stream = cfg.Chat.Stream
			fmt.Println(StyleSuccess.Render("  ✓ Configuration saved"))
		}
	case "/dashboard":
		action := RunDashboard(apiURL, "")
		if action.Action == "select_model" && action.Model != "" {
			model = action.Model
		}
	case "/help":
		printChatHelp()
	default:
		fmt.Printf("  %s Unknown command: %s\n", StyleError.Render("✗"), parts[0])
		fmt.Printf("  Type %s for available commands\n", StyleHelpKey.Render("/help"))
	}

	return model, stream
}

func getConfigPath(cfg *Config) string {
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		return ""
	}
	return homeDir + "/.openaide/config.yaml"
}

func renderWelcome(apiURL, model string, stream bool) {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary)

	fmt.Println()
	fmt.Println(titleStyle.Render("  ╭─────────────────────────────╮"))
	fmt.Println(titleStyle.Render("  │  OpenAIDE CLI               │"))
	fmt.Println(titleStyle.Render("  ╰─────────────────────────────╯"))
	fmt.Println()

	infoStyle := lipgloss.NewStyle().PaddingLeft(4)
	fmt.Println(infoStyle.Render(fmt.Sprintf("API:   %s", StyleBannerValue.Render(apiURL))))
	if model != "" {
		fmt.Println(infoStyle.Render(fmt.Sprintf("Model: %s", StyleBannerValue.Render(model))))
	} else {
		fmt.Println(infoStyle.Render(fmt.Sprintf("Model: %s", StyleWarning.Render("(auto - no enabled model found)"))))
	}
	if stream {
		fmt.Println(infoStyle.Render(fmt.Sprintf("Mode:  %s", StyleBannerValue.Render("Streaming"))))
	}
	fmt.Println()
	fmt.Println(StyleMuted.Render("  Type /help for commands, exit or /exit to quit"))
	fmt.Println()
}

func renderGoodbye() {
	fmt.Printf("%s Goodbye!\n", StyleTitle.Render("OpenAIDE:"))
}

func printChatHelp() {
	fmt.Println()
	fmt.Println(StyleBold.Render("  Available Commands:"))
	fmt.Println()

	commands := []struct {
		key  string
		desc string
	}{
		{"/help", "Show this help"},
		{"/model", "Interactive model selector"},
		{"/model <name>", "Set model directly"},
		{"/stream", "Toggle streaming (on/off)"},
		{"/clear", "Clear conversation context"},
		{"/history", "Show message count"},
		{"/config", "Open settings wizard"},
		{"/dashboard", "Open dashboard"},
		{"exit, /exit", "Exit chat session"},
	}

	for _, cmd := range commands {
		fmt.Printf("  %s  %s\n",
			lipgloss.NewStyle().Width(18).Foreground(ColorAccent).Bold(true).Render(cmd.key),
			StyleMuted.Render(cmd.desc))
	}
	fmt.Println()
}

func PrintModelList(apiURL string) {
	models, err := FetchModels(apiURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to fetch models: %v\n", err)
		return
	}

	fmt.Println(StyleBold.Render("  Available Models:"))
	fmt.Println()
	for _, m := range models {
		if m.Status != "enabled" {
			continue
		}
		status := StyleStatusEnabled.Render(" ● enabled")
		provider := StyleMuted.Render(fmt.Sprintf("[%s]", m.Provider))
		fmt.Printf("  %s %s  %s %s\n",
			StyleStatusEnabled.Render("●"),
			StyleBold.Render(m.Name),
			provider,
			status)
	}
	fmt.Println()
}

func ShowConfig(cfg *Config, path string) {
	fmt.Printf("Configuration file: %s\n\n", StyleBold.Render(path))

	fmt.Println(StyleBold.Render("  API Configuration:"))
	fmt.Printf("    Base URL: %s\n", StyleBannerValue.Render(cfg.API.BaseURL))
	fmt.Printf("    Timeout:   %d seconds\n", cfg.API.TimeoutSec)

	fmt.Println()
	fmt.Println(StyleBold.Render("  Chat Configuration:"))
	fmt.Printf("    Default Model: %s\n", StyleBannerValue.Render(cfg.Chat.DefaultModel))
	fmt.Printf("    Streaming:     %v\n", cfg.Chat.Stream)
	fmt.Printf("    Context Limit: %d\n", cfg.Chat.ContextLimit)
}

func initReadline() {
	homeDir, _ := os.UserHomeDir()
	histFile := "/tmp/.openaide_history"
	if homeDir != "" {
		histFile = homeDir + "/.openaide/history"
	}

	var err error
	rl, err = readline.NewEx(&readline.Config{
		Prompt:          "",
		HistoryFile:     histFile,
		AutoComplete:    nil,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		rl = nil
	}
}

func readLine() (string, error) {
	if rl != nil {
		line, err := rl.Readline()
		return strings.TrimSpace(line), err
	}
	buf := make([]byte, 1024)
	n, err := os.Stdin.Read(buf)
	return strings.TrimSpace(string(buf[:n])), err
}

func closeReadline() {
	if rl != nil {
		rl.Close()
		rl = nil
	}
}

func RestoreReadline() {
	closeReadline()
	initReadline()
}

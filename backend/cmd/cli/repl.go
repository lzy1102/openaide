package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lmorg/readline/v4"

	"openaide/backend/internal/infra"
	"openaide/backend/internal/kernel"
	"openaide/backend/internal/lang"
)

// ── REPL (Read-Eval-Print Loop) ───────────────────────────

func runREPL(app *infra.Application) {
	// Welcome
	info := app.LLMGateway.GetProviderInfos()
	modelName := ""
	if len(info) > 0 {
		modelName = info[0].Model
	}
	fmt.Println()
	fmt.Printf("  %sOpenAIDE REPL%s\n", bold, reset)
	if modelName != "" {
		fmt.Printf("  Model: %s%s%s\n", green, modelName, reset)
	}
	fmt.Printf("  %s/h%s help  %s|%s  %s/q%s quit  %s|%s  Ctrl+C interrupt\n", yellow, reset, gray, reset, yellow, reset, gray, reset)
	fmt.Println()

	// Session
	sess, _ := app.Orchestrator.CreateSession(context.Background(), "default", "cli-user")
	sessionID := sess.ID

	// Readline setup
	rl := readline.NewInstance()
	rl.SetPrompt(PromptStyle(sessionID, modelName))
	rl.HistoryAutoWrite = true

	// Tab completion for commands
	commands := []string{"/help", "/clear", "/model", "/lang", "/log", "/sessions", "/handoff", "/exit", "/quit", "/q"}
	rl.TabCompleter = func(line []rune, pos int, _ readline.DelayedTabContext) *readline.TabCompleterReturnT {
		prefix := string(line[:pos])
		if strings.HasPrefix(prefix, "/") {
			var matches []string
			for _, cmd := range commands {
				if strings.HasPrefix(cmd, prefix) {
					matches = append(matches, cmd)
				}
			}
			if len(matches) > 0 {
				return &readline.TabCompleterReturnT{
					Prefix:       prefix,
					Suggestions:  matches,
					Descriptions: make(map[string]string),
				}
			}
		}
		return &readline.TabCompleterReturnT{}
	}

	// Hint text
	rl.HintText = func(line []rune, pos int) []rune {
		if len(line) == 0 {
			return []rune("type your question or /help")
		}
		return nil
	}

	// Main loop
	for {
		line, err := rl.Readline()
		if err != nil {
			// Ctrl+D or read error
			fmt.Println("\n  Goodbye.")
			return
		}

		query := strings.TrimSpace(line)
		if query == "" {
			continue
		}

		// Handle commands
		if strings.HasPrefix(query, "/") {
			handleREPLCommand(app, query, &sessionID, &modelName)
			rl.SetPrompt(PromptStyle(sessionID, modelName))
			continue
		}

		// Execute with streaming
		fmt.Println()
		ctx, cancel := context.WithCancel(context.Background())

		stream, err := app.Orchestrator.ProcessQueryStream(ctx, "cli-user", "default", query, kernel.QueryOptions{})
		if err != nil {
			PrintError(fmt.Sprintf("%v", err))
			cancel()
			rl.SetPrompt(PromptStyle(sessionID, modelName))
			continue
		}

		// Goroutine to handle Ctrl+C during streaming

		startTime := time.Now()
	totalTools := 0
	totalTokens := 0
	var fullResponse strings.Builder
	thinkShown := false

	for chunk := range stream {
		if chunk.Error != nil {
			PrintError(chunk.Error.Error())
			break
		}
		if chunk.Done {
			break
		}
		if chunk.ReasoningContent != "" && !thinkShown {
			PrintThinking(chunk.ReasoningContent)
			thinkShown = true
		}
		// 积累正文（不流式打印），结束后统一 Markdown 渲染
		if chunk.Content != "" {
			fullResponse.WriteString(chunk.Content)
		}
		if chunk.Type == kernel.ChunkTypeToolCall && chunk.ToolName != "" {
			PrintToolCall(chunk.ToolName)
			totalTools++
			thinkShown = false
		}
		if chunk.Type == kernel.ChunkTypeToolDone {
			summary := ""
			if chunk.ToolResult != nil {
				raw := fmt.Sprintf("%v", chunk.ToolResult.Content)
				summary = strings.TrimPrefix(strings.SplitN(raw, "\n", 2)[0], "// ")
			}
			PrintToolDone(summary)
		}
	}

	elapsed := time.Since(startTime)
	cancel()

	// 流式结束后一次性渲染完整回答（Markdown + 代码高亮）
	if fullResponse.Len() > 0 {
		fmt.Println()
		rendered := RenderMarkdown(fullResponse.String())
		fmt.Println(rendered)
	}

	PrintStatusLine(totalTokens, totalTools, elapsed)
		rl.SetPrompt(PromptStyle(sessionID, modelName))
	}
}

// handleREPLCommand processes slash commands
func handleREPLCommand(app *infra.Application, cmd string, sessionID *string, modelName *string) {
	parts := strings.Fields(cmd)
	switch parts[0] {
	case "/exit", "/quit", "/q":
		fmt.Println("  Goodbye.")
		os.Exit(0)

	case "/help":
		fmt.Println()
		fmt.Printf("  %sCommands%s\n", bold, reset)
		fmt.Printf("  %s/h%s help              Show help\n", yellow, reset)
		fmt.Printf("  %s/c%s clear             Clear screen\n", yellow, reset)
		fmt.Printf("  %s/m%s model [name]       View/switch model\n", yellow, reset)
		fmt.Printf("  %s/l%s lang zh|en         Switch language\n", yellow, reset)
		fmt.Printf("  %s/l%s log                Recent logs\n", yellow, reset)
		fmt.Printf("  %s/s%s sessions           List sessions\n", yellow, reset)
		fmt.Printf("  %s/h%s handoff            Save session\n", yellow, reset)
		fmt.Printf("  %s/q%s quit, /exit         Exit\n", yellow, reset)
		fmt.Println()
		fmt.Printf("  %sCtrl+C%s  Interrupt stream\n", gray, reset)
		fmt.Printf("  %sCtrl+D%s  Exit on empty line\n", gray, reset)
		fmt.Println()

	case "/clear":
		fmt.Print("\033[2J\033[H")

	case "/model":
		if len(parts) >= 2 {
			app.SetModel(parts[1])
			*modelName = parts[1]
			PrintSuccess("Model: " + parts[1])
		} else {
			info := app.LLMGateway.GetProviderInfos()
			for _, p := range info {
				marker := " "
				if p.Default {
					marker = "*"
				}
				fmt.Printf("  %s %s%s: %s%s\n", marker, green, p.Name, reset, p.Model)
			}
		}

	case "/lang":
		if len(parts) >= 2 {
			switch parts[1] {
			case "zh":
				lang.SetLang(lang.ZH)
				PrintSuccess("语言: 中文")
			case "en":
				lang.SetLang(lang.EN)
				PrintSuccess("Language: English")
			}
		}

	case "/log":
		lines := tuiLogBuf.snapshot()
		start := 0
		if len(lines) > 20 {
			start = len(lines) - 20
		}
		for i := start; i < len(lines); i++ {
			fmt.Printf("  %s%s%s\n", gray, lines[i], reset)
		}

	case "/sessions":
		sessions, err := app.Orchestrator.ListSessions(context.Background(), "default", "cli-user", 10, 0)
		if err != nil {
			PrintError(fmt.Sprintf("%v", err))
			return
		}
		fmt.Println()
		for _, s := range sessions {
			marker := " "
			if s.ID == *sessionID {
				marker = "*"
			}
			title := s.ID[:8]
			for i := len(s.Messages) - 1; i >= 0; i-- {
				if s.Messages[i].Role == "user" {
					title = trunc(s.Messages[i].Content, 40)
					break
				}
			}
			fmt.Printf("  %s %s%s%s  [%d msgs]  %s\n", marker, reset, title, gray, len(s.Messages), s.UpdatedAt.Format("15:04"))
		}
		fmt.Println()

	case "/handoff":
		PrintSuccess("Session saved to ~/.openaide/data/handoff.json")

	default:
		PrintWarning("Unknown: " + parts[0])
	}
}

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"openaide/backend/internal/infra"
	"openaide/backend/internal/kernel"
	"openaide/backend/internal/lang"
)

// ── Enhanced REPL ──────────────────────────────────────────

func runREPL(app *infra.Application) {
	// Welcome
	fmt.Println()
	fmt.Println("  OpenAIDE REPL")
	info := app.LLMGateway.GetProviderInfos()
	if len(info) > 0 {
		fmt.Printf("  Model: %s (%s)\n", info[0].Model, info[0].Name)
	}
	fmt.Println("  /help 查看命令 | /exit 退出 | Ctrl+C 中断流式")
	fmt.Println()

	// Create or resume session
	sess, _ := app.Orchestrator.CreateSession(context.Background(), "default", "cli-user")
	sessionID := sess.ID

	// Readline setup
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		// Fallback to simple scanner mode
		runSimpleREPL(app)
		return
	}
	defer term.Restore(fd, oldState)

	// History and input state
	var history []string
	histIdx := -1
	var line []rune
	pos := 0

	printPrompt := func() {
		fmt.Fprint(os.Stderr, "\r\033[K") // clear line
		if app.LLMGateway != nil {
			info := app.LLMGateway.GetProviderInfos()
			if len(info) > 0 && sessionID != "" {
				fmt.Fprintf(os.Stderr, "\033[90m%s │ \033[0m❯ ", sessionID[:8])
			} else {
				fmt.Fprint(os.Stderr, "❯ ")
			}
		} else {
			fmt.Fprint(os.Stderr, "❯ ")
		}
		fmt.Fprint(os.Stderr, string(line))
		// Move cursor to position
		if pos < len(line) {
			fmt.Fprintf(os.Stderr, "\033[%dD", len(line)-pos)
		}
	}

	printPrompt()

	buf := make([]byte, 16)
	streaming := false
	streamCancel := func() {}

	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			break
		}

		if streaming {
			// During streaming, only handle Ctrl+C
			if n == 1 && buf[0] == 3 { // Ctrl+C
				streamCancel()
				streaming = false
				fmt.Fprint(os.Stderr, "\r\033[K") // clear line
				fmt.Println("\n  ⏸ 已中断")
				line = nil
				pos = 0
				printPrompt()
				continue
			}
			continue
		}

		for i := 0; i < n; i++ {
			b := buf[i]

			switch {
			case b == 3: // Ctrl+C
				// In REPL, Ctrl+C on empty line = exit
				if len(line) == 0 {
					fmt.Println("\n  Goodbye.")
					return
				}
				// Otherwise clear current line
				line = nil
				pos = 0
				printPrompt()

			case b == 4: // Ctrl+D (EOF on empty line)
				if len(line) == 0 {
					fmt.Println("\n  Goodbye.")
					return
				}

			case b == 13 || b == 10: // Enter
				fmt.Fprint(os.Stderr, "\r\033[K\n") // clear prompt line

				query := strings.TrimSpace(string(line))
				line = nil
				pos = 0

				if query == "" {
					printPrompt()
					continue
				}

				// Add to history
				if len(history) == 0 || history[len(history)-1] != query {
					history = append(history, query)
				}
				histIdx = len(history)

				// Handle commands
				if strings.HasPrefix(query, "/") {
					handleREPLCommand(app, query, &sessionID)
					printPrompt()
					continue
				}

				// Execute query with streaming
				fmt.Println()
				ctx, cancel := context.WithCancel(context.Background())
				streamCancel = cancel
				streaming = true

				stream, err := app.Orchestrator.ProcessQueryStream(ctx, "cli-user", "default", query, kernel.QueryOptions{})
				if err != nil {
					fmt.Fprintf(os.Stderr, "  Error: %v\n\n", err)
					cancel()
					streaming = false
					printPrompt()
					continue
				}

				totalTools := 0
				for chunk := range stream {
					if chunk.Error != nil {
						fmt.Fprintf(os.Stderr, "\n  Error: %v\n", chunk.Error)
						break
					}
					if chunk.Done {
						fmt.Println()
						fmt.Println()
						break
					}
					if chunk.Content != "" {
						fmt.Print(chunk.Content)
					}
					if chunk.ReasoningContent != "" && chunk.Content == "" {
						// Show first line of thinking dimmed
						firstLine := strings.SplitN(chunk.ReasoningContent, "\n", 2)[0]
						if firstLine != "" && len(firstLine) < 200 {
							fmt.Printf("\033[90m  [think] %s\033[0m\n", firstLine)
						}
					}
					if chunk.Type == kernel.ChunkTypeToolCall && chunk.ToolName != "" {
						totalTools++
						fmt.Printf("\n  \033[33m⚙ %s\033[0m ", chunk.ToolName)
					}
					if chunk.Type == kernel.ChunkTypeToolDone {
						fmt.Print("✓")
					}
				}
				if totalTools > 0 {
					fmt.Printf("  \033[90m(%d tools)\033[0m\n", totalTools)
				}
				cancel()
				streaming = false
				printPrompt()

			case b == 127: // Backspace
				if pos > 0 {
					copy(line[pos-1:], line[pos:])
					line = line[:len(line)-1]
					pos--
					printPrompt()
				}

			case b == 27: // Escape sequence
				if i+2 < n {
					switch buf[i+2] {
					case 'A': // Up arrow
						if histIdx > 0 {
							histIdx--
							line = []rune(history[histIdx])
							pos = len(line)
							printPrompt()
						}
						i += 2
					case 'B': // Down arrow
						if histIdx < len(history)-1 {
							histIdx++
							line = []rune(history[histIdx])
							pos = len(line)
							printPrompt()
						} else if histIdx == len(history)-1 {
							histIdx = len(history)
							line = nil
							pos = 0
							printPrompt()
						}
						i += 2
					case 'C': // Right arrow
						if pos < len(line) {
							pos++
							fmt.Fprint(os.Stderr, "\033[1C")
						}
						i += 2
					case 'D': // Left arrow
						if pos > 0 {
							pos--
							fmt.Fprint(os.Stderr, "\033[1D")
						}
						i += 2
					}
				}

			case b >= 32: // Printable
				line = append(line[:pos], append([]rune{rune(b)}, line[pos:]...)...)
				pos++
				printPrompt()
			}
		}
	}
}

// handleREPLCommand processes slash commands in REPL mode
func handleREPLCommand(app *infra.Application, cmd string, sessionID *string) {
	parts := strings.Fields(cmd)
	switch parts[0] {
	case "/exit", "/quit", "/q":
		fmt.Println("  Goodbye.")
		os.Exit(0)
	case "/help":
		fmt.Println("  /exit, /quit, /q   退出")
		fmt.Println("  /clear             清屏")
		fmt.Println("  /model [name]      查看/切换模型")
		fmt.Println("  /lang zh|en        切换语言")
		fmt.Println("  /log               最近日志")
		fmt.Println("  /sessions          会话列表")
		fmt.Println("  /handoff           保存会话")
		fmt.Println("  Ctrl+C             中断流式输出")
	case "/clear":
		fmt.Print("\033[2J\033[H")
	case "/model":
		if len(parts) >= 2 {
			app.SetModel(parts[1])
			fmt.Printf("  Model: %s\n", parts[1])
		} else {
			info := app.LLMGateway.GetProviderInfos()
			for _, p := range info {
				marker := " "
				if p.Default { marker = "*" }
				fmt.Printf("  %s %s: %s\n", marker, p.Name, p.Model)
			}
		}
	case "/lang":
		if len(parts) >= 2 {
			switch parts[1] {
			case "zh": lang.SetLang(lang.ZH); fmt.Println("  语言已切换为中文")
			case "en": lang.SetLang(lang.EN); fmt.Println("  Language: English")
			}
		}
	case "/log":
		lines := tuiLogBuf.snapshot()
		start := 0
		if len(lines) > 20 { start = len(lines) - 20 }
		for i := start; i < len(lines); i++ {
			fmt.Printf("  \033[90m%s\033[0m\n", lines[i])
		}
	case "/sessions":
		sessions, err := app.Orchestrator.ListSessions(context.Background(), "default", "cli-user", 10, 0)
		if err != nil {
			fmt.Printf("  Error: %v\n", err)
			return
		}
		for _, s := range sessions {
			marker := " "
			if s.ID == *sessionID { marker = "*" }
			fmt.Printf("  %s %s  [%d msgs]  %s\n", marker, s.ID[:8], len(s.Messages), s.UpdatedAt.Format("15:04"))
		}
	default:
		fmt.Printf("  未知命令: %s\n", parts[0])
	}
}

// runSimpleREPL is a fallback without raw terminal support
func runSimpleREPL(app *infra.Application) {
	fmt.Println("  (简化的逐行输入模式)")
	scanner := newLineScanner()
	for {
		fmt.Print("> ")
		query := scanner()
		if query == "" {
			break
		}

		if strings.HasPrefix(query, "/") {
			parts := strings.Fields(query)
			if parts[0] == "/exit" || parts[0] == "/quit" || parts[0] == "/q" {
				fmt.Println("Goodbye.")
				return
			}
		}

		fmt.Println()
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		stream, err := app.Orchestrator.ProcessQueryStream(ctx, "cli-user", "default", query, kernel.QueryOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Error: %v\n\n", err)
			cancel()
			continue
		}
		for chunk := range stream {
			if chunk.Error != nil {
				fmt.Fprintf(os.Stderr, "\n  Error: %v\n\n", chunk.Error)
				break
			}
			if chunk.Done {
				fmt.Println()
				break
			}
			if chunk.Content != "" {
				fmt.Print(chunk.Content)
			}
			if chunk.Type == kernel.ChunkTypeToolCall && chunk.ToolName != "" {
				fmt.Printf("\n  ⚙ %s ", chunk.ToolName)
			}
		}
		cancel()
		fmt.Println()
	}
}

func newLineScanner() func() string {
	// Simple bufio-based scanner as fallback
	ch := make(chan string, 1)
	go func() {
		var line string
		for {
			n, err := fmt.Scanln(&line)
			if err != nil || n == 0 {
				ch <- ""
				return
			}
			ch <- line
			line = ""
		}
	}()
	return func() string { return <-ch }
}

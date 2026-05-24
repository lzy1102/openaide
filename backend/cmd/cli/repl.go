package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"openaide/backend/internal/infra"
	"openaide/backend/internal/kernel"
	"openaide/backend/internal/lang"
)

func runREPL(app *infra.Application) {
	fmt.Println()
	fmt.Println("  OpenAIDE REPL — type /help for commands, /exit to quit")
	fmt.Println()

	info := app.LLMGateway.GetProviderInfos()
	if len(info) > 0 {
		fmt.Printf("  Model: %s (%s)\n", info[0].Model, info[0].Name)
	}
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			fmt.Println()
			break
		}

		query := strings.TrimSpace(scanner.Text())
		if query == "" {
			continue
		}

		// 处理斜杠命令
		if strings.HasPrefix(query, "/") {
			parts := strings.Fields(query)
			switch parts[0] {
			case "/exit", "/quit", "/q":
				fmt.Println("Goodbye.")
				return
			case "/help":
				fmt.Println("  /exit, /quit, /q — 退出")
				fmt.Println("  /clear        — 清屏")
				fmt.Println("  /model <name> — 切换模型")
				fmt.Println("  /lang zh|en   — 切换语言")
				fmt.Println("  /log          — 最近日志")
				fmt.Println("  /sessions     — 会话列表")
				continue
			case "/clear":
				fmt.Print("\033[2J\033[H")
				continue
			case "/model":
				if len(parts) >= 2 {
					app.SetModel(parts[1])
					fmt.Printf("  Model switched to: %s\n", parts[1])
				} else {
					info := app.LLMGateway.GetProviderInfos()
					for _, p := range info {
						fmt.Printf("  %s: %s %s\n", p.Name, p.Model, map[bool]string{true: "(default)", false: ""}[p.Default])
					}
				}
				continue
			case "/lang":
				if len(parts) >= 2 {
					switch parts[1] {
					case "zh":
						lang.SetLang(lang.ZH)
						fmt.Println("  语言已切换为中文")
					case "en":
						lang.SetLang(lang.EN)
						fmt.Println("  Language switched to English")
					}
				}
				continue
			case "/log":
				lines := tuiLogBuf.buf
				start := 0
				if len(lines) > 20 {
					start = len(lines) - 20
				}
				for i := start; i < len(lines); i++ {
					fmt.Println("  " + lines[i])
				}
				continue
			case "/sessions":
				sessions, err := app.Orchestrator.ListSessions(context.Background(), "default", "cli-user", 10, 0)
				if err != nil {
					fmt.Printf("  Error: %v\n", err)
					continue
				}
				for _, s := range sessions {
					fmt.Printf("  %s  [%d msgs]  %s\n", s.ID[:8], len(s.Messages), s.UpdatedAt.Format("15:04"))
				}
				continue
			}
		}

		// 流式执行
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
			// 显示工具调用
			if chunk.Type == kernel.ChunkTypeToolCall && chunk.ToolName != "" {
				fmt.Printf("\n  ⚙ %s ", chunk.ToolName)
			}
		}
		cancel()
		fmt.Println()
	}
}

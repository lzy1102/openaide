package main

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lmorg/readline/v4"

	"openaide/backend/internal/infra"
	"openaide/backend/internal/kernel"
	"openaide/backend/internal/lang"
	"openaide/backend/internal/orchestration"
)

// ── REPL ──────────────────────────────────────────────────

func runREPL(app *infra.Application) {
	info := app.LLMGateway.GetProviderInfos()
	modelName := ""
	if len(info) > 0 {
		modelName = fmt.Sprintf("%s (%s)", info[0].Model, info[0].Name)
	}

	// Project context
	cwd, _ := os.Getwd()
	gitBranch := ""
	if out, err := execCmd("git", "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		gitBranch = strings.TrimSpace(out)
	}

	fmt.Println()
	fmt.Printf("  %sOpenAIDE%s", bold, reset)
	if gitBranch != "" {
		fmt.Printf("  %s%s%s", gray, gitBranch, reset)
	}
	fmt.Println()
	if modelName != "" {
		fmt.Printf("  %s%s%s", green, modelName, reset)
		if gitBranch == "" {
			fmt.Printf("  %s%s%s", gray, filepath.Base(cwd), reset)
		}
	}
	fmt.Println()
	fmt.Println()
	fmt.Printf("  %s/h%s help  %s|%s  %s/q%s quit  %s|%s  Ctrl+C interrupt", yellow, reset, gray, reset, yellow, reset, gray, reset)
	fmt.Println()
	fmt.Println()

	sess, _ := app.Orchestrator.CreateSession(context.Background(), "default", "cli-user")
	sessionID := sess.ID

	rl := readline.NewInstance()
	rl.SetPrompt(PromptStyle(sessionID, modelName))
	rl.HistoryAutoWrite = true

	commands := []string{"/help", "/clear", "/model", "/lang", "/log", "/sessions",
		"/handoff", "/exit", "/quit", "/q", "/analyst", "/coder", "/reviewer", "/executor", "/team"}
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
					Prefix: prefix, Suggestions: matches, Descriptions: make(map[string]string),
				}
			}
		}
		return &readline.TabCompleterReturnT{}
	}
	rl.HintText = func(line []rune, pos int) []rune {
		if len(line) == 0 { return []rune("type your question or /help") }
		return nil
	}

	for {
		line, err := rl.Readline()
		if err != nil {
			fmt.Println("\n  Goodbye.")
			return
		}
		query := strings.TrimSpace(line)
		if query == "" { continue }

		if strings.HasPrefix(query, "/") {
			handleREPLCommand(app, query, &sessionID, &modelName)
			rl.SetPrompt(PromptStyle(sessionID, modelName))
			continue
		}

		// ── Smart routing: PreviewPlan → direct or team execution ──
		fmt.Println()
		fmt.Printf("  %s分析任务…%s", gray, reset)
		planCtx, planCancel := context.WithTimeout(context.Background(), app.Orchestrator.PreviewTimeout)
		plan, planErr := app.Orchestrator.PreviewPlan(planCtx, query)
		planCancel()
		fmt.Print("\r\033[K")

		if planErr != nil || plan == nil || len(plan.Subtasks) <= 1 {
			executeStreamQuery(app, query, &sessionID)
		} else {
			fmt.Printf("  %s📋 %s%s (%d steps)\n", yellow, reset, plan.Goal, len(plan.Subtasks))
			for i, st := range plan.Subtasks {
				fmt.Printf("    %d. %s\n", i+1, st.Title)
			}
			fmt.Println()
			executePlanQuery(app, query, plan)
		}
		rl.SetPrompt(PromptStyle(sessionID, modelName))
	}
}

func execCmd(name string, args ...string) (string, error) {
	cmd := osexec.Command(name, args...)
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil { return "", err }
	return strings.TrimSpace(string(out)), nil
}

// ── Simple Query (direct ReAct stream) ────────────────────

func executeStreamQuery(app *infra.Application, query string, sessionID *string) {
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := app.Orchestrator.ProcessQueryStream(ctx, "cli-user", "default", query, kernel.QueryOptions{})
	if err != nil {
		PrintError(fmt.Sprintf("%v", err))
		cancel()
		return
	}

	startTime := time.Now()
	totalTools := 0
	var fullResponse strings.Builder
	thinkShown := false

	for chunk := range stream {
		if chunk.Error != nil { PrintError(chunk.Error.Error()); break }
		if chunk.Done { break }
		if chunk.ReasoningContent != "" && !thinkShown {
			PrintThinking(chunk.ReasoningContent)
			thinkShown = true
		}
		if chunk.Content != "" { fullResponse.WriteString(chunk.Content) }
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

	if fullResponse.Len() > 0 {
		fmt.Println()
		fmt.Println(RenderMarkdown(fullResponse.String()))
	}
	PrintStatusLine(0, totalTools, elapsed)
}

// ── Complex Query (sub-agent team execution) ──────────────

func executePlanQuery(app *infra.Application, query string, plan *orchestration.Plan) {
	startTime := time.Now()

	// Progress callback
	app.Orchestrator.OnProgress = func(phase, detail string) {
		elapsed := time.Since(startTime).Round(time.Second)
		fmt.Printf("\r\033[K  %s🔧%s %s %s(%v)%s\n", yellow, reset, detail, gray, elapsed, reset)
	}

	fmt.Printf("  %s执行中…%s\n", gray, reset)

	// Heartbeat goroutine
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				elapsed := time.Since(startTime).Round(time.Second)
				fmt.Printf("\r\033[K  %s⏳ 执行中… (%v)%s\n", gray, elapsed, reset)
				fmt.Print("\033[1A") // move cursor up to overwrite on next update
			case <-done:
				return
			}
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	resp, err := app.Orchestrator.ExecuteWithPlan(ctx, "cli-user", "default", query, plan, kernel.QueryOptions{})
	cancel()
	close(done)

	elapsed := time.Since(startTime)
	fmt.Print("\r\033[K") // clear progress line

	if err != nil {
		PrintError(fmt.Sprintf("%v", err))
		return
	}

	totalTools := resp.ToolCalls
	if resp.Content != "" {
		fmt.Println()
		fmt.Println(RenderMarkdown(resp.Content))
	}
	PrintStatusLine(resp.TokensUsed, totalTools, elapsed)
}

// ── Commands ──────────────────────────────────────────────

func handleREPLCommand(app *infra.Application, cmd string, sessionID *string, modelName *string) {
	parts := strings.Fields(cmd)
	switch parts[0] {
	case "/exit", "/quit", "/q":
		fmt.Println("  Goodbye.")
		os.Exit(0)

	case "/help":
		fmt.Println()
		fmt.Printf("  %sCommands%s\n", bold, reset)
		fmt.Printf("  %s/h%s help              Help\n", yellow, reset)
		fmt.Printf("  %s/c%s clear             Clear screen\n", yellow, reset)
		fmt.Printf("  %s/m%s model [name]       View/switch model\n", yellow, reset)
		fmt.Printf("  %s/l%s lang zh|en         Switch language\n", yellow, reset)
		fmt.Printf("  %s/l%s log                Recent logs\n", yellow, reset)
		fmt.Printf("  %s/s%s sessions           List sessions\n", yellow, reset)
		fmt.Printf("  %s/a%s analyst <task>     Run analyst sub-agent\n", yellow, reset)
		fmt.Printf("  %s/c%s coder <task>       Run coder sub-agent\n", yellow, reset)
		fmt.Printf("  %s/r%s eviewer <task>     Run reviewer sub-agent\n", yellow, reset)
		fmt.Printf("  %s/e%s xecutor <task>     Run executor sub-agent\n", yellow, reset)
		fmt.Printf("  %s/t%s eam <task>         Run full team chain\n", yellow, reset)
		fmt.Println()

	case "/clear":
		fmt.Print("\033[2J\033[H")

	case "/model":
		if len(parts) >= 2 {
			app.SetModel(parts[1]); *modelName = parts[1]
			PrintSuccess("Model: " + parts[1])
		} else {
			info := app.LLMGateway.GetProviderInfos()
			for _, p := range info {
				m := " "; if p.Default { m = "*" }
				fmt.Printf("  %s %s%s: %s%s\n", m, green, p.Name, reset, p.Model)
			}
		}

	case "/lang":
		if len(parts) >= 2 {
			switch parts[1] {
			case "zh": lang.SetLang(lang.ZH); PrintSuccess("中文")
			case "en": lang.SetLang(lang.EN); PrintSuccess("English")
			}
		}

	case "/log":
		lines := tuiLogBuf.snapshot()
		start := 0; if len(lines) > 20 { start = len(lines) - 20 }
		for i := start; i < len(lines); i++ { fmt.Printf("  %s%s%s\n", gray, lines[i], reset) }

	case "/sessions":
		sessions, _ := app.Orchestrator.ListSessions(context.Background(), "default", "cli-user", 10, 0)
		fmt.Println()
		for _, s := range sessions {
			m := " "; if s.ID == *sessionID { m = "*" }
			title := s.ID[:8]
			for i := len(s.Messages) - 1; i >= 0; i-- {
				if s.Messages[i].Role == "user" { title = trunc(s.Messages[i].Content, 40); break }
			}
			fmt.Printf("  %s %s%s%s  [%d msgs]  %s\n", m, reset, title, gray, len(s.Messages), s.UpdatedAt.Format("15:04"))
		}
		fmt.Println()

	case "/analyst", "/coder", "/reviewer", "/executor":
		role := strings.TrimPrefix(parts[0], "/")
		task := strings.TrimSpace(strings.TrimPrefix(cmd, parts[0]))
		if task == "" { PrintWarning("usage: " + parts[0] + " <task>"); return }
		fmt.Printf("  %s⚙%s %s running…\n", yellow, reset, role)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result, err := app.Orchestrator.RunSubAgent(ctx, "cli-user", "default", role, task, nil)
		if err != nil { PrintError(fmt.Sprintf("%v", err)); return }
		if result != "" { fmt.Println(); fmt.Println(RenderMarkdown(result)) }
		PrintSuccess(role + " done")

	case "/team":
		task := strings.TrimSpace(strings.TrimPrefix(cmd, "/team"))
		if task == "" { PrintWarning("usage: /team <task>"); return }
		fmt.Printf("  %s⚙%s team: analyst → coder → reviewer\n", yellow, reset)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		prevResults := []string{}
		for _, role := range []string{"analyst", "coder", "reviewer"} {
			fmt.Printf("    %s…%s", role, reset)
			result, err := app.Orchestrator.RunSubAgent(ctx, "cli-user", "default", role, task, prevResults)
			if err != nil { PrintError(fmt.Sprintf("%v", err)); return }
			prevResults = append(prevResults, result)
			fmt.Print("\r\033[K")
		}
		fmt.Println()
		final := prevResults[len(prevResults)-1]
		fmt.Println(RenderMarkdown(final))
		PrintSuccess("team done")

	case "/handoff":
		PrintSuccess("Session saved")

	default:
		PrintWarning("Unknown: " + parts[0])
	}
}

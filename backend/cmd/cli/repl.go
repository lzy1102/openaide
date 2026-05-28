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
	"github.com/pterm/pterm"

	"openaide/backend/internal/infra"
	"openaide/backend/internal/kernel"
	"openaide/backend/internal/lang"
	"openaide/backend/internal/orchestration"
)

// ── REPL ──────────────────────────────────────────────────

func runREPL(app *infra.Application, continueSess bool) {
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
	fmt.Printf(`%s
   ██████╗ ██████╗ ███████╗███╗   ██╗ █████╗ ██╗██████╗ ███████╗
  ██╔═══██╗██╔══██╗██╔════╝████╗  ██║██╔══██╗██║██╔══██╗██╔════╝
  ██║   ██║██████╔╝█████╗  ██╔██╗ ██║███████║██║██║  ██║█████╗
  ██║   ██║██╔═══╝ ██╔══╝  ██║╚██╗██║██╔══██║██║██║  ██║██╔══╝
  ╚██████╔╝██║     ███████╗██║ ╚████║██║  ██║██║██████╔╝███████╗
   ╚═════╝ ╚═╝     ╚══════╝╚═╝  ╚═══╝╚═╝  ╚═╝╚═╝╚═════╝ ╚══════╝
%s`, cCyan, cReset)
	fmt.Println()
	if modelName != "" {
		fmt.Printf("  %s%s%s", cGreen, modelName, cReset)
	}
	if gitBranch != "" {
		fmt.Printf("  %s%s%s", cInfo, "  ◆  "+gitBranch, cReset)
	}
	fmt.Printf("  %s%s%s", cInfo, "  ◆  "+filepath.Base(cwd), cReset)
	fmt.Println()
	fmt.Println()
	fmt.Printf("  %s/h%s help  %s|%s  %s/q%s quit  %s|%s  Ctrl+C interrupt", cYellow, cReset, cInfo, cReset, cYellow, cReset, cInfo, cReset)
	fmt.Println()
	fmt.Println()

	// Session: resume or create
	var sessionID string
	if continueSess {
		sessions, _ := app.Orchestrator.ListSessions(context.Background(), "default", "cli-user", 10, 0)
		for _, s := range sessions {
			if len(s.Messages) > 0 { // 跳过空会话
				sessionID = s.ID
				msgCount := len(s.Messages)
				fmt.Printf("  %s📋 恢复会话%s: %s (%d 条消息)\n",
					cGreen, cReset, sessionID[:8], msgCount)

				// 显示最近几条历史消息
				history, _ := app.Orchestrator.GetSessionHistory(context.Background(), sessionID, 3)
				if len(history) > 0 {
					fmt.Printf("  %s最近消息:%s\n", cInfo, cReset)
					for _, msg := range history {
						switch msg.Role {
						case "user":
							fmt.Printf("    %s▸ %s%s\n", cUser, trunc(msg.Content, 80), cReset)
						case "assistant":
							fmt.Printf("    %s✓ %s%s\n", cToolOK, trunc(msg.Content, 80), cReset)
						}
					}
					fmt.Println()
				}
				break // 取第一个有消息的会话
			}
		}
	}
	if sessionID == "" {
		sess, _ := app.Orchestrator.CreateSession(context.Background(), "default", "cli-user")
		sessionID = sess.ID
	}

	rl := readline.NewInstance()
	rl.SetPrompt(PromptStyle(sessionID, modelName, false))
	rl.HistoryAutoWrite = true

	commands := []string{"/help", "/clear", "/mode", "/model", "/lang", "/log", "/sessions", "/session",
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
	// 文件路径补全
	if strings.Contains(prefix, "/") || strings.HasPrefix(prefix, ".") {
		if matches, err := filepath.Glob(prefix + "*"); err == nil && len(matches) > 0 && len(matches) < 50 {
			return &readline.TabCompleterReturnT{
				Prefix: prefix, Suggestions: matches, Descriptions: make(map[string]string),
			}
		}
	}
	return &readline.TabCompleterReturnT{}
}
var history []string // 会话内查询历史（Ctrl+R 搜索）

	// Ctrl+R 历史搜索
	rl.AutocompleteHistory = func(search string) ([]string, map[string]string) {
		var matches []string
		desc := make(map[string]string)
		for i := len(history) - 1; i >= 0 && len(matches) < 20; i-- {
			if strings.Contains(strings.ToLower(history[i]), strings.ToLower(search)) {
				matches = append(matches, history[i])
				desc[history[i]] = fmt.Sprintf("match %d", len(matches))
			}
		}
		return matches, desc
	}

	// 多行编辑：Alt+Enter → $EDITOR
	rl.GetMultiLine = func(line []rune) []rune {
		editor := os.Getenv("EDITOR")
		if editor == "" { editor = "vim" }
		tmp, _ := os.CreateTemp("", "openaide-*.md")
		if tmp == nil { return line }
		defer os.Remove(tmp.Name())
		tmp.WriteString(string(line))
		tmp.Close()
		cmd := osexec.Command(editor, tmp.Name())
		cmd.Stdin = os.Stdin; cmd.Stdout = os.Stdout; cmd.Stderr = os.Stderr
		cmd.Run()
		data, _ := os.ReadFile(tmp.Name())
		if len(data) > 0 {
			return []rune(strings.TrimSpace(string(data)))
		}
		return line
	}

	rl.HintText = func(line []rune, pos int) []rune {
		if len(line) == 0 {
			return []rune("/help for commands")
		}
		prefix := string(line)
		if strings.HasPrefix(prefix, "/") {
			var matches []string
			for _, cmd := range commands {
				if strings.HasPrefix(cmd, prefix) {
					matches = append(matches, cmd)
				}
			}
			if len(matches) > 0 && len(matches) <= 5 {
				return []rune(strings.Join(matches, "  "))
			}
		}
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

		// 添加到历史
		if len(history) == 0 || history[len(history)-1] != query {
			history = append(history, query)
		}

		if strings.HasPrefix(query, "/") {
			handleREPLCommand(app, query, &sessionID, &modelName)
			rl.SetPrompt(PromptStyle(sessionID, modelName, false))
			continue
		}

		// ── Smart routing: PreviewPlan → direct or team execution ──
		fmt.Println()
		fmt.Printf("  %s分析任务…%s", cInfo, cReset)
		planCtx, planCancel := context.WithTimeout(context.Background(), app.Orchestrator.PreviewTimeout)
		plan, planErr := app.Orchestrator.PreviewPlan(planCtx, query)
		planCancel()
		fmt.Print("\r\033[K")

		if planErr != nil || plan == nil || len(plan.Subtasks) <= 1 {
			executeStreamQuery(app, query, &sessionID)
		} else if len(plan.Subtasks) >= 4 {
			// DeepPlan: 深度研究 + 方案对比
			pterm.Info.Println("复杂任务，启动深度分析…")
			deepCtx, deepCancel := context.WithTimeout(context.Background(), app.Orchestrator.DeepTimeout)
			deepResult, deepErr := app.Orchestrator.DeepPlan(deepCtx, query)
			deepCancel()

			if deepErr != nil || deepResult == nil || len(deepResult.Proposals.Options) == 0 {
				PrintWarning("深度分析失败，使用默认计划")
				executePlanQuery(app, query, plan)
			} else {
				// 交互式方案选择
				var options []string
				for _, opt := range deepResult.Proposals.Options {
					options = append(options, fmt.Sprintf("%s  (风险:%s 工作量:%s)", opt.Name, opt.Risk, opt.Effort))
				}
				result, _ := pterm.DefaultInteractiveSelect.
					WithOptions(options).
					WithDefaultText("选择方案 (\u2191\u2193 移动, Enter 确认)").
					WithMaxHeight(10).
					Show()

				if result != "" {
					for i, opt := range options {
						if opt == result {
							selectedPlan, planErr := app.Orchestrator.DeepPlanFinalize(
								context.Background(), query, deepResult, i)
							if planErr != nil || selectedPlan == nil {
								PrintWarning("计划生成失败，使用默认计划")
								executePlanQuery(app, query, plan)
							} else {
								pterm.Success.Printfln("已选择: %s", deepResult.Proposals.Options[i].Name)
								executePlanQuery(app, query, selectedPlan)
							}
							break
						}
					}
				} else {
					executeStreamQuery(app, query, &sessionID)
				}
			}
		} else {
			// 显示规划
			Println()
			var items []pterm.BulletListItem
			items = append(items, pterm.BulletListItem{
				Level: 0, Text: pterm.Cyan(plan.Goal),
			})
			for _, st := range plan.Subtasks {
				items = append(items, pterm.BulletListItem{
					Level: 1, Text: st.Title,
				})
			}
			pterm.DefaultBulletList.WithItems(items).Render()
			Println()

			// 交互确认
			confirmed, _ := pterm.DefaultInteractiveConfirm.
				WithDefaultText(fmt.Sprintf("Execute plan? (%d subtasks)", len(plan.Subtasks))).
				WithConfirmText("y").
				WithRejectText("n").
				Show()

			if confirmed {
				executePlanQuery(app, query, plan)
			} else {
				executeStreamQuery(app, query, &sessionID)
			}
		}
		rl.SetPrompt(PromptStyle(sessionID, modelName, false))
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
	opts := kernel.QueryOptions{
		OnApproval: func(tool, path string) bool {
			result, _ := pterm.DefaultInteractiveConfirm.
				WithDefaultText(fmt.Sprintf("Allow %s %s?", pterm.Yellow(tool), path)).
				WithConfirmText("y").
				WithRejectText("n").
				Show()
			return result
		},
		OnBudgetExhausted: func(round, maxRounds int) bool {
			result, _ := pterm.DefaultInteractiveConfirm.
				WithDefaultText(fmt.Sprintf("Rounds exhausted (%d/%d), continue?", round, maxRounds)).
				WithConfirmText("y").
				WithRejectText("n").
				Show()
			return result
		},
	}
	stream, err := app.Orchestrator.ProcessQueryStream(ctx, "cli-user", "default", query, opts)
	if err != nil {
		PrintError(fmt.Sprintf("%v", err))
		cancel()
		return
	}

	startTime := time.Now()
	totalTools := 0
	totalTokens := 0
	cacheHit := 0
	cacheMiss := 0
	var fullResponse strings.Builder
	var streamBuffer strings.Builder // 增量渲染缓冲区
	var toolNames []string
	thinkShown := false

	for chunk := range stream {
		if chunk.Error != nil { PrintError(chunk.Error.Error()); break }
		if chunk.Content != "" {
		fullResponse.WriteString(chunk.Content)
		streamBuffer.WriteString(chunk.Content)
		// 增量渲染：遇段落分隔或代码块闭合时立即输出
		buf := streamBuffer.String()
		if strings.Contains(buf, "\n\n") || strings.Contains(buf, "```") {
			backtickCount := strings.Count(buf, "```")
			if strings.Contains(buf, "\n\n") || backtickCount%2 == 0 {
				fmt.Print(RenderMarkdown(buf))
				streamBuffer.Reset()
			}
		}
	}
		if chunk.Done {
			if chunk.Usage != nil { totalTokens = chunk.Usage.TotalTokens; cacheHit = chunk.Usage.PromptCacheHitTokens; cacheMiss = chunk.Usage.PromptCacheMissTokens }
			break
		}
		if chunk.ReasoningContent != "" && !thinkShown {
			// 只显示一行思考摘要，带工具上下文
			firstLine := strings.SplitN(chunk.ReasoningContent, "\n", 2)[0]
			if len(firstLine) > 100 { firstLine = firstLine[:97] + "..." }
			fmt.Printf("\r\033[K  %s[think]%s %s%s%s\n", cThink, cReset, cDim, firstLine, cReset)
			thinkShown = true
		}
		if chunk.Type == kernel.ChunkTypeToolCall && chunk.ToolName != "" {
			toolNames = append(toolNames, chunk.ToolName)
			totalTools++
			thinkShown = false
			// 实时更新工具状态行
			display := toolNames
			if len(display) > 4 { display = display[len(display)-4:] }
			toolStr := strings.Join(display, ", ")
		if len(toolStr) > 80 { toolStr = toolStr[:77] + "..." }
		fmt.Printf("\r\033[K  %s%s%s\n", pterm.Cyan("🔧"), cDim, toolStr)
			fmt.Print("\033[1A") // cursor up to overwrite on next update
		}
	}
	elapsed := time.Since(startTime)
	cancel()

	// 清除工具状态行
	fmt.Print("\r\033[K")

	if fullResponse.Len() > 0 {
		// 渲染最终回答
		rendered := RenderMarkdown(fullResponse.String())
		// 用分隔线将回答与工具区隔开
		fmt.Println()
		fmt.Println(rendered)
		fmt.Printf("\n%s──%s\n", cInfo, cReset)
	}
	PrintStatusBar(totalTokens, totalTools, elapsed, "deepseek-v4-pro", cacheHit, cacheMiss)
}

// ── Complex Query (sub-agent team execution) ──────────────

func executePlanQuery(app *infra.Application, query string, plan *orchestration.Plan) {
	startTime := time.Now()

	// pterm 进度条
	totalSteps := len(plan.Subtasks) + 2 // + verify + review
	progressBar, _ := pterm.DefaultProgressbar.
		WithTotal(totalSteps).
		WithTitle("执行中").
		WithShowCount(true).
		WithShowPercentage(true).
		WithRemoveWhenDone(true).
		Start()

	app.Orchestrator.OnProgress = func(phase, detail string) {
		progressBar.Add(1)
		progressBar.UpdateTitle(detail)
	}

	fmt.Printf("  %s执行中…%s\n", cInfo, cReset)

	// Heartbeat goroutine
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				elapsed := time.Since(startTime).Round(time.Second)
				fmt.Printf("\r\033[K  %s⏳ 执行中… (%v)%s\n", cInfo, elapsed, cReset)
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
	PrintStatusBar(resp.TokensUsed, totalTools, elapsed, "deepseek-v4-pro", resp.CacheHit, resp.CacheMiss)
}

// ── Commands ──────────────────────────────────────────────

func handleREPLCommand(app *infra.Application, cmd string, sessionID *string, modelName *string) {
	parts := strings.Fields(cmd)
	switch parts[0] {
	case "/exit", "/quit", "/q":
		fmt.Println("  Goodbye.")
		os.Exit(0)

	case "/help":
		Println()
		PrintPanel("OpenAIDE", "通用 AI 助手 — 编程 · 写作 · 研究 · 日常")
		Println()
		PrintList([]string{
			pterm.Cyan("/help") + " — 帮助",
			pterm.Cyan("/clear") + " — 清屏",
			pterm.Cyan("/mode [code|write|research|general]") + " — 切换模式",
			pterm.Cyan("/model [name]") + " — 查看/切换模型",
			pterm.Cyan("/lang zh|en") + " — 切换语言",
			pterm.Cyan("/log") + " — 最近日志",
			pterm.Cyan("/sessions") + " — 会话列表",
			pterm.Cyan("/handoff") + " — 保存会话",
			pterm.Cyan("/exit /quit /q") + " — 退出",
		}, false)
		Println()
		PrintList([]string{
			pterm.Cyan("/analyst <task>") + " — 分析任务",
			pterm.Cyan("/coder <task>") + " — 编码任务",
			pterm.Cyan("/reviewer <task>") + " — 审查任务",
			pterm.Cyan("/executor <task>") + " — 执行/验证",
			pterm.Cyan("/team <task>") + " — 完整团队链",
		}, false)
		Println()
		pterm.Info.Println("Ctrl+C 中断 | Ctrl+R 搜索历史 | Tab 补全 | ↑↓ 历史")
		Println()

	case "/clear":
		confirmed, _ := pterm.DefaultInteractiveConfirm.
			WithDefaultText("Clear all messages?").
			WithConfirmText("y").
			WithRejectText("n").
			Show()
		if confirmed {
			fmt.Print("\033[2J\033[H")
		}

	case "/model":
		if len(parts) >= 2 {
			app.SetModel(parts[1]); *modelName = parts[1]
			PrintSuccess("Model: " + parts[1])
		} else {
			info := app.LLMGateway.GetProviderInfos()
			if len(info) == 0 { PrintInfo("没有可用的模型"); return }

			var options []string
			for _, p := range info {
				m := " "; if p.Default { m = "●" }
				options = append(options, fmt.Sprintf("%s  %s  %s", m, p.Name, p.Model))
			}

			result, _ := pterm.DefaultInteractiveSelect.
				WithOptions(options).
				WithDefaultText("选择模型 (\u2191\u2193 移动, Enter 确认, Esc 取消)").
				WithMaxHeight(10).
				Show()

			if result != "" {
				fields := strings.Fields(result)
				if len(fields) >= 3 {
					app.SetModel(fields[2])
					*modelName = fields[2]
					PrintSuccess("Model: " + fields[2])
				}
			} else {
				PrintInfo("已取消")
			}
		}
		return

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
		for i := start; i < len(lines); i++ { fmt.Printf("  %s%s%s\n", cInfo, lines[i], cReset) }

		case "/sessions":
		sessions, _ := app.Orchestrator.ListSessions(context.Background(), "default", "cli-user", 10, 0)
		if len(sessions) == 0 { PrintInfo("没有会话"); return }

		var options []string
		for _, s := range sessions {
			title := s.ID[:8]
			for j := len(s.Messages) - 1; j >= 0; j-- {
				if s.Messages[j].Role == "user" { title = trunc(s.Messages[j].Content, 40); break }
			}
			marker := " "
			if s.ID == *sessionID { marker = "●" }
			options = append(options, fmt.Sprintf("%s  %s  [%d msgs]  %s", marker, title, len(s.Messages), s.UpdatedAt.Format("15:04")))
		}

		// 交互式选择（上下键 + Enter）
		result, _ := pterm.DefaultInteractiveSelect.
			WithOptions(options).
			WithDefaultText("选择会话 (↑↓ 移动, Enter 确认, Esc 取消)").
			WithMaxHeight(10).
			Show()

		if result != "" {
			for i, opt := range options {
				if opt == result {
					*sessionID = sessions[i].ID
					pterm.Success.Printfln("已切换到会话 %s", trunc(sessions[i].ID, 8))
					return
				}
			}
		}
		PrintInfo("已取消")
		return

	case "/session":
		if len(parts) >= 2 {
			sessions, _ := app.Orchestrator.ListSessions(context.Background(), "default", "cli-user", 10, 0)
			idx := 0
			fmt.Sscanf(parts[1], "%d", &idx)
			if idx > 0 && idx <= len(sessions) {
				*sessionID = sessions[idx-1].ID
				msgCount := len(sessions[idx-1].Messages)
				title := (*sessionID)[:8]
				for j := len(sessions[idx-1].Messages) - 1; j >= 0; j-- {
					if sessions[idx-1].Messages[j].Role == "user" { title = trunc(sessions[idx-1].Messages[j].Content, 30); break }
				}
				fmt.Printf("  %s✓ 切换到会话 %s (%d 条消息)%s\n", cSuccess, title, msgCount, cReset)
			} else {
				PrintWarning("无效的会话编号")
			}
		} else {
			PrintInfo("用法: /session <编号>")
		}
		return

case "/analyst", "/coder", "/reviewer", "/executor":
		role := strings.TrimPrefix(parts[0], "/")
		task := strings.TrimSpace(strings.TrimPrefix(cmd, parts[0]))
		if task == "" { PrintWarning("usage: " + parts[0] + " <task>"); return }
		fmt.Printf("  %s⚙%s %s running…\n", cYellow, cReset, role)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result, err := app.Orchestrator.RunSubAgent(ctx, "cli-user", "default", role, task, nil)
		if err != nil { PrintError(fmt.Sprintf("%v", err)); return }
		if result != "" { fmt.Println(); fmt.Println(RenderMarkdown(result)) }
		PrintSuccess(role + " done")

	case "/team":
		task := strings.TrimSpace(strings.TrimPrefix(cmd, "/team"))
		if task == "" { PrintWarning("usage: /team <task>"); return }
		fmt.Printf("  %s⚙%s team: analyst → coder → reviewer\n", cYellow, cReset)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		prevResults := []string{}
		for _, role := range []string{"analyst", "coder", "reviewer"} {
			fmt.Printf("    %s…%s", role, cReset)
			result, err := app.Orchestrator.RunSubAgent(ctx, "cli-user", "default", role, task, prevResults)
			if err != nil { PrintError(fmt.Sprintf("%v", err)); return }
			prevResults = append(prevResults, result)
			fmt.Print("\r\033[K")
		}
		fmt.Println()
		final := prevResults[len(prevResults)-1]
		fmt.Println(RenderMarkdown(final))
		PrintSuccess("team done")

	case "/mode":
		if len(parts) >= 2 {
			switch parts[1] {
			case "code", "coder", "dev":
				PrintSuccess("模式: 编程助手")
			case "write", "writer", "写作":
				PrintSuccess("模式: 写作助手")
			case "research", "研究", "分析":
				PrintSuccess("模式: 研究分析")
			case "general", "通用":
				PrintSuccess("模式: 通用助手")
			default:
				PrintInfo("可用模式: code, write, research, general")
			}
		} else {
			var options []string
			for _, m := range []string{"code  编程开发", "write  写作创作", "research  研究分析", "general  通用助手"} {
				options = append(options, m)
			}
			result, _ := pterm.DefaultInteractiveSelect.
				WithOptions(options).
				WithDefaultText("选择模式").
				Show()
			if result != "" {
				PrintSuccess("已切换: " + result)
			}
		}
		return

	case "/handoff":
		PrintSuccess("Session saved")

	default:
		PrintWarning("Unknown: " + parts[0])
	}
}

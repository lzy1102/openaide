package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/lmorg/readline/v4"
	"github.com/pterm/pterm"

	"openaide/backend/internal/infra"
	"openaide/backend/internal/kernel"
	"openaide/backend/internal/lang"
	"openaide/backend/internal/orchestration"
)

// ── File-backed History ───────────────────────────────────

type fileHistory struct {
	items []string
	path  string
}

func newFileHistory(path string) *fileHistory {
	h := &fileHistory{path: path}
	data, _ := os.ReadFile(path)
	if len(data) > 0 {
		h.items = strings.Split(strings.TrimSpace(string(data)), "\n")
	}
	return h
}

func (h *fileHistory) Write(s string) (int, error) {
	h.items = append(h.items, s)
	if len(h.items) > 1000 {
		h.items = h.items[len(h.items)-1000:]
	}
	os.WriteFile(h.path, []byte(strings.Join(h.items, "\n")), 0600)
	return len(h.items), nil
}

func (h *fileHistory) GetLine(i int) (string, error) {
	if i < 0 || i >= len(h.items) {
		return "", fmt.Errorf("out of range")
	}
	return h.items[i], nil
}

func (h *fileHistory) Len() int     { return len(h.items) }
func (h *fileHistory) Dump() interface{} { return h.items }

// ── REPL ──────────────────────────────────────────────────

func runREPL(app *infra.Application, continueSess, autoYes bool) {
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

	fmt.Printf(`%s
   ██████╗ ██████╗ ███████╗███╗   ██╗ █████╗ ██╗██████╗ ███████╗
  ██╔═══██╗██╔══██╗██╔════╝████╗  ██║██╔══██╗██║██╔══██╗██╔════╝
  ██║   ██║██████╔╝█████╗  ██╔██╗ ██║███████║██║██║  ██║█████╗
  ██║   ██║██╔═══╝ ██╔══╝  ██║╚██╗██║██╔══██║██║██║  ██║██╔══╝
  ╚██████╔╝██║     ███████╗██║ ╚████║██║  ██║██║██████╔╝███████╗
   ╚═════╝ ╚═╝     ╚══════╝╚═╝  ╚═══╝╚═╝  ╚═╝╚═╝╚═════╝ ╚══════╝
%s`, cCyan, cReset)
	if modelName != "" {
		fmt.Printf("  %s%s%s\n", cGreen, modelName, cReset)
	} else {
		fmt.Printf("  %s⚠ No API key configured%s\n", pterm.Yellow(""), cReset)
		fmt.Printf("  %s→ Edit ~/.openaide/config.yaml and add your API key%s\n", cInfo, cReset)
	}
	if gitBranch != "" {
		fmt.Printf("  %s%s%s", cInfo, "  ◆  "+gitBranch, cReset)
	}
	fmt.Printf("  %s%s%s", cInfo, "  ◆  "+filepath.Base(cwd), cReset)
	fmt.Println()
	fmt.Println()
	fmt.Printf("  %s ", Version)
	fmt.Printf("  %s/h%s help  %s|%s  %s/q%s quit  %s|%s  @file  Ctrl+C interrupt\n", cYellow, cReset, cInfo, cReset, cYellow, cReset, cInfo, cReset)
	fmt.Println()
	// Project context: OPENAIDE.md status
	if _, err := os.Stat(filepath.Join(cwd, "OPENAIDE.md")); err == nil {
		fmt.Printf("  %s📋 OPENAIDE.md loaded%s\n", cGreen, cReset)
	}

	// Provider status
	if len(info) > 0 {
		fmt.Printf("  %s✓ %d provider(s) ready%s\n", cGreen, len(info), cReset)
	} else {
		fmt.Printf("  %s⚠ No provider configured%s\n", pterm.Yellow(""), cReset)
		fmt.Printf("  %s→ Edit ~/.openaide/config.yaml to add your API key, then restart%s\n", cInfo, cReset)
	}
	fmt.Println()

	// Session: resume or create
	var sessionID string
	if continueSess {
		sessions, _ := app.Orchestrator.ListSessions(context.Background(), "default", "cli-user", 10, 0)
		for _, s := range sessions {
			if len(s.Messages) > 0 { // 跳过空会话
				sessionID = s.ID
				msgCount := len(s.Messages)
				fmt.Printf("  %s" + lang.T("repl.resume") + "%s: %s (%d msgs)\n",
					cGreen, cReset, sessionID[:8], msgCount)

				// 显示最近几条历史消息
				history, _ := app.Orchestrator.GetSessionHistory(context.Background(), sessionID, 3)
				if len(history) > 0 {
					fmt.Printf("  %s" + lang.T("repl.recent") + ":%s\n", cInfo, cReset)
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
		if continueSess {
			fmt.Printf("  %sNo previous sessions — starting a new one%s\n", cInfo, cReset)
			fmt.Println()
		}
		sess, _ := app.Orchestrator.CreateSession(context.Background(), "default", "cli-user")
		sessionID = sess.ID
	}
	sessionTitle := ""

	rl := readline.NewInstance()
	rl.SetPrompt(PromptStyle(sessionID, modelName, false, sessionTitle))
	rl.HistoryAutoWrite = true
	rl.History = newFileHistory(os.Getenv("HOME") + "/.openaide/history")


	commands := []string{"/help", "/clear", "/model", "/lang", "/log", "/sessions", "/session",
		"/handoff", "/exit", "/quit", "/q", "/analyst", "/coder", "/reviewer", "/executor", "/team", "/tree", "/init", "/status", "/undo", "/auto"}
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
			fmt.Println("\n  " + lang.T("repl.goodbye"))
			return
		}
		query := strings.TrimSpace(line)
		if query == "" { continue }

		// @file 引用：自动读取文件内容拼入 prompt
		query = expandAtRefs(query)
		// Set session title from first query
		if sessionTitle == "" { sessionTitle = trunc(query, 30) }

		// 添加到历史
		if len(history) == 0 || history[len(history)-1] != query {
			history = append(history, query)
		}

		if strings.HasPrefix(query, "/") {
			handleREPLCommand(app, query, &sessionID, &modelName, &autoYes)
			rl.SetPrompt(PromptStyle(sessionID, modelName, false, sessionTitle))
			continue
		}

		// ── Smart routing: PreviewPlan → direct or team execution ──
		fmt.Println()
		spinner, _ := pterm.DefaultSpinner.WithShowTimer(false).Start(lang.T("repl.analyzing"))
		planCtx, planCancel := context.WithTimeout(context.Background(), app.Orchestrator.PreviewTimeout)
		plan, planErr := app.Orchestrator.PreviewPlan(planCtx, query)
		planCancel()
		spinner.Stop()
		fmt.Print("\r\033[K")

		if planErr != nil || plan == nil || len(plan.Subtasks) <= 3 {
			executeStreamQuery(app, query, &sessionID, autoYes)
		} else if len(plan.Subtasks) >= 6 {
			// DeepPlan: 深度研究 + 方案对比
			pterm.Info.Println(lang.T("repl.deep_analysis"))
			deepCtx, deepCancel := context.WithTimeout(context.Background(), app.Orchestrator.DeepTimeout)
			deepResult, deepErr := app.Orchestrator.DeepPlan(deepCtx, query)
			deepCancel()

			if deepErr != nil || deepResult == nil || len(deepResult.Proposals.Options) == 0 {
				PrintWarning(lang.T("repl.deep_failed"))
				executePlanQuery(app, query, plan)
			} else {
				// 交互式方案选择
				var options []string
				for _, opt := range deepResult.Proposals.Options {
					options = append(options, fmt.Sprintf("%s  "+lang.T("repl.risk_effort"), opt.Name, opt.Risk, opt.Effort))
				}
				result, _ := pterm.DefaultInteractiveSelect.
					WithOptions(options).
					WithDefaultText("Select approach (↑↓ move, Enter select)").
					WithMaxHeight(10).
					Show()

				if result != "" {
					for i, opt := range options {
						if opt == result {
							selectedPlan, planErr := app.Orchestrator.DeepPlanFinalize(
								context.Background(), query, deepResult, i)
							if planErr != nil || selectedPlan == nil {
								PrintWarning(lang.T("repl.plan_failed"))
								executePlanQuery(app, query, plan)
							} else {
								fmt.Println("  "+lang.T("repl.selected", deepResult.Proposals.Options[i].Name))
								executePlanQuery(app, query, selectedPlan)
							}
							break
						}
					}
				} else {
					executeStreamQuery(app, query, &sessionID, autoYes)
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

			// 勾选要执行的子任务（空格=选中/取消，回车=确认）
			var taskOpts []string
			taskMap := make(map[string]int)
			for i, st := range plan.Subtasks {
				label := fmt.Sprintf("  %d. %s", i+1, st.Title)
				taskOpts = append(taskOpts, label)
				taskMap[label] = i
			}
			selected, _ := pterm.DefaultInteractiveMultiselect.
				WithOptions(taskOpts).
				WithDefaultText(lang.T("repl.select_subtasks", len(plan.Subtasks))).
				WithMaxHeight(12).
				Show()
			if len(selected) > 0 {
				var filtered []orchestration.SubTask
				for _, label := range selected {
					if idx, ok := taskMap[label]; ok {
						filtered = append(filtered, plan.Subtasks[idx])
					}
				}
				plan.Subtasks = filtered
				executePlanQuery(app, query, plan)
			} else {
				executeStreamQuery(app, query, &sessionID, autoYes)
			}

		}
		rl.SetPrompt(PromptStyle(sessionID, modelName, false, sessionTitle))
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

func executeStreamQuery(app *infra.Application, query string, sessionID *string, autoYes bool) {
	ctx, cancel := context.WithCancel(context.Background())
	opts := kernel.QueryOptions{
		OnApproval: func(tool, path, args string) bool {
			if autoYes { return true }
			fmt.Print("\r\033[K")
			slog.Warn("REPL waiting for tool approval", "tool", tool, "path", path)
			icon := toolIcon(tool)
			fmt.Println()
			fmt.Printf("  %s┌─ ⚡ Permission Required ─────────────────────%s\n", cYellow, cReset)
			fmt.Printf("  %s│%s  %s %s %s\n", cYellow, cReset, icon, cBold+tool+cReset, cDim+path+cReset)
			if args != "" {
				var prettyArgs map[string]interface{}
				if json.Unmarshal([]byte(args), &prettyArgs) == nil {
					for k, v := range prettyArgs {
						if k != "path" {
							fmt.Printf("  %s│%s    %s: %v\n", cYellow, cReset, cDim+k+cReset, v)
						}
					}
				}
			}
			fmt.Printf("  %s└──────────────────────────────────────────%s\n", cYellow, cReset)
			fmt.Println()
			options := []string{
				"  " + lang.T("repl.approve_yes"),
				"  " + lang.T("repl.approve_always"),
				"  " + lang.T("repl.approve_no"),
			}
			choice, _ := pterm.DefaultInteractiveSelect.
				WithOptions(options).
				WithDefaultText(lang.T("repl.allow_tool")).
				WithMaxHeight(5).
				Show()
			if choice == options[1] {
				autoYes = true
				return true
			}
			if choice == options[2] {
				return false
			}
			return true
		},
		OnBudgetExhausted: func(round, maxRounds int) bool {
			if autoYes { return true }
			slog.Warn("REPL waiting for budget extension approval", "round", round, "max", maxRounds)
			result, _ := pterm.DefaultInteractiveConfirm.
				WithDefaultText(lang.T("repl.rounds_exhausted", round, maxRounds)).
				WithConfirmText("y").
				WithRejectText("n").
				Show()
			return result
		},
	}
	stream, err := app.Orchestrator.ProcessQueryStream(ctx, "cli-user", "default", query, opts)
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "no provider configured"):
			PrintError("No LLM provider configured. Run 'openaide setup' or edit ~/.openaide/config.yaml")
		case strings.Contains(errMsg, "401") || strings.Contains(errMsg, "unauthorized"):
			PrintError("API key rejected. Check your API key in ~/.openaide/config.yaml")
		case strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "deadline"):
			PrintError("LLM request timed out. Check your network or try a different model.")
		default:
			PrintError(errMsg)
		}
		cancel()
		return
	}

	startTime := time.Now()
	totalTools := 0
	totalTokens := 0
	cacheHit := 0
	cacheMiss := 0
	var fullResponse strings.Builder
	var streamBuffer strings.Builder // 未完成行缓冲
	var rendered int               // 已渲染位置
	var toolNames []string
	thinkCount := 0
	firstChunk := true

	// 等待第一个chunk时显示思考中指示器
	thinkingSpinner, _ := pterm.DefaultSpinner.WithShowTimer(false).WithText(pterm.Cyan("Thinking...")).Start()

	for chunk := range stream {
		if firstChunk {
			thinkingSpinner.Stop()
			firstChunk = false
		}
		if chunk.Error != nil { PrintError(chunk.Error.Error()); break }
		if chunk.Content != "" {
		fullResponse.WriteString(chunk.Content)
		streamBuffer.WriteString(chunk.Content)
		// 逐行增量渲染：遇到 \n 就立即输出已完成的行
		buf := streamBuffer.String()
		lastNL := strings.LastIndex(buf, "\n")
		if lastNL >= 0 {
			complete := buf[:lastNL+1]
			if len(complete) > rendered {
				newPart := complete[rendered:]
				fmt.Print(RenderMarkdown(newPart))
				rendered = len(complete)
			}
			streamBuffer.Reset()
			streamBuffer.WriteString(buf[lastNL+1:])
			rendered = 0
		}
	}
		if chunk.Done {
			if chunk.Usage != nil { totalTokens = chunk.Usage.TotalTokens; cacheHit = chunk.Usage.PromptCacheHitTokens; cacheMiss = chunk.Usage.PromptCacheMissTokens }
			break
		}
		if chunk.ReasoningContent != "" && thinkCount < 2 {
			// 只显示一行思考摘要，带工具上下文
			firstLine := strings.SplitN(chunk.ReasoningContent, "\n", 2)[0]
			if len(firstLine) > 100 { firstLine = firstLine[:97] + "..." }
			fmt.Printf("\r\033[K  %s[think]%s %s%s%s\n", cThink, cReset, cDim, firstLine, cReset)
			thinkCount++
		}
		if chunk.Type == kernel.ChunkTypeToolCall && chunk.ToolName != "" {
			toolNames = append(toolNames, chunk.ToolName)
			totalTools++
			icon := toolIcon(chunk.ToolName)
			fmt.Printf("\r\033[K  %s%s %s%s\n", cYellow, icon, chunk.ToolName, cReset)
		}
	}
	elapsed := time.Since(startTime)
	cancel()

	// 清除工具状态行
	fmt.Print("\r\033[K")

	if fullResponse.Len() > 0 {
		// 渲染最终回答
		rendered := RenderMarkdown(fullResponse.String())
		fmt.Println(rendered)
	}
	PrintStatusBar(totalTokens, totalTools, elapsed, "deepseek-v4-pro", cacheHit, cacheMiss)

	fmt.Printf("\n%s▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸▸%s\n\n", cDim, cReset)
	if qs := app.ToolRegistry.GetPendingQuestions(); len(qs) > 0 {
		fmt.Println()
		for _, q := range qs {
			fmt.Printf("  %s❓ %s%s\n", pterm.Yellow(""), q, cReset)
		}
	}
}

// ── Complex Query (sub-agent team execution) ──────────────

func executePlanQuery(app *infra.Application, query string, plan *orchestration.Plan) {
	startTime := time.Now()

	// pterm 进度条
	totalSteps := len(plan.Subtasks) + 2 // + verify + review
	progressBar, _ := pterm.DefaultProgressbar.
		WithTotal(totalSteps).
		WithTitle(lang.T("repl.executing")).
		WithShowCount(true).
		WithShowPercentage(true).
		WithRemoveWhenDone(true).
		Start()

	app.Orchestrator.OnProgress = func(phase, detail string) {
		progressBar.Add(1)
		progressBar.UpdateTitle(detail)
	}

	fmt.Printf("  %s" + lang.T("repl.executing") + "…%s\n", cInfo, cReset)

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
	if qs := app.ToolRegistry.GetPendingQuestions(); len(qs) > 0 {
		fmt.Println()
		for _, q := range qs {
			fmt.Printf("  %s❓ %s%s\n", pterm.Yellow(""), q, cReset)
		}
	}
}

// ── Commands ──────────────────────────────────────────────

// toolIcon returns a color-coded icon for the tool
func toolIcon(name string) string {
	switch name {
	case "read_file", "list_directory", "search_files", "search_symbols", "search_knowledge":
		return pterm.Cyan("📖") // read-only
	case "write_file", "diff_edit", "execute_command":
		return pterm.Yellow("✏️") // write
	case "web_search", "web_fetch", "ai_search":
		return pterm.Magenta("🌐") // network
	case "git_status", "git_diff", "git_log", "git_blame":
		return pterm.Green("🔀") // git
	default:
		return "🔧"
	}
}

// showFileTree prints a directory tree using pterm
func showFileTree() {
	cwd, _ := os.Getwd()
	var items []pterm.TreeNode
	filepath.WalkDir(cwd, func(path string, d os.DirEntry, err error) error {
		if err != nil { return nil }
		rel, _ := filepath.Rel(cwd, path)
		if rel == "." { return nil }
		// Skip hidden dirs
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
			return filepath.SkipDir
		}
		// Limit depth
		depth := len(strings.Split(rel, string(filepath.Separator)))
		if depth > 4 { return filepath.SkipDir }
		if d.IsDir() {
			items = append(items, pterm.TreeNode{Text: pterm.Cyan("📁 " + d.Name()), Children: []pterm.TreeNode{}})
		} else {
			items = append(items, pterm.TreeNode{Text: "  " + d.Name()})
		}
		return nil
	})
	if len(items) > 50 {
		items = items[:50]
		pterm.Info.Println("(showing first 50 entries)")
	}
	pterm.DefaultTree.WithRoot(pterm.TreeNode{Text: pterm.Green("📁 " + filepath.Base(cwd)), Children: items}).Render()
}


// handleInit analyzes the current project and generates OPENAIDE.md
func handleInit(app *infra.Application) {
	cwd, _ := os.Getwd()
	projectName := filepath.Base(cwd)

	// Check if OPENAIDE.md already exists
	if _, err := os.Stat(filepath.Join(cwd, "OPENAIDE.md")); err == nil {
		pterm.Warning.Println("OPENAIDE.md already exists. Delete it first to regenerate, or edit it directly.")
		return
	}

	// Collect project context
	var ctx strings.Builder
	ctx.WriteString(fmt.Sprintf("Project directory: %s\n", cwd))
	ctx.WriteString(fmt.Sprintf("Project name: %s\n\n", projectName))

	// Scan for key files
	entries, _ := os.ReadDir(cwd)
	fileTypes := map[string]int{}
	var keyFiles []string
	keyNames := map[string]bool{
		"go.mod": true, "package.json": true, "Cargo.toml": true, "Makefile": true,
		"pyproject.toml": true, "requirements.txt": true, "CMakeLists.txt": true,
		"Dockerfile": true, "docker-compose.yml": true, "docker-compose.yaml": true,
		".gitignore": true, "README.md": true,
	}
	for _, e := range entries {
		if e.IsDir() { continue }
		ext := filepath.Ext(e.Name())
		if ext != "" { fileTypes[ext]++ }
		if keyNames[e.Name()] { keyFiles = append(keyFiles, e.Name()) }
	}
	ctx.WriteString(fmt.Sprintf("File types: %v\n", fileTypes))
	ctx.WriteString(fmt.Sprintf("Key files: %v\n\n", keyFiles))

	// Top-level directories
	ctx.WriteString("Top-level directories:\n")
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			ctx.WriteString(fmt.Sprintf("  %s/\n", e.Name()))
		}
	}

	spinner, _ := pterm.DefaultSpinner.Start("Analyzing project and generating OPENAIDE.md…")
	queryContent := fmt.Sprintf(
		"Based on this project analysis, write an OPENAIDE.md file. Include:\n"+
			"1. Project name and one-line summary\n"+
			"2. Common commands (build, test, run)\n"+
			"3. Architecture overview (key directories and their purposes)\n"+
			"4. Conventions observed from file types\n\n"+
			"%s\n\nWrite ONLY the OPENAIDE.md content, no preamble. Use Markdown format. Keep it concise.",
		ctx.String())
	resp, err := app.Orchestrator.ProcessQuery(context.Background(), "cli-user", "default", queryContent, kernel.QueryOptions{MaxTokens: 2000})
	spinner.Stop()
	fmt.Print("\r\033[K")

	if err != nil {
		PrintError(fmt.Sprintf("Failed: %v", err))
		return
	}
	if resp.Content == "" {
		PrintWarning("Empty response, try again")
		return
	}

	os.WriteFile(filepath.Join(cwd, "OPENAIDE.md"), []byte(resp.Content), 0644)
	pterm.Success.Printfln("OPENAIDE.md generated (%d chars) — will be loaded in future sessions", len(resp.Content))
}

// expandAtRefs finds @filename references in the query and prepends their content
func expandAtRefs(query string) string {
	re := regexp.MustCompile(`@(\S+)`)
	matches := re.FindAllStringSubmatch(query, -1)
	if len(matches) == 0 { return query }

	var files []string
	for _, m := range matches {
		pattern := m[1]
		// Support glob: @*.go, @src/*.go
		if strings.ContainsAny(pattern, "*?[]") {
			globs, _ := filepath.Glob(pattern)
			for _, p := range globs {
				if data, err := os.ReadFile(p); err == nil {
					files = append(files, p)
					fmt.Printf("  %s@%s%s %s(%db)%s\n", cGreen, p, cReset, cDim, len(data), cReset)
				}
			}
		} else if _, err := os.ReadFile(pattern); err == nil {
			files = append(files, pattern)
			fmt.Printf("  %s@%s%s %s\n", cGreen, pattern, cReset, cDim+"(included)"+cReset)
		}
	}
	if len(files) == 0 { return query }

	var sb strings.Builder
	for i, path := range files {
		if i >= 20 { sb.WriteString(fmt.Sprintf("... (%d more files)\n", len(files)-i)); break }
		data, _ := os.ReadFile(path)
		c := string(data)
		if len(c) > 5000 { c = c[:5000] + "\n... (truncated)" }
		sb.WriteString(fmt.Sprintf("Content of %s:\n---\n%s\n---\n\n", path, c))
	}
	sb.WriteString("User prompt: " + query)
	return sb.String()
}

// expandAtRefs finds @filename references and prepends file content
// handleUndo restores the session to the last checkpoint
func handleUndo(app *infra.Application, sessionID string) {
	ctx := context.Background()
	if ak, ok := app.Kernel.(*kernel.AgentKernel); ok {
		msgs, _, found, err := ak.ResumeSession(ctx, sessionID)
		if err != nil || !found || len(msgs) == 0 {
			PrintWarning("No checkpoint to restore")
			return
		}
		pterm.Success.Printfln("Restored checkpoint (%d messages)", len(msgs))
		// Clear screen to show fresh context
		fmt.Print("[2J[H")
	}
}

func handleREPLCommand(app *infra.Application, cmd string, sessionID *string, modelName *string, autoYes *bool) {
	parts := strings.Fields(cmd)
	switch parts[0] {
	case "/exit", "/quit", "/q":
		fmt.Println("  Goodbye.")
		os.Exit(0)

	case "/help":
		Println()
		PrintPanel("OpenAIDE", lang.T("repl.help_title"))
		Println()
		PrintList([]string{
			pterm.Cyan("/help") + " — " + lang.T("cli.help"),
			pterm.Cyan("/clear") + " — " + lang.T("repl.help_clear"),
						pterm.Cyan("/model [name]") + " — " + lang.T("repl.help_model"),
			pterm.Cyan("/lang zh|en") + " — " + lang.T("repl.help_lang"),
			pterm.Cyan("/log") + " — " + lang.T("repl.help_log"),
			pterm.Cyan("/sessions") + " — " + lang.T("repl.help_sessions"),
			pterm.Cyan("/handoff") + " — " + lang.T("repl.help_handoff"),
			pterm.Cyan("/exit /quit /q") + " — " + lang.T("repl.help_exit"),
		}, false)
		Println()
		PrintList([]string{
			pterm.Cyan("/analyst <task>") + " — " + lang.T("repl.help_analyst"),
			pterm.Cyan("/coder <task>") + " — " + lang.T("repl.help_coder"),
			pterm.Cyan("/reviewer <task>") + " — " + lang.T("repl.help_reviewer"),
			pterm.Cyan("/executor <task>") + " — " + lang.T("repl.help_executor"),
			pterm.Cyan("/team <task>") + " — " + lang.T("repl.help_team"),
		pterm.Cyan("/tree") + " — browse project files",
		pterm.Cyan("/status") + " — system health & providers",
		pterm.Cyan("/undo") + " — rollback to last checkpoint",
		pterm.Cyan("/auto") + " — toggle autonomous (goal-driven) mode",
		pterm.Cyan("/init") + " — generate OPENAIDE.md for this project",
		}, false)
		Println()
		pterm.Info.Println(lang.T("repl.help_intro"))
		Println()

	case "/clear":
		fmt.Print("\033[2J\033[H")
		app.Orchestrator.DeleteSession(context.Background(), *sessionID)
		sess, _ := app.Orchestrator.CreateSession(context.Background(), "default", "cli-user")
		*sessionID = sess.ID

	case "/model":
		if len(parts) >= 2 {
			app.SetModel(parts[1]); *modelName = parts[1]
			PrintSuccess("Model: " + parts[1])
		} else {
			info := app.LLMGateway.GetProviderInfos()
			if len(info) == 0 { PrintInfo(lang.T("repl.no_models")); return }

			fmt.Printf("  %sProviders:%s\n", cBold, cReset)
			var options []string
			for _, p := range info {
				m := " "; if p.Default { m = "●" }
				label := fmt.Sprintf("%s  %s  %s", m, p.Name, p.Model)
				if p.Default { label += "  (default)" }
				options = append(options, label)
			}

			result, _ := pterm.DefaultInteractiveSelect.
				WithOptions(options).
				WithDefaultText("Select model (↑↓ move, Enter select, Esc cancel)").
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
				PrintInfo(lang.T("repl.canceled"))
			}
		}
		return

	case "/tree":
		showFileTree()
		return

	case "/init":
		handleInit(app)
		return

	case "/lang":
		if len(parts) >= 2 {
			switch parts[1] {
			case "zh": lang.SetLang(lang.ZH); PrintSuccess(lang.T("repl.lang_zh"))
			case "en": lang.SetLang(lang.EN); PrintSuccess("English")
			}
		}

	case "/log":
		lines := tuiLogBuf.snapshot()
		start := 0; if len(lines) > 20 { start = len(lines) - 20 }
		for i := start; i < len(lines); i++ { fmt.Printf("  %s%s%s\n", cInfo, lines[i], cReset) }

		case "/sessions":
		sessions, _ := app.Orchestrator.ListSessions(context.Background(), "default", "cli-user", 10, 0)
		if len(sessions) == 0 { PrintInfo(lang.T("repl.no_sessions")); return }

		var options []string
		for _, s := range sessions {
			title := s.ID[:8]
			// Use metadata title or first user message
			if t, ok := s.Metadata["title"]; ok {
				if ts, ok2 := t.(string); ok2 && ts != "" {
					title = trunc(ts, 30)
				}
			} else {
				for j := len(s.Messages) - 1; j >= 0; j-- {
					if s.Messages[j].Role == "user" { title = trunc(s.Messages[j].Content, 40); break }
				}
			}
			marker := " "
			if s.ID == *sessionID { marker = "●" }
			options = append(options, fmt.Sprintf("%s  %s  [%d msgs]  %s", marker, title, len(s.Messages), s.UpdatedAt.Format("15:04")))
		}

		// 交互式选择（上下键 + Enter）
		result, _ := pterm.DefaultInteractiveSelect.
			WithOptions(options).
			WithDefaultText(lang.T("repl.select_session")).
			WithMaxHeight(10).
			Show()

		if result != "" {
			for i, opt := range options {
				if opt == result {
					action, _ := pterm.DefaultInteractiveSelect.
						WithOptions([]string{lang.T("repl.action_switch"), lang.T("repl.action_delete"), lang.T("repl.action_cancel")}).
						WithDefaultText(lang.T("repl.select_action")).
						WithMaxHeight(5).
						Show()
					if strings.HasPrefix(action, "Switch") {
						*sessionID = sessions[i].ID
						pterm.Success.Printfln("Switched to %s", trunc(sessions[i].ID, 8))
					} else if strings.HasPrefix(action, "Delete") {
						app.Orchestrator.DeleteSession(context.Background(), sessions[i].ID)
						pterm.Success.Printfln("Deleted %s", trunc(sessions[i].ID, 8))
					}
					return
				}
			}
		}
		PrintInfo(lang.T("repl.canceled"))
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
				fmt.Printf("  %s"+lang.T("repl.switched_sess", title, msgCount)+"%s\n", cSuccess, cReset)
			} else {
				PrintWarning(lang.T("repl.invalid_sess"))
			}
		} else {
			PrintInfo(lang.T("repl.usage_sess"))
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


	case "/handoff":
		PrintSuccess("Session saved")

	default:
		PrintWarning("Unknown: " + parts[0])
	}
}

func askSessionFeedback(app *infra.Application, sessionID string) {
	// Phase-based feedback: ask once at session boundaries (/clear, /exit).
	// Only prompt if the session actually involved meaningful work.
	session, err := app.Orchestrator.GetSession(context.Background(), sessionID)
	if err != nil || session == nil || len(session.Messages) < 4 {
		return // too short to be meaningful
	}

	fmt.Printf("\n  %sHow was this session?%s [%sy=good  n=bad  ↵=skip%s] ", cBold, cReset, cReset, cDim)
	var feedback string
	fmt.Scanf("%s", &feedback)
	feedback = strings.TrimSpace(strings.ToLower(feedback))

	if feedback == "y" || feedback == "yes" || feedback == "good" || feedback == "g" {
		if ak, ok := app.Kernel.(*kernel.AgentKernel); ok {
			ak.SetUserVerdict(context.Background(), sessionID, "good")
		}
		fmt.Printf("\r\033[K  %s✓ Thanks — this helps me improve.%s\n\n", pterm.Green(""), cReset)
	} else if feedback == "n" || feedback == "no" || feedback == "bad" || feedback == "b" {
		if ak, ok := app.Kernel.(*kernel.AgentKernel); ok {
			ak.SetUserVerdict(context.Background(), sessionID, "bad")
		}
		fmt.Printf("\r\033[K  %s✗ Noted — I'll learn from this.%s\n\n", pterm.Yellow(""), cReset)
	}
}

package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
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

	projectMemory := LoadProjectMemory()
	if projectMemory != "" {
		fmt.Printf("  %s\n", R.Info.Render("📖 Project memory loaded (OPENAIDE.md)"))
	}

	RenderWelcome(apiURL, model, stream)

	dialogueID := ""
	var history []Message
	currentProjectID := ""
	workspaceLoaded := false

	// 1. 优先从 .openaide 工作区恢复（同步，因为需要 dialogueID）
	if HasWorkspace() {
		ws, err := LoadWorkspaceState()
		if err == nil && ws.ProjectID != "" {
			currentProjectID = ws.ProjectID
			projects, _ := FetchProjects(apiURL)
			for _, p := range projects {
				if p.ID == currentProjectID {
					fmt.Printf("  %s\n", R.Info.Render("📂 Workspace restored: "+p.Name))
					workspaceLoaded = true
					break
				}
			}
			// 恢复对话
			if ws.DialogueID != "" {
				messages, err := FetchMessages(apiURL, ws.DialogueID)
				if err == nil && len(messages) > 0 {
					dialogueID = ws.DialogueID
					history = messages
					shortID := dialogueID
					if len(shortID) > 8 {
						shortID = shortID[:8]
					}
					RenderSessionLine(0, "Workspace Session", shortID, "", true)
					fmt.Println()
				}
			}
		}
	}

	// 2. 如果没有工作区，尝试自动检测项目（同步）
	if currentProjectID == "" {
		detectedProject := AutoDetectProject(apiURL)
		if detectedProject != "" {
			currentProjectID = detectedProject
			projects, _ := FetchProjects(apiURL)
			for _, p := range projects {
				if p.ID == detectedProject {
					fmt.Printf("  %s\n", R.Info.Render("📂 Auto-detected project: "+p.Name))
					break
				}
			}
		}
	}

	// 3. 异步加载项目历史对话，不阻塞启动
	historyLoaded := make(chan struct{})
	go func() {
		defer close(historyLoaded)
		if dialogueID != "" {
			return // 已有工作区对话
		}
		dialogues, _ := FetchDialoguesByProject(apiURL, "cli-user", currentProjectID)
		if len(dialogues) == 0 {
			return
		}
		latest := dialogues[0]
		for _, d := range dialogues {
			if d.UpdatedAt > latest.UpdatedAt {
				latest = d
			}
		}
		messages, err := FetchMessages(apiURL, latest.ID)
		if err == nil && len(messages) > 0 {
			// 注意：这里不能直接修改 dialogueID/history，因为主循环可能已开始
			// 使用回调方式通知
			onHistoryLoaded(latest.ID, messages)
		}
	}()

	// 4. 如果没有恢复对话，立即创建新对话（不等待历史加载）
	if dialogueID == "" {
		dialogue, err := CreateDialogueWithProject(apiURL, currentProjectID)
		if err != nil {
			RenderErrorBlock(fmt.Sprintf("Failed to create dialogue: %v", err))
		} else {
			dialogueID = dialogue.ID
		}
	}

	// 5. 如果是新进入的项目且没有工作区，创建 .openaide 标记
	if currentProjectID != "" && !workspaceLoaded && !HasWorkspace() {
		if err := InitWorkspace(currentProjectID); err == nil {
			slog.Debug("workspace initialized", "project_id", currentProjectID)
		}
	}

	// 6. 保存对话 ID 到工作区
	if HasWorkspace() && dialogueID != "" {
		_ = UpdateWorkspaceDialogue(dialogueID)
	}

	// 等待历史加载完成（短暂等待，不阻塞用户输入）
	select {
	case <-historyLoaded:
		// 历史加载完成，如果异步加载找到了更早的对话，可以选择切换
	case <-time.After(500 * time.Millisecond):
		// 超时，继续启动，历史在后台加载
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
				RenderGoodbye()
				return
			}
			if err == readline.ErrInterrupt {
				fmt.Printf("\n  %s\n", R.Error.Render("✗ Interrupted"))
				continue
			}
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		if input == "exit" || input == "quit" || input == "/exit" || input == "/quit" {
			RenderGoodbye()
			return
		}

		if strings.HasPrefix(input, "/") {
			var newDialogueID string
			newDialogueID, model, stream, currentProjectID = handleSlashCommand(input, apiURL, dialogueID, model, stream, &history, cfg, ctx, currentProjectID)
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

		input = expandFileReferences(input)

		if projectMemory != "" {
			input = fmt.Sprintf("[Project Context]\n%s\n[/Project Context]\n\n%s", projectMemory, input)
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
			response, err = runStreamChat(ctx, apiURL, dialogueID, input, model, timeout, cfg)
		} else {
			response, err = runSyncChat(apiURL, dialogueID, input, model)
		}

		if err != nil {
			if ctx.Err() == context.Canceled {
				fmt.Printf("\n  %s\n", R.Warning.Render("⚠ Response interrupted"))
			} else {
				RenderErrorBlock(fmt.Sprintf("%v", err))
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

		if compacted, didCompact := AutoCompactIfNeeded(apiURL, dialogueID, history, cfg.Chat.MaxContextTokens); didCompact {
			if compacted != nil {
				history = *compacted
			}
		}

		RenderTurnDivider()
	}
}

func runStreamChat(ctx context.Context, apiURL, dialogueID, input, model string, timeout int, cfg *Config) (string, error) {
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
			spinner.UpdateLabel("thinking...")
			thinkingLineCount++
			ShowThinkingBlock(content)
		},
		OnGuardianReview: func(tool, verdict, riskLevel, reason string) {
			if currentSpinner != nil {
				currentSpinner.Pause()
			}
			ShowGuardianReview(tool, verdict, riskLevel, reason)
			if currentSpinner != nil {
				currentSpinner.Resume()
			}
		},
		OnProgress: func(content string) {
			spinner.UpdateLabel(content)
		},
		OnToolCall: func(tool string, params string) {
			allowed, needsConfirm := checkToolPermission(tool, cfg)
			if !allowed {
				if needsConfirm {
					if promptToolConfirmation(tool, params) {
						spinner.UpdateLabel("tool: " + tool)
						ShowToolCall(tool, params)
					} else {
						fmt.Printf("  %s\n", R.Warning.Render("🔒 Tool call denied: "+tool))
					}
				} else {
					fmt.Printf("  %s\n", R.Error.Render("🚫 Tool call blocked: "+tool))
				}
				return
			}
			spinner.UpdateLabel("tool: " + tool)
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
			go func() {
				if err := TriggerMemoryExtraction(apiURL, dialogueID); err == nil {
					slog.Debug("memory extraction triggered", "dialogue_id", dialogueID)
				}
			}()
		},
		OnCompact: func(reason string, beforeMsgs, afterMsgs, savedTokens int) {
			fmt.Println(RenderCompactLine(reason, beforeMsgs, afterMsgs, savedTokens))
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
		}
	}

	if response != "" {
		fmt.Println()
		fmt.Println(RenderResponseBoxBottom())
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
			if totalTokens > 0 {
				contextPercent = streamUsage.PromptTokens * 100 / totalTokens
			}
			fmt.Println(RenderContextUsage(streamUsage.PromptTokens, streamUsage.CompletionTokens, totalTokens, 0))
			if warn := RenderContextWarning(contextPercent); warn != "" {
				fmt.Println(warn)
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
		fmt.Println()
		fmt.Println(RenderResponseBoxBottom())
		ShowResponseFooter(model, elapsed, 0)
	}

	return response, nil
}

func ShowResponseFooter(model string, elapsed time.Duration, tokens int) {
	fmt.Println(RenderResponseFooter(model, elapsed, tokens))
}

func handleSlashCommand(cmd, apiURL, dialogueID string, model string, stream bool, history *[]Message, cfg *Config, ctx context.Context, currentProjectID string) (string, string, bool, string) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", model, stream, currentProjectID
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
		dialogue, err := CreateDialogueWithProject(apiURL, currentProjectID)
		if err != nil {
			RenderErrorBlock(fmt.Sprintf("Failed: %v", err))
		} else {
			newDialogueID = dialogue.ID
			*history = []Message{}
			if HasWorkspace() {
				_ = UpdateWorkspaceDialogue(newDialogueID)
			}
			if carrySummary != "" {
				_, _ = SendMessage(apiURL, dialogue.ID, carrySummary, model)
				*history = append(*history, Message{
					ID:         GenerateID(),
					DialogueID: dialogue.ID,
					Sender:     "user",
					Content:    carrySummary,
				})
				fmt.Printf("  %s\n", RenderStatusLine("new conversation", dialogue.ID[:8]+"...", "carried context"))
			} else {
				fmt.Printf("  %s\n", RenderStatusLine("new conversation", dialogue.ID[:8]+"..."))
			}
		}
	case "/compact":
		if dialogueID == "" {
			RenderErrorBlock("No active conversation")
			break
		}
		fmt.Printf("  %s\n", RenderToolCallLine("compact", "compressing context..."))
		result, err := RequestCompaction(apiURL, dialogueID, "full")
		if err != nil {
			trimmed := TrimToolResults(*history, 1000)
			summarized := SummarizeOldMessages(trimmed, 4)
			*history = summarized
			oldTokens := estimateHistoryTokens(*history)
			RenderInfoLine(fmt.Sprintf("Local compression applied (%d tokens)", oldTokens))
		} else {
			RenderCompactionResult(result)
		}
	case "/context":
		chapters := SplitIntoChapters(*history)
		tokens := estimateHistoryTokens(*history)
		fmt.Println()
		fmt.Printf("  %s\n\n", R.Bold.Render("Context Status"))
		fmt.Printf("  %-14s %s\n", "Messages:", R.Accent.Render(fmt.Sprintf("%d", len(*history))))
		fmt.Printf("  %-14s %s\n", "Chapters:", R.Accent.Render(fmt.Sprintf("%d", len(chapters))))
		fmt.Printf("  %-14s %s\n", "Est. Tokens:", R.Accent.Render(FormatTokenCount(tokens)))
		if cfg.Chat.MaxContextTokens > 0 {
			pct := tokens * 100 / cfg.Chat.MaxContextTokens
			fmt.Printf("  %-14s %s\n", "Context Window:", R.Dim.Render(FormatTokenCount(cfg.Chat.MaxContextTokens)))
			fmt.Printf("  %-14s %s\n", "Usage:", R.Accent.Render(fmt.Sprintf("%d%%", pct)))
		}
		if len(chapters) > 0 {
			fmt.Printf("\n  %s\n", R.Dim.Render("Chapter preview:"))
			maxPreview := 5
			if len(chapters) < maxPreview {
				maxPreview = len(chapters)
			}
			for i := 0; i < maxPreview; i++ {
				ch := chapters[i]
				fmt.Printf("  %s %s %s\n",
					R.Accent.Render(fmt.Sprintf("§%d", ch.ID)),
					R.Bold.Render(truncateStr(ch.Title, 40)),
					R.Dim.Render(fmt.Sprintf("(%d msgs)", ch.MsgCount)))
			}
			if len(chapters) > maxPreview {
				fmt.Printf("  %s\n", R.Dim.Render(fmt.Sprintf("  ... +%d more chapters", len(chapters)-maxPreview)))
			}
		}
		if dialogueID != "" {
			budget, err := GetContextBudget(apiURL, dialogueID)
			if err == nil && budget != nil {
				RenderContextBudget(budget)
			}
		}
	case "/chapters":
		chapters := SplitIntoChapters(*history)
		if len(chapters) == 0 {
			RenderInfoLine("No conversation history to analyze")
			break
		}
		totalTokens := 0
		for _, ch := range chapters {
			totalTokens += ch.Tokens
		}
		outline := &ChapterOutline{
			Chapters:    chapters,
			TotalTokens: totalTokens,
			CreatedAt:   time.Now(),
		}
		RenderChapterOutline(outline)
	case "/clear":
		if dialogueID == "" {
			*history = []Message{}
			RenderSuccessLine("Context cleared")
		} else {
			if err := ClearMessages(apiURL, dialogueID); err != nil {
				RenderErrorBlock(fmt.Sprintf("Clear failed: %v", err))
			} else {
				*history = []Message{}
				RenderSuccessLine("Context cleared")
			}
		}
	case "/model":
		if len(parts) > 1 {
			model = parts[1]
			fmt.Printf("  %s\n", RenderStatusLine("model", R.Accent.Render(model)))
		} else {
			result := RunModelSelect(apiURL, model)
			if result.Changed {
				model = result.Selected
				fmt.Printf("  %s\n", RenderStatusLine("model", R.Accent.Render(model)))
			}
		}
	case "/stream":
		if len(parts) > 1 {
			if parts[1] == "on" {
				stream = true
				fmt.Printf("  %s\n", RenderStatusLine("streaming", "on"))
			} else if parts[1] == "off" {
				stream = false
				fmt.Printf("  %s\n", RenderStatusLine("streaming", "off"))
			}
		} else {
			stream = !stream
			label := "off"
			if stream {
				label = "on"
			}
			fmt.Printf("  %s\n", RenderStatusLine("streaming", label))
		}
	case "/sessions":
		dialogues, err := FetchDialoguesByProject(apiURL, "cli-user", currentProjectID)
		if err != nil || len(dialogues) == 0 {
			projectLabel := ""
			if currentProjectID != "" {
				projectLabel = " in current project"
			}
			RenderInfoLine("No sessions found" + projectLabel)
			break
		}
		fmt.Println()
		fmt.Printf("  %s\n\n", R.Bold.Render("Sessions"))
		maxShow := 15
		if len(dialogues) > maxShow {
			dialogues = dialogues[:maxShow]
		}
		for i, d := range dialogues {
			shortID := d.ID
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			title := d.Title
			if title == "" || title == "CLI Chat" {
				title = "Untitled"
			}
			updated := d.UpdatedAt
			if len(updated) > 16 {
				updated = updated[:16]
			}
			fmt.Println(RenderSessionLine(i+1, title, shortID, updated, d.ID == dialogueID))
		}
		fmt.Printf("\n  %s\n", R.Dim.Render("Use /sessions <number> to switch"))
		if len(parts) > 1 {
			idx := 0
			if _, err := fmt.Sscanf(parts[1], "%d", &idx); err == nil && idx >= 1 && idx <= len(dialogues) {
				target := dialogues[idx-1]
				messages, err := FetchMessages(apiURL, target.ID)
				if err != nil {
					RenderErrorBlock(fmt.Sprintf("Failed to load session: %v", err))
				} else {
					dialogueID = target.ID
					*history = messages
					newDialogueID = target.ID
					title := target.Title
					if title == "" || title == "CLI Chat" {
						title = "Untitled"
					}
					fmt.Printf("  %s\n", RenderStatusLine("switched to", title, fmt.Sprintf("%d messages", len(messages))))
				}
			} else {
				RenderErrorBlock(fmt.Sprintf("Invalid session number (1-%d)", len(dialogues)))
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
		fmt.Printf("  %s\n", RenderStatusLine(
			fmt.Sprintf("%d messages", len(*history)),
			fmt.Sprintf("%d user", userCount),
			fmt.Sprintf("%d assistant", asstCount),
		))
		showCount := 6
		start := len(*history) - showCount
		if start < 0 {
			start = 0
		}
		if len(*history) > showCount {
			fmt.Printf("  %s\n", R.Dim.Render(fmt.Sprintf("  showing last %d messages", showCount)))
		}
		for i := start; i < len(*history); i++ {
			m := (*history)[i]
			summary := strings.ReplaceAll(m.Content, "\n", " ")
			if len(summary) > 80 {
				summary = summary[:77] + "..."
			}
			RenderHistoryMessage(m.Sender, summary)
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
			RenderErrorBlock("No assistant response to copy")
		} else {
			if err := copyToClipboard(lastAsst); err != nil {
				RenderErrorBlock(fmt.Sprintf("Copy failed: %v", err))
			} else {
				preview := strings.ReplaceAll(lastAsst, "\n", " ")
				if len(preview) > 40 {
					preview = preview[:37] + "..."
				}
				RenderSuccessLine("Copied: " + preview)
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
				RenderErrorBlock(fmt.Sprintf("%v", err))
			} else {
				wd, _ := os.Getwd()
				fmt.Printf("  %s\n", R.FilePath.Render(wd))
			}
		} else {
			wd, _ := os.Getwd()
			fmt.Printf("  %s\n", R.FilePath.Render(wd))
		}
	case "/mode":
		modes := []struct {
			key   string
			label string
			desc  string
		}{
			{"build", "Build", "Full access: code editing, execution, all tools"},
			{"explore", "Explore", "Read-only: search, read files, no writes"},
			{"plan", "Plan", "Planning: analysis only, no code changes"},
			{"general", "General", "Multi-step tasks with all tools"},
		}
		if len(parts) > 1 {
			valid := false
			for _, m := range modes {
				if parts[1] == m.key {
					_, err := SetToolMode(apiURL, m.key)
					if err != nil {
						RenderErrorBlock(fmt.Sprintf("Failed to set mode: %v", err))
					} else {
						fmt.Printf("  %s\n", RenderStatusLine("mode", R.Accent.Render(m.key), R.Dim.Render(m.desc)))
					}
					valid = true
					break
				}
			}
			if !valid {
				RenderErrorBlock("Invalid mode. Use: build, explore, plan, general")
			}
		} else {
			fmt.Println()
			fmt.Printf("  %s\n\n", R.Bold.Render("Tool Mode"))
			currentMode := GetCurrentToolMode(apiURL)
			for _, m := range modes {
				current := ""
				if m.key == currentMode {
					current = " " + R.Success.Render("✓")
				}
				fmt.Printf("  %s %s%s\n",
					R.KeyHint.Render(m.key),
					R.Dim.Render(m.desc),
					current,
				)
			}
			fmt.Printf("\n  %s\n", R.Dim.Render("Use /mode <name> to switch"))
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
			RenderErrorBlock("No user message to redo")
		} else {
			fmt.Printf("  %s\n", RenderStatusLine("retry", truncateStr(lastUser, 60)))
			fmt.Println()
			var response string
			timeout := GetTimeout(cfg)
			var err error
			if stream {
				response, err = runStreamChat(ctx, apiURL, dialogueID, lastUser, model, timeout, cfg)
			} else {
				response, err = runSyncChat(apiURL, dialogueID, lastUser, model)
			}
			if err != nil {
				if ctx.Err() == context.Canceled {
					fmt.Printf("\n  %s\n", R.Warning.Render("⚠ Response interrupted"))
				} else {
					RenderErrorBlock(fmt.Sprintf("%v", err))
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
			RenderTurnDivider()
		}
	case "/config":
		result := RunSettings(cfg, getConfigPath(cfg))
		if result.Saved && result.Config != nil {
			cfg = result.Config
			stream = cfg.Chat.Stream
			RenderSuccessLine("Configuration saved")
		}
	case "/dashboard":
		action := RunDashboard(apiURL, "")
		if action.Action == "select_model" && action.Model != "" {
			model = action.Model
		}
	case "/plan":
		if len(parts) < 2 {
			RenderInfoLine("Usage: /plan <task description>")
			break
		}
		planInput := strings.Join(parts[1:], " ")
		planPrompt := fmt.Sprintf(
			"[PLAN MODE - READ ONLY]\nYou are in plan mode. Analyze the task and create a detailed plan, but do NOT make any code changes.\n"+
				"Only use read-only tools (read_file, search, list_directory, etc.). Do NOT use write_file, execute_command, or any modifying tools.\n"+
				"Provide:\n1. Analysis of the current codebase\n2. Step-by-step implementation plan\n3. Files that need to be modified\n4. Potential risks and considerations\n\nTask: %s", planInput)
		fmt.Printf("  %s\n", RenderToolCallLine("plan", truncateStr(planInput, 45)))
		fmt.Println()
		var response string
		var planErr error
		timeout := GetTimeout(cfg)
		if stream {
			response, planErr = runStreamChat(ctx, apiURL, dialogueID, planPrompt, model, timeout, cfg)
		} else {
			response, planErr = runSyncChat(apiURL, dialogueID, planPrompt, model)
		}
		if planErr != nil {
			if ctx.Err() == context.Canceled {
				fmt.Printf("\n  %s\n", R.Warning.Render("⚠ Plan interrupted"))
			} else {
				RenderErrorBlock(fmt.Sprintf("%v", planErr))
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
		RenderTurnDivider()
	case "/undo":
		gitStatus, err := exec.Command("git", "status", "--porcelain").Output()
		if err != nil {
			RenderErrorBlock("Not a git repository or git not available")
			break
		}
		lines := strings.Split(strings.TrimSpace(string(gitStatus)), "\n")
		var modified, untracked []string
		for _, line := range lines {
			if line == "" {
				continue
			}
			status := strings.TrimSpace(line)
			path := strings.TrimSpace(status[3:])
			if strings.HasPrefix(status, "M") || strings.HasPrefix(status, "A") {
				modified = append(modified, path)
			} else if strings.HasPrefix(status, "??") {
				untracked = append(untracked, path)
			}
		}
		if len(modified) == 0 && len(untracked) == 0 {
			RenderInfoLine("No changes to undo")
			break
		}
		fmt.Println()
		fmt.Printf("  %s\n\n", R.Bold.Render("Git Changes"))
		if len(modified) > 0 {
			fmt.Printf("  %s\n", R.Warning.Render("Modified:"))
			for _, f := range modified {
				fmt.Printf("    %s %s\n", R.Dim.Render("│"), R.FilePath.Render(f))
			}
		}
		if len(untracked) > 0 {
			fmt.Printf("  %s\n", R.Info.Render("Untracked:"))
			for _, f := range untracked {
				fmt.Printf("    %s %s\n", R.Dim.Render("│"), R.FilePath.Render(f))
			}
		}
		fmt.Printf("\n  %s\n", R.Dim.Render("/undo all  - revert all modified files"))
		fmt.Printf("  %s\n", R.Dim.Render("/undo clean - also remove untracked files"))
		if len(parts) > 1 {
			switch parts[1] {
			case "all":
				for _, f := range modified {
					if strings.HasPrefix(strings.TrimSpace(string(gitStatus)), "A ") {
						exec.Command("git", "rm", "--cached", f).Run()
					}
					exec.Command("git", "checkout", "--", f).Run()
				}
				RenderSuccessLine(fmt.Sprintf("Reverted %d modified files", len(modified)))
			case "clean":
				for _, f := range modified {
					exec.Command("git", "checkout", "--", f).Run()
				}
				if len(untracked) > 0 {
					exec.Command("git", "clean", "-fd").Run()
				}
				RenderSuccessLine(fmt.Sprintf("Reverted %d files, cleaned %d untracked", len(modified), len(untracked)))
			default:
				RenderErrorBlock(fmt.Sprintf("Unknown option: %s (use 'all' or 'clean')", parts[1]))
			}
		}
	case "/help":
		RenderHelp()
	case "/permissions":
		fmt.Println()
		fmt.Printf("  %s\n\n", R.Bold.Render("Permission Mode"))
		modes := []struct {
			key   string
			label string
			desc  string
		}{
			{"allow", "Allow All", "Auto-approve all tool calls"},
			{"ask", "Ask", "Confirm before risky tool calls"},
			{"deny", "Deny All", "Block all tool calls"},
		}
		for _, m := range modes {
			current := ""
			if m.key == cfg.Permissions.Mode {
				current = " " + R.Success.Render("✓")
			}
			fmt.Printf("  %s %s%s\n",
				R.KeyHint.Render(m.key),
				R.Dim.Render(m.desc),
				current,
			)
		}

		if len(cfg.Permissions.AutoAllow) > 0 {
			fmt.Printf("\n  %s\n", R.Bold.Render("Auto-Allow (config)"))
			for _, t := range cfg.Permissions.AutoAllow {
				fmt.Printf("  %s %s\n", R.Success.Render("✓"), R.Dim.Render(t))
			}
		}
		if len(cfg.Permissions.AutoDeny) > 0 {
			fmt.Printf("\n  %s\n", R.Bold.Render("Auto-Deny (config)"))
			for _, t := range cfg.Permissions.AutoDeny {
				fmt.Printf("  %s %s\n", R.Error.Render("✗"), R.Dim.Render(t))
			}
		}
		if len(cfg.Permissions.SessionAllow) > 0 {
			fmt.Printf("\n  %s\n", R.Bold.Render("Session Allow"))
			for t := range cfg.Permissions.SessionAllow {
				fmt.Printf("  %s %s\n", R.Success.Render("✓"), R.Dim.Render(t))
			}
		}
		if len(cfg.Permissions.SessionDeny) > 0 {
			fmt.Printf("\n  %s\n", R.Bold.Render("Session Deny"))
			for t := range cfg.Permissions.SessionDeny {
				fmt.Printf("  %s %s\n", R.Error.Render("✗"), R.Dim.Render(t))
			}
		}

		fmt.Printf("\n  %s\n", R.Dim.Render("Usage:"))
		fmt.Printf("  %s\n", R.Dim.Render("  /permissions <mode>      Set mode (allow/ask/deny)"))
		fmt.Printf("  %s\n", R.Dim.Render("  /permissions allow <tool>  Auto-allow a tool"))
		fmt.Printf("  %s\n", R.Dim.Render("  /permissions deny <tool>   Auto-deny a tool"))
		fmt.Printf("  %s\n", R.Dim.Render("  /permissions reset        Clear session decisions"))

		if len(parts) > 1 {
			switch parts[1] {
			case "allow":
				if len(parts) > 2 {
					tool := parts[2]
					cfg.Permissions.AutoAllow = append(cfg.Permissions.AutoAllow, tool)
					RenderSuccessLine(tool + " added to auto-allow")
				} else {
					cfg.Permissions.Mode = "allow"
					fmt.Printf("  %s\n", RenderStatusLine("permission", R.Accent.Render("allow")))
				}
			case "ask":
				cfg.Permissions.Mode = "ask"
				fmt.Printf("  %s\n", RenderStatusLine("permission", R.Accent.Render("ask")))
			case "deny":
				if len(parts) > 2 {
					tool := parts[2]
					cfg.Permissions.AutoDeny = append(cfg.Permissions.AutoDeny, tool)
					RenderSuccessLine(tool + " added to auto-deny")
				} else {
					cfg.Permissions.Mode = "deny"
					fmt.Printf("  %s\n", RenderStatusLine("permission", R.Accent.Render("deny")))
				}
			case "reset":
				cfg.Permissions.SessionAllow = make(map[string]bool)
				cfg.Permissions.SessionDeny = make(map[string]bool)
				RenderSuccessLine("Session permissions reset")
			default:
				RenderErrorBlock("Unknown option. Use: allow, ask, deny, reset")
			}
		}
	case "/fork":
		name := "branch"
		if len(parts) > 1 {
			name = parts[1]
		}
		branchPoint := len(*history)
		result, err := ForkSession(apiURL, dialogueID, "cli-user", name, branchPoint)
		if err != nil {
			RenderErrorBlock(fmt.Sprintf("Failed to fork session: %v", err))
		} else {
			RenderSuccessLine(fmt.Sprintf("Forked session at message %d as %s", branchPoint, name))
			if id, ok := result["id"]; ok {
				fmt.Printf("  %s\n", RenderStatusLine("branch", fmt.Sprintf("%v", id)))
			}
		}
	case "/branches":
		branches, err := ListBranches(apiURL, dialogueID)
		if err != nil || len(branches) == 0 {
			RenderInfoLine("No branches found")
			break
		}
		fmt.Println()
		fmt.Printf("  %s\n\n", R.Bold.Render("Session Branches"))
		for _, b := range branches {
			name, _ := b["name"].(string)
			isActive, _ := b["is_active"].(bool)
			point, _ := b["branch_point"].(float64)
			active := ""
			if isActive {
				active = " " + R.Success.Render("✓")
			}
			fmt.Printf("  %s point:%.0f%s\n",
				R.Accent.Render(name),
				point,
				active,
			)
		}
	case "/memories":
		if len(parts) > 2 && parts[1] == "add" {
			key := parts[2]
			value := strings.Join(parts[3:], " ")
			if value == "" {
				RenderInfoLine("Usage: /memories add <key> <value>")
				break
			}
			_, err := RememberPersistent(apiURL, "cli-user", "preference", key, value)
			if err != nil {
				RenderErrorBlock(fmt.Sprintf("Failed to save memory: %v", err))
			} else {
				RenderSuccessLine(fmt.Sprintf("Remembered: %s = %s", key, truncateStr(value, 50)))
			}
			break
		}
		memories, err := GetPersistentMemories(apiURL, "cli-user")
		if err != nil || len(memories) == 0 {
			RenderInfoLine("No memories stored")
			break
		}
		fmt.Println()
		fmt.Printf("  %s\n\n", R.Bold.Render("Persistent Memories"))
		for _, m := range memories {
			cat, _ := m["category"].(string)
			key, _ := m["key"].(string)
			value, _ := m["value"].(string)
			fmt.Printf("  %s\n", RenderStatusLine(cat, key, truncateStr(value, 50)))
		}
	case "/memory":
		if len(parts) > 1 {
			switch parts[1] {
			case "init":
				if err := InitProjectMemory(); err != nil {
					RenderErrorBlock(fmt.Sprintf("Failed: %v", err))
				} else {
					RenderSuccessLine("OPENAIDE.md created in project root")
				}
			case "save":
				if len(parts) < 4 {
					RenderInfoLine("Usage: /memory save <type> <content>")
					RenderInfoLine("Types: user, feedback, project, reference")
					break
				}
				memType := parts[2]
				content := strings.Join(parts[3:], " ")
				entry := MemoryEntry{
					Name:        sanitizeName(content),
					Description: truncateStr(content, 80),
					Type:        MemoryType(memType),
					Content:     content,
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				if err := SaveMemoryEntry(entry); err != nil {
					RenderErrorBlock(fmt.Sprintf("Failed: %v", err))
				} else {
					RenderSuccessLine("Memory saved: " + truncateStr(content, 50))
				}
			case "search":
				if len(parts) < 3 {
					RenderInfoLine("Usage: /memory search <query>")
					break
				}
				query := strings.Join(parts[2:], " ")
				memories, err := FetchRelevantMemories(apiURL, query, 5)
				if err != nil || len(memories) == 0 {
					RenderInfoLine("No relevant memories found")
					break
				}
				fmt.Println()
				fmt.Printf("  %s\n\n", R.Bold.Render("Relevant Memories"))
				for _, m := range memories {
					content, _ := m["content"].(string)
					memType, _ := m["type"].(string)
					fmt.Printf("  %s %s\n",
						R.Dim.Render("["+memType+"]"),
						R.Dim.Render(truncateStr(content, 80)))
				}
			case "extract":
				if dialogueID == "" {
					RenderErrorBlock("No active conversation")
					break
				}
				fmt.Printf("  %s\n", RenderToolCallLine("memory", "extracting from conversation..."))
				if err := TriggerMemoryExtraction(apiURL, dialogueID); err != nil {
					RenderErrorBlock(fmt.Sprintf("Extraction failed: %v", err))
				} else {
					fmt.Println(RenderToolResultLine("memory", true, 0))
				}
			case "delete":
				if len(parts) < 3 {
					RenderInfoLine("Usage: /memory delete <name>")
					break
				}
				if err := DeleteMemoryEntry(parts[2]); err != nil {
					RenderErrorBlock(fmt.Sprintf("Failed: %v", err))
				} else {
					RenderSuccessLine("Memory deleted: " + parts[2])
				}
			default:
				RenderInfoLine("Usage: /memory [init|save|search|extract|delete]")
			}
		} else {
			fmt.Println()
			fmt.Printf("  %s\n\n", R.Bold.Render("Memory"))

			if content, err := os.ReadFile(claudeMDPath()); err == nil {
				lines := strings.Count(string(content), "\n") + 1
				fmt.Printf("  %s %s\n", R.Success.Render("✓"), R.Dim.Render(fmt.Sprintf("OPENAIDE.md (%d lines)", lines)))
			} else {
				fmt.Printf("  %s %s\n", R.Dim.Render("│"), R.Dim.Render("No OPENAIDE.md (use /memory init)"))
			}

			entries := LoadMemoryEntries()
			if len(entries) > 0 {
				fmt.Printf("\n  %s\n", R.Bold.Render("Local Memories"))
				for _, e := range entries {
					fmt.Printf("  %s %s %s\n",
						R.Dim.Render("["+string(e.Type)+"]"),
						R.Accent.Render(e.Name),
						R.Dim.Render(truncateStr(e.Description, 50)))
				}
			}

			memories, _ := GetPersistentMemories(apiURL, "cli-user")
			if len(memories) > 0 {
				fmt.Printf("\n  %s\n", R.Bold.Render("Server Memories"))
				for _, m := range memories {
					cat, _ := m["category"].(string)
					key, _ := m["key"].(string)
					value, _ := m["value"].(string)
					fmt.Printf("  %s %s\n",
						R.Dim.Render("["+cat+"]"),
						R.Dim.Render(key+": "+truncateStr(value, 50)))
				}
			}

			fmt.Printf("\n  %s\n", R.Dim.Render("Commands: init, save, search, extract, delete"))
		}
	case "/skill":
		if len(parts) > 1 {
			switch parts[1] {
			case "create":
				if len(parts) < 3 {
					RenderInfoLine("Usage: /skill create <name> <description>")
					break
				}
				name := parts[2]
				desc := ""
				if len(parts) > 3 {
					desc = strings.Join(parts[3:], " ")
				}
				if err := CreateSkill(name, desc, "# "+name+"\n\nDescribe the workflow steps here."); err != nil {
					RenderErrorBlock(fmt.Sprintf("Failed: %v", err))
				} else {
					RenderSuccessLine("Skill created: " + name)
				}
			default:
				skill := LoadSkill(parts[1])
				if skill == nil {
					RenderErrorBlock("Skill not found: " + parts[1])
					break
				}
				skillInput := skill.Content
				if len(parts) > 2 {
					args := strings.Join(parts[2:], " ")
					skillInput = strings.ReplaceAll(skillInput, "{{args}}", args)
					skillInput = strings.ReplaceAll(skillInput, "{{input}}", args)
				}
				wd, _ := os.Getwd()
				skillInput = strings.ReplaceAll(skillInput, "{{cwd}}", wd)
				fmt.Printf("  %s\n", RenderToolCallLine("skill:"+skill.Name, truncateStr(skill.Description, 45)))
				fmt.Println()
				var response string
				var skillErr error
				timeout := GetTimeout(cfg)
				if stream {
					response, skillErr = runStreamChat(ctx, apiURL, dialogueID, skillInput, model, timeout, cfg)
				} else {
					response, skillErr = runSyncChat(apiURL, dialogueID, skillInput, model)
				}
				if skillErr != nil {
					if ctx.Err() == context.Canceled {
						fmt.Printf("\n  %s\n", R.Warning.Render("⚠ Skill interrupted"))
					} else {
						RenderErrorBlock(fmt.Sprintf("%v", skillErr))
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
				RenderTurnDivider()
			}
		} else {
			skills := LoadSkills()
			RenderSkillList(skills)
		}
	case "/feedback":
		if len(parts) < 2 {
			RenderInfoLine("Usage: /feedback <positive|negative> [comment]")
			break
		}
		fbType := parts[1]
		if fbType != "positive" && fbType != "negative" {
			RenderInfoLine("Type must be: positive or negative")
			break
		}
		comment := ""
		if len(parts) > 2 {
			comment = strings.Join(parts[2:], " ")
		}
		if err := SaveFeedback(apiURL, dialogueID, "", fbType, comment); err != nil {
			RenderErrorBlock(fmt.Sprintf("Failed: %v", err))
		} else {
			icon := "👍"
			if fbType == "negative" {
				icon = "👎"
			}
			RenderSuccessLine(icon + " Feedback recorded (" + fbType + ")")
		}
	case "/project":
		if len(parts) > 1 {
			switch parts[1] {
			case "list", "ls":
				projects, err := FetchProjects(apiURL)
				if err != nil || len(projects) == 0 {
					RenderInfoLine("No projects found")
					break
				}
				fmt.Println()
				fmt.Printf("  %s\n\n", R.Bold.Render("Projects"))
				for i, p := range projects {
					active := ""
					if p.ID == currentProjectID {
						active = " " + R.Success.Render("✓")
					}
					defaultLabel := ""
					if p.IsDefault {
						defaultLabel = " " + R.Dim.Render("(default)")
					}
					fmt.Printf("  %s %s%s%s\n",
						R.Accent.Render(fmt.Sprintf("%d.", i+1)),
						R.Bold.Render(p.Name),
						defaultLabel,
						active,
					)
					if p.Description != "" {
						fmt.Printf("  %s %s\n", R.Dim.Render("│"), R.Dim.Render(truncateStr(p.Description, 60)))
					}
				}
				fmt.Printf("\n  %s\n", R.Dim.Render("Use /project switch <number> to switch"))
			case "switch", "use":
				if len(parts) >= 3 {
					projects, err := FetchProjects(apiURL)
					if err != nil || len(projects) == 0 {
						RenderInfoLine("No projects found")
						break
					}
					var target *Project
					idx := 0
					if _, err := fmt.Sscanf(parts[2], "%d", &idx); err == nil && idx >= 1 && idx <= len(projects) {
						target = &projects[idx-1]
					} else {
						for i := range projects {
							if strings.EqualFold(projects[i].Name, parts[2]) {
								target = &projects[i]
								break
							}
						}
					}
					if target == nil {
					RenderErrorBlock(fmt.Sprintf("Project not found: %s (use /project list)", parts[2]))
					break
				}
				currentProjectID = target.ID
				_ = InitWorkspace(currentProjectID)
				fmt.Printf("  %s\n", RenderStatusLine("project", R.Accent.Render(target.Name)))
					if target.SystemPrompt != "" {
						fmt.Printf("  %s\n", R.Dim.Render("  system prompt: "+truncateStr(target.SystemPrompt, 60)))
					}
					if target.ModelID != "" {
						fmt.Printf("  %s\n", R.Dim.Render("  model: "+target.ModelID))
					}
				} else {
					RestoreReadline()
				result := RunProjectSelect(apiURL, currentProjectID)
				if result.Changed && result.Selected != nil {
					currentProjectID = result.Selected.ID
					_ = InitWorkspace(currentProjectID)
					fmt.Printf("  %s\n", RenderStatusLine("project", R.Accent.Render(result.Selected.Name)))
						if result.Selected.SystemPrompt != "" {
							fmt.Printf("  %s\n", R.Dim.Render("  system prompt: "+truncateStr(result.Selected.SystemPrompt, 60)))
						}
						if result.Selected.ModelID != "" {
							fmt.Printf("  %s\n", R.Dim.Render("  model: "+result.Selected.ModelID))
						}
					}
				}
			case "create", "new":
				name := ""
				if len(parts) > 2 {
					name = parts[2]
				} else {
					fmt.Printf("  %s ", R.Dim.Render("Project name:"))
					fmt.Scanln(&name)
				}
				name = strings.TrimSpace(name)
				if name == "" {
					RenderInfoLine("Project name cannot be empty")
					break
				}
				desc := ""
				if len(parts) > 3 {
					desc = strings.Join(parts[3:], " ")
				}
				project, err := CreateProject(apiURL, name, desc, "")
				if err != nil {
					RenderErrorBlock(fmt.Sprintf("Failed to create project: %v", err))
					break
				}
				currentProjectID = project.ID
				_ = InitWorkspace(currentProjectID)
				fmt.Printf("  %s\n", RenderStatusLine("created project", R.Accent.Render(project.Name), project.ID[:8]+"..."))
				fmt.Printf("  %s\n", RenderStatusLine("switched to", R.Accent.Render(project.Name)))
			case "delete", "rm":
				if len(parts) < 3 {
					RenderInfoLine("Usage: /project delete <number|name>")
					break
				}
				projects, err := FetchProjects(apiURL)
				if err != nil || len(projects) == 0 {
					RenderInfoLine("No projects found")
					break
				}
				var target *Project
				idx := 0
				if _, err := fmt.Sscanf(parts[2], "%d", &idx); err == nil && idx >= 1 && idx <= len(projects) {
					target = &projects[idx-1]
				} else {
					for i := range projects {
						if strings.EqualFold(projects[i].Name, parts[2]) {
							target = &projects[i]
							break
						}
					}
				}
				if target == nil {
					RenderErrorBlock(fmt.Sprintf("Project not found: %s", parts[2]))
					break
				}
				if target.IsDefault {
					RenderErrorBlock("Cannot delete the default project")
					break
				}
				if err := DeleteProject(apiURL, target.ID); err != nil {
					RenderErrorBlock(fmt.Sprintf("Failed to delete project: %v", err))
					break
				}
				if currentProjectID == target.ID {
					currentProjectID = ""
				}
				fmt.Printf("  %s\n", RenderStatusLine("deleted project", R.Accent.Render(target.Name)))
			default:
				RenderInfoLine("Usage: /project [list|switch|create|delete]")
			}
		} else {
			if currentProjectID != "" {
				projects, err := FetchProjects(apiURL)
				if err == nil {
					for _, p := range projects {
						if p.ID == currentProjectID {
							fmt.Println()
							fmt.Printf("  %s\n\n", R.Bold.Render("Current Project"))
							fmt.Printf("  %s\n", RenderStatusLine("name", R.Accent.Render(p.Name)))
							if p.Description != "" {
								fmt.Printf("  %s\n", RenderStatusLine("description", truncateStr(p.Description, 60)))
							}
							if p.SystemPrompt != "" {
								fmt.Printf("  %s\n", RenderStatusLine("system prompt", truncateStr(p.SystemPrompt, 60)))
							}
							if p.ModelID != "" {
								fmt.Printf("  %s\n", RenderStatusLine("model", p.ModelID))
							}
							if p.WorkingDir != "" {
								fmt.Printf("  %s\n", RenderStatusLine("working dir", p.WorkingDir))
							}
							fmt.Printf("\n  %s\n", R.Dim.Render("Commands: list, switch, create, delete"))
							break
						}
					}
				} else {
					fmt.Printf("  %s\n", RenderStatusLine("project", currentProjectID[:8]+"..."))
				}
			} else {
				fmt.Println()
				fmt.Printf("  %s\n\n", R.Bold.Render("Project"))
				fmt.Printf("  %s\n", R.Dim.Render("No project selected (using default)"))
				fmt.Printf("\n  %s\n", R.Dim.Render("Commands: list, switch, create, delete"))
			}
		}
	case "/workspace":
		if len(parts) > 1 {
			switch parts[1] {
			case "init", "bind":
				if currentProjectID == "" {
					RenderErrorBlock("No project selected. Use /project switch first")
					break
				}
				if err := InitWorkspace(currentProjectID); err != nil {
					RenderErrorBlock(fmt.Sprintf("Failed to init workspace: %v", err))
				} else {
					_ = UpdateWorkspaceDialogue(dialogueID)
					RenderSuccessLine("Workspace bound to current directory")
				}
			case "clear", "rm":
				if err := ClearWorkspace(); err != nil {
					RenderErrorBlock(fmt.Sprintf("Failed to clear workspace: %v", err))
				} else {
					RenderSuccessLine("Workspace marker removed")
				}
			default:
				RenderInfoLine("Usage: /workspace [init|clear]")
			}
		} else {
			if HasWorkspace() {
				ws, err := LoadWorkspaceState()
				if err == nil {
					fmt.Println()
					fmt.Printf("  %s\n\n", R.Bold.Render("Workspace"))
					fmt.Printf("  %s\n", RenderStatusLine("project", ws.ProjectID[:8]+"..."))
					fmt.Printf("  %s\n", RenderStatusLine("dialogue", ws.DialogueID[:8]+"..."))
					fmt.Printf("  %s\n", RenderStatusLine("directory", ws.WorkingDir))
					fmt.Printf("\n  %s\n", R.Dim.Render("Commands: init, clear"))
				} else {
					RenderErrorBlock("Failed to load workspace state")
				}
			} else {
				fmt.Println()
				fmt.Printf("  %s\n\n", R.Bold.Render("Workspace"))
				fmt.Printf("  %s\n", R.Dim.Render("No workspace marker in current directory"))
				fmt.Printf("\n  %s\n", R.Dim.Render("Use /workspace init to bind current project"))
			}
		}
	case "/cost":
		days := 30
		if len(parts) > 1 {
			if d, err := fmt.Sscanf(parts[1], "%d", &days); err != nil || d != 1 {
				days = 30
			}
		}
		summary, err := GetCostSummary(apiURL, "cli-user", days)
		if err != nil {
			RenderErrorBlock(fmt.Sprintf("Failed to get cost summary: %v", err))
			break
		}
		fmt.Println()
		fmt.Printf("  %s\n\n", R.Bold.Render(fmt.Sprintf("Cost Summary (last %d days)", days)))
		if totalCost, ok := summary["total_cost_usd"].(float64); ok {
			fmt.Printf("  %s\n", RenderStatusLine("total", fmt.Sprintf("$%.4f", totalCost)))
		}
		if totalReqs, ok := summary["total_requests"].(int64); ok {
			fmt.Printf("  %s\n", RenderStatusLine("requests", fmt.Sprintf("%d", totalReqs)))
		} else if totalReqs, ok := summary["total_requests"].(float64); ok {
			fmt.Printf("  %s\n", RenderStatusLine("requests", fmt.Sprintf("%.0f", totalReqs)))
		}
		if totalTokens, ok := summary["total_tokens"].(int64); ok {
			fmt.Printf("  %s\n", RenderStatusLine("tokens", formatTokenCount(int(totalTokens))))
		} else if totalTokens, ok := summary["total_tokens"].(float64); ok {
			fmt.Printf("  %s\n", RenderStatusLine("tokens", formatTokenCount(int(totalTokens))))
		}
		if successRate, ok := summary["success_rate"].(float64); ok && successRate > 0 {
			fmt.Printf("  %s\n", RenderStatusLine("success", fmt.Sprintf("%.1f%%", successRate)))
		}
		if byModel, ok := summary["by_model"].([]interface{}); ok && len(byModel) > 0 {
			fmt.Printf("\n  %s\n", R.Bold.Render("By Model"))
			for _, m := range byModel {
				if mMap, ok := m.(map[string]interface{}); ok {
					modelID, _ := mMap["model_id"].(string)
					reqCount, _ := mMap["request_count"].(int64)
					if reqCount == 0 {
						if rc, ok := mMap["request_count"].(float64); ok {
							reqCount = int64(rc)
						}
					}
					cost, _ := mMap["total_cost"].(float64)
					fmt.Printf("  %s\n", RenderStatusLine(modelID, fmt.Sprintf("%d reqs", reqCount), fmt.Sprintf("$%.4f", cost)))
				}
			}
		}
	default:
		if strings.HasPrefix(parts[0], "/") && !strings.HasPrefix(parts[0], "/ ") {
			cmdName := parts[0][1:]
			customPrompt := loadCustomCommand(cmdName)
			if customPrompt != "" {
				args := ""
				if len(parts) > 1 {
					args = strings.Join(parts[1:], " ")
				}
				fullPrompt := customPrompt
				if args != "" {
					fullPrompt = strings.ReplaceAll(customPrompt, "{{args}}", args)
					fullPrompt = strings.ReplaceAll(fullPrompt, "{{input}}", args)
				}
				wd, _ := os.Getwd()
				fullPrompt = strings.ReplaceAll(fullPrompt, "{{cwd}}", wd)
				fmt.Printf("  %s\n", RenderToolCallLine(cmdName, "running custom command..."))
				fmt.Println()
				var response string
				var cmdErr error
				timeout := GetTimeout(cfg)
				if stream {
					response, cmdErr = runStreamChat(ctx, apiURL, dialogueID, fullPrompt, model, timeout, cfg)
				} else {
					response, cmdErr = runSyncChat(apiURL, dialogueID, fullPrompt, model)
				}
				if cmdErr != nil {
					if ctx.Err() == context.Canceled {
						fmt.Printf("\n  %s\n", R.Warning.Render("✗ Interrupted"))
					} else {
						RenderErrorBlock(fmt.Sprintf("%v", cmdErr))
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
				RenderTurnDivider()
			} else {
				RenderErrorBlock("Unknown command: " + parts[0])
				fmt.Printf("  %s\n", R.Dim.Render("Type /help for available commands"))
			}
		} else {
			RenderErrorBlock("Unknown command: " + parts[0])
			fmt.Printf("  %s\n", R.Dim.Render("Type /help for available commands"))
		}
	}

	return newDialogueID, model, stream, currentProjectID
}

// onHistoryLoaded 异步历史加载回调
func onHistoryLoaded(loadedDialogueID string, messages []Message) {
	// 目前仅打印提示，不自动切换对话（避免打断用户输入）
	if len(messages) > 0 {
		slog.Debug("history loaded async", "dialogue_id", loadedDialogueID, "messages", len(messages))
	}
}

func getConfigPath(cfg *Config) string {
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		return ""
	}
	return homeDir + "/.openaide/config.yaml"
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
	readline.PcItem("/compact"),
	readline.PcItem("/context"),
	readline.PcItem("/chapters"),
	readline.PcItem("/clear"),
	readline.PcItem("/mode",
		readline.PcItem("build"),
		readline.PcItem("explore"),
		readline.PcItem("plan"),
		readline.PcItem("general"),
	),
	readline.PcItem("/sessions"),
	readline.PcItem("/history"),
	readline.PcItem("/copy"),
	readline.PcItem("/redo"),
	readline.PcItem("/plan"),
	readline.PcItem("/undo",
		readline.PcItem("all"),
		readline.PcItem("clean"),
	),
	readline.PcItem("/permissions",
		readline.PcItem("allow"),
		readline.PcItem("ask"),
		readline.PcItem("deny"),
		readline.PcItem("reset"),
	),
	readline.PcItem("/fork"),
	readline.PcItem("/branches"),
	readline.PcItem("/memories",
		readline.PcItem("add"),
	),
	readline.PcItem("/memory",
		readline.PcItem("init"),
		readline.PcItem("save"),
		readline.PcItem("search"),
		readline.PcItem("extract"),
		readline.PcItem("delete"),
	),
	readline.PcItem("/skill",
		readline.PcItem("create"),
	),
	readline.PcItem("/feedback",
		readline.PcItem("positive"),
		readline.PcItem("negative"),
	),
	readline.PcItem("/project",
		readline.PcItem("list"),
		readline.PcItem("switch"),
		readline.PcItem("create"),
		readline.PcItem("delete"),
	),
	readline.PcItem("/workspace",
		readline.PcItem("init"),
		readline.PcItem("clear"),
	),
	readline.PcItem("/cost"),
	readline.PcItem("/cd"),
	readline.PcItem("/config"),
	readline.PcItem("/dashboard"),
	readline.PcItem("/exit"),
	readline.PcItem("/quit"),
)

func readLine() (string, error) {
	if rl != nil {
		rl.SetPrompt(R.PromptArrow.Render("▶"))
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
		if !strings.Contains(line, "\n") {
			return line, nil
		}
		var lines []string
		lines = append(lines, line)
		for {
			rl.SetPrompt(R.Dim.Render("…"))
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
	fmt.Println(RenderToolCallLine("shell", "$ "+command))
	reqBody := map[string]interface{}{
		"user_id":     "cli-user",
		"content":     fmt.Sprintf("执行命令: %s", command),
		"model_id":    model,
		"dialogue_id": dialogueID,
		"options": map[string]interface{}{
			"tool_filter": []string{"execute_command"},
			"system":      fmt.Sprintf("用户要求直接执行命令，请调用 execute_command 工具执行以下命令，不需要确认：\n%s\n执行后展示结果即可，不需要额外解释…", command),
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		RenderErrorBlock(fmt.Sprintf("Failed: %v", err))
		return
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL+"/chat/route", strings.NewReader(string(jsonData)))
	if err != nil {
		RenderErrorBlock(fmt.Sprintf("Failed: %v", err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	addAuthHeader(req)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		RenderErrorBlock(fmt.Sprintf("Failed: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		RenderErrorBlock(string(body))
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
				fmt.Println(RenderToolCallLine(tool, detail))
			case "tool_done":
				tool, _ := chunk["tool"].(string)
				result, _ := chunk["result"].(string)
				fmt.Println(RenderToolResultLine(tool, true, 0))
				if result != "" {
					RenderToolResultOutput(result, 20)
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

var fileRefPattern = regexp.MustCompile(`@([\w./\-]+\.[\w]+)`)

func expandFileReferences(input string) string {
	matches := fileRefPattern.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return input
	}

	var result strings.Builder
	lastEnd := 0
	for _, match := range matches {
		result.WriteString(input[lastEnd:match[2]])
		filePath := input[match[2]:match[3]]

		absPath := filePath
		if !filepath.IsAbs(filePath) {
			absPath = filepath.Join(".", filePath)
		}

		content, err := os.ReadFile(absPath)
		if err != nil {
			result.WriteString(fmt.Sprintf("@%s", filePath))
			fmt.Printf("  %s\n", R.Warning.Render("⚠ Cannot read @"+filePath+": "+err.Error()))
		} else {
			ext := filepath.Ext(filePath)
			lang := strings.TrimPrefix(ext, ".")
			result.WriteString(fmt.Sprintf("<file path=\"%s\">\n```%s\n%s\n```\n</file>", filePath, lang, string(content)))
			lineCount := strings.Count(string(content), "\n") + 1
			fmt.Println(RenderToolResultLine("read_file", true, 0))
			fmt.Printf("  %s\n", RenderStatusLine("attached", filePath, fmt.Sprintf("%d lines", lineCount)))
		}
		lastEnd = match[3]
	}
	result.WriteString(input[lastEnd:])
	return result.String()
}

func loadCustomCommand(name string) string {
	searchPaths := []string{
		".openaide/commands",
	}
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		searchPaths = append(searchPaths, homeDir+"/.openaide/commands")
	}
	for _, dir := range searchPaths {
		for _, ext := range []string{".md", ".txt", ".prompt"} {
			filePath := filepath.Join(dir, name+ext)
			content, err := os.ReadFile(filePath)
			if err == nil {
				return strings.TrimSpace(string(content))
			}
		}
	}
	return ""
}

func checkToolPermission(tool string, cfg *Config) (bool, bool) {
	if cfg.Permissions.SessionAllow == nil {
		cfg.Permissions.SessionAllow = make(map[string]bool)
	}
	if cfg.Permissions.SessionDeny == nil {
		cfg.Permissions.SessionDeny = make(map[string]bool)
	}

	if cfg.Permissions.SessionAllow[tool] {
		return true, false
	}
	if cfg.Permissions.SessionDeny[tool] {
		return false, false
	}

	for _, t := range cfg.Permissions.AutoAllow {
		if t == tool {
			return true, false
		}
	}
	for _, t := range cfg.Permissions.AutoDeny {
		if t == tool {
			return false, false
		}
	}

	mode := cfg.Permissions.Mode
	if mode == "" || mode == "allow" {
		return true, false
	}
	if mode == "deny" {
		return false, true
	}
	if mode == "ask" {
		readOnlyTools := map[string]bool{
			"read_file": true, "search_files": true, "list_directory": true,
			"get_file_info": true, "search_code": true, "grep": true,
			"find": true, "cat": true, "ls": true, "head": true,
			"web_search": true, "web_fetch": true,
		}
		if readOnlyTools[tool] {
			return true, false
		}
		return false, true
	}
	return true, false
}

func riskLevelForTool(tool string) string {
	highRisk := map[string]bool{
		"execute_command": true, "run_command": true, "shell": true, "bash": true,
		"delete_file": true, "rm": true,
	}
	mediumRisk := map[string]bool{
		"write_file": true, "edit_file": true, "create_file": true,
		"move_file": true, "apply_patch": true,
	}
	if highRisk[tool] {
		return "high"
	}
	if mediumRisk[tool] {
		return "medium"
	}
	return "low"
}

func promptToolConfirmation(tool, params string) bool {
	risk := riskLevelForTool(tool)

	prefix := R.Border.Render("│")
	fmt.Printf("\n  %s %s\n", prefix, R.Warning.Render("⚠ Tool call requires confirmation"))

	emoji := toolEmoji(tool)
	verb := toolVerb(tool)
	verbStr := padVerb(verb, verbWidth)

	var riskLabel string
	switch risk {
	case "high":
		riskLabel = R.Error.Render("▲ HIGH")
	case "medium":
		riskLabel = R.Warning.Render("● MEDIUM")
	default:
		riskLabel = R.Info.Render("◽ LOW")
	}

	fmt.Printf("  %s %s %s %s %s\n",
		prefix, emoji, R.ToolTitle.Render(verbStr), riskLabel, R.Dim.Render(tool))

	detail := parseToolDetail(tool, params)
	if detail != "" {
		fmt.Printf("  %s %s %s\n",
			prefix, R.Dim.Render("│"), R.Dim.Render(truncateStr(detail, 80)))
	}

	switch tool {
	case "execute_command", "run_command", "shell", "bash":
		var p map[string]interface{}
		if json.Unmarshal([]byte(params), &p) == nil {
			if cmd, ok := p["command"].(string); ok {
				fmt.Printf("  %s %s %s\n",
					prefix, R.Dim.Render("│"), R.Command.Render(truncateStr(cmd, 100)))
			}
		}
	case "write_file", "edit_file", "create_file":
		var p map[string]interface{}
		if json.Unmarshal([]byte(params), &p) == nil {
			if path, ok := p["path"].(string); ok {
				if path == "" {
					path, _ = p["file_path"].(string)
				}
				if path != "" {
					fmt.Printf("  %s %s %s\n",
						prefix, R.Dim.Render("│"), R.FilePath.Render(path))
				}
			}
			if content, ok := p["content"].(string); ok && content != "" {
				lines := strings.Split(content, "\n")
				preview := 3
				if len(lines) < preview {
					preview = len(lines)
				}
				for i := 0; i < preview; i++ {
					fmt.Printf("  %s %s %s\n",
						prefix, R.Dim.Render("│"), R.Dim.Render(truncateStr(lines[i], 80)))
				}
				if len(lines) > preview {
					fmt.Printf("  %s %s %s\n",
						prefix, R.Dim.Render("│"), R.Dim.Render(fmt.Sprintf("…+%d more lines", len(lines)-preview)))
				}
			}
		}
	}

	fmt.Printf("  %s\n", prefix)
	fmt.Printf("  %s %s %s %s\n",
		prefix,
		R.KeyHint.Render("[y]"),
		R.Dim.Render("Yes"),
		R.Dim.Render("· [n] No (default)"))
	fmt.Printf("  %s %s %s %s\n",
		prefix,
		R.KeyHint.Render("[a]"),
		R.Dim.Render("Always allow"),
		R.Dim.Render("(session)"))
	fmt.Printf("  %s %s %s %s\n",
		prefix,
		R.KeyHint.Render("[d]"),
		R.Dim.Render("Always deny"),
		R.Dim.Render("(session)"))

	fmt.Printf("  %s ", R.Warning.Render("Allow?"))
	var answer string
	fmt.Scanln(&answer)
	answer = strings.ToLower(strings.TrimSpace(answer))

	switch answer {
	case "y", "yes":
		return true
	case "a", "always":
		if currentConfig != nil {
			if currentConfig.Permissions.SessionAllow == nil {
				currentConfig.Permissions.SessionAllow = make(map[string]bool)
			}
			currentConfig.Permissions.SessionAllow[tool] = true
		}
		RenderSuccessLine(tool + " allowed for this session")
		return true
	case "d", "deny":
		if currentConfig != nil {
			if currentConfig.Permissions.SessionDeny == nil {
				currentConfig.Permissions.SessionDeny = make(map[string]bool)
			}
			currentConfig.Permissions.SessionDeny[tool] = true
		}
		fmt.Printf("  %s\n", R.Error.Render("🚫 "+tool+" denied for this session"))
		return false
	default:
		return false
	}
}

func PrintModelList(apiURL string) {
	models, err := FetchModels(apiURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to fetch models: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Printf("  %s\n\n", R.Bold.Render("Available Models"))

	for _, m := range models {
		if m.Status != "enabled" {
			continue
		}
		provider := R.Dim.Render(fmt.Sprintf("[%s]", m.Provider))
		fmt.Printf("  %s %s  %s\n",
			R.Success.Render("✓"),
			R.Bold.Render(m.Name),
			provider)
	}
	fmt.Println()
}

func ShowConfig(cfg *Config, path string) {
	fmt.Println()
	fmt.Printf("  %s %s\n\n", R.Dim.Render("Config"), R.Dim.Render(path))

	fmt.Printf("  %s\n", R.Bold.Render("API Configuration"))
	fmt.Printf("  %s\n", RenderStatusLine("base URL", cfg.API.BaseURL))
	fmt.Printf("  %s\n", RenderStatusLine("timeout", fmt.Sprintf("%d seconds", cfg.API.TimeoutSec)))

	fmt.Println()
	fmt.Printf("  %s\n", R.Bold.Render("Chat Configuration"))
	fmt.Printf("  %s\n", RenderStatusLine("default model", cfg.Chat.DefaultModel))
	fmt.Printf("  %s\n", RenderStatusLine("streaming", fmt.Sprintf("%v", cfg.Chat.Stream)))
	fmt.Printf("  %s\n", RenderStatusLine("context limit", fmt.Sprintf("%d", cfg.Chat.ContextLimit)))
}

// unused but kept for compatibility
var _ = lipgloss.Color("")

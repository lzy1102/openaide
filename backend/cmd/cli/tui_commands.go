package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"openaide/backend/internal/infra"
	"openaide/backend/internal/kernel"
	"openaide/backend/internal/lang"
)

// handleTUICommand 斜杠命令分发
func (m tuiModel) handleTUICommand(cmd string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(cmd)
	switch parts[0] {
	case "/exit", "/quit", "/q":
		m.appendHistory("\n" + styleInfo.Render(lang.T("repl.goodbye")) + "\n")
		return m, tea.Quit

	case "/help":
		m.appendHistory(stylePrompt.Render("OpenAIDE "+lang.T("repl.help_title")) + "\n\n")
		for _, line := range []string{
			styleTool.Render("/help") + " — " + lang.T("cli.help"),
			styleTool.Render("/clear") + " — " + lang.T("repl.help_clear"),
			styleTool.Render("/model [name]") + " — " + lang.T("repl.help_model"),
			styleTool.Render("/lang zh|en") + " — " + lang.T("repl.help_lang"),
			styleTool.Render("/log") + " — " + lang.T("repl.help_log"),
			styleTool.Render("/sessions") + " — " + lang.T("repl.help_sessions"),
			styleTool.Render("/handoff") + " — " + lang.T("repl.help_handoff"),
			styleTool.Render("/compact") + " — " + lang.T("repl.help_compact"),
			styleTool.Render("/exit /quit /q") + " — " + lang.T("repl.help_exit"),
		} {
			m.appendHistory("  " + line + "\n")
		}
		m.appendHistory("\n")
		for _, line := range []string{
			styleTool.Render("/analyst <task>") + " — " + lang.T("repl.help_analyst"),
			styleTool.Render("/coder <task>") + " — " + lang.T("repl.help_coder"),
			styleTool.Render("/reviewer <task>") + " — " + lang.T("repl.help_reviewer"),
			styleTool.Render("/executor <task>") + " — " + lang.T("repl.help_executor"),
			styleTool.Render("/team <task>") + " — " + lang.T("repl.help_team"),
			styleTool.Render("/tree") + " — " + lang.T("repl.help_tree"),
			styleTool.Render("/status") + " — " + lang.T("repl.help_status"),
			styleTool.Render("/undo") + " — " + lang.T("repl.help_undo"),
			styleTool.Render("/auto") + " — " + lang.T("repl.help_auto"),
			styleTool.Render("/research") + " — " + lang.T("repl.help_research"),
			styleTool.Render("/init") + " — " + lang.T("repl.help_init"),
		} {
			m.appendHistory("  " + line + "\n")
		}
		m.appendHistory("\n" + styleDim.Render(lang.T("repl.help_intro")) + "\n")
		m.refreshViewport()
		return m, nil

	case "/clear":
		m.app.Orchestrator.DeleteSession(context.Background(), m.sessionID)
		sess, _ := m.app.Orchestrator.CreateSession(context.Background(), m.projectID, "cli-user")
		if sess != nil {
			m.sessionID = sess.ID
		}
		m.history.Reset()
		m.appendHistory(styleSuccess.Render(lang.T("repl.session_cleared")) + "\n")
		return m, nil

	case "/compact":
		if ak, ok := m.app.Kernel.(*kernel.AgentKernel); ok {
			before, after, err := ak.CompressNow(context.Background(), m.sessionID)
			if err != nil {
				m.appendHistory(styleWarn.Render(lang.T("repl.compact_failed", err.Error())) + "\n")
			} else if before == 0 && after == 0 {
				m.appendHistory(styleInfo.Render(lang.T("repl.compact_nothing")) + "\n")
			} else {
				pct := 0
				if before > 0 {
					pct = 100 - after*100/before
				}
				m.appendHistory(styleSuccess.Render(lang.T("repl.compact_done", before, after, pct)) + "\n")
			}
		} else {
			m.appendHistory(styleWarn.Render(lang.T("repl.compact_unavailable")) + "\n")
		}
		return m, nil

	case "/undo":
		m.undoLastMessage()
		return m, nil

	case "/model":
		return m.handleModelCmd(parts)

	case "/tree":
		return m.handleTreeCmd()

	case "/init":
		m.mode = modeThinking
		m.statusMsg = lang.T("repl.generating_doc")
		m.textarea.Blur()
		m.refreshViewport()
		return m, func() tea.Msg {
			size, err := handleInitSize(m.app)
			return initMsg{size: size, err: err}
		}

	case "/lang":
		if len(parts) >= 2 {
			switch parts[1] {
			case "zh":
				lang.SetLang(lang.ZH)
				m.appendHistory(styleSuccess.Render(lang.T("repl.lang_zh")) + "\n")
			case "en":
				lang.SetLang(lang.EN)
				m.appendHistory(styleSuccess.Render(lang.T("repl.lang_en")) + "\n")
			}
		}
		return m, nil

	case "/log":
		lines := tuiLogBuf.snapshot()
		start := 0
		if len(lines) > 20 {
			start = len(lines) - 20
		}
		for i := start; i < len(lines); i++ {
			m.appendHistory(styleInfo.Render("  "+lines[i]) + "\n")
		}
		return m, nil

	case "/sessions":
		return m.handleSessionsCmd()

	case "/session":
		return m.handleSessionCmd(parts)

	case "/handoff":
		m.appendHistory(styleSuccess.Render("✓ "+lang.T("repl.export_saved")) + "\n")
		return m, nil

	case "/status":
		return m.handleStatusCmd()

	case "/auto":
		on := m.autoYes.Get()
		m.autoYes.Set(!on)
		if !on {
			m.appendHistory(styleSuccess.Render(lang.T("repl.auto_on")) + "\n")
		} else {
			m.appendHistory(styleSuccess.Render(lang.T("repl.auto_off")) + "\n")
		}
		return m, nil

	case "/research":
		on := m.research.Get()
		m.research.Set(!on)
		if !on {
			m.appendHistory(styleSuccess.Render(lang.T("repl.research_on")) + "\n")
		} else {
			m.appendHistory(styleSuccess.Render(lang.T("repl.research_off")) + "\n")
		}
		return m, nil

	case "/analyst", "/coder", "/reviewer", "/executor":
		role := strings.TrimPrefix(parts[0], "/")
		task := strings.TrimSpace(strings.TrimPrefix(cmd, parts[0]))
		if task == "" {
			m.appendHistory(styleWarn.Render(lang.T("repl.usage_role", parts[0])) + "\n")
			return m, nil
		}
		return m.startSubAgent(role, task)

	case "/team":
		task := strings.TrimSpace(strings.TrimPrefix(cmd, "/team"))
		if task == "" {
			m.appendHistory(styleWarn.Render(lang.T("repl.usage_team")) + "\n")
			return m, nil
		}
		return m.startTeam(task)

	default:
		m.appendHistory(styleWarn.Render(lang.T("err.unknown_cmd", parts[0])) + "\n")
		return m, nil
	}
}

// handleModelCmd /model [name]
func (m tuiModel) handleModelCmd(parts []string) (tea.Model, tea.Cmd) {
	if len(parts) >= 2 {
		m.app.SetModel(parts[1])
		m.modelName = parts[1]
		m.appendHistory(styleSuccess.Render(lang.T("repl.model_switched", parts[1])) + "\n")
		return m, nil
	}
	info := m.app.LLMGateway.GetProviderInfos()
	if len(info) == 0 {
		m.appendHistory(styleWarn.Render(lang.T("repl.no_models")) + "\n")
		return m, nil
	}
	var options []string
	for _, p := range info {
		mark := " "
		if p.Default {
			mark = "●"
		}
		label := fmt.Sprintf("%s  %s  %s", mark, p.Name, p.Model)
		if p.Default {
			label += "  " + lang.T("model.default")
		}
		options = append(options, label)
	}
	m.mode = modeSelect
	m.selectTitle = lang.T("repl.select_model")
	m.selectItems = options
	m.selectIdx = 0
	m.selectOnPick = func(idx int) tea.Cmd {
		fields := strings.Fields(options[idx])
		if len(fields) >= 3 {
			m.app.SetModel(fields[2])
			m.modelName = fields[2]
			m.appendHistory(styleSuccess.Render(lang.T("repl.model_switched", fields[2])) + "\n")
		}
		return nil
	}
	m.textarea.Blur()
	m.refreshViewport()
	return m, nil
}

// handleSessionsCmd /sessions 交互式会话列表
func (m tuiModel) handleSessionsCmd() (tea.Model, tea.Cmd) {
	sessions, _ := m.app.Orchestrator.ListSessions(context.Background(), m.projectID, "cli-user", 10, 0)
	if len(sessions) == 0 {
		m.appendHistory(styleWarn.Render(lang.T("repl.no_sessions")) + "\n")
		return m, nil
	}
	var options []string
	for _, s := range sessions {
		title := s.ID[:8]
		if t, ok := s.Metadata["title"]; ok {
			if ts, ok2 := t.(string); ok2 && ts != "" {
				title = trunc(ts, 30)
			}
		} else {
			for j := len(s.Messages) - 1; j >= 0; j-- {
				if s.Messages[j].Role == "user" {
					title = trunc(s.Messages[j].Content, 40)
					break
				}
			}
		}
		marker := " "
		if s.ID == m.sessionID {
			marker = "●"
		}
		options = append(options, fmt.Sprintf("%s  %s  [%d msgs]  %s", marker, title, len(s.Messages), s.UpdatedAt.Format("15:04")))
	}
	m.mode = modeSelect
	m.selectTitle = lang.T("repl.select_session")
	m.selectItems = options
	m.selectIdx = 0
	m.selectOnPick = func(idx int) tea.Cmd {
		sess := sessions[idx]
		m.sessionID = sess.ID
		m.appendHistory(styleSuccess.Render(fmt.Sprintf("✓ %s", trunc(sess.ID, 8))) + "\n")
		history, _ := m.app.Orchestrator.GetSessionHistory(context.Background(), sess.ID, 3)
		for _, msg := range history {
			switch msg.Role {
			case "user":
				m.appendHistory("    " + styleUser.Render("▸ "+trunc(msg.Content, 80)) + "\n")
			case "assistant":
				m.appendHistory("    " + styleToolDone.Render("✓ "+trunc(msg.Content, 80)) + "\n")
			}
		}
		return nil
	}
	m.textarea.Blur()
	m.refreshViewport()
	return m, nil
}

// handleSessionCmd /session <idx>
func (m tuiModel) handleSessionCmd(parts []string) (tea.Model, tea.Cmd) {
	if len(parts) < 2 {
		m.appendHistory(styleWarn.Render(lang.T("repl.usage_sess")) + "\n")
		return m, nil
	}
	sessions, _ := m.app.Orchestrator.ListSessions(context.Background(), m.projectID, "cli-user", 10, 0)
	idx := 0
	fmt.Sscanf(parts[1], "%d", &idx)
	if idx > 0 && idx <= len(sessions) {
		m.sessionID = sessions[idx-1].ID
		msgCount := len(sessions[idx-1].Messages)
		title := m.sessionID[:8]
		for j := len(sessions[idx-1].Messages) - 1; j >= 0; j-- {
			if sessions[idx-1].Messages[j].Role == "user" {
				title = trunc(sessions[idx-1].Messages[j].Content, 30)
				break
			}
		}
		m.appendHistory(styleSuccess.Render(lang.T("repl.switched_sess", title, msgCount)) + "\n")
	} else {
		m.appendHistory(styleWarn.Render(lang.T("repl.invalid_sess")) + "\n")
	}
	return m, nil
}

// handleTreeCmd /tree 文件树
func (m tuiModel) handleTreeCmd() (tea.Model, tea.Cmd) {
	m.appendHistory(stylePrompt.Render(lang.T("repl.tree_title")) + "\n")
	showFileTreeInto(m.history, ".")
	m.appendHistory("\n")
	return m, nil
}

// handleStatusCmd /status 系统健康与 providers
func (m tuiModel) handleStatusCmd() (tea.Model, tea.Cmd) {
	m.appendHistory(stylePrompt.Render(lang.T("repl.status_title")) + "\n")
	info := m.app.LLMGateway.GetProviderInfos()
	if len(info) == 0 {
		m.appendHistory("  " + styleWarn.Render(lang.T("tui.no_providers")) + "\n")
		return m, nil
	}
	for _, p := range info {
		mark := "○"
		if p.Default {
			mark = "●"
		}
		m.appendHistory(fmt.Sprintf("  %s %s — %s\n", mark, styleSuccess.Render(p.Name), p.Model))
	}
	if m.modelName != "" {
		m.appendHistory("  " + styleDim.Render(lang.T("repl.status_active", m.modelName)) + "\n")
	}
	return m, nil
}

// startSubAgent 单角色子代理
func (m tuiModel) startSubAgent(role, task string) (tea.Model, tea.Cmd) {
	m.subRole = role
	m.mode = modeSubAgent
	m.subStatus = ""
	m.statusMsg = lang.T("repl.sub_running", role)
	m.textarea.Blur()
	m.layoutViewport()
	m.refreshViewport()

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.subProgressCh = make(chan subProgressMsg, 32)
	m.subResultCh = make(chan subAgentMsg, 1)

	progress := func(r string, round int, status string) {
		select {
		case m.subProgressCh <- subProgressMsg{role: r, round: round, status: status}:
		default:
		}
	}

	return m, tea.Batch(
		func() tea.Msg {
			defer cancel()
			result, err := m.app.Orchestrator.RunSubAgent(ctx, "cli-user", m.projectID, role, task, nil, progress)
			m.subResultCh <- subAgentMsg{role: role, result: result, err: err}
			return nil
		},
		waitForSubProgress(m.subProgressCh, m.subResultCh),
	)
}

// startTeam /team 顺序流水线
func (m tuiModel) startTeam(task string) (tea.Model, tea.Cmd) {
	m.subRole = "team"
	m.mode = modeSubAgent
	m.subStatus = ""
	m.statusMsg = lang.T("repl.team_pipeline")
	m.textarea.Blur()
	m.layoutViewport()
	m.refreshViewport()

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.subProgressCh = make(chan subProgressMsg, 32)
	m.subResultCh = make(chan subAgentMsg, 1)

	progress := func(r string, round int, status string) {
		select {
		case m.subProgressCh <- subProgressMsg{role: r, round: round, status: status}:
		default:
		}
	}

	return m, tea.Batch(
		func() tea.Msg {
			defer cancel()
			prevResults := []string{}
			for _, role := range []string{"analyst", "coder", "reviewer"} {
				result, err := m.app.Orchestrator.RunSubAgent(ctx, "cli-user", m.projectID, role, task, prevResults, progress)
				if err != nil {
					m.subResultCh <- subAgentMsg{role: role, result: "", err: err}
					return nil
				}
				prevResults = append(prevResults, result)
			}
			m.subResultCh <- subAgentMsg{role: "team", result: prevResults[len(prevResults)-1], err: nil}
			return nil
		},
		waitForSubProgress(m.subProgressCh, m.subResultCh),
	)
}

// handleTabComplete Tab 补全：/命令、@文件、工具名
func (m tuiModel) handleTabComplete() (tea.Model, tea.Cmd) {
	val := m.textarea.Value()
	li := m.textarea.LineInfo()
	row := m.textarea.Line()
	cursor := li.StartColumn + li.ColumnOffset
	if row > 0 {
		lines := strings.Split(val, "\n")
		for i := 0; i < row && i < len(lines); i++ {
			cursor += len(lines[i]) + 1
		}
	}
	if cursor > len(val) {
		cursor = len(val)
	}
	lineStart := strings.LastIndex(val[:cursor], "\n") + 1
	prefix := val[lineStart:cursor]

	// /命令补全
	if strings.HasPrefix(prefix, "/") {
		commands := []string{"/help", "/clear", "/compact", "/model", "/lang", "/log", "/sessions", "/session", "/handoff", "/exit", "/analyst", "/coder", "/reviewer", "/executor", "/team", "/tree", "/status", "/undo", "/auto", "/research", "/init"}
		var match string
		for _, c := range commands {
			if strings.HasPrefix(c, prefix) {
				match = c
				break
			}
		}
		if match != "" {
			val = val[:lineStart] + match + " " + val[cursor:]
			slog.Info("tui textarea set from completion", "content", strconv.Quote(val))
			m.textarea.SetValue(val)
			m.textarea.CursorEnd()
		}
		return m, nil
	}

	// @文件补全
	if strings.HasPrefix(prefix, "@") {
		pattern := strings.TrimPrefix(prefix, "@")
		matches, _ := filepath.Glob(pattern + "*")
		if len(matches) > 0 {
			val = val[:lineStart] + "@" + matches[0] + " " + val[cursor:]
			slog.Info("tui textarea set from completion", "content", strconv.Quote(val))
			m.textarea.SetValue(val)
			m.textarea.CursorEnd()
		}
		return m, nil
	}
	return m, nil
}

func showFileTreeInto(sb *strings.Builder, root string) {
	cwd, _ := os.Getwd()
	_ = root
	filepath.WalkDir(cwd, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(cwd, path)
		if rel == "." {
			return nil
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
			return filepath.SkipDir
		}
		depth := len(strings.Split(rel, string(filepath.Separator)))
		if depth > 4 {
			return filepath.SkipDir
		}
		if d.IsDir() {
			sb.WriteString("  📁 " + d.Name() + "/\n")
		} else {
			sb.WriteString("    " + d.Name() + "\n")
		}
		return nil
	})
}

func handleInitSize(app *infra.Application) (int, error) {
	cwd, _ := os.Getwd()
	projectName := filepath.Base(cwd)

	if _, err := os.Stat(filepath.Join(cwd, "OPENAIDE.md")); err == nil {
		return 0, fmt.Errorf("OPENAIDE.md already exists. Delete it first to regenerate, or edit it directly.")
	}

	var ctx strings.Builder
	ctx.WriteString(fmt.Sprintf("Project directory: %s\n", cwd))
	ctx.WriteString(fmt.Sprintf("Project name: %s\n\n", projectName))

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
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != "" {
			fileTypes[ext]++
		}
		if keyNames[e.Name()] {
			keyFiles = append(keyFiles, e.Name())
		}
	}
	ctx.WriteString(fmt.Sprintf("File types: %v\n", fileTypes))
	ctx.WriteString(fmt.Sprintf("Key files: %v\n\n", keyFiles))

	ctx.WriteString("Top-level directories:\n")
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			ctx.WriteString(fmt.Sprintf("  %s/\n", e.Name()))
		}
	}

	queryContent := fmt.Sprintf(
		"Based on this project analysis, write an OPENAIDE.md file. Include:\n"+
			"1. Project name and one-line summary\n"+
			"2. Common commands (build, test, run)\n"+
			"3. Architecture overview (key directories and their purposes)\n"+
			"4. Conventions observed from file types\n\n"+
			"%s\n\nWrite ONLY the OPENAIDE.md content, no preamble. Use Markdown format. Keep it concise.",
		ctx.String())
	resp, err := app.Orchestrator.ProcessQuery(context.Background(), "cli-user", projectName, queryContent, kernel.QueryOptions{MaxTokens: 2000})
	if err != nil {
		return 0, err
	}
	if resp.Content == "" {
		return 0, fmt.Errorf("empty response, try again")
	}
	os.WriteFile(filepath.Join(cwd, "OPENAIDE.md"), []byte(resp.Content), 0644)
	return len(resp.Content), nil
}

// toolIcon 工具名 → 图标
func toolIcon(name string) string {
	switch {
	case strings.Contains(name, "read"):
		return "📖"
	case strings.Contains(name, "write"):
		return "✏️"
	case strings.Contains(name, "execute"):
		return "⚡"
	case strings.Contains(name, "web"):
		return "🌐"
	case strings.Contains(name, "search"):
		return "🔍"
	case strings.Contains(name, "git"):
		return "🔀"
	case strings.Contains(name, "web"):
		return "🌐"
	case strings.Contains(name, "browser"):
		return "🖥"
	case strings.Contains(name, "memory"):
		return "🧠"
	case strings.Contains(name, "diff"):
		return "🔀"
	default:
		return "🔧"
	}
}

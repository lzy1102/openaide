package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"openaide/backend/internal/config"
	"openaide/backend/internal/infra"
	"openaide/backend/internal/kernel"
	"openaide/backend/internal/lang"
)

// Build info — injected via ldflags at build time
var (
	Version   = "dev"
	BuildTime = "unknown"
)

type cliFlags struct {
	contextFiles []string
	prompt       string
	continueSess bool
	yes          bool
	model        string
	verbose      bool
	logLevel     string
	outputFormat string
}

func defaultConfigPath() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = "."
	}
	return home + "/.openaide/config.yaml"
}

func isExistingFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func detectFiles(args []string) (files []string, promptParts []string) {
	for _, a := range args {
		if isExistingFile(a) {
			files = append(files, a)
		} else {
			promptParts = append(promptParts, a)
		}
	}
	return
}

func parseFlags(args []string) cliFlags {
	f := cliFlags{
		outputFormat: "text",
	}
	var positional []string

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-c" || a == "--continue":
			f.continueSess = true
		case a == "-y" || a == "--yes":
			f.yes = true
		case a == "--verbose":
			f.verbose = true
		case a == "--log-level" && i+1 < len(args):
			i++
			f.logLevel = args[i]
		case a == "--model" && i+1 < len(args):
			i++
			f.model = args[i]
		case a == "--output" && i+1 < len(args):
			i++
			if args[i] == "json" {
				f.outputFormat = "json"
			}
		case a == "-h" || a == "--help":
			printHelp()
			os.Exit(0)
		case a == "-v" || a == "--version":
			fmt.Printf("OpenAIDE CLI %s (built %s)\n", Version, BuildTime)
			os.Exit(0)
		case a == "help":
			cmdHelp()
			os.Exit(0)
		case a == "version":
			fmt.Printf("OpenAIDE CLI %s (built %s)\n", Version, BuildTime)
			os.Exit(0)
		case a == "update" || a == "upgrade":
			cmdUpdate(args[i+1:])
			os.Exit(0)
		case a == "sessions":
			cmdSessions(args[i+1:])
			os.Exit(0)
		case a == "plugins":
			cmdPlugins(args[i+1:])
			os.Exit(0)
		case a == "setup":
			cmdSetup()
			os.Exit(0)
		case a == "server":
			cmdServer(args[i+1:])
			os.Exit(0)
		case !strings.HasPrefix(a, "-"):
			positional = append(positional, a)
		}
	}

	if len(positional) > 0 {
		files, promptParts := detectFiles(positional)
		f.contextFiles = files
		f.prompt = strings.Join(promptParts, " ")
	}

	return f
}

func buildPrompt(files []string, prompt string) string {
	var parts []string
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", lang.T("warn.read_file", path, err))
			continue
		}
		parts = append(parts, lang.T("prompt.file_content", path, string(data)))
	}
	if len(parts) > 0 && prompt != "" {
		return strings.Join(parts, "\n\n") + "\n\n" + prompt
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n\n")
	}
	return prompt
}

func doAutoCommit(prompt string) {
	if _, err := os.Stat(".git"); os.IsNotExist(err) {
		return
	}
	msg := prompt
	if msg == "" {
		msg = lang.T("git.auto_commit_msg")
	}
	if len(msg) > 72 {
		msg = msg[:72]
	}

	if err := exec.Command("git", "add", "-A").Run(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: git add failed: %v\n", err)
		return
	}

	if exec.Command("git", "diff", "--cached", "--quiet").Run() != nil {
		cmd := exec.Command("git", "commit", "-m", msg)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: git commit failed: %v\n", err)
		}
	}
}

func main() {
	flags := parseFlags(os.Args[1:])

	cfg, err := config.Load(defaultConfigPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: config load failed, using defaults: %v\n", err)
		cfg = config.DefaultConfig()
	}
	cfg.Server.Mode = "direct"
	if flags.logLevel != "" {
		cfg.Log.Level = flags.logLevel
	} else if flags.verbose {
		cfg.Log.Level = "debug"
	}
	infra.TUILogWriter = tuiLogBuf
	infra.InitLogger(cfg.Log.Level, cfg.Log.Format)

	if flags.model != "" {
		for i := range cfg.LLM.Providers {
			if cfg.LLM.Providers[i].DefaultModel != "" {
				cfg.LLM.Providers[i].DefaultModel = flags.model
				break
			}
		}
	}

	// 加载全局语言偏好
	loadGlobalLang(cfg)

	app, err := infra.NewApplication(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", lang.T("err.start_failed", err))
		os.Exit(1)
	}
	if flags.yes {
		app.SetAutoApprove(true)
	}

	// One-shot: prompt from positional args
	if flags.prompt != "" || len(flags.contextFiles) > 0 {
		prompt := buildPrompt(flags.contextFiles, flags.prompt)
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()

		ch, err := app.Orchestrator.ProcessQueryStream(ctx, "cli-user", "default", prompt, kernel.QueryOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", lang.T("err.process", err))
			os.Exit(1)
		}
		var full strings.Builder
		shownTools := map[string]bool{}
		for chunk := range ch {
			switch chunk.Type {
			case kernel.ChunkTypeError:
				fmt.Fprintf(os.Stderr, "\n✗ %s\n", chunk.Content)
				os.Exit(1)
			case kernel.ChunkTypeContent:
				fmt.Print(chunk.Content)
				full.WriteString(chunk.Content)
			case kernel.ChunkTypeToolCall:
				name := chunk.Content
				if !shownTools[name] {
					shownTools[name] = true
					fmt.Fprintf(os.Stderr, "  ⚙ %s …\r", name)
				}
			case kernel.ChunkTypeToolDone:
				fmt.Fprintf(os.Stderr, "\033[K")
			}
		}
		fmt.Println()
		if flags.outputFormat == "json" {
			out := map[string]string{"content": full.String()}
			data, _ := json.Marshal(out)
			fmt.Println(string(data))
		}

		doAutoCommit(flags.prompt)
		return
	}

	// REPL 模式
	// 提前初始化 markdown renderer：glamour.WithAutoStyle() 会触发 termenv 的
	// OSC 11 背景色查询（ESC]11;?ESC\），若在 bubbletea 接管 stdin 后才首次调用，
	// 终端响应会与 bubbletea 的输入读取竞争，泄漏为输入框乱码（gb:0c0c 问题）。
	// 在 TUI 启动前完成查询，避免竞态。
	initMarkdown()
	runREPL(app, flags.continueSess, flags.yes)
}

func printHelp() {
	fmt.Println(lang.T("cli.usage"))
	fmt.Println()
	fmt.Println(lang.T("cli.usage_detail"))
	fmt.Println(lang.T("cli.oneshot"))
	fmt.Println(lang.T("cli.file_oneshot"))
	fmt.Println(lang.T("cli.c"))
	fmt.Println(lang.T("cli.y"))
	fmt.Println(lang.T("cli.model"))
	fmt.Println(lang.T("cli.output"))
	fmt.Println(lang.T("cli.verbose"))
	fmt.Println(lang.T("cli.log_level"))
	fmt.Println(lang.T("cli.sessions"))
	fmt.Println(lang.T("cli.update"))
	fmt.Println(lang.T("cli.setup"))
	fmt.Println("  server            Start API server (web mode)")
	fmt.Println()
	fmt.Println(lang.T("cli.examples"))
	fmt.Println(lang.T("cli.ex_oneshot"))
	fmt.Println(lang.T("cli.ex_file"))
	fmt.Println(lang.T("cli.ex_continue"))
	fmt.Println(lang.T("cli.ex_model"))
	fmt.Println()
	fmt.Println(lang.T("cli.keybindings"))
	fmt.Println(lang.T("cli.kb_quit"))
	fmt.Println(lang.T("cli.kb_sessions"))
	fmt.Println(lang.T("cli.kb_help"))
	fmt.Println(lang.T("cli.kb_history"))
	fmt.Println(lang.T("cli.kb_scroll"))
	fmt.Println()
	fmt.Println(lang.T("cli.commands"))
	fmt.Println(lang.T("cli.cmd_help"))
	fmt.Println(lang.T("cli.cmd_clear"))
	fmt.Println(lang.T("cli.cmd_model"))
	fmt.Println(lang.T("cli.cmd_analyst"))
	fmt.Println(lang.T("cli.cmd_coder"))
	fmt.Println(lang.T("cli.cmd_reviewer"))
	fmt.Println(lang.T("cli.cmd_executor"))
	fmt.Println(lang.T("cli.cmd_team"))
}

func cmdSessions(args []string) {
	cfg, err := config.Load(defaultConfigPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: config load failed, using defaults: %v\n", err)
		cfg = config.DefaultConfig()
	}
	cfg.Server.Mode = "direct"
	infra.TUILogWriter = tuiLogBuf
	infra.InitLogger(cfg.Log.Level, cfg.Log.Format)

	app, err := infra.NewApplication(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", lang.T("err.start_failed", err))
		os.Exit(1)
	}
	ctx := context.Background()

	sessions, err := app.Orchestrator.ListSessions(ctx, "default", "cli-user", 100, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", lang.T("err.process", err))
		os.Exit(1)
	}
	if len(sessions) == 0 {
		fmt.Println(lang.T("sess.none"))
		return
	}
	fmt.Println(lang.T("sess.info"))
	for _, s := range sessions {
		msgCount := len(s.Messages)
		preview := ""
		for _, m := range s.Messages {
			if m.Role == "user" && m.Content != "" {
				preview = truncate(m.Content, 60)
				break
			}
		}
		fmt.Printf(lang.T("sess.list_format"), s.ID, msgCount, preview)
	}
}

func cmdHelp() {
	printHelp()
	os.Exit(0)
}

func cmdUpdate(args []string) {
	fmt.Println(lang.T("update.title"))
	installDir := os.Getenv("HOME") + "/.openaide"
	script := filepath.Join(installDir, "scripts", "update.sh")
	if _, err := os.Stat(script); os.IsNotExist(err) {
		script = filepath.Join(installDir, "install.sh")
		if _, err := os.Stat(script); os.IsNotExist(err) {
			fmt.Println(lang.T("update.script_not_found"))
			os.Exit(1)
		}
	}

	cmdArgs := []string{script}
	for _, arg := range args {
		if arg == "--local" || arg == "-l" {
			cmdArgs = append(cmdArgs, "--local")
		}
	}

	cmd := exec.Command("bash", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fmt.Printf(lang.T("update.failed"), err)
		os.Exit(1)
	}
	fmt.Println(lang.T("update.complete"))
}

// loadGlobalLang 从全局配置加载语言偏好
func loadGlobalLang(cfg *config.Config) {
	if cfg.Lang == "zh" {
		lang.SetLang(lang.ZH)
	} else if cfg.Lang == "en" {
		lang.SetLang(lang.EN)
	}
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return string(runes)
	}
	return string(runes[:n]) + "…"
}

func cmdServer(args []string) {
	var configPath string
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	fs.StringVar(&configPath, "config", defaultConfigPath(), "Path to config file")
	fs.Parse(args)

	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Warn("Failed to load config, using default", "error", err)
		cfg = config.DefaultConfig()
	}
	cfg.Server.Mode = "server"

	infra.InitLogger(cfg.Log.Level, cfg.Log.Format)

	app, err := infra.NewApplication(cfg)
	if err != nil {
		slog.Error("Failed to create application", "error", err)
		os.Exit(1)
	}

	// 嵌入前端
	if h := FrontendHandler(); h != nil {
		app.APIServer.SetFrontendHandler(h)
		slog.Info("Frontend embedded in binary")
	}

	// 配置文件热加载
	reloader := infra.NewConfigReloader(configPath, app)
	if err := reloader.Start(); err != nil {
		slog.Warn("Config hot-reload unavailable", "error", err)
	}
	defer reloader.Stop()

	go func() {
		if err := app.Start(); err != nil {
			slog.Error("Application error", "error", err)
			os.Exit(1)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	slog.Info("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := app.Stop(ctx); err != nil {
		slog.Error("Shutdown error", "error", err)
	}

	slog.Info("Goodbye!")
}

func cmdPlugins(args []string) {
	cfg, err := config.Load(defaultConfigPath())
	if err != nil {
		cfg = config.DefaultConfig()
	}
	infra.TUILogWriter = tuiLogBuf
	infra.InitLogger(cfg.Log.Level, cfg.Log.Format)

	app, err := infra.NewApplication(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "启动失败: %v\n", err)
		os.Exit(1)
	}

	plugins := app.PluginManager.List()
	if len(args) > 0 && args[0] == "install" && len(args) >= 2 {
		fmt.Printf("要从 GitHub 安装插件: %s\n", args[1])
		fmt.Println("运行: git clone", args[1], cfg.Storage.DataDir+"/plugins/"+filepath.Base(args[1]))
		cmd := exec.Command("git", "clone", args[1], cfg.Storage.DataDir+"/plugins/"+filepath.Base(args[1]))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "安装失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ 插件已安装。重启 openaide 生效。")
		return
	}

	if len(args) > 0 && args[0] == "search" {
		fmt.Println("搜索 Claude 插件：")
		fmt.Println("  GitHub: https://github.com/topics/claude-plugin")
		fmt.Println("  Claude 官方: https://github.com/anthropics/claude-plugins-official")
		fmt.Println("  Superpowers: https://github.com/obra/superpowers")
		fmt.Println()
		fmt.Println("安装: openaide plugins install <github-url>")
		return
	}

	if len(plugins) == 0 {
		fmt.Println("没有安装的插件。")
		fmt.Println("搜索: openaide plugins search")
		fmt.Println("安装: openaide plugins install <github-url>")
		return
	}

	fmt.Printf("已安装 %d 个插件:\n\n", len(plugins))
	for _, p := range plugins {
		status := "✓"
		if !p.Enabled {
			status = "✗"
		}
		fmt.Printf("  %s %s (%s)\n", status, p.Name, p.Version)
		if p.Description != "" {
			fmt.Printf("    %s\n", p.Description)
		}
	}
}

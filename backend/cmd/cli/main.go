package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/term"
	"openaide/backend/internal/config"
	"openaide/backend/internal/infra"
	"openaide/backend/internal/kernel"
)

// ANSI 颜色码
const (
	Reset   = "\033[0m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"
	Bold    = "\033[1m"
	Dim     = "\033[2m"
)

func main() {
	// 解析子命令
	args := os.Args[1:]

	// 如果有子命令，处理子命令
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "update", "upgrade":
			cmdUpdate(args[1:])
			return
		case "version", "-v", "--version":
			cmdVersion()
			return
		case "help", "-h", "--help":
			cmdHelp()
			return
		case "tui":
			cmdTUI(args[1:])
			return
		case "chat", "run":
			args = args[1:]
		}
	}

	// 默认启动 TUI
	cmdTUI(args)
}

// cmdChat 启动交互式聊天
func cmdChat(args []string) {
	configPath := os.Getenv("HOME") + "/.openaide/config.yaml"

	// 解析 -config 参数
	for i, arg := range args {
		if arg == "-config" && i+1 < len(args) {
			configPath = args[i+1]
			break
		}
	}

	// 加载配置
	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Warn("Failed to load config, using default", "error", err)
		cfg = config.DefaultConfig()
	}

	// 强制 direct 模式
	cfg.Server.Mode = "direct"

	// 初始化日志
	infra.InitLogger(cfg.Log.Level, cfg.Log.Format)

	// 创建应用
	app, err := infra.NewApplication(cfg)
	if err != nil {
		slog.Error("Failed to create application", "error", err)
		os.Exit(1)
	}

	// 打印彩色欢迎信息
	printWelcome()

	// 设置终端为 raw mode，支持退格、删除等
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		runSimpleMode(app)
		return
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	terminal := term.NewTerminal(os.Stdin, color(Cyan, "> "))

	for {
		input, err := terminal.ReadLine()
		if err != nil {
			break
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		switch input {
		case "quit", "exit":
			write(terminal, Green+"Goodbye!"+Reset+"\r\n")
			return
		case "/clear":
			write(terminal, Yellow+"上下文已清除"+Reset+"\r\n")
			continue
		}

		// 使用流式输出处理查询
		ctx := context.Background()
		if err := processStream(ctx, app, input, terminal); err != nil {
			write(terminal, Red+"Error: "+err.Error()+Reset+"\r\n")
			continue
		}
	}
}

// cmdUpdate 更新命令
func cmdUpdate(args []string) {
	fmt.Println(color(Bold+Blue, "▶ OpenAIDE 更新"))
	fmt.Println()

	installDir := os.Getenv("HOME") + "/.openaide"
	updateScript := filepath.Join(installDir, "scripts", "update.sh")

	// 检查更新脚本是否存在
	if _, err := os.Stat(updateScript); os.IsNotExist(err) {
		// 尝试使用 install.sh
		installScript := filepath.Join(installDir, "install.sh")
		if _, err := os.Stat(installScript); os.IsNotExist(err) {
			fmt.Printf("%s错误: 未找到更新脚本%s\n", Red, Reset)
			fmt.Println("请确保 OpenAIDE 已正确安装")
			fmt.Println()
			fmt.Println("手动更新:")
			fmt.Println("  cd ~/.openaide && git pull && go build -o bin/openaide-cli ./backend/cmd/cli")
			os.Exit(1)
		}
		updateScript = installScript
	}

	// 构建命令参数
	cmdArgs := []string{updateScript}

	// 检查是否有 --local 参数
	useLocal := false
	for _, arg := range args {
		if arg == "--local" || arg == "-l" {
			useLocal = true
			cmdArgs = append(cmdArgs, "--local")
		}
	}

	if !useLocal {
		fmt.Println(color(Blue, "[INFO]"), "正在检查最新版本...")
	}

	// 执行更新脚本
	cmd := exec.Command("bash", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	fmt.Println()
	if err := cmd.Run(); err != nil {
		fmt.Printf("\n%s更新失败: %v%s\n", Red, err, Reset)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Printf("%s%s 更新完成!%s\n", Bold+Green, "✓", Reset)
}

// cmdVersion 显示版本
func cmdVersion() {
	fmt.Println("OpenAIDE CLI")
	fmt.Println("版本: dev")
	fmt.Println("用法: openaide [chat|update|version|help]")
}

// cmdHelp 显示帮助
func cmdHelp() {
	fmt.Println(color(Bold+Cyan, "▶ OpenAIDE CLI 帮助"))
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  openaide              启动 TUI 界面 (默认)")
	fmt.Println("  openaide chat         启动交互式聊天 (raw mode)")
	fmt.Println("  openaide tui          启动 TUI 界面 (推荐)")
	fmt.Println("  openaide update       更新到最新版本")
	fmt.Println("  openaide update --local  本地重新编译")
	fmt.Println("  openaide version      显示版本信息")
	fmt.Println("  openaide help         显示此帮助")
	fmt.Println()
	fmt.Println("聊天命令:")
	fmt.Println("  quit/exit             退出")
	fmt.Println("  /clear                清除上下文")
	fmt.Println()
	fmt.Println("示例:")
	fmt.Println("  openaide              # 启动聊天")
	fmt.Println("  openaide update       # 更新程序")
	fmt.Println()
}

// processStream 流式处理并输出
// 首次写入用 term 清除提示符，后续流式内容直接写 stdout 以保证实时显示
func processStream(ctx context.Context, app *infra.Application, input string, term *term.Terminal) error {
	// 使用流式处理
	stream, err := app.Orchestrator.ProcessQueryStream(ctx, "cli-user", "default", input, kernel.QueryOptions{})
	if err != nil {
		return err
	}

	// 通过 terminal 写入前缀（同时清除提示符）
	term.Write([]byte(Bold + Green + "AI:" + Reset + " "))

	var fullContent strings.Builder
	var tokensUsed int
	var toolCalls int

	// 接收流式数据 — 直接写 stdout，绕过 terminal 的提示符管理以保证流式效果
	for chunk := range stream {
		if chunk.Error != nil {
			os.Stdout.Write([]byte("\r\n" + Red + "Error: " + chunk.Error.Error() + Reset + "\r\n"))
			return chunk.Error
		}

		if chunk.Done {
			if chunk.Usage != nil {
				tokensUsed = chunk.Usage.TotalTokens
			}
			break
		}

		// 思考过程（DeepSeek thinking mode）
		if chunk.ReasoningContent != "" {
			thinking := strings.ReplaceAll(chunk.ReasoningContent, "\n", "\r\n")
			os.Stdout.Write([]byte(Dim + thinking + Reset))
		}

		// 正文内容
		if chunk.Content != "" {
			content := strings.ReplaceAll(chunk.Content, "\n", "\r\n")
			os.Stdout.Write([]byte(content))
			fullContent.WriteString(chunk.Content)
		}

		// 统计工具调用
		if len(chunk.ToolCalls) > 0 {
			toolCalls = len(chunk.ToolCalls)
		}
	}

	os.Stdout.Write([]byte("\r\n"))

	// 显示工具调用信息
	if toolCalls > 0 {
		os.Stdout.Write([]byte(Dim + "(使用了 " + itoa(toolCalls) + " 个工具)" + Reset + "\r\n"))
	}

	// 显示 token 使用统计
	if tokensUsed > 0 {
		os.Stdout.Write([]byte(Dim + "[Tokens: " + itoa(tokensUsed) + "]" + Reset + "\r\n"))
	}
	os.Stdout.Write([]byte("\r\n"))

	return nil
}

// write 向终端写入内容（通过 term 以正确管理提示符重绘）
func write(term *term.Terminal, s string) {
	term.Write([]byte(s))
}

// itoa 简单整数转字符串（避免引入 strconv）
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

// printWelcome 打印彩色欢迎信息
func printWelcome() {
	fmt.Println()
	fmt.Printf("%s%s OpenAIDE CLI %s\n", Bold+Cyan, "▶", Reset)
	fmt.Printf("%s  AI 编程助手%s\n", Dim, Reset)
	fmt.Println()
	fmt.Printf("%s命令:%s\n", Bold, Reset)
	fmt.Printf("  %squit/exit%s  退出\n", Yellow, Reset)
	fmt.Printf("  %s/clear%s     清除上下文\n", Yellow, Reset)
	fmt.Println()
	fmt.Printf("%s---%s\n", Dim, Reset)
	fmt.Println()
}

// color 包装颜色
func color(c, s string) string {
	return c + s + Reset
}

// runSimpleMode 简单模式（非流式）
func runSimpleMode(app *infra.Application) {
	fmt.Printf("%s[警告] 终端不支持高级输入模式，使用简单模式%s\n", Yellow, Reset)

	var input string
	for {
		fmt.Print(color(Cyan, "> "))
		if _, err := fmt.Scanln(&input); err != nil {
			break
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		switch input {
		case "quit", "exit":
			fmt.Println(color(Green, "Goodbye!"))
			return
		case "/clear":
			fmt.Println(color(Yellow, "上下文已清除"))
			continue
		}

		// 简单模式使用同步处理
		ctx := context.Background()
		resp, err := app.Orchestrator.ProcessQuery(ctx, "cli-user", "default", input, kernel.QueryOptions{})
		if err != nil {
			fmt.Printf("%sError: %v%s\n", Red, err, Reset)
			continue
		}

		fmt.Printf("%sAI:%s %s\n", Bold+Green, Reset, resp.Content)
		if resp.ToolCalls > 0 {
			fmt.Printf("%s(使用了 %d 个工具)%s\n", Dim, resp.ToolCalls, Reset)
		}
		if resp.TokensUsed > 0 {
			fmt.Printf("%s[Tokens: %d]%s\n", Dim, resp.TokensUsed, Reset)
		}
		fmt.Println()
	}
}

// cmdTUI 启动 Bubbletea TUI
func cmdTUI(args []string) {
	configPath := os.Getenv("HOME") + "/.openaide/config.yaml"
	for i, arg := range args {
		if arg == "-config" && i+1 < len(args) {
			configPath = args[i+1]
			break
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		cfg = config.DefaultConfig()
	}
	cfg.Server.Mode = "direct"

	infra.InitLogger(cfg.Log.Level, cfg.Log.Format)

	app, err := infra.NewApplication(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create application: %v\n", err)
		os.Exit(1)
	}

	if err := runTUI(app); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}
}

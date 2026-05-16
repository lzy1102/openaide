package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
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
	var configPath string
	flag.StringVar(&configPath, "config", os.Getenv("HOME")+"/.openaide/config.yaml", "Path to config file")
	flag.Parse()

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
			fmt.Println(color(Green, "Goodbye!"))
			return
		case "/clear":
			fmt.Println(color(Yellow, "上下文已清除"))
			continue
		}

		// 处理查询
		ctx := context.Background()
		resp, err := app.Orchestrator.ProcessQuery(ctx, "cli-user", "default", input, kernel.QueryOptions{})
		if err != nil {
			fmt.Printf("%sError: %v%s\n", Red, err, Reset)
			continue
		}

		// 彩色输出 AI 回复
		fmt.Printf("%sAI:%s %s\n", Bold+Green, Reset, resp.Content)
		if resp.ToolCalls > 0 {
			fmt.Printf("%s(使用了 %d 个工具)%s\n", Dim, resp.ToolCalls, Reset)
		}
		fmt.Println()
	}
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

// runSimpleMode 简单模式
func runSimpleMode(app *infra.Application) {
	fmt.Printf("%s[警告] 终端不支持高级输入模式%s\n", Yellow, Reset)

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
		fmt.Println()
	}
}

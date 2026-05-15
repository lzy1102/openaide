package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"openaide/backend/internal/config"
	"openaide/backend/internal/infra"
	"openaide/backend/internal/kernel"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "./config.json", "Path to config file")
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

	fmt.Println("OpenAIDE CLI - AI 编程助手")
	fmt.Println("输入 'quit' 或 'exit' 退出")
	fmt.Println("输入 '/clear' 清除上下文")
	fmt.Println("---")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		switch input {
		case "quit", "exit":
			fmt.Println("Goodbye!")
			return
		case "/clear":
			fmt.Println("上下文已清除")
			continue
		}

		// 处理查询
		ctx := context.Background()
		resp, err := app.Orchestrator.ProcessQuery(ctx, "cli-user", "default", input, kernel.QueryOptions{})
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		fmt.Println(resp.Content)
		if resp.ToolCalls > 0 {
			fmt.Printf("(使用了 %d 个工具)\n", resp.ToolCalls)
		}
		fmt.Println()
	}
}

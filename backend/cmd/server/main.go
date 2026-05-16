package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"openaide/backend/internal/config"
	"openaide/backend/internal/infra"
)

func main() {
	var configPath string
	defaultConfig := os.Getenv("HOME") + "/.openaide/config.yaml"
	if _, err := os.Stat(defaultConfig); err != nil {
		defaultConfig = "./config.yaml"
	}
	flag.StringVar(&configPath, "config", defaultConfig, "Path to config file")
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(configPath)
	if err != nil {
		// 使用默认配置
		slog.Warn("Failed to load config, using default", "error", err)
		cfg = config.DefaultConfig()
	}

	// 初始化日志
	infra.InitLogger(cfg.Log.Level, cfg.Log.Format)

	// 创建应用
	app, err := infra.NewApplication(cfg)
	if err != nil {
		slog.Error("Failed to create application", "error", err)
		os.Exit(1)
	}

	// 启动应用
	go func() {
		if err := app.Start(); err != nil {
			slog.Error("Application error", "error", err)
			os.Exit(1)
		}
	}()

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	slog.Info("Shutting down...")
	if err := app.Stop(context.Background()); err != nil {
		slog.Error("Shutdown error", "error", err)
	}

	slog.Info("Goodbye!")
}

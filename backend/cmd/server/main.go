package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"openaide/backend/config"
	"openaide/backend/internal/infra"
	"openaide/backend/internal/webfront"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", os.Getenv("HOME")+"/.openaide/config.yaml", "Path to config file")
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

	// 嵌入前端 (webfront 共享包)
	if h := webfront.FrontendHandler(); h != nil {
		app.APIServer.SetFrontendHandler(h)
		slog.Info("Frontend embedded in binary")
	}

	// 启动应用
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

	// 等待中断信号
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

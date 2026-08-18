package infra

import (
	"context"
	"log/slog"
	"time"

	"openaide/backend/internal/channel"
	"openaide/backend/internal/config"
	"openaide/backend/core"
	"openaide/backend/orchestration"
	"openaide/backend/internal/plugin"
)

func setupChannels(app *Application, cfg *config.Config, orch *orchestration.Orchestrator, pluginMgr *plugin.Manager) error {
	// 搭建渠道层（外部消息接入）
	chRegistry := channel.NewRegistry()
	app.ChannelRegistry = chRegistry
	app.APIServer.SetChannelRegistry(chRegistry)

	// 启动异步任务队列
	taskQueue := channel.NewTaskQueue(channel.QueueConfig{
		WorkerCount: cfg.Channels.TaskQueue.WorkerCount,
		QueueSize:   cfg.Channels.TaskQueue.QueueSize,
	})
	app.TaskQueue = taskQueue

	// 注册Webhook渠道
	for _, whCfg := range cfg.Channels.Webhooks {
		wh := channel.NewWebhookReceiver(
			channel.WebhookConfig{ID: whCfg.ID, Name: whCfg.Name, SecretToken: whCfg.SecretToken, CallbackURL: whCfg.CallbackURL},
			app.APIServer.RegisterHandler(),
			"/api/v1/channels",
		)
		if err := chRegistry.Register(wh); err != nil {
			slog.Warn("Failed to register webhook channel", "id", whCfg.ID, "error", err)
		}
	}

	// 注册飞书渠道
	for _, fsCfg := range cfg.Channels.Feishu {
		fs := channel.NewFeishuBot(
			channel.FeishuConfig{ID: fsCfg.ID, Name: fsCfg.Name, AppID: fsCfg.AppID, AppSecret: fsCfg.AppSecret, VerifyToken: fsCfg.VerifyToken, AESKey: fsCfg.AESKey},
			app.APIServer.RegisterHandler(),
			"/api/v1/channels",
		)
		if err := chRegistry.Register(fs); err != nil {
			slog.Warn("Failed to register feishu channel", "id", fsCfg.ID, "error", err)
		}
	}

	// 注册Telegram渠道
	for _, tgCfg := range cfg.Channels.Telegram {
		tg := channel.NewTelegramBot(
			channel.TelegramConfig{ID: tgCfg.ID, Name: tgCfg.Name, Token: tgCfg.Token},
			app.APIServer.RegisterHandler(),
			"/api/v1/channels",
		)
		if err := chRegistry.Register(tg); err != nil {
			slog.Warn("Failed to register telegram channel", "id", tgCfg.ID, "error", err)
		}
	}

	// 创建渠道消息处理器 — 通过任务队列异步处理
	channelHandler := func(ctx context.Context, msg *channel.Message) (*channel.Response, error) {
		content := msg.Content

		// 插件消息钩子
		if pluginMgr != nil && content != "" {
			kernelMsg := &kernel.Message{Role: "user", Content: content}
			modified, err := pluginMgr.RunMessageHooks(ctx, kernelMsg)
			if err != nil {
				return &channel.Response{Content: "消息被插件拦截"}, nil
			}
			if modified != nil {
				content = modified.Content
			}
		}

		// 入队异步处理
		chID := msg.ChannelID
		userID := msg.UserID
		task := &channel.Task{
			ChannelID: chID,
			UserID:    userID,
			Content:   content,
			OnResult: func(result *channel.TaskResult) {
				ch := chRegistry.Get(chID)
				if ch == nil {
					return
				}
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				respContent := result.Content
				if result.Error != "" {
					respContent = "处理失败: " + result.Error
				}
				if respContent == "" {
					return
				}
				_ = ch.Send(ctx, userID, &channel.Response{Content: respContent})
			},
		}
		if err := taskQueue.Enqueue(task); err != nil {
			slog.Error("Failed to enqueue channel task", "channel", msg.ChannelID, "error", err)
			return nil, err
		}

		return &channel.Response{Content: "任务已接收入队处理"}, nil
	}

	// 启动所有渠道
	if err := chRegistry.StartAll(context.Background(), channelHandler); err != nil {
		slog.Warn("Failed to start some channels", "error", err)
	}

	// 启动任务队列工作池
	_ = taskQueue.Start(context.Background(), func(ctx context.Context, task *channel.Task) *channel.TaskResult {
		resp, err := orch.ProcessQuery(ctx, task.UserID, task.ChannelID, task.Content, kernel.QueryOptions{})
		if err != nil {
			result := &channel.TaskResult{TaskID: task.ID, Error: err.Error(), Completed: false}
			if task.OnResult != nil {
				task.OnResult(result)
			}
			return result
		}
		result := &channel.TaskResult{TaskID: task.ID, Content: resp.Content, Completed: true}
		if task.OnResult != nil {
			task.OnResult(result)
		}
		return result
	})

	return nil
}

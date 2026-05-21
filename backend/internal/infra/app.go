package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"openaide/backend/internal/api"
	"openaide/backend/internal/auth"
	"openaide/backend/internal/channel"
	"openaide/backend/internal/compress"
	"openaide/backend/internal/config"
	"openaide/backend/internal/event"
	"openaide/backend/internal/feedback"
	"openaide/backend/internal/identity"
	"openaide/backend/internal/kernel"
	"openaide/backend/internal/knowledge"
	"openaide/backend/internal/llm"
	"openaide/backend/internal/mcp"
	"openaide/backend/internal/memory"
	"openaide/backend/internal/orchestration"
	"openaide/backend/internal/plugin"
	"openaide/backend/internal/tools"
)

// Application 应用容器
type Application struct {
	Config          *config.Config
	Kernel          kernel.Kernel
	Orchestrator    *orchestration.Orchestrator
	APIServer       *api.Server
	LLMGateway      *llm.Gateway
	ChannelRegistry *channel.Registry
	TaskQueue       *channel.TaskQueue
	PluginManager   *plugin.Manager
	MCPManager      *mcp.Manager
}

// NewApplication 创建应用容器
func NewApplication(cfg *config.Config) (*Application, error) {
	app := &Application{Config: cfg}

	// 1. 创建 LLM 网关
	gateway := llm.NewGateway()
	for _, providerCfg := range cfg.LLM.Providers {
		if !providerCfg.Enabled {
			continue
		}

		var provider llm.Provider
		llmConfig := &llm.ProviderConfig{
			Name:            providerCfg.Name,
			Type:            providerCfg.Type,
			BaseURL:         providerCfg.BaseURL,
			APIKey:          providerCfg.APIKey,
			DefaultModel:    providerCfg.DefaultModel,
			Timeout:         providerCfg.Timeout,
			Headers:         providerCfg.Headers,
			Enabled:         providerCfg.Enabled,
			Thinking:        providerCfg.Thinking,
			ReasoningEffort: providerCfg.ReasoningEffort,
			JSONMode:        providerCfg.JSONMode,
			StrictTools:     providerCfg.StrictTools,
		}

		switch providerCfg.Type {
		case "openai", "openai-compatible":
			provider = llm.NewOpenAIProvider(llmConfig)
		case "anthropic":
			provider = llm.NewAnthropicProvider(llmConfig)
		default:
			slog.Warn("Unknown provider type, skipping", "name", providerCfg.Name, "type", providerCfg.Type)
			continue
		}

		gateway.RegisterProvider(providerCfg.Name, provider, llmConfig)
		slog.Info("Provider registered", "name", providerCfg.Name, "model", providerCfg.DefaultModel)
	}

	if cfg.LLM.DefaultProvider != "" {
		if err := gateway.SetDefaultProvider(cfg.LLM.DefaultProvider); err != nil {
			slog.Warn("Failed to set default provider", "error", err)
		}
	}

	app.LLMGateway = gateway
	gateway.SetPromptCache(llm.NewPromptCache(cfg.Storage.DataDir + "/cache"))

	// 创建 Embedder 适配器（用于记忆/知识库语义搜索）
	embedderDim := 1536 // OpenAI text-embedding-ada-002 默认维度
	embedder := llm.NewEmbedderFunc(gateway.Embed, gateway.EmbedBatch, embedderDim)

	// 模型路由：按任务类型自动选择provider/model
	if cfg.Router.Enabled {
		gateway.SetRouter(llm.NewRouter(cfg.Router.Rules))
	} else {
		gateway.SetRouter(llm.DefaultRouter()) // 内置路由规则
	}

	// 浏览器配置：配置文件 browser.enabled 或 环境变量 OPENAIDE_BROWSER
	tools.SetBrowserEnabled(cfg.Browser.Enabled)

	// 2. 创建工具注册表
	toolRegistry := tools.NewRegistry()
	if err := tools.RegisterBuiltins(toolRegistry); err != nil {
		return nil, fmt.Errorf("register builtin tools failed: %w", err)
	}

	// MCP 服务器连接（可选，通过配置启用）
	mcpManager := mcp.NewManager()
	if cfg.MCP.Enabled {
		for _, srvCfg := range cfg.MCP.Servers {
			slog.Info("Connecting MCP server", "id", srvCfg.ID, "command", srvCfg.Command)
			if err := mcpManager.ConnectServer(srvCfg.ID, srvCfg.Command, srvCfg.Args...); err != nil {
				slog.Warn("Failed to connect MCP server, skipping", "id", srvCfg.ID, "error", err)
				continue
			}
			for _, mcpTool := range mcpManager.GetServerTools(srvCfg.ID) {
				def := kernel.ToolDefinition{
					Type: "function",
					Function: kernel.FunctionDef{
						Name:        mcpTool.Name,
						Description: mcpTool.Description,
						Parameters:  mcpTool.InputSchema,
					},
				}
				serverID, toolName := srvCfg.ID, mcpTool.Name
				handler := kernel.ToolHandler(func(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
					var args map[string]interface{}
					if arguments != "" {
						if err := json.Unmarshal([]byte(arguments), &args); err != nil {
							return &kernel.ToolResult{Error: fmt.Sprintf("invalid args: %v", err)}, nil
						}
					}
					result, err := mcpManager.CallTool(serverID, toolName, args)
					if err != nil {
						return &kernel.ToolResult{Error: err.Error()}, nil
					}
					var content string
					for _, item := range result.Content {
						if item.Type == "text" && item.Text != "" {
							if content != "" {
								content += "\n"
							}
							content += item.Text
						}
					}
					errStr := ""
					if result.IsError {
						errStr = content
					}
					return &kernel.ToolResult{Content: content, Error: errStr}, nil
				})
				if err := toolRegistry.Register(def, handler); err != nil {
					slog.Warn("MCP tool registration skipped, duplicate name", "tool", mcpTool.Name, "error", err)
				} else {
					slog.Info("MCP tool registered", "server", srvCfg.ID, "tool", mcpTool.Name)
				}
			}
		}
	}
	app.MCPManager = mcpManager

	// 3. 创建记忆管理器
	memManager, err := memory.NewFileMemory(cfg.Memory.DataDir)
	if err != nil {
		slog.Warn("Failed to create memory manager", "error", err)
		memManager = nil
	} else {
		memManager.SetEmbedder(embedder)
	}

	// 4. 创建会话存储
	// 使用文件持久化会话存储（死机可恢复）
	var sessionStore kernel.SessionStore
	fileStore, err := kernel.NewFileSessionStore(cfg.Storage.DataDir + "/sessions")
	if err != nil {
		slog.Warn("Failed to create file session store, using memory", "error", err)
		sessionStore = kernel.NewSessionStoreAdapter()
	} else {
		sessionStore = fileStore
	}

	// 5. 创建内核
	kernelConfig := &kernel.Config{
		MaxRounds:    cfg.Kernel.MaxRounds,
		MaxTokens:    cfg.Kernel.MaxTokens,
		SystemPrompt: cfg.Kernel.SystemPrompt,
	}
	if kernelConfig.MaxRounds == 0 {
		kernelConfig.MaxRounds = 10
	}
	if kernelConfig.MaxTokens == 0 {
		kernelConfig.MaxTokens = 4000
	}

	agentKernel := kernel.NewAgentKernel(gateway, toolRegistry, memManager, sessionStore, kernelConfig)
	app.Kernel = agentKernel

	// 接入增强能力 — LLM Reflection（降级到 SimpleReflection）
	agentKernel.SetReflection(kernel.NewLLMReflection(gateway, kernel.NewSimpleReflection()))
	sm := kernel.NewSkillManager(cfg.Storage.DataDir + "/skills")
	agentKernel.SetSkillManager(sm)
	if learner, err := kernel.NewSimpleLearner(cfg.Storage.DataDir + "/insights"); err == nil {
		agentKernel.SetLearner(learner)
		slog.Info("Learner enabled", "dir", cfg.Storage.DataDir+"/insights")
	} else {
		slog.Warn("Failed to create learner, learning disabled", "error", err)
	}
	agentKernel.SetPatternDetector(kernel.NewSimplePatternDetector())
	agentKernel.SetSkillEvolution(kernel.NewSkillEvolution(sm, cfg.Storage.DataDir+"/skills"))
	approver := kernel.NewAutoApprover()
	approver.UnsafeMode = true // 保留本地便利模式；设为 false 启用危险工具拦截
	agentKernel.SetApprover(approver)
	agentKernel.SetAdaptiveRounds(kernel.NewAdaptiveRounds(5, 30))

	if cp, err := kernel.NewFileCheckpointer(kernel.FileCheckpointerConfig{
		Dir: cfg.Storage.DataDir + "/checkpoints",
	}); err == nil {
		agentKernel.SetCheckpointer(cp)
		slog.Info("Checkpoint enabled", "dir", cfg.Storage.DataDir+"/checkpoints")
	} else {
		slog.Warn("Failed to create checkpointer, checkpoint disabled", "error", err)
	}

	if tracer, err := kernel.NewFileTracer(kernel.FileTracerConfig{
		FilePath: cfg.Storage.DataDir + "/traces.jsonl",
	}); err == nil {
		agentKernel.SetTracer(tracer)
		slog.Info("Tracing enabled", "file", cfg.Storage.DataDir+"/traces.jsonl")
	} else {
		slog.Warn("Failed to create tracer, tracing disabled", "error", err)
	}

	// 接入插件管理器
	pluginMgr := plugin.NewManager(cfg.Storage.DataDir + "/plugins")
	pluginPrompt := pluginMgr.GetPrompt()
	if pluginPrompt != "" {
		kernelConfig.SystemPrompt += "\n\n" + pluginPrompt
	}

	// 接入知识库 + 质量门控 + 语义搜索
	kb, err := knowledge.NewBase(cfg.Storage.DataDir + "/knowledge")
	if err == nil {
		kb.SetEmbedder(embedder)
		agentKernel.SetKnowledgeCollector(kb)
		gate := feedback.NewGate()
		agentKernel.SetQualityGate(gate)
	} else {
		slog.Warn("Failed to create knowledge base", "error", err)
	}

	// 接入身份检测 + 事件总线 + 高级压缩器
	if projIdentity, err := identity.NewDetector().Detect(context.Background(), "."); err == nil && projIdentity != nil {
		slog.Info("Project identity detected", "type", projIdentity.ProjectType)
	}
	eventBus := event.NewBus()
	eventBus.EnablePersistence(cfg.Storage.DataDir + "/events")
	agentKernel.SetContextCompressor(compress.NewLLMCompressor(gateway, compress.NewNovelCompressor()))

	agentKernel.Subscribe(kernel.EventHandlerFunc(func(evt kernel.Event) {
		pluginMgr.RunEventHooks(context.Background(), evt)
		eventBus.Publish(evt)
	}))

	// 6. 创建编排器
	orch := orchestration.NewOrchestrator(agentKernel, gateway, toolRegistry, memManager, sessionStore)
	if kb != nil {
		orch.SetKnowledgeCollector(kb)
	}
	app.Orchestrator = orch

	// 7. 创建 API 服务器
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	// 认证默认关闭（本地Agent不需要）。OPENAIDE_AUTH=true 开启JWT
	var authSvc *auth.Service
	if os.Getenv("OPENAIDE_AUTH") == "true" {
		authSvc = auth.NewService(os.Getenv("OPENAIDE_JWT_SECRET"))
	}
	app.APIServer = api.NewServer(orch, addr, authSvc)
	app.PluginManager = pluginMgr

	// 8. 搭建渠道层（外部消息接入）
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
	taskQueue.Start(context.Background(), func(ctx context.Context, task *channel.Task) *channel.TaskResult {
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

	return app, nil
}

// Start 启动应用
func (app *Application) Start() error {
	// 根据模式启动
	switch app.Config.Server.Mode {
	case "server":
		return app.APIServer.Start()
	case "direct", "":
		slog.Info("Direct mode - API server not started")
		return nil
	default:
		return fmt.Errorf("unknown server mode: %s", app.Config.Server.Mode)
	}
}

// Stop 停止应用（优雅关闭所有组件）
func (app *Application) Stop(ctx context.Context) error {
	// 0. 关闭 MCP 服务器连接
	if app.MCPManager != nil {
		app.MCPManager.Shutdown()
	}

	// 1. 停止任务队列（等待运行中任务完成）
	if app.TaskQueue != nil {
		if err := app.TaskQueue.Stop(ctx); err != nil {
			slog.Error("Failed to stop task queue", "error", err)
		}
	}

	// 2. 停止所有渠道
	if app.ChannelRegistry != nil {
		if err := app.ChannelRegistry.StopAll(ctx); err != nil {
			slog.Error("Failed to stop channels", "error", err)
		}
	}

	// 3. 停止API服务器
	if app.APIServer != nil {
		if err := app.APIServer.Stop(ctx); err != nil {
			return err
		}
	}
	return nil
}

// InitLogger 初始化日志
func InitLogger(level, format string) {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: logLevel}

	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}

package infra

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"openaide/backend/internal/api"
	"openaide/backend/internal/auth"
	"openaide/backend/internal/compress"
	"openaide/backend/internal/config"
	"openaide/backend/internal/event"
	"openaide/backend/internal/feedback"
	"openaide/backend/internal/identity"
	"openaide/backend/internal/kernel"
	"openaide/backend/internal/knowledge"
	"openaide/backend/internal/mcp"
	"openaide/backend/internal/llm"
	"openaide/backend/internal/memory"
	"openaide/backend/internal/orchestration"
	"openaide/backend/internal/plugin"
	"openaide/backend/internal/tools"
)

// Application 应用容器
type Application struct {
	Config       *config.Config
	Kernel       kernel.Kernel
	Orchestrator *orchestration.Orchestrator
	APIServer    *api.Server
	LLMGateway   *llm.Gateway
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

	// 2. 创建工具注册表
	toolRegistry := tools.NewRegistry()
	if err := tools.RegisterBuiltins(toolRegistry); err != nil {
		return nil, fmt.Errorf("register builtin tools failed: %w", err)
	}

	// 3. 创建记忆管理器
	memManager, err := memory.NewFileMemory(cfg.Memory.DataDir)
	if err != nil {
		slog.Warn("Failed to create memory manager", "error", err)
		memManager = nil
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

	// 接入增强能力 — Reflection、Learner、PatternDetector
	agentKernel.SetReflection(kernel.NewSimpleReflection())
	if learner, err := kernel.NewSimpleLearner(cfg.Memory.DataDir); err == nil {
		agentKernel.SetLearner(learner)
	}
	agentKernel.SetPatternDetector(kernel.NewSimplePatternDetector())
	agentKernel.SetSkillManager(kernel.NewSkillManager(cfg.Storage.DataDir + "/skills"))
	agentKernel.SetApprover(kernel.NewAutoApprover())
	agentKernel.SetAdaptiveRounds(kernel.NewAdaptiveRounds(5, 30))

	// 接入插件管理器
	mcpMgr := mcp.NewManager()
	pluginMgr := plugin.NewManager(cfg.Storage.DataDir + "/plugins")
	_ = mcpMgr
	pluginPrompt := pluginMgr.GetPrompt()
	if pluginPrompt != "" {
		kernelConfig.SystemPrompt += "\n\n" + pluginPrompt
	}

	// 接入知识库 + 质量门控
	kb, err := knowledge.NewBase(cfg.Storage.DataDir + "/knowledge")
	if err == nil {
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
	agentKernel.SetContextCompressor(compress.NewNovelCompressor())
	_ = eventBus // 事件总线已在后台运行

	// 6. 创建编排器
	orch := orchestration.NewOrchestrator(agentKernel, gateway, toolRegistry, memManager, sessionStore)
	if kb != nil {
		orch.SetKnowledgeCollector(kb)
	}
	app.Orchestrator = orch

	// 7. 创建 API 服务器
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	authSvc := auth.NewService(os.Getenv("OPENAIDE_JWT_SECRET"))
	app.APIServer = api.NewServer(orch, addr, authSvc)

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

// Stop 停止应用
func (app *Application) Stop(ctx context.Context) error {
	if app.APIServer != nil {
		return app.APIServer.Stop(ctx)
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

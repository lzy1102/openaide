package main

import (
	"log"
	"time"

	"openaide/backend/src/config"
	"openaide/backend/src/handlers"
	"openaide/backend/src/models"
	"openaide/backend/src/services"
	"openaide/backend/src/services/mcp"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Application 封装所有应用组件，替代 main.go 中的全局变量
type Application struct {
	// 基础设施
	DB     *gorm.DB
	Config *config.Config

	// 核心服务
	CacheService        *services.CacheService
	LoggerService       *services.LoggerService
	ModelService        *services.ModelService
	DialogueService     *services.DialogueService
	WorkflowService     *services.WorkflowService
	SkillService        *services.SkillService
	PluginService       *services.PluginService
	AutomationService   *services.AutomationService
	ThinkingService     *services.ThinkingService
	CodeService         *services.CodeService
	ConfirmationService *services.ConfirmationService
	FeedbackService     *services.FeedbackService

	// 记忆服务
	MemoryRegistry         *services.MemoryProviderRegistry
	MemoryService          *services.MemoryService
	MemoryEmbeddingService *services.MemoryEmbeddingService

	// 学习与服务
	LearningService *services.LearningService
	TaskService     *services.TaskService

	// 工具与权限
	MCPManager         *mcp.MCPManager
	ToolService        *services.ToolService
	EventBus           *services.EventBus
	PermissionService  *services.PermissionService
	ToolCallingService *services.ToolCallingService
	TaskTool           *services.TaskTool

	// Agent 与路由
	AgentRouter   *services.AgentRouter
	SlashRegistry *services.SlashCommandRegistry
	AgentExecutor *services.AgentExecutor
	ModelRouter   *services.ModelRouter

	// 编排与规划
	OrchestrationService *services.OrchestrationService
	StructuredPlanner    *services.StructuredPlanner
	PlanReviewService    *services.PlanReviewService
	ReplanningEngine     *services.ReplanningEngine
	PlanService          *services.PlanService

	// 其他服务
	MultiAgentService       *services.MultiAgentService
	VoiceService            *services.VoiceService
	SandboxService          *services.SandboxService
	ChannelRegistry         *services.ChannelRegistry
	PromptTemplateService   *services.PromptTemplateService
	UsageService            *services.UsageService
	TokenEstimator          *services.TokenEstimator
	TokenLimitService       *services.TokenLimitService
	CostOptimizer           *services.CostOptimizer
	EmbeddingService        services.EmbeddingService
	VectorService           services.VectorService
	KnowledgeService        services.KnowledgeService
	DocumentService         services.DocumentService
	RAGService              services.RAGService
	ContextManager          services.ContextManager
	ExtractionService       services.KnowledgeExtractionService
	PromptService           *services.PromptService
	SelfReflectionService   *services.SelfReflectionService
	PatternDetectorService  *services.PatternDetectorService
	SkillEvolutionService   *services.SkillEvolutionService
	CapabilityGapService    *services.CapabilityGapService
	PostHookService         *services.PostHookService
	EnhancedDialogueService *services.EnhancedDialogueService
	LocalKnowledgeFirst     *services.LocalKnowledgeFirst

	// 认证与通信
	AuthService      *services.AuthService
	WebSocketService *services.WebSocketService
	SchedulerService *services.SchedulerService
	RateLimitService *services.RateLimitService

	// 外部集成
	FeishuService *services.FeishuService

	// Handler
	AuthHandler           *handlers.AuthHandler
	WebSocketHandler      *handlers.WebSocketHandler
	RateLimitHandler      *handlers.RateLimitHandler
	LearningHandler       *handlers.LearningHandler
	TaskHandler           *handlers.TaskHandler
	KnowledgeHandler      *handlers.KnowledgeHandler
	ContextHandler        *handlers.ContextHandler
	ExtractionHandler     *handlers.ExtractionHandler
	ToolHandler           *handlers.ToolHandler
	PromptTemplateHandler *handlers.PromptTemplateHandler
	UsageHandler          *handlers.UsageHandler
	SchedulerHandler      *handlers.SchedulerHandler
	FeishuHandler         *handlers.FeishuHandler
	PluginHandler         *handlers.PluginHandler
	AutomationHandler     *handlers.AutomationHandler
	CodeHandler           *handlers.CodeHandler
	ConfirmationHandler   *handlers.ConfirmationHandler
	ThinkingHandler       *handlers.ThinkingHandler
	SandboxHandler        *handlers.SandboxHandler
	ChannelHandler        *handlers.ChannelHandler
	MultiAgentHandler     *handlers.MultiAgentHandler
	VoiceHandler          *handlers.VoiceHandler
	OrchestrationHandler  *handlers.OrchestrationHandler
	MCPHandler            *handlers.MCPHandler
	SkillHandler          *handlers.SkillHandler
	SkillImportService    *services.SkillImportService

	// 后台服务
	MemoryExtractionService   *services.MemoryExtractionService
	SkillDiscoveryService     *services.SkillDiscoveryService
	PatternDetector           *services.PatternDetector
	UserFeedbackCollector     *services.UserFeedbackCollector
	ActivityTracker           *services.ActivityTracker
}

// NewApplication 创建并初始化应用
func NewApplication() (*Application, error) {
	app := &Application{}

	if err := app.initInfrastructure(); err != nil {
		return nil, err
	}
	if err := app.initCoreServices(); err != nil {
		return nil, err
	}
	if err := app.initMemoryServices(); err != nil {
		return nil, err
	}
	if err := app.initToolServices(); err != nil {
		return nil, err
	}
	if err := app.initAgentServices(); err != nil {
		return nil, err
	}
	if err := app.initOrchestrationServices(); err != nil {
		return nil, err
	}
	if err := app.initAdvancedServices(); err != nil {
		return nil, err
	}
	if err := app.initKnowledgeServices(); err != nil {
		return nil, err
	}
	if err := app.initEvolutionServices(); err != nil {
		return nil, err
	}
	if err := app.initCommunicationServices(); err != nil {
		return nil, err
	}
	if err := app.initExternalServices(); err != nil {
		return nil, err
	}
	if err := app.initHandlers(); err != nil {
		return nil, err
	}
	if err := app.initBackgroundServices(); err != nil {
		return nil, err
	}
	if err := app.wireDependencies(); err != nil {
		return nil, err
	}

	return app, nil
}

// Close 清理资源
func (app *Application) Close() {
	if app.LoggerService != nil {
		app.LoggerService.Close()
	}
	if app.VectorService != nil {
		if vm, ok := app.VectorService.(*services.VectorManager); ok && vm != nil {
			vm.Close()
		}
	}
	if app.SchedulerService != nil {
		app.SchedulerService.Stop()
	}
	if app.FeishuService != nil {
		app.FeishuService.Stop()
	}
}

// initInfrastructure 初始化数据库和配置
func (app *Application) initInfrastructure() error {
	// 初始化数据目录
	if err := config.DefaultPaths.EnsureDirs(); err != nil {
		log.Fatalf("Failed to create data directories: %v", err)
	}
	log.Printf("Data directory: %s", config.DefaultPaths.HomeDir)

	// 加载配置文件
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	app.Config = cfg
	log.Printf("Config loaded from: %s", config.GetConfigPath())

	// 初始化数据库
	dbPath := config.DefaultPaths.GetDBPath("openaide")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	app.DB = db

	// 自动迁移
	if err := db.AutoMigrate(
		&models.User{}, &models.APIKey{}, &models.UserSession{}, &models.Role{},
		&models.Dialogue{}, &models.Message{},
		&models.Workflow{}, &models.WorkflowStep{}, &models.WorkflowInstance{}, &models.StepInstance{},
		&models.Skill{}, &models.SkillParameter{}, &models.SkillExecution{},
		&models.Plugin{}, &models.PluginInstance{}, &models.PluginExecution{},
		&models.ModelInstance{}, &models.ModelExecution{},
		&models.AutomationExecution{}, &models.CodeExecution{}, &models.Confirmation{},
		&models.Thought{}, &models.Correction{}, &models.Feedback{}, &models.Memory{},
		&models.LearningRecord{}, &models.UserPreference{}, &models.PromptOptimization{}, &models.WorkflowOptimization{},
		&models.InteractionRecord{}, &models.LearningMetrics{},
		&models.Knowledge{}, &models.KnowledgeCategory{}, &models.KnowledgeTag{}, &models.KnowledgeTagRelation{}, &models.Document{},
		&models.Task{}, &models.Subtask{}, &models.TaskContext{}, &models.TaskDependency{}, &models.SubtaskDependency{},
		&models.TaskResult{}, &models.TeamMember{}, &models.TaskAssignment{}, &models.TaskDecomposition{},
		&models.TaskProgress{}, &models.TaskSummary{}, &models.TaskStatusUpdate{},
		&models.Tool{}, &models.ToolExecution{},
		&models.PromptTemplate{}, &models.PromptInstance{},
		&models.UsageRecord{}, &models.DailyUsage{}, &models.MonthlyUsage{}, &models.ModelPricing{}, &models.UserBudget{},
		&models.ScheduledTask{}, &models.TaskExecution{}, &models.TaskReminder{},
		&models.FeishuUser{}, &models.FeishuSession{}, &models.FeishuMessageLog{},
		&models.Event{},
		&models.ShortTermMemory{}, &models.MCPServer{}, &models.SelfReflection{}, &models.RepetitivePattern{},
		&models.SkillEvolution{}, &models.CapabilityGap{}, &models.EvolutionMetrics{},
		&models.CompressedContext{},
		&models.OrchestrationRecord{}, &models.SubtaskExecutionRecord{},
	); err != nil {
		log.Printf("AutoMigrate warning: %v", err)
	}

	memoryIndexService := services.NewMemoryIndexService(db)
	if err := memoryIndexService.CreateIndexes(); err != nil {
		log.Printf("Memory index creation warning: %v", err)
	}

	return nil
}

// initCoreServices 初始化核心服务
func (app *Application) initCoreServices() error {
	app.CacheService = services.NewCacheService()

	loggerService, err := services.NewLoggerService(services.LogLevelInfo, "")
	if err != nil {
		return err
	}
	app.LoggerService = loggerService

	app.ModelService = services.NewModelService(app.Config, app.CacheService, app.DB)
	app.DialogueService = services.NewDialogueService(app.DB, app.ModelService, app.LoggerService)
	app.WorkflowService = services.NewWorkflowService(app.DB, app.ModelService.GetLLMClient())
	app.SkillService = services.NewSkillService(app.DB, app.ModelService, app.LoggerService)
	app.SkillService.InitBuiltinSkills()
	app.PluginService = services.NewPluginService(app.DB, app.CacheService)
	app.AutomationService = services.NewAutomationService(app.DB)
	app.ThinkingService = services.NewThinkingService(app.DB, app.ModelService.GetLLMClient())
	app.CodeService = services.NewCodeService(app.DB, app.ModelService.GetLLMClient(), app.ThinkingService)
	app.ConfirmationService = services.NewConfirmationService(app.DB)
	app.FeedbackService = services.NewFeedbackService(app.DB)

	return nil
}

// initMemoryServices 初始化记忆服务
func (app *Application) initMemoryServices() error {
	app.MemoryRegistry = services.NewMemoryProviderRegistry()
	app.MemoryRegistry.Register("gorm", func() services.MemoryProvider {
		return services.NewGormMemoryProvider(app.DB)
	})
	app.MemoryRegistry.SetActiveProvider(services.NewGormMemoryProvider(app.DB))
	memoryStore := app.MemoryRegistry.GetActiveProvider()
	app.MemoryService = services.NewMemoryServiceWithStore(app.DB, memoryStore, app.CacheService)

	return nil
}

// initToolServices 初始化工具与权限服务
func (app *Application) initToolServices() error {
	app.MCPManager = mcp.NewMCPManager(app.DB, app.LoggerService)
	app.ToolService = services.NewToolService(app.DB, app.CacheService, app.LoggerService, app.MCPManager)
	app.EventBus = services.NewEventBus(app.DB, app.LoggerService, true)
	app.PermissionService = services.NewPermissionService(app.ToolService, app.EventBus)
	app.ToolCallingService = services.NewToolCallingService(app.ToolService, app.ModelService, app.LoggerService)
	app.ToolCallingService.SetEventBus(app.EventBus)
	app.ToolCallingService.SetDialogueService(app.DialogueService)

	app.TaskTool = services.NewTaskTool(app.ToolService, app.ModelService, app.PermissionService, app.DialogueService, app.EventBus)
	app.ToolService.RegisterSelfRegisteringTool(app.TaskTool)

	return nil
}

// initAgentServices 初始化 Agent 相关服务
func (app *Application) initAgentServices() error {
	app.AgentRouter = services.NewAgentRouter(app.ModelService)
	app.SlashRegistry = services.NewSlashCommandRegistry()
	app.SlashRegistry.SetDialogueService(app.DialogueService)
	app.SlashRegistry.SetToolService(app.ToolService)
	app.SlashRegistry.SetModelService(app.ModelService)
	app.SlashRegistry.SetAgentRouter(app.AgentRouter)

	app.AgentExecutor = services.NewAgentExecutor(app.ModelService, app.ToolService, app.LoggerService)
	app.ModelRouter = services.NewModelRouter(app.ModelService, app.LoggerService)

	return nil
}

// initOrchestrationServices 初始化编排与规划服务
func (app *Application) initOrchestrationServices() error {
	app.OrchestrationService = services.NewOrchestrationServiceWithModel(app.DB, app.ModelService, nil, app.AgentExecutor)
	app.OrchestrationService.SetToolCallingService(app.ToolCallingService)
	app.OrchestrationService.SetDialogueService(app.DialogueService)

	app.StructuredPlanner = services.NewStructuredPlanner(app.ModelService.GetLLMClient(), "", app.MemoryService, app.SkillService)
	app.OrchestrationService.SetStructuredPlanner(app.StructuredPlanner)

	app.PlanReviewService = services.NewPlanReviewService(app.ModelService.GetLLMClient(), "")
	app.OrchestrationService.SetPlanReview(app.PlanReviewService)

	app.ReplanningEngine = services.NewReplanningEngine(app.ModelService.GetLLMClient(), "", app.StructuredPlanner)
	app.OrchestrationService.SetReplanningEngine(app.ReplanningEngine)

	app.PlanService = services.NewPlanService(app.DB, app.OrchestrationService, app.ModelService, app.ModelRouter, app.LoggerService)

	return nil
}

// initAdvancedServices 初始化高级服务
func (app *Application) initAdvancedServices() error {
	app.MultiAgentService = services.NewMultiAgentService(app.ModelService, app.ModelRouter, app.LoggerService)
	app.VoiceService = services.NewVoiceService(services.VoiceConfig{})
	app.SandboxService = services.NewSandboxService(services.SandboxConfig{})
	app.ChannelRegistry = services.NewChannelRegistry()
	app.ChannelRegistry.Register(services.NewAPIChannel())
	app.PromptTemplateService = services.NewPromptTemplateService(app.DB, app.CacheService, app.LoggerService)
	app.UsageService = services.NewUsageService(app.DB, app.CacheService, app.LoggerService)
	app.TokenEstimator = services.NewTokenEstimator()
	app.TokenLimitService = services.NewTokenLimitService(app.DB, app.UsageService, app.LoggerService)
	app.CostOptimizer = services.NewCostOptimizer(app.ModelService, app.UsageService, app.LoggerService)

	return nil
}

// initKnowledgeServices 初始化知识库服务
func (app *Application) initKnowledgeServices() error {
	app.EmbeddingService = services.NewOpenAIEmbeddingService("", "", "", app.CacheService)

	vectorManager, err := services.NewVectorManager(config.DefaultPaths.VectorDir, app.EmbeddingService)
	if err != nil {
		log.Printf("Failed to initialize vector manager: %v", err)
	}
	if vectorManager != nil {
		app.VectorService = vectorManager
	} else {
		app.VectorService = services.NewNoopVectorService()
	}

	app.KnowledgeService = services.NewKnowledgeService(app.DB, app.EmbeddingService, app.VectorService, app.CacheService)
	app.DocumentService = services.NewDocumentService(app.DB, app.EmbeddingService, app.KnowledgeService, app.CacheService)
	app.RAGService = services.NewRAGService(app.KnowledgeService, app.ModelService.GetLLMClient(), app.CacheService)

	_ = services.NewDefaultContextEngine(app.DB, app.DialogueService, app.CacheService, app.LoggerService, services.DefaultCompressionConfig, true)
	app.ContextManager = services.NewContextManager(app.DB, app.DialogueService, app.CacheService, app.LoggerService, 100, 4000, 24*time.Hour, true)
	app.ExtractionService = services.NewKnowledgeExtractionService(app.DB, app.ModelService.GetLLMClient(), app.KnowledgeService, app.DialogueService, app.LoggerService)
	app.PromptService = services.NewPromptService(app.DB, app.MemoryService, app.RAGService)

	return nil
}

// initEvolutionServices 初始化进化服务
func (app *Application) initEvolutionServices() error {
	app.SelfReflectionService = services.NewSelfReflectionService(app.DB, app.ModelService, app.SkillService, app.PromptService)
	app.PatternDetectorService = services.NewPatternDetectorService(app.DB, app.ModelService, app.SkillService)
	app.SkillEvolutionService = services.NewSkillEvolutionService(app.DB, app.ModelService, app.SkillService)
	app.CapabilityGapService = services.NewCapabilityGapService(app.DB, app.ModelService, app.SkillService)

	app.PostHookService = services.NewPostHookService(app.EventBus, app.ExtractionService, app.LearningService)
	app.PostHookService.SetEvolutionServices(app.SelfReflectionService, app.PatternDetectorService, app.SkillEvolutionService, app.CapabilityGapService)

	app.EnhancedDialogueService = services.NewEnhancedDialogueService(
		app.DialogueService, app.ModelService, app.CacheService, app.LoggerService,
		app.ToolCallingService, app.ModelRouter, app.PlanService, app.SkillService,
		app.EventBus, app.PromptService, app.PostHookService,
	)
	app.LocalKnowledgeFirst = services.NewLocalKnowledgeFirst(app.KnowledgeService, app.RAGService, app.TokenEstimator)
	app.EnhancedDialogueService.SetLocalKnowledge(app.LocalKnowledgeFirst)

	return nil
}

// initCommunicationServices 初始化认证、WebSocket、调度服务
func (app *Application) initCommunicationServices() error {
	app.AuthService = services.NewAuthService(app.DB, app.CacheService, nil)
	app.WebSocketService = services.NewWebSocketService(app.DialogueService, app.ModelService, app.TaskService, app.WorkflowService, nil)
	app.SchedulerService = services.NewSchedulerService(app.DB, app.LoggerService, app.WebSocketService, app.WorkflowService)
	app.RateLimitService = services.NewRateLimitService(services.RateLimitServiceConfig{
		IPRequestsPerMinute:     100,
		UserRequestsPerMinute:   200,
		APIKeyRequestsPerMinute: 500,
	})

	return nil
}

// initExternalServices 初始化外部集成服务
func (app *Application) initExternalServices() error {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Failed to load config, using defaults: %v", err)
		cfg = &config.Config{Models: []config.ModelConfig{}}
	}
	app.Config = cfg

	// 应用语音配置
	if cfg.Voice.Enabled {
		app.VoiceService = services.NewVoiceService(services.VoiceConfig{
			Enabled: cfg.Voice.Enabled, WhisperAPI: cfg.Voice.WhisperAPI, WhisperKey: cfg.Voice.WhisperKey,
			TTSAPI: cfg.Voice.TTSAPI, TTSKey: cfg.Voice.TTSKey, TTSVoice: cfg.Voice.TTSVoice, DefaultLang: cfg.Voice.DefaultLang,
		})
	}
	// 应用沙箱配置
	if cfg.Sandbox.Enabled {
		app.SandboxService = services.NewSandboxService(services.SandboxConfig{
			Enabled: cfg.Sandbox.Enabled, DockerImage: cfg.Sandbox.DockerImage, Timeout: cfg.Sandbox.Timeout, MaxMemoryMB: cfg.Sandbox.MaxMemoryMB,
		})
	}

	// 应用上下文引擎配置
	if cfg.Context.CompressionMode != "" {
		compressionMode := services.CompressionMode(cfg.Context.CompressionMode)
		_ = services.NewDefaultContextEngine(app.DB, app.DialogueService, app.CacheService, app.LoggerService, services.CompressionConfig{
			Mode:              compressionMode,
			MaxTokens:         cfg.Context.MaxTokens,
			KeepLastN:         cfg.Context.KeepLastN,
			PreserveToolCalls: cfg.Context.PreserveToolCalls,
			FallbackToSummary: cfg.Context.FallbackToSummary,
		}, cfg.Context.CompressionEnabled)
		log.Printf("[Hermes Agent] Context engine initialized with mode: %s", compressionMode)
	}

	// 初始化记忆向量嵌入服务
	app.initEmbeddingService(cfg)

	// 初始化飞书服务
	app.initFeishuService(cfg)

	return nil
}

// initEmbeddingService 初始化向量嵌入服务
func (app *Application) initEmbeddingService(cfg *config.Config) {
	if !cfg.Embedding.Enabled {
		return
	}
	var embeddingSvc services.EmbeddingService
	switch cfg.Embedding.Provider {
	case "ollama":
		embeddingSvc = services.NewOllamaEmbeddingService(cfg.Embedding.BaseURL, cfg.Embedding.Model, app.CacheService)
	default:
		embeddingSvc = services.NewOpenAIEmbeddingService(cfg.Embedding.APIKey, cfg.Embedding.BaseURL, cfg.Embedding.Model, app.CacheService)
	}
	app.MemoryEmbeddingService = services.NewMemoryEmbeddingService(app.DB, embeddingSvc, app.CacheService)
	app.MemoryService.SetEmbeddingService(app.MemoryEmbeddingService)
	log.Printf("[Memory] Semantic search enabled with embedding provider: %s", cfg.Embedding.Provider)
}

// initFeishuService 初始化飞书服务
func (app *Application) initFeishuService(cfg *config.Config) {
	feishuConfig := services.FeishuConfig{
		Enabled:        cfg.Feishu.Enabled,
		AppID:          cfg.Feishu.AppID,
		AppSecret:      cfg.Feishu.AppSecret,
		DefaultModel:   cfg.Feishu.DefaultModel,
		SystemPrompt:   cfg.Feishu.SystemPrompt,
		StreamInterval: cfg.Feishu.StreamInterval,
	}
	app.FeishuService = services.NewFeishuService(
		app.DB, app.EnhancedDialogueService, app.ModelService, app.SkillService,
		app.VoiceService, app.FeedbackService, app.CacheService, app.LoggerService, feishuConfig,
	)
	if feishuConfig.Enabled && feishuConfig.AppID != "" {
		if err := app.FeishuService.Start(); err != nil {
			log.Printf("Failed to start feishu service: %v", err)
		}
	}
}

// initHandlers 初始化所有 Handler
func (app *Application) initHandlers() error {
	app.AuthHandler = handlers.NewAuthHandler(app.AuthService)
	app.WebSocketHandler = handlers.NewWebSocketHandler(app.WebSocketService)
	app.RateLimitHandler = handlers.NewRateLimitHandler(app.RateLimitService)
	app.LearningHandler = handlers.NewLearningHandler(app.LearningService)
	app.TaskHandler = handlers.NewTaskHandler(app.TaskService)
	app.KnowledgeHandler = handlers.NewKnowledgeHandler(app.KnowledgeService, app.DocumentService, app.RAGService, app.LoggerService)
	app.ContextHandler = handlers.NewContextHandler(app.ContextManager, app.DialogueService, app.LoggerService)
	app.ExtractionHandler = handlers.NewExtractionHandler(app.ExtractionService, app.DialogueService, app.KnowledgeService, app.LoggerService)
	app.ToolHandler = handlers.NewToolHandler(app.ToolService)
	app.PromptTemplateHandler = handlers.NewPromptTemplateHandler(app.PromptTemplateService)
	app.UsageHandler = handlers.NewUsageHandler(app.UsageService)
	app.SchedulerHandler = handlers.NewSchedulerHandler(app.SchedulerService)
	app.FeishuHandler = handlers.NewFeishuHandler(app.FeishuService, app.LearningService, app.FeedbackService)
	app.PluginHandler = handlers.NewPluginHandler(app.PluginService)
	app.AutomationHandler = handlers.NewAutomationHandler(app.AutomationService)
	app.CodeHandler = handlers.NewCodeHandler(app.CodeService)
	app.ConfirmationHandler = handlers.NewConfirmationHandler(app.ConfirmationService)
	app.ThinkingHandler = handlers.NewThinkingHandler(app.ThinkingService)
	app.SandboxHandler = handlers.NewSandboxHandler(app.SandboxService)
	app.ChannelHandler = handlers.NewChannelHandler(app.ChannelRegistry)
	app.MultiAgentHandler = handlers.NewMultiAgentHandler(app.MultiAgentService)
	app.VoiceHandler = handlers.NewVoiceHandler(app.VoiceService)
	app.OrchestrationHandler = handlers.NewOrchestrationHandler(app.OrchestrationService)
	app.MCPHandler = handlers.NewMCPHandler(app.MCPManager)
	app.SkillHandler = handlers.NewSkillHandler(app.SkillService)
	app.SkillImportService = services.NewSkillImportService(app.DB, app.SkillService)
	app.SkillHandler.SetImportService(app.SkillImportService)

	return nil
}

// initBackgroundServices 初始化后台服务
func (app *Application) initBackgroundServices() error {
	app.MemoryExtractionService = services.NewMemoryExtractionService(app.DB, app.MemoryService, app.ModelService.GetLLMClient(), true)
	app.SkillDiscoveryService = services.NewSkillDiscoveryService(app.DB, app.SkillService, app.ModelService, app.MemoryService, true)
	app.PatternDetector = services.NewPatternDetector(app.DB, app.ModelService, true)
	app.UserFeedbackCollector = services.NewUserFeedbackCollector(app.DB, app.ModelService, app.SkillService, app.MemoryService, app.WebSocketService, true)

	// 初始化活动超时跟踪器
	activityTimeout := 30 * time.Minute
	if app.Config.ActivityTimeout != "" {
		if d, err := time.ParseDuration(app.Config.ActivityTimeout); err == nil {
			activityTimeout = d
		}
	}
	app.ActivityTracker = services.NewActivityTracker(activityTimeout, func(sessionID string) {
		log.Printf("[ActivityTracker] Session %s timed out due to inactivity", sessionID)
	})
	app.WebSocketService.ActivityTracker = app.ActivityTracker

	return nil
}

// wireDependencies 连接服务间的依赖关系
func (app *Application) wireDependencies() error {
	// 注入使用量统计服务
	app.DialogueService.SetUsageService(app.UsageService)
	app.ToolCallingService.SetUsageService(app.UsageService)
	app.SlashRegistry.SetUsageService(app.UsageService)

	// 注入缓存服务
	app.DialogueService.SetCacheService(app.CacheService)

	return nil
}

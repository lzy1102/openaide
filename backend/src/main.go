package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"openaide/backend/src/config"
	"openaide/backend/src/handlers"
	"openaide/backend/src/models"
	"openaide/backend/src/services"
	"openaide/backend/src/services/llm"
	"openaide/backend/src/services/mcp"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func main() {
	// 设置默认端口
	port := os.Getenv("PORT")
	if port == "" {
		port = "19375"
	}

	// 初始化数据目录
	if err := config.DefaultPaths.EnsureDirs(); err != nil {
		log.Fatalf("Failed to create data directories: %v", err)
	}
	log.Printf("Data directory: %s", config.DefaultPaths.HomeDir)

	// 初始化数据库
	dbPath := config.DefaultPaths.GetDBPath("openaide")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// 自动迁移
	if err := db.AutoMigrate(
		// 认证相关
		&models.User{},
		&models.APIKey{},
		&models.UserSession{},
		&models.Role{},
		// 对话相关
		&models.Dialogue{},
		&models.Message{},
		&models.Workflow{},
		&models.WorkflowStep{},
		&models.WorkflowInstance{},
		&models.StepInstance{},
		&models.Skill{},
		&models.SkillParameter{},
		&models.SkillExecution{},
		&models.Plugin{},
		&models.PluginInstance{},
		&models.PluginExecution{},
		&models.Model{},
		&models.ModelInstance{},
		&models.ModelExecution{},
		&models.AutomationExecution{},
		&models.CodeExecution{},
		&models.Confirmation{},
		&models.Thought{},
		&models.Correction{},
		&models.Feedback{},
		&models.Memory{},
		&models.LearningRecord{},
		&models.UserPreference{},
		&models.PromptOptimization{},
		&models.WorkflowOptimization{},
		&models.InteractionRecord{},
		&models.LearningMetrics{},
		// 知识库相关模型
		&models.Knowledge{},
		&models.KnowledgeCategory{},
		&models.KnowledgeTag{},
		&models.KnowledgeTagRelation{},
		&models.Document{},
		// 任务管理相关模型
		&models.Task{},
		&models.Subtask{},
		&models.TaskContext{},
		&models.TaskDependency{},
		&models.SubtaskDependency{},
		&models.TaskResult{},
		&models.TeamMember{},
		&models.TaskAssignment{},
		&models.TaskDecomposition{},
		&models.TaskProgress{},
		&models.TaskSummary{},
		&models.TaskStatusUpdate{},
		// 工具调用相关模型
		&models.Tool{},
		&models.ToolExecution{},
		// 提示词模板相关模型
		&models.PromptTemplate{},
		&models.PromptInstance{},
		// 使用量统计相关模型
		&models.UsageRecord{},
		&models.DailyUsage{},
		&models.MonthlyUsage{},
		&models.ModelPricing{},
		&models.UserBudget{},
		// 定时任务相关模型
		&models.ScheduledTask{},
		&models.TaskExecution{},
		&models.TaskReminder{},
		// 飞书相关模型
		&models.FeishuUser{},
		&models.FeishuSession{},
		&models.FeishuMessageLog{},
		// 事件相关模型
		&models.Event{},
		// 短期记忆模型
		&models.ShortTermMemory{},
		&models.MCPServer{},
		&models.SelfReflection{},
		&models.RepetitivePattern{},
		&models.SkillEvolution{},
		&models.CapabilityGap{},
		&models.EvolutionMetrics{},
		// 上下文压缩模型 (Hermes Agent)
		&models.CompressedContext{},
	); err != nil {
		log.Printf("AutoMigrate warning: %v", err)
	}

	memoryIndexService := services.NewMemoryIndexService(db)
	if err := memoryIndexService.CreateIndexes(); err != nil {
		log.Printf("Memory index creation warning: %v", err)
	}

	// 初始化缓存服务
	cacheService := services.NewCacheService()

	// 初始化日志服务
	loggerService, err := services.NewLoggerService(services.LogLevelInfo, "")
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer loggerService.Close()

	// 初始化服务
	modelService := services.NewModelService(db, cacheService)
	dialogueService := services.NewDialogueService(db, modelService, loggerService)
	workflowService := services.NewWorkflowService(db, modelService.GetLLMClient())
	skillService := services.NewSkillService(db, modelService, loggerService)
	skillService.InitBuiltinSkills()
	pluginService := services.NewPluginService(db, cacheService)
	automationService := services.NewAutomationService(db)
	codeService := services.NewCodeService(db)
	confirmationService := services.NewConfirmationService(db)
	thinkingService := services.NewThinkingService(db, modelService.GetLLMClient())
	feedbackService := services.NewFeedbackService(db)
	memoryService := services.NewMemoryService(db, cacheService)
	
	// 初始化记忆向量嵌入服务（可选，需要配置 embedding 服务）
	var memoryEmbeddingSvc *services.MemoryEmbeddingService
	// 将在后面配置加载后初始化
	
	learningService := services.NewLearningService(db, modelService.GetLLMClient(), feedbackService, memoryService)
	taskService := services.NewTaskService(db, modelService.GetLLMClient())

	// 初始化工具服务
	mcpManager := mcp.NewMCPManager(db, loggerService)
	toolService := services.NewToolService(db, cacheService, loggerService, mcpManager)

	// 初始化工具调用服务
	toolCallingService := services.NewToolCallingService(toolService, modelService, loggerService)

	// 初始化 Agent 执行引擎
	agentExecutor := services.NewAgentExecutor(modelService, toolService, loggerService)

	// 初始化模型路由服务
	modelRouter := services.NewModelRouter(db, modelService, loggerService)

	// 初始化编排服务（用于任务规划）
	orchestrationService := services.NewOrchestrationServiceWithModel(db, modelService, nil, agentExecutor)

	// 初始化结构化规划引擎（深度理解 + 结构化规划 + 依赖分析 + 工具规划 + 回退策略）
	structuredPlanner := services.NewStructuredPlanner(modelService.GetLLMClient(), "", memoryService, skillService)
	orchestrationService.SetStructuredPlanner(structuredPlanner)

	// 初始化计划回顾服务（执行检查点、偏差检测、深度回顾）
	planReviewService := services.NewPlanReviewService(modelService.GetLLMClient(), "")
	orchestrationService.SetPlanReview(planReviewService)

	// 初始化动态重规划引擎（局部调整、完整重规划、降级方案）
	replanningEngine := services.NewReplanningEngine(modelService.GetLLMClient(), "", structuredPlanner)
	orchestrationService.SetReplanningEngine(replanningEngine)

	// 初始化规划服务
	planService := services.NewPlanService(db, orchestrationService, modelService, modelRouter, loggerService)

	// 初始化事件总线
	eventBus := services.NewEventBus(db, loggerService, true)
	toolCallingService.SetEventBus(eventBus)

	// 初始化多 Agent 协作服务
	multiAgentService := services.NewMultiAgentService(modelService, modelRouter, loggerService)

	// 初始化语音服务（配置稍后加载）
	voiceService := services.NewVoiceService(services.VoiceConfig{})

	// 初始化沙箱服务（配置稍后加载）
	sandboxService := services.NewSandboxService(services.SandboxConfig{})

	// 初始化多渠道服务
	channelRegistry := services.NewChannelRegistry()
	channelRegistry.Register(services.NewAPIChannel())
	// 初始化提示词模板服务
	promptTemplateService := services.NewPromptTemplateService(db, cacheService, loggerService)

	// 初始化使用量统计服务
	usageService := services.NewUsageService(db, cacheService, loggerService)

	// 将使用量统计服务注入到对话服务和工具调用服务
	dialogueService.SetUsageService(usageService)
	toolCallingService.SetUsageService(usageService)

	// 初始化智能缓存服务（注入到对话服务）
	dialogueService.SetCacheService(cacheService)

	// 初始化Token限制和告警服务
	tokenEstimator := services.NewTokenEstimator()
	tokenLimitService := services.NewTokenLimitService(db, usageService, loggerService)
	_ = tokenLimitService

	// 初始化成本优化服务
	costOptimizer := services.NewCostOptimizer(modelService, usageService, loggerService)
	_ = costOptimizer

	// 初始化知识库相关服务
	embeddingService := services.NewOpenAIEmbeddingService("", "", "", cacheService)
	vectorManager, err := services.NewVectorManager(config.DefaultPaths.VectorDir, embeddingService)
	if err != nil {
		log.Printf("Failed to initialize vector manager: %v", err)
	}
	if vectorManager != nil {
		defer vectorManager.Close()
	}
	var vectorSvc services.VectorService = vectorManager
	if vectorSvc == nil {
		vectorSvc = services.NewNoopVectorService()
	}
	knowledgeService := services.NewKnowledgeService(db, embeddingService, vectorSvc, cacheService)
	documentService := services.NewDocumentService(db, embeddingService, knowledgeService, cacheService)
	ragService := services.NewRAGService(knowledgeService, modelService.GetLLMClient(), cacheService)

	// 初始化上下文管理和知识提取服务
	_ = services.NewDefaultContextEngine(db, dialogueService, cacheService, loggerService, services.DefaultCompressionConfig, true)
	contextManager := services.NewContextManager(db, dialogueService, cacheService, loggerService, 100, 4000, 24*time.Hour, true)
	extractionService := services.NewKnowledgeExtractionService(db, modelService.GetLLMClient(), knowledgeService, dialogueService, loggerService)

	// 初始化提示词组装服务
	promptService := services.NewPromptService(db, memoryService, ragService)

	selfReflectionService := services.NewSelfReflectionService(db, modelService, skillService, promptService)
	patternDetectorService := services.NewPatternDetectorService(db, modelService, skillService)
	skillEvolutionService := services.NewSkillEvolutionService(db, modelService, skillService)
	capabilityGapService := services.NewCapabilityGapService(db, modelService, skillService)

	// 初始化后置钩子服务
	postHookService := services.NewPostHookService(eventBus, extractionService, learningService)
	postHookService.SetEvolutionServices(selfReflectionService, patternDetectorService, skillEvolutionService, capabilityGapService)

	// 初始化增强对话服务
	enhancedDialogueService := services.NewEnhancedDialogueService(
		dialogueService,
		modelService,
		cacheService,
		loggerService,
		toolCallingService,
		modelRouter,
		planService,
		skillService,
		eventBus,
		promptService,
		postHookService,
	)

	localKnowledgeFirst := services.NewLocalKnowledgeFirst(knowledgeService, ragService, tokenEstimator)
	enhancedDialogueService.SetLocalKnowledge(localKnowledgeFirst)

	// 初始化认证服务
	authService := services.NewAuthService(db, cacheService, nil)
	authHandler := handlers.NewAuthHandler(authService)

	// 初始化 WebSocket 服务
	wsService := services.NewWebSocketService(dialogueService, modelService, taskService, workflowService, nil)
	wsHandler := handlers.NewWebSocketHandler(wsService)

	// 初始化调度服务 (依赖 WebSocket 服务)
	schedulerService := services.NewSchedulerService(db, loggerService, wsService, workflowService)

	// 初始化限流服务
	rateLimitService := services.NewRateLimitService(services.RateLimitServiceConfig{
		IPRequestsPerMinute:     100,
		UserRequestsPerMinute:   200,
		APIKeyRequestsPerMinute: 500,
	})
	rateLimitHandler := handlers.NewRateLimitHandler(rateLimitService)

	// 确保调度服务在退出时停止
	defer schedulerService.Stop()

	// 初始化飞书服务
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Failed to load config, using defaults: %v", err)
		cfg = &config.Config{Models: []config.ModelConfig{}}
	}
	// 应用语音和沙箱配置
	if cfg.Voice.Enabled {
		voiceService = services.NewVoiceService(services.VoiceConfig{
			Enabled: cfg.Voice.Enabled, WhisperAPI: cfg.Voice.WhisperAPI, WhisperKey: cfg.Voice.WhisperKey,
			TTSAPI: cfg.Voice.TTSAPI, TTSKey: cfg.Voice.TTSKey, TTSVoice: cfg.Voice.TTSVoice, DefaultLang: cfg.Voice.DefaultLang,
		})
	}
	if cfg.Sandbox.Enabled {
		sandboxService = services.NewSandboxService(services.SandboxConfig{
			Enabled: cfg.Sandbox.Enabled, DockerImage: cfg.Sandbox.DockerImage, Timeout: cfg.Sandbox.Timeout, MaxMemoryMB: cfg.Sandbox.MaxMemoryMB,
		})
	}

	// 应用上下文引擎配置 (Hermes Agent)
	if cfg.Context.CompressionMode != "" {
		compressionMode := services.CompressionMode(cfg.Context.CompressionMode)
		_ = services.NewDefaultContextEngine(db, dialogueService, cacheService, loggerService, services.CompressionConfig{
			Mode:              compressionMode,
			MaxTokens:         cfg.Context.MaxTokens,
			KeepLastN:         cfg.Context.KeepLastN,
			PreserveToolCalls: cfg.Context.PreserveToolCalls,
			FallbackToSummary: cfg.Context.FallbackToSummary,
		}, cfg.Context.CompressionEnabled)
		log.Printf("[Hermes Agent] Context engine initialized with mode: %s", compressionMode)
	}

	// 初始化基于活动的超时跟踪器 (Hermes Agent 智能超时)
	activityTimeout := 30 * time.Minute
	if cfg.ActivityTimeout != "" {
		if d, err := time.ParseDuration(cfg.ActivityTimeout); err == nil {
			activityTimeout = d
		}
	}
	activityTracker := services.NewActivityTracker(activityTimeout, func(sessionID string) {
		log.Printf("[ActivityTracker] Session %s timed out due to inactivity", sessionID)
	})
	wsService.ActivityTracker = activityTracker

	// 同步配置文件中的模型到数据库
	if len(cfg.Models) > 0 {
		for _, modelConfig := range cfg.Models {
			// 检查模型是否已存在
			existingModels, _ := modelService.ListModels()
			modelExists := false
			for _, existingModel := range existingModels {
				if existingModel.Name == modelConfig.Name {
					modelExists = true
					break
				}
			}
			
			if !modelExists {
				// 创建新模型
				model := &models.Model{
					Name:        modelConfig.Name,
					Description: modelConfig.Description,
					Type:        modelConfig.Type,
					Provider:    modelConfig.Provider,
					Version:     modelConfig.Version,
					APIKey:      modelConfig.APIKey,
					BaseURL:     modelConfig.BaseURL,
					Config:      modelConfig.Config,
					Status:      modelConfig.Status,
					Priority:    0,
				}
				if err := modelService.CreateModel(model); err != nil {
					log.Printf("Failed to create model %s: %v", modelConfig.Name, err)
				} else {
					log.Printf("Created model from config: %s", modelConfig.Name)
				}
			}
		}
	}
	
	// 初始化记忆向量嵌入服务（配置加载后）
	if cfg.Embedding.Enabled {
		var embeddingSvc services.EmbeddingService
		switch cfg.Embedding.Provider {
		case "ollama":
			embeddingSvc = services.NewOllamaEmbeddingService(cfg.Embedding.BaseURL, cfg.Embedding.Model, cacheService)
		default:
			embeddingSvc = services.NewOpenAIEmbeddingService(cfg.Embedding.APIKey, cfg.Embedding.BaseURL, cfg.Embedding.Model, cacheService)
		}
		memoryEmbeddingSvc = services.NewMemoryEmbeddingService(db, embeddingSvc, cacheService)
	}
	
	// 初始化 Memory Provider 注册表 (Hermes Agent 插件化架构)
	memoryRegistry := services.NewMemoryProviderRegistry()
	memoryRegistry.Register("gorm", func() services.MemoryProvider {
		return services.NewGormMemoryProvider(db)
	})
	memoryRegistry.SetActiveProvider(services.NewGormMemoryProvider(db))
	
	// 使用 Memory Provider 替代原有 memoryService
	memoryStore := memoryRegistry.GetActiveProvider()
	memoryService = services.NewMemoryServiceWithStore(db, memoryStore, cacheService)
	
	// 将向量嵌入服务注入到 memoryService
	if memoryEmbeddingSvc != nil {
		memoryService.SetEmbeddingService(memoryEmbeddingSvc)
		log.Printf("[Memory] Semantic search enabled with embedding provider: %s", cfg.Embedding.Provider)
	}
	
	// 初始化自动记忆提取服务 (LLM 驱动)
	memoryExtractionSvc := services.NewMemoryExtractionService(db, memoryService, modelService.GetLLMClient(), true)
	go func() {
		time.Sleep(5 * time.Second)
		if err := memoryExtractionSvc.BatchExtractPendingDialogues("", 5); err != nil {
			log.Printf("[MemoryExtract] batch extraction failed: %v", err)
		}
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if err := memoryExtractionSvc.BatchExtractPendingDialogues("", 10); err != nil {
				log.Printf("[MemoryExtract] periodic extraction failed: %v", err)
			}
		}
	}()
	
	feishuConfig := services.FeishuConfig{
		Enabled:        cfg.Feishu.Enabled,
		AppID:          cfg.Feishu.AppID,
		AppSecret:      cfg.Feishu.AppSecret,
		DefaultModel:   cfg.Feishu.DefaultModel,
		SystemPrompt:   cfg.Feishu.SystemPrompt,
		StreamInterval: cfg.Feishu.StreamInterval,
	}
	feishuService := services.NewFeishuService(db, enhancedDialogueService, modelService, skillService, voiceService, feedbackService, cacheService, loggerService, feishuConfig)
	if feishuConfig.Enabled && feishuConfig.AppID != "" {
		if err := feishuService.Start(); err != nil {
			log.Printf("Failed to start feishu service: %v", err)
		}
	}
	defer feishuService.Stop()

	// 启动技能进化定时任务 (Hermes Agent 自我进化)
	go skillEvolutionService.RunPeriodicEvolution(context.Background())

	// 启动技能发现服务 (从对话中自动提炼新技能)
	skillDiscoverySvc := services.NewSkillDiscoveryService(db, skillService, modelService, memoryService, true)
	go func() {
		time.Sleep(30 * time.Second) // 等待系统稳定
		skillDiscoverySvc.RunPeriodicDiscovery(context.Background())
		ticker := time.NewTicker(6 * time.Hour) // 每 6 小时扫描一次
		defer ticker.Stop()
		for range ticker.C {
			skillDiscoverySvc.RunPeriodicDiscovery(context.Background())
		}
	}()

	// 启动模式检测器 (检测重复操作序列)
	patternDetector := services.NewPatternDetector(db, modelService, true)
	go func() {
		time.Sleep(60 * time.Second)
		patternDetector.RunPeriodicPatternDetection(context.Background())
		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			patternDetector.RunPeriodicPatternDetection(context.Background())
		}
	}()

	// 启动用户反馈收集器
	userFeedbackCollector := services.NewUserFeedbackCollector(db, modelService, skillService, memoryService, wsService, true)
	go func() {
		time.Sleep(90 * time.Second)
		userFeedbackCollector.RunPeriodicFeedbackAnalysis(context.Background())
		ticker := time.NewTicker(4 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			userFeedbackCollector.RunPeriodicFeedbackAnalysis(context.Background())
		}
	}()

	log.Printf("[Self-Evolution] Skill discovery, pattern detection, and feedback collection enabled")

	// ==================== 初始化所有 Handler ====================

	// 已有 handler
	learningHandler := handlers.NewLearningHandler(learningService)
	taskHandler := handlers.NewTaskHandler(taskService)
	knowledgeHandler := handlers.NewKnowledgeHandler(knowledgeService, documentService, ragService, loggerService)
	contextHandler := handlers.NewContextHandler(contextManager, dialogueService, loggerService)
	extractionHandler := handlers.NewExtractionHandler(extractionService, dialogueService, knowledgeService, loggerService)
	toolHandler := handlers.NewToolHandler(toolService)
	promptTemplateHandler := handlers.NewPromptTemplateHandler(promptTemplateService)
	usageHandler := handlers.NewUsageHandler(usageService)
	schedulerHandler := handlers.NewSchedulerHandler(schedulerService)
	feishuHandler := handlers.NewFeishuHandler(feishuService, learningService, feedbackService)

	// 新建 handler
	pluginHandler := handlers.NewPluginHandler(pluginService)
	automationHandler := handlers.NewAutomationHandler(automationService)
	codeHandler := handlers.NewCodeHandler(codeService)
	confirmationHandler := handlers.NewConfirmationHandler(confirmationService)
	thinkingHandler := handlers.NewThinkingHandler(thinkingService)
	sandboxHandler := handlers.NewSandboxHandler(sandboxService)
	channelHandler := handlers.NewChannelHandler(channelRegistry)
	multiAgentHandler := handlers.NewMultiAgentHandler(multiAgentService)
	voiceHandler := handlers.NewVoiceHandler(voiceService)
	orchestrationHandler := handlers.NewOrchestrationHandler(orchestrationService)

	// MCP 管理器
	mcpHandler := handlers.NewMCPHandler(mcpManager)

	// 技能管理器
	skillHandler := handlers.NewSkillHandler(skillService)
	
	// 技能导入服务
	skillImportService := services.NewSkillImportService(db, skillService)
	skillHandler.SetImportService(skillImportService)

	// ==================== 初始化 Gin 路由 ====================

	r := gin.Default()

	// 全局限流中间件
	r.Use(rateLimitHandler.RateLimitMiddleware())

	// 静态文件服务 - 提供前端文件
	frontendDir := os.Getenv("OPENAIDE_FRONTEND_DIR")
	if frontendDir == "" {
		execPath, _ := os.Executable()
		candidates := []string{
			filepath.Join(filepath.Dir(execPath), "frontend"),
			"/usr/share/openaide/frontend",
			"./frontend",
		}
		for _, dir := range candidates {
			if _, err := os.Stat(dir); err == nil {
				frontendDir = dir
				break
			}
		}
	}
	if frontendDir != "" {
		log.Printf("Frontend directory: %s", frontendDir)
		r.Static("/src", filepath.Join(frontendDir, "src"))
		r.Static("/public", filepath.Join(frontendDir, "public"))
		faviconPath := filepath.Join(frontendDir, "favicon.ico")
		if _, err := os.Stat(faviconPath); err == nil {
			r.StaticFile("/favicon.ico", faviconPath)
		}

		r.GET("/", func(c *gin.Context) {
			c.File(filepath.Join(frontendDir, "index.html"))
		})

		r.NoRoute(func(c *gin.Context) {
			path := c.Request.URL.Path
			filePath := filepath.Join(frontendDir, path)
			if _, err := os.Stat(filePath); err == nil {
				c.File(filePath)
				return
			}
			c.File(filepath.Join(frontendDir, "index.html"))
		})
	}

	// 健康检查接口
	r.GET("/health", func(c *gin.Context) {
		enabledModels, _ := modelService.ListEnabledModels()
		activeProvider := memoryRegistry.GetActiveProvider()
		services := map[string]interface{}{
			"models":      len(enabledModels),
			"voice":       voiceService.IsEnabled(),
			"sandbox":     sandboxService.IsEnabled() && sandboxService.IsDockerAvailable(),
			"channels":    len(channelRegistry.ListEnabled()),
			"event_bus":   true,
			"multi_agent": true,
			"skill_service": true,
			"plan_service":   true,
			"model_router":   true,
			"tool_calling":   true,
			"memory_provider": activeProvider.Name(),
			"context_engine":   "default",
		}
		c.JSON(http.StatusOK, gin.H{
			"status":   "ok",
			"message":  "OpenAIDE backend is running",
			"version":  "2.0",
			"services": services,
		})
	})

	// API路由组
	api := r.Group("/api")
	{
		// 认证接口 (无需认证)
		auth := api.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
			auth.POST("/refresh", authHandler.RefreshToken)
			auth.GET("/permissions", authHandler.GetPermissions)
		}

		// 需要认证的接口
		protected := api.Group("")
		protected.Use(authHandler.AuthMiddleware())
		{
			// 用户信息
			protected.GET("/profile", authHandler.GetProfile)
			protected.PUT("/profile", authHandler.UpdateProfile)
			protected.POST("/change-password", authHandler.ChangePassword)

			// 会话管理
			protected.GET("/sessions", authHandler.GetSessions)
			protected.DELETE("/sessions/:id", authHandler.LogoutSession)
			protected.POST("/logout", authHandler.Logout)

			// API Key 管理
			apiKeys := protected.Group("/api-keys")
			{
				apiKeys.GET("", authHandler.ListAPIKeys)
				apiKeys.POST("", authHandler.CreateAPIKey)
				apiKeys.DELETE("/:id", authHandler.RevokeAPIKey)
			}

			// 管理员接口
			admin := protected.Group("/admin")
			admin.Use(authHandler.AdminRequired())
			{
				admin.GET("/users", authHandler.ListUsers)
				admin.GET("/users/:id", authHandler.GetUser)
				admin.PUT("/users/:id", authHandler.UpdateUser)
				admin.DELETE("/users/:id", authHandler.DeleteUser)
			}
		}
	}
	{
		// 对话接口（保留内联 - 核心路由）
		dialogues := api.Group("/dialogues")
		{
			dialogues.GET("", func(c *gin.Context) {
				dialogues := dialogueService.ListDialogues()
				c.JSON(http.StatusOK, dialogues)
			})
			dialogues.GET("/user/:userID", func(c *gin.Context) {
				userID := c.Param("userID")
				dialogues := dialogueService.ListDialoguesByUser(userID)
				c.JSON(http.StatusOK, dialogues)
			})
			dialogues.POST("", func(c *gin.Context) {
				var req struct {
					UserID string `json:"user_id"`
					Title  string `json:"title"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				dialogue := dialogueService.CreateDialogue(req.UserID, req.Title)
				c.JSON(http.StatusOK, dialogue)
			})
			dialogues.GET("/:id", func(c *gin.Context) {
				id := c.Param("id")
				dialogue, found := dialogueService.GetDialogue(id)
				if !found {
					c.JSON(http.StatusNotFound, gin.H{"error": "Dialogue not found"})
					return
				}
				c.JSON(http.StatusOK, dialogue)
			})
			dialogues.PUT("/:id", func(c *gin.Context) {
				id := c.Param("id")
				var req struct {
					Title string `json:"title"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				dialogue, found := dialogueService.UpdateDialogue(id, req.Title)
				if !found {
					c.JSON(http.StatusNotFound, gin.H{"error": "Dialogue not found"})
					return
				}
				c.JSON(http.StatusOK, dialogue)
			})
			dialogues.DELETE("/:id", func(c *gin.Context) {
				id := c.Param("id")
				if err := dialogueService.DeleteDialogue(id); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Dialogue deleted successfully"})
			})
			dialogues.GET("/:id/messages", func(c *gin.Context) {
				id := c.Param("id")
				messages := dialogueService.GetMessages(id)
				c.JSON(http.StatusOK, messages)
			})
			dialogues.POST("/:id/messages", func(c *gin.Context) {
				id := c.Param("id")
				var req struct {
					UserID  string                 `json:"user_id"`
					Content string                 `json:"content"`
					ModelID string                 `json:"model_id"`
					Options map[string]interface{} `json:"options"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				ctx := c.Request.Context()
				message, err := enhancedDialogueService.SendMessage(ctx, id, req.UserID, req.Content, req.ModelID, req.Options)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, message)
			})
			dialogues.POST("/:id/stream", func(c *gin.Context) {
				id := c.Param("id")
				var req struct {
					UserID  string                 `json:"user_id"`
					Content string                 `json:"content"`
					ModelID string                 `json:"model_id"`
					Options map[string]interface{} `json:"options"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				ctx := c.Request.Context()
				chunkChan, err := enhancedDialogueService.SendMessageStream(ctx, id, req.UserID, req.Content, req.ModelID, req.Options)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}

				c.Header("Content-Type", "text/event-stream")
				c.Header("Cache-Control", "no-cache")
				c.Header("Connection", "keep-alive")
				c.Header("X-Accel-Buffering", "no")

				usedModel := ""
				for chunk := range chunkChan {
					if chunk.Error != nil {
						c.SSEvent("error", map[string]interface{}{"type": "error", "content": chunk.Error.Error()})
						return
					}
					if chunk.Model != "" {
						usedModel = chunk.Model
					}
					if len(chunk.Choices) > 0 {
						choice := chunk.Choices[0]
						delta := choice.Delta

						if delta.ReasoningContent != "" {
							c.SSEvent("message", map[string]interface{}{
								"type":    "thinking",
								"content": delta.ReasoningContent,
							})
						}

						if len(delta.ToolCalls) > 0 {
							for _, tc := range delta.ToolCalls {
								toolName := ""
								toolArgs := ""
								if tc.Function != nil {
									toolName = tc.Function.Name
									toolArgs = tc.Function.Arguments
								}
								c.SSEvent("message", map[string]interface{}{
									"type":   "tool_call",
									"tool":   toolName,
									"params": toolArgs,
								})
							}
						}

						if delta.Content != "" {
							c.SSEvent("message", map[string]interface{}{
								"type":    "content",
								"content": delta.Content,
							})
						}

						if choice.FinishReason != "" {
							c.SSEvent("message", map[string]interface{}{
								"type":  "done",
								"model": usedModel,
								"done":  true,
							})
						}
					}
				}
				c.SSEvent("message", map[string]interface{}{"type": "done", "model": usedModel, "done": true})
			})
			dialogues.DELETE("/:id/messages", func(c *gin.Context) {
				id := c.Param("id")
				if err := dialogueService.ClearMessages(id); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Messages cleared successfully"})
			})
		}

		// 工作流接口（保留内联 - 核心路由）
		workflows := api.Group("/workflows")
		{
			workflows.GET("", func(c *gin.Context) {
				workflows := workflowService.ListWorkflows()
				c.JSON(http.StatusOK, workflows)
			})
			workflows.POST("", func(c *gin.Context) {
				var req struct {
					Name        string                  `json:"name"`
					Description string                  `json:"description"`
					Steps       []models.WorkflowStep `json:"steps"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				workflow := workflowService.CreateWorkflow(req.Name, req.Description, req.Steps)
				c.JSON(http.StatusOK, workflow)
			})
			workflows.GET("/:id", func(c *gin.Context) {
				id := c.Param("id")
				workflow, found := workflowService.GetWorkflow(id)
				if !found {
					c.JSON(http.StatusNotFound, gin.H{"error": "Workflow not found"})
					return
				}
				c.JSON(http.StatusOK, workflow)
			})
			workflows.PUT("/:id", func(c *gin.Context) {
				id := c.Param("id")
				var req struct {
					Name        string                  `json:"name"`
					Description string                  `json:"description"`
					Steps       []models.WorkflowStep `json:"steps"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				workflow, found := workflowService.UpdateWorkflow(id, req.Name, req.Description, req.Steps)
				if !found {
					c.JSON(http.StatusNotFound, gin.H{"error": "Workflow not found"})
					return
				}
				c.JSON(http.StatusOK, workflow)
			})
			workflows.DELETE("/:id", func(c *gin.Context) {
				id := c.Param("id")
				if workflowService.DeleteWorkflow(id) {
					c.JSON(http.StatusOK, gin.H{"message": "Workflow deleted successfully"})
				} else {
					c.JSON(http.StatusNotFound, gin.H{"error": "Workflow not found"})
				}
			})
			workflows.POST("/:id/instances", func(c *gin.Context) {
				id := c.Param("id")
				var req struct {
					InputVariables map[string]interface{} `json:"input_variables"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					req.InputVariables = make(map[string]interface{})
				}
				instance, found := workflowService.CreateWorkflowInstance(id, req.InputVariables)
				if !found {
					c.JSON(http.StatusNotFound, gin.H{"error": "Workflow not found"})
					return
				}
				c.JSON(http.StatusOK, instance)
			})
			workflows.POST("/instances/:id/execute", func(c *gin.Context) {
				id := c.Param("id")
				instance, err := workflowService.ExecuteWorkflowInstance(id)
				if err != nil {
					c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, instance)
			})
		}

		// 技能接口 → skillHandler
		skillHandler.RegisterRoutes(api)

		// 插件接口 → pluginHandler		// 插件接口 → pluginHandler
		pluginHandler.RegisterRoutes(api)

		// 聊天接口（保留内联 - 核心路由）
		chat := api.Group("/chat")
		{
			chat.POST("/tools", func(c *gin.Context) {
				var req struct {
					ModelID    string                 `json:"model_id" binding:"required"`
					UserID     string                 `json:"user_id"`
					DialogueID string                 `json:"dialogue_id"`
					Content    string                 `json:"content" binding:"required"`
					Options    map[string]interface{} `json:"options"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}

				ctx := c.Request.Context()
				msg, err := enhancedDialogueService.SendMessageWithTools(ctx, req.DialogueID, req.UserID, req.Content, req.ModelID, req.Options)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, msg)
			})
			chat.POST("/route", func(c *gin.Context) {
				var req struct {
					UserID     string                 `json:"user_id"`
					DialogueID string                 `json:"dialogue_id"`
					Content    string                 `json:"content" binding:"required"`
					Options    map[string]interface{} `json:"options"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}

				ctx := c.Request.Context()
				chunkChan, err := enhancedDialogueService.SendMessageStreamRouted(ctx, req.DialogueID, req.UserID, req.Content, "", req.Options)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}

				c.Header("Content-Type", "text/event-stream")
				c.Header("Cache-Control", "no-cache")
				c.Header("Connection", "keep-alive")
				c.Header("X-Accel-Buffering", "no")

				usedModel := ""
				for chunk := range chunkChan {
					if chunk.Error != nil {
						c.SSEvent("error", map[string]interface{}{"type": "error", "content": chunk.Error.Error()})
						return
					}
					if chunk.Model != "" {
						usedModel = chunk.Model
					}
					if len(chunk.Choices) > 0 {
						choice := chunk.Choices[0]
						delta := choice.Delta

						if delta.ReasoningContent != "" {
							c.SSEvent("message", map[string]interface{}{
								"type":    "thinking",
								"content": delta.ReasoningContent,
							})
						}

						if len(delta.ToolCalls) > 0 {
							for _, tc := range delta.ToolCalls {
								toolName := ""
								toolArgs := ""
								if tc.Function != nil {
									toolName = tc.Function.Name
									toolArgs = tc.Function.Arguments
								}
								c.SSEvent("message", map[string]interface{}{
									"type":   "tool_call",
									"tool":   toolName,
									"params": toolArgs,
								})
							}
						}

						if delta.Content != "" {
							c.SSEvent("message", map[string]interface{}{
								"type":    "content",
								"content": delta.Content,
							})
						}

						if choice.FinishReason != "" {
							c.SSEvent("message", map[string]interface{}{
								"type":  "done",
								"model": usedModel,
								"done":  true,
							})
						}
					}
				}
				c.SSEvent("message", map[string]interface{}{"type": "done", "model": usedModel, "done": true})
			})
			chat.GET("/route-info", func(c *gin.Context) {
				content := c.Query("content")
				if content == "" {
					c.JSON(http.StatusBadRequest, gin.H{"error": "content parameter required"})
					return
				}
				info := modelRouter.GetRouteInfo(c.Request.Context(), content)
				c.JSON(http.StatusOK, info)
			})
			chat.POST("/plan", func(c *gin.Context) {
				var req struct {
					UserID     string                 `json:"user_id"`
					DialogueID string                 `json:"dialogue_id"`
					Content    string                 `json:"content" binding:"required"`
					ModelID    string                 `json:"model_id"`
					Options    map[string]interface{} `json:"options"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				ctx := c.Request.Context()
				result, err := enhancedDialogueService.SendMessageWithPlan(ctx, req.DialogueID, req.UserID, req.Content, req.ModelID, req.Options)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, result)
			})
			chat.POST("", func(c *gin.Context) {
				var req struct {
					ModelID  string                 `json:"model_id" binding:"required"`
					Messages []services.ChatMessage `json:"messages" binding:"required"`
					Options  map[string]interface{} `json:"options"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}

				llmMessages := make([]llm.Message, len(req.Messages))
				for i, msg := range req.Messages {
					llmMessages[i] = llm.Message{
						Role:    msg.Role,
						Content: msg.Content,
					}
				}

				resp, err := modelService.Chat(req.ModelID, llmMessages, req.Options)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, resp)
			})
			chat.POST("/stream", func(c *gin.Context) {
				var req struct {
					ModelID  string                 `json:"model_id" binding:"required"`
					Messages []services.ChatMessage `json:"messages" binding:"required"`
					Options  map[string]interface{} `json:"options"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}

				llmMessages := make([]llm.Message, len(req.Messages))
				for i, msg := range req.Messages {
					llmMessages[i] = llm.Message{
						Role:    msg.Role,
						Content: msg.Content,
					}
				}

				chunkChan, err := modelService.ChatStream(req.ModelID, llmMessages, req.Options)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}

				c.Header("Content-Type", "text/event-stream")
				c.Header("Cache-Control", "no-cache")
				c.Header("Connection", "keep-alive")
				c.Header("X-Accel-Buffering", "no")

				usedModel := ""
				for chunk := range chunkChan {
					if chunk.Error != nil {
						c.SSEvent("error", map[string]interface{}{"type": "error", "content": chunk.Error.Error()})
						return
					}
					if chunk.Model != "" {
						usedModel = chunk.Model
					}
					if len(chunk.Choices) > 0 {
						choice := chunk.Choices[0]
						delta := choice.Delta

						if delta.ReasoningContent != "" {
							c.SSEvent("message", map[string]interface{}{
								"type":    "thinking",
								"content": delta.ReasoningContent,
							})
						}

						if len(delta.ToolCalls) > 0 {
							for _, tc := range delta.ToolCalls {
								toolName := ""
								toolArgs := ""
								if tc.Function != nil {
									toolName = tc.Function.Name
									toolArgs = tc.Function.Arguments
								}
								c.SSEvent("message", map[string]interface{}{
									"type":   "tool_call",
									"tool":   toolName,
									"params": toolArgs,
								})
							}
						}

						if delta.Content != "" {
							c.SSEvent("message", map[string]interface{}{
								"type":    "content",
								"content": delta.Content,
							})
						}

						if choice.FinishReason != "" {
							c.SSEvent("message", map[string]interface{}{
								"type":  "done",
								"model": usedModel,
								"done":  true,
							})
						}
					}
				}
				c.SSEvent("message", map[string]interface{}{"type": "done", "model": usedModel, "done": true})
			})
		}

		// 规划接口（保留内联 - 核心路由）
		planGroup := api.Group("/plan")
		{
			planGroup.POST("/execute", func(c *gin.Context) {
				var req struct {
					SessionID string `json:"session_id" binding:"required"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				result, err := planService.ExecutePlan(c.Request.Context(), req.SessionID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, result)
			})
			planGroup.GET("/:sessionId", func(c *gin.Context) {
				sessionID := c.Param("sessionId")
				result, err := planService.GetPlanStatus(c.Request.Context(), sessionID)
				if err != nil {
					c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, result)
			})
			planGroup.POST("/cancel", func(c *gin.Context) {
				var req struct {
					SessionID string `json:"session_id" binding:"required"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				err = planService.CancelPlan(c.Request.Context(), req.SessionID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "plan cancelled"})
			})
		}

		// 多 Agent 协作接口 → multiAgentHandler
		multiAgentHandler.RegisterRoutes(api)

		// 语音接口 → voiceHandler
		voiceHandler.RegisterRoutes(api)

		// 沙箱接口 → sandboxHandler
		sandboxHandler.RegisterRoutes(api)

		// 渠道接口 → channelHandler
		channelHandler.RegisterRoutes(api)

		// 模型接口（保留内联 - 核心路由）
		modelGroup := api.Group("/models")
		{
			modelGroup.GET("", func(c *gin.Context) {
				models, err := modelService.ListModels()
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, models)
			})
			modelGroup.POST("", func(c *gin.Context) {
				var model models.Model
				if err := c.ShouldBindJSON(&model); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				err := modelService.CreateModel(&model)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, model)
			})
			modelGroup.GET("/:id", func(c *gin.Context) {
				id := c.Param("id")
				model, err := modelService.GetModel(id)
				if err != nil {
					c.JSON(http.StatusNotFound, gin.H{"error": "Model not found"})
					return
				}
				c.JSON(http.StatusOK, model)
			})
			modelGroup.PUT("/:id", func(c *gin.Context) {
				id := c.Param("id")
				var model models.Model
				if err := c.ShouldBindJSON(&model); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				model.ID = id
				err := modelService.UpdateModel(&model)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, model)
			})
			modelGroup.DELETE("/:id", func(c *gin.Context) {
				id := c.Param("id")
				err := modelService.DeleteModel(id)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Model deleted successfully"})
			})
			modelGroup.POST("/:id/enable", func(c *gin.Context) {
				id := c.Param("id")
				err := modelService.EnableModel(id)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Model enabled successfully"})
			})
			modelGroup.POST("/:id/disable", func(c *gin.Context) {
				id := c.Param("id")
				err := modelService.DisableModel(id)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Model disabled successfully"})
			})
			modelGroup.POST("/:id/instances", func(c *gin.Context) {
				id := c.Param("id")
				var req struct {
					Config map[string]interface{} `json:"config"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				instance, err := modelService.CreateModelInstance(id, req.Config)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, instance)
			})
			modelGroup.POST("/instances/:id/execute", func(c *gin.Context) {
				id := c.Param("id")
				var req struct {
					Parameters map[string]interface{} `json:"parameters"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				execution, err := modelService.ExecuteModel(id, req.Parameters)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, execution)
			})
		}

		// 自动化接口 → automationHandler
		automationHandler.RegisterRoutes(api)

		// 代码执行接口 → codeHandler
		codeHandler.RegisterRoutes(api)

		// 确认接口 → confirmationHandler
		confirmationHandler.RegisterRoutes(api)

		// 思考接口 → thinkingHandler
		thinkingHandler.RegisterRoutes(api)

		// 反馈接口（保留内联）
		feedback := api.Group("/feedback")
		{
			feedback.POST("", func(c *gin.Context) {
				var fb models.Feedback
				if err := c.ShouldBindJSON(&fb); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				err := feedbackService.CreateFeedback(&fb)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, fb)
			})
			feedback.GET("/task/:id", func(c *gin.Context) {
				id := c.Param("id")
				feedbacks, err := feedbackService.GetFeedbackByTask(id)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, feedbacks)
			})
			feedback.GET("/average/:type", func(c *gin.Context) {
				typeStr := c.Param("type")
				average, err := feedbackService.GetAverageRating(typeStr)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"average_rating": average})
			})
		}

		// 学习接口 → learningHandler
		learning := api.Group("/learning")
		{
			learning.Use(func(c *gin.Context) {
				c.Set("db", db)
				c.Next()
			})
			learning.POST("/learn", learningHandler.LearnFromFeedbackHandler)
			learning.GET("/preferences", learningHandler.GetUserPreferencesHandler)
			learning.POST("/preferences/learn", learningHandler.LearnUserPreferencesHandler)
			learning.POST("/workflows/:workflow_id/optimize", learningHandler.OptimizeWorkflowHandler)
			learning.POST("/evaluate", learningHandler.EvaluateLearningEffectHandler)
			learning.GET("/prompts/recommended", learningHandler.GetRecommendedPromptsHandler)
			learning.POST("/prompts/:optimization_id/apply", learningHandler.ApplyPromptOptimizationHandler)
			learning.POST("/interactions", learningHandler.RecordInteractionHandler)
			learning.GET("/insights", learningHandler.GetLearningInsightsHandler)
			learning.GET("/records", learningHandler.GetLearningRecordsHandler)
			learning.GET("/optimizations/prompts", learningHandler.GetPromptOptimizationsHandler)
			learning.GET("/optimizations/workflows", learningHandler.GetWorkflowOptimizationsHandler)
			learning.POST("/optimizations/workflows/:optimization_id/apply", learningHandler.ApplyWorkflowOptimizationHandler)
			learning.GET("/interactions/history", learningHandler.GetInteractionHistoryHandler)
		}

		evolution := api.Group("/evolution")
		{
			evolution.GET("/reflections", func(c *gin.Context) {
				dialogueID := c.Query("dialogue_id")
				limit := 20
				if l, err := strconv.Atoi(c.DefaultQuery("limit", "20")); err == nil && l > 0 {
					limit = l
				}
				if dialogueID != "" {
					reflections, err := selfReflectionService.GetReflectionsByDialogue(dialogueID)
					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
						return
					}
					c.JSON(http.StatusOK, reflections)
					return
				}
				reflections, err := selfReflectionService.GetRecentReflections(limit)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, reflections)
			})
			evolution.POST("/reflect", func(c *gin.Context) {
				var req struct {
					DialogueID string `json:"dialogue_id"`
					UserID     string `json:"user_id"`
					Query      string `json:"query"`
					Response   string `json:"response"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				reflection, err := selfReflectionService.ReflectOnResponse(c.Request.Context(), req.DialogueID, req.UserID, req.Query, req.Response)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, reflection)
			})
			evolution.POST("/reflections/:id/apply", func(c *gin.Context) {
				id := c.Param("id")
				if err := selfReflectionService.ApplyReflection(id); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Reflection applied successfully"})
			})
			evolution.GET("/quality-trend", func(c *gin.Context) {
				days := 7
				if d, err := strconv.Atoi(c.DefaultQuery("days", "7")); err == nil && d > 0 {
					days = d
				}
				trend, err := selfReflectionService.GetQualityTrend(days)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, trend)
			})
			evolution.GET("/patterns", func(c *gin.Context) {
				userID := c.Query("user_id")
				patternType := c.Query("type")
				patterns, err := patternDetectorService.GetPatterns(userID, patternType)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, patterns)
			})
			evolution.POST("/patterns/detect", func(c *gin.Context) {
				userID := c.Query("user_id")
				patterns, err := patternDetectorService.DetectPatterns(c.Request.Context(), userID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, patterns)
			})
			evolution.POST("/patterns/:id/create-skill", func(c *gin.Context) {
				id := c.Param("id")
				skill, err := patternDetectorService.CreateSkillFromPattern(c.Request.Context(), id)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Skill created from pattern", "skill": skill})
			})
			evolution.POST("/patterns/:id/ignore", func(c *gin.Context) {
				id := c.Param("id")
				if err := patternDetectorService.IgnorePattern(id); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Pattern ignored"})
			})
			evolution.GET("/skill-evolutions", func(c *gin.Context) {
				skillID := c.Query("skill_id")
				if skillID != "" {
					evolutions, err := skillEvolutionService.GetEvolutionHistory(skillID)
					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
						return
					}
					c.JSON(http.StatusOK, evolutions)
					return
				}
				evolutions, err := skillEvolutionService.GetPendingEvolutions()
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, evolutions)
			})
			evolution.POST("/skill-evolutions/:id/apply", func(c *gin.Context) {
				id := c.Param("id")
				if err := skillEvolutionService.ApplyEvolution(id); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Skill evolution applied successfully"})
			})
			evolution.POST("/skill-evolutions/:id/rollback", func(c *gin.Context) {
				id := c.Param("id")
				if err := skillEvolutionService.RollbackEvolution(id); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Skill evolution rolled back successfully"})
			})
			evolution.POST("/skills/:skill_id/evolve", func(c *gin.Context) {
				skillID := c.Param("skill_id")
				evolution, err := skillEvolutionService.EvolveSkillFromFeedback(c.Request.Context(), skillID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				if evolution == nil {
					c.JSON(http.StatusOK, gin.H{"message": "No evolution needed"})
					return
				}
				c.JSON(http.StatusOK, evolution)
			})
			evolution.GET("/gaps", func(c *gin.Context) {
				gapType := c.Query("type")
				severity := c.Query("severity")
				gaps, err := capabilityGapService.GetGaps(gapType, severity)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gaps)
			})
			evolution.POST("/gaps/detect", func(c *gin.Context) {
				userID := c.Query("user_id")
				gaps, err := capabilityGapService.DetectGaps(c.Request.Context(), userID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gaps)
			})
			evolution.POST("/gaps/:id/create-skill", func(c *gin.Context) {
				id := c.Param("id")
				skill, err := capabilityGapService.CreateSkillFromGap(c.Request.Context(), id)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Skill created from capability gap", "skill": skill})
			})
			evolution.POST("/gaps/:id/ignore", func(c *gin.Context) {
				id := c.Param("id")
				if err := capabilityGapService.IgnoreGap(id); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Gap ignored"})
			})
		}

		// 记忆接口（保留内联）
		memory := api.Group("/memory")
		{
			memory.POST("", func(c *gin.Context) {
				var mem models.Memory
				if err := c.ShouldBindJSON(&mem); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				err := memoryService.CreateMemory(&mem)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				
				// 自动向量化新记忆
				if memoryEmbeddingSvc != nil {
					go memoryEmbeddingSvc.AutoEmbedNewMemories(mem.ID)
				}
				
				c.JSON(http.StatusOK, mem)
			})
			memory.GET("/user/:id", func(c *gin.Context) {
				userID := c.Param("id")
				memories, err := memoryService.GetMemoriesByUser(userID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, memories)
			})
			memory.GET("/search", func(c *gin.Context) {
				userID := c.Query("user_id")
				keyword := c.Query("keyword")
				memories, err := memoryService.SearchMemories(userID, keyword)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, memories)
			})
			
			// 语义搜索
			memory.POST("/semantic-search", func(c *gin.Context) {
				if memoryEmbeddingSvc == nil {
					c.JSON(http.StatusServiceUnavailable, gin.H{"error": "semantic search not available"})
					return
				}
				
				var req struct {
					UserID string `json:"user_id" binding:"required"`
					Query  string `json:"query" binding:"required"`
					Limit  int    `json:"limit"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				
				if req.Limit <= 0 {
					req.Limit = 5
				}
				
				results, err := memoryEmbeddingSvc.SemanticSearch(c.Request.Context(), req.UserID, req.Query, req.Limit)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"data":    results,
					"count":   len(results),
				})
			})
			
			// 混合搜索（语义 + 关键词）
			memory.POST("/hybrid-search", func(c *gin.Context) {
				if memoryEmbeddingSvc == nil {
					c.JSON(http.StatusServiceUnavailable, gin.H{"error": "hybrid search not available"})
					return
				}
				
				var req struct {
					UserID         string  `json:"user_id" binding:"required"`
					Query          string  `json:"query" binding:"required"`
					Limit          int     `json:"limit"`
					SemanticWeight float64 `json:"semantic_weight"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				
				if req.Limit <= 0 {
					req.Limit = 5
				}
				
				results, err := memoryEmbeddingSvc.HybridSearch(
					c.Request.Context(),
					req.UserID,
					req.Query,
					req.Limit,
					req.SemanticWeight,
				)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"data":    results,
					"count":   len(results),
				})
			})
			
			// 批量向量化用户的记忆
			memory.POST("/batch-embed", func(c *gin.Context) {
				if memoryEmbeddingSvc == nil {
					c.JSON(http.StatusServiceUnavailable, gin.H{"error": "embedding service not available"})
					return
				}
				
				userID := c.Query("user_id")
				if userID == "" {
					c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
					return
				}
				
				count, err := memoryEmbeddingSvc.BatchEmbedUserMemories(userID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"message": fmt.Sprintf("Embedded %d memories", count),
					"count":   count,
				})
			})
			
			memory.PUT("/:id", func(c *gin.Context) {
				id := c.Param("id")
				var mem models.Memory
				if err := c.ShouldBindJSON(&mem); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				mem.ID = id
				err := memoryService.UpdateMemory(id, &mem)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				
				// 重新向量化
				if memoryEmbeddingSvc != nil {
					go memoryEmbeddingSvc.AutoEmbedNewMemories(id)
				}
				
				c.JSON(http.StatusOK, mem)
			})
			memory.DELETE("/:id", func(c *gin.Context) {
				id := c.Param("id")
				err := memoryService.DeleteMemory(id)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Memory deleted successfully"})
			})
			memory.POST("/adjust-priority", func(c *gin.Context) {
				err := memoryService.AdjustPriority()
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Memory priorities adjusted"})
			})
		}

		// 任务管理接口 → taskHandler
		tasks := api.Group("/tasks")
		{
			tasks.POST("", taskHandler.CreateTaskHandler)
			tasks.POST("/decompose", taskHandler.DecomposeTaskHandler)
			tasks.GET("", taskHandler.ListTasksHandler)
			tasks.GET("/:id", taskHandler.GetTaskHandler)
			tasks.PUT("/:id", taskHandler.UpdateTaskHandler)
			tasks.DELETE("/:id", taskHandler.DeleteTaskHandler)
			tasks.PATCH("/:id/status", taskHandler.UpdateTaskStatusHandler)
			tasks.PATCH("/subtasks/:id/status", taskHandler.UpdateSubtaskStatusHandler)
			tasks.POST("/progress", taskHandler.UpdateProgressHandler)
			tasks.GET("/:id/progress", taskHandler.GetTaskProgressHandler)
			tasks.GET("/overview", taskHandler.GetTaskOverviewHandler)
			tasks.POST("/:id/retry", taskHandler.RetryFailedTaskHandler)
			tasks.POST("/:id/summary", taskHandler.GenerateTaskSummaryHandler)
			tasks.POST("/:id/cancel", taskHandler.CancelTaskHandler)
			tasks.POST("/:id/reassign", taskHandler.ReassignTaskHandler)
			tasks.GET("/:id/can-start", taskHandler.CanStartTaskHandler)

			members := api.Group("/members")
			{
				members.POST("", taskHandler.CreateMemberHandler)
				members.GET("", taskHandler.ListMembersHandler)
				members.GET("/:id", taskHandler.GetMemberTasksHandler)
				members.PUT("/:id", taskHandler.UpdateMemberHandler)
			}

			templates := api.Group("/task-templates")
			{
				templates.GET("", taskHandler.ListTemplatesHandler)
				templates.GET("/:type", taskHandler.GetTemplateHandler)
			}
		}

		// 知识库和 RAG 接口 → knowledgeHandler
		knowledge := api.Group("/knowledge")
		{
			knowledge.GET("", knowledgeHandler.ListKnowledge)
			knowledge.POST("", knowledgeHandler.CreateKnowledge)
			knowledge.GET("/:id", knowledgeHandler.GetKnowledge)
			knowledge.PUT("/:id", knowledgeHandler.UpdateKnowledge)
			knowledge.DELETE("/:id", knowledgeHandler.DeleteKnowledge)
			knowledge.POST("/search", knowledgeHandler.SearchKnowledge)
			knowledge.POST("/hybrid-search", knowledgeHandler.HybridSearchKnowledge)
		}

		categories := api.Group("/knowledge/categories")
		{
			categories.GET("", knowledgeHandler.ListCategories)
			categories.POST("", knowledgeHandler.CreateCategory)
			categories.GET("/:id", knowledgeHandler.GetCategory)
			categories.DELETE("/:id", knowledgeHandler.DeleteCategory)
		}

		documents := api.Group("/documents")
		{
			documents.GET("", knowledgeHandler.ListDocuments)
			documents.POST("/import", knowledgeHandler.ImportDocument)
			documents.GET("/:id", knowledgeHandler.GetDocument)
			documents.DELETE("/:id", knowledgeHandler.DeleteDocument)
		}

		rag := api.Group("/rag")
		{
			rag.POST("/query", knowledgeHandler.RAGQuery)
			rag.POST("/stream", knowledgeHandler.RAGStream)
		}

		// 上下文管理接口 → contextHandler
		contextGroup := api.Group("/context")
		{
			contextGroup.POST("/compress", contextHandler.CompressContext)
			contextGroup.POST("/summarize", contextHandler.SummarizeContext)
			contextGroup.GET("/metrics", contextHandler.GetMetrics)
			contextGroup.DELETE("/expired", contextHandler.ClearExpired)
			contextGroup.GET("/dialogue/:id", contextHandler.GetDialogueContext)
		}

		// 知识提取接口 → extractionHandler
		extractionGroup := api.Group("/extraction")
		{
			extractionGroup.POST("/extract", extractionHandler.ExtractFromDialogue)
			extractionGroup.POST("/auto", extractionHandler.AutoExtract)
			extractionGroup.POST("/batch", extractionHandler.BatchExtract)
			extractionGroup.GET("/config", extractionHandler.GetConfig)
			extractionGroup.PUT("/config", extractionHandler.UpdateConfig)
		}

		// 工具调用接口 → toolHandler
		toolHandler.RegisterRoutes(api)

		// MCP 管理接口 → mcpHandler
		mcpHandler.RegisterRoutes(api)

		// 提示词模板接口 → promptTemplateHandler
		promptTemplateHandler.RegisterRoutes(api)

		// 使用量统计接口 → usageHandler
		usageHandler.RegisterRoutes(api)

		// 事件接口（保留内联）
		eventGroup := api.Group("/events")
		{
			eventGroup.GET("", func(c *gin.Context) {
				topic := c.Query("topic")
				events, err := eventBus.GetEvents(topic, 50)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, events)
			})
			eventGroup.GET("/stats", func(c *gin.Context) {
				stats, err := eventBus.GetStats()
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, stats)
			})
			eventGroup.GET("/:id", func(c *gin.Context) {
				id := c.Param("id")
				event, err := eventBus.GetEvent(id)
				if err != nil {
					c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
					return
				}
				c.JSON(http.StatusOK, event)
			})
			eventGroup.POST("/publish", func(c *gin.Context) {
				var req struct {
					Topic   string                 `json:"topic" binding:"required"`
					Type    string                 `json:"type" binding:"required"`
					Source  string                 `json:"source"`
					Data    map[string]interface{} `json:"data"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				eventBus.Publish(c.Request.Context(), req.Topic, req.Type, req.Source, req.Data)
				c.JSON(http.StatusOK, gin.H{"message": "event published"})
			})
		}

		// 定时任务接口 → schedulerHandler
		schedulerHandler.RegisterRoutes(api)

		// 智能编排接口 → orchestrationHandler
		orchestrationHandler.RegisterRoutes(api)

		// 飞书管理接口 → feishuHandler
		feishu := api.Group("/feishu")
		{
			feishu.GET("/status", feishuHandler.GetStatus)
			feishu.POST("/start", feishuHandler.Start)
			feishu.POST("/stop", feishuHandler.Stop)
			feishu.GET("/sessions", feishuHandler.ListSessions)
			feishu.DELETE("/session/:key", feishuHandler.DeleteSession)
			feishu.GET("/users", feishuHandler.ListUsers)
			feishu.POST("/card-callback", feishuHandler.HandleCardCallback)
		}

		// WebSocket 接口
		r.GET("/ws", wsHandler.HandleWebSocket)
		ws := api.Group("/ws")
		{
			ws.GET("/stats", wsHandler.HandleWebSocketStats)
			ws.POST("/broadcast", wsHandler.HandleBroadcast)
			ws.POST("/send/:user_id", wsHandler.HandleSendToUser)
			ws.POST("/notify/task/:id", wsHandler.HandleNotifyTask)
			ws.GET("/dialogue/stream", wsHandler.DialogueStreamHandler(dialogueService))
		}
	}

	// 启动服务器
	serverAddr := fmt.Sprintf(":%s", port)
	log.Printf("Server starting on %s", serverAddr)
	if err := r.Run(serverAddr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

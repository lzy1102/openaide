package main

import (
	"net/http"
	"strconv"
	"time"

	"openaide/backend/src/logger"
	"openaide/backend/src/middleware"
	"openaide/backend/src/models"
	"openaide/backend/src/response"
	"openaide/backend/src/services"
	"openaide/backend/src/services/llm"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Router 封装路由注册
type Router struct {
	app *Application
}

// NewRouter 创建路由器
func NewRouter(app *Application) *Router {
	return &Router{app: app}
}

// Register 注册所有路由
func (r *Router) Register(engine *gin.Engine) {
	// 请求ID中间件
	engine.Use(middleware.RequestIDMiddleware())

	// CORS 中间件
	engine.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-API-Key", "X-Request-ID"},
		ExposeHeaders:    []string{"Content-Length", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// 全局限流中间件
	engine.Use(r.app.RateLimitHandler.RateLimitMiddleware())

	// 健康检查接口（无需认证）
	r.registerHealth(engine)

	// 公开路由（无需认证）
	publicAPI := engine.Group("/api")
	{
		r.registerAuthRoutes(publicAPI)
	}

	// 受保护路由（需要认证）
	protectedAPI := engine.Group("/api")
	protectedAPI.Use(r.app.AuthHandler.AuthMiddleware())
	{
		r.registerDialogueRoutes(protectedAPI)
		r.registerWorkflowRoutes(protectedAPI)
		r.registerChatRoutes(protectedAPI)
		r.registerModelRoutes(protectedAPI)
		r.registerPlanRoutes(protectedAPI)
		r.registerFeedbackRoutes(protectedAPI)
		r.registerLearningRoutes(protectedAPI)
		r.registerEvolutionRoutes(protectedAPI)
		r.registerMemoryRoutes(protectedAPI)
		r.registerTaskRoutes(protectedAPI)
		r.registerKnowledgeRoutes(protectedAPI)
		r.registerContextRoutes(protectedAPI)
		r.registerExtractionRoutes(protectedAPI)
		r.registerPermissionRoutes(protectedAPI)
		r.registerAgentRoutingRoutes(protectedAPI)
		r.registerSlashCommandRoutes(protectedAPI)
		r.registerEventRoutes(protectedAPI)
		r.registerFeishuRoutes(protectedAPI)
		r.registerWebSocketRoutes(protectedAPI, engine)

		// Handler 注册的路由
		r.app.SkillHandler.RegisterRoutes(protectedAPI)
		r.app.PluginHandler.RegisterRoutes(protectedAPI)
		r.app.AutomationHandler.RegisterRoutes(protectedAPI)
		r.app.CodeHandler.RegisterRoutes(protectedAPI)
		r.app.ConfirmationHandler.RegisterRoutes(protectedAPI)
		r.app.ThinkingHandler.RegisterRoutes(protectedAPI)
		r.app.SandboxHandler.RegisterRoutes(protectedAPI)
		r.app.ChannelHandler.RegisterRoutes(protectedAPI)
		r.app.MultiAgentHandler.RegisterRoutes(protectedAPI)
		r.app.VoiceHandler.RegisterRoutes(protectedAPI)
		r.app.OrchestrationHandler.RegisterRoutes(protectedAPI)
		r.app.MCPHandler.RegisterRoutes(protectedAPI)
		r.app.ToolHandler.RegisterRoutes(protectedAPI)
		r.app.PromptTemplateHandler.RegisterRoutes(protectedAPI)
		r.app.UsageHandler.RegisterRoutes(protectedAPI)
		r.app.SchedulerHandler.RegisterRoutes(protectedAPI)
	}
}

// registerHealth 注册健康检查路由
func (r *Router) registerHealth(engine *gin.Engine) {
	engine.GET("/health", func(c *gin.Context) {
		dbStatus := "ok"
		dbErr := ""
		if r.app.DB != nil {
			sqlDB, err := r.app.DB.DB()
			if err != nil {
				dbStatus = "error"
				dbErr = err.Error()
			} else if err := sqlDB.Ping(); err != nil {
				dbStatus = "error"
				dbErr = err.Error()
			}
		} else {
			dbStatus = "not_configured"
		}

		enabledModels, _ := r.app.ModelService.ListEnabledModels()
		activeProvider := r.app.MemoryRegistry.GetActiveProvider()
		services := map[string]interface{}{
			"database":        dbStatus,
			"models":          len(enabledModels),
			"voice":           r.app.VoiceService.IsEnabled(),
			"sandbox":         r.app.SandboxService.IsEnabled() && r.app.SandboxService.IsDockerAvailable(),
			"channels":        len(r.app.ChannelRegistry.ListEnabled()),
			"event_bus":       true,
			"multi_agent":     true,
			"skill_service":   true,
			"plan_service":    true,
			"model_router":    true,
			"tool_calling":    true,
			"memory_provider": activeProvider.Name(),
			"context_engine":  "default",
		}
		if dbErr != "" {
			services["database_error"] = dbErr
		}

		httpStatus := http.StatusOK
		if dbStatus == "error" {
			httpStatus = http.StatusServiceUnavailable
		}

		c.JSON(httpStatus, response.Response{
			Code:    0,
			Message: "health_check",
			Data: gin.H{
				"version":  "2.0",
				"services": services,
			},
		})
	})
}

// registerAuthRoutes 注册认证路由
func (r *Router) registerAuthRoutes(api *gin.RouterGroup) {
	auth := api.Group("/auth")
	{
		auth.POST("/register", r.app.AuthHandler.Register)
		auth.POST("/login", r.app.AuthHandler.Login)
		auth.POST("/refresh", r.app.AuthHandler.RefreshToken)
		auth.GET("/permissions", r.app.AuthHandler.GetPermissions)
	}

	protected := api.Group("")
	protected.Use(r.app.AuthHandler.AuthMiddleware())
	{
		protected.GET("/profile", r.app.AuthHandler.GetProfile)
		protected.PUT("/profile", r.app.AuthHandler.UpdateProfile)
		protected.POST("/change-password", r.app.AuthHandler.ChangePassword)
		protected.GET("/sessions", r.app.AuthHandler.GetSessions)
		protected.DELETE("/sessions/:id", r.app.AuthHandler.LogoutSession)
		protected.POST("/logout", r.app.AuthHandler.Logout)

		apiKeys := protected.Group("/api-keys")
		{
			apiKeys.GET("", r.app.AuthHandler.ListAPIKeys)
			apiKeys.POST("", r.app.AuthHandler.CreateAPIKey)
			apiKeys.DELETE("/:id", r.app.AuthHandler.RevokeAPIKey)
		}

		admin := protected.Group("/admin")
		admin.Use(r.app.AuthHandler.AdminRequired())
		{
			admin.GET("/users", r.app.AuthHandler.ListUsers)
			admin.GET("/users/:id", r.app.AuthHandler.GetUser)
			admin.PUT("/users/:id", r.app.AuthHandler.UpdateUser)
			admin.DELETE("/users/:id", r.app.AuthHandler.DeleteUser)
		}
	}
}

// registerDialogueRoutes 注册对话路由
func (r *Router) registerDialogueRoutes(api *gin.RouterGroup) {
	dialogues := api.Group("/dialogues")
	{
		dialogues.GET("", func(c *gin.Context) {
			pagination := middleware.GetPagination(c)
			allDialogues := r.app.DialogueService.ListDialogues()
			total := int64(len(allDialogues))
			end := pagination.Offset + pagination.PageSize
			if end > len(allDialogues) {
				end = len(allDialogues)
			}
			if pagination.Offset >= len(allDialogues) {
				response.OKPage(c, []interface{}{}, total, pagination.Page, pagination.PageSize)
				return
			}
			response.OKPage(c, allDialogues[pagination.Offset:end], total, pagination.Page, pagination.PageSize)
		})
		dialogues.GET("/user/:userID", func(c *gin.Context) {
			userID := c.Param("userID")
			pagination := middleware.GetPagination(c)
			allDialogues := r.app.DialogueService.ListDialoguesByUser(userID)
			total := int64(len(allDialogues))
			end := pagination.Offset + pagination.PageSize
			if end > len(allDialogues) {
				end = len(allDialogues)
			}
			if pagination.Offset >= len(allDialogues) {
				response.OKPage(c, []interface{}{}, total, pagination.Page, pagination.PageSize)
				return
			}
			response.OKPage(c, allDialogues[pagination.Offset:end], total, pagination.Page, pagination.PageSize)
		})
		dialogues.POST("", func(c *gin.Context) {
			var req struct {
				UserID string `json:"user_id"`
				Title  string `json:"title"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			dialogue := r.app.DialogueService.CreateDialogue(req.UserID, req.Title)
			response.OK(c, dialogue)
		})
		dialogues.GET("/:id", func(c *gin.Context) {
			id := c.Param("id")
			dialogue, found := r.app.DialogueService.GetDialogue(id)
			if !found {
				response.NotFound(c, "Dialogue not found")
				return
			}
			response.OK(c, dialogue)
		})
		dialogues.PUT("/:id", func(c *gin.Context) {
			id := c.Param("id")
			var req struct {
				Title string `json:"title"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			dialogue, found := r.app.DialogueService.UpdateDialogue(id, req.Title)
			if !found {
				response.NotFound(c, "Dialogue not found")
				return
			}
			response.OK(c, dialogue)
		})
		dialogues.DELETE("/:id", func(c *gin.Context) {
			id := c.Param("id")
			if err := r.app.DialogueService.DeleteDialogue(id); err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OKWithMessage(c, "Dialogue deleted successfully", nil)
		})
		dialogues.GET("/:id/messages", func(c *gin.Context) {
			id := c.Param("id")
			messages := r.app.DialogueService.GetMessages(id)
			response.OK(c, messages)
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
				response.BadRequest(c, err.Error())
				return
			}
			ctx := c.Request.Context()
			message, err := r.app.EnhancedDialogueService.SendMessage(ctx, id, req.UserID, req.Content, req.ModelID, req.Options)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, message)
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
				response.BadRequest(c, err.Error())
				return
			}
			ctx := c.Request.Context()
			chunkChan, err := r.app.EnhancedDialogueService.SendMessageStream(ctx, id, req.UserID, req.Content, req.ModelID, req.Options)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			r.writeSSEStream(c, chunkChan)
		})
		dialogues.DELETE("/:id/messages", func(c *gin.Context) {
			id := c.Param("id")
			if err := r.app.DialogueService.ClearMessages(id); err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OKWithMessage(c, "Messages cleared successfully", nil)
		})
		dialogues.POST("/:id/save-stream", func(c *gin.Context) {
			id := c.Param("id")
			var req struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			message, err := r.app.DialogueService.SaveStreamMessage(id, req.Content, req.ReasoningContent)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, message)
		})
	}

	// ReAct 会话接口
	react := api.Group("/react")
	{
		react.GET("/sessions", func(c *gin.Context) {
			sessions := r.app.ToolCallingService.ListReActSessions()
			response.OK(c, gin.H{"sessions": sessions})
		})
		react.GET("/sessions/:id/export", func(c *gin.Context) {
			id := c.Param("id")
			data, err := r.app.ToolCallingService.GetSessionExport(id)
			if err != nil {
				response.NotFound(c, err.Error())
				return
			}
			c.Data(http.StatusOK, "application/json", data)
		})
		react.GET("/metrics", func(c *gin.Context) {
			metrics := r.app.ToolCallingService.GetSessionMetrics()
			response.OK(c, metrics)
		})
	}
}

// registerWorkflowRoutes 注册工作流路由
func (r *Router) registerWorkflowRoutes(api *gin.RouterGroup) {
	workflows := api.Group("/workflows")
	{
		workflows.GET("", func(c *gin.Context) {
			workflows := r.app.WorkflowService.ListWorkflows()
			response.OK(c, workflows)
		})
		workflows.POST("", func(c *gin.Context) {
			var req struct {
				Name        string                  `json:"name"`
				Description string                  `json:"description"`
				Steps       []models.WorkflowStep `json:"steps"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			workflow := r.app.WorkflowService.CreateWorkflow(req.Name, req.Description, req.Steps)
			response.OK(c, workflow)
		})
		workflows.GET("/:id", func(c *gin.Context) {
			id := c.Param("id")
			workflow, found := r.app.WorkflowService.GetWorkflow(id)
			if !found {
				response.NotFound(c, "Workflow not found")
				return
			}
			response.OK(c, workflow)
		})
		workflows.PUT("/:id", func(c *gin.Context) {
			id := c.Param("id")
			var req struct {
				Name        string                  `json:"name"`
				Description string                  `json:"description"`
				Steps       []models.WorkflowStep `json:"steps"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			workflow, found := r.app.WorkflowService.UpdateWorkflow(id, req.Name, req.Description, req.Steps)
			if !found {
				response.NotFound(c, "Workflow not found")
				return
			}
			response.OK(c, workflow)
		})
		workflows.DELETE("/:id", func(c *gin.Context) {
			id := c.Param("id")
			if r.app.WorkflowService.DeleteWorkflow(id) {
				response.OKWithMessage(c, "Workflow deleted successfully", nil)
			} else {
				response.NotFound(c, "Workflow not found")
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
			instance, found := r.app.WorkflowService.CreateWorkflowInstance(id, req.InputVariables)
			if !found {
				response.NotFound(c, "Workflow not found")
				return
			}
			response.OK(c, instance)
		})
		workflows.POST("/instances/:id/execute", func(c *gin.Context) {
			id := c.Param("id")
			instance, err := r.app.WorkflowService.ExecuteWorkflowInstance(id)
			if err != nil {
				response.NotFound(c, err.Error())
				return
			}
			response.OK(c, instance)
		})
	}
}

// registerChatRoutes 注册聊天路由
func (r *Router) registerChatRoutes(api *gin.RouterGroup) {
	chat := api.Group("/chat")
	{
		chat.POST("/tools", func(c *gin.Context) {
			var req struct {
				ModelID    string                 `json:"model_id"`
				UserID     string                 `json:"user_id"`
				DialogueID string                 `json:"dialogue_id"`
				Content    string                 `json:"content" binding:"required"`
				Options    map[string]interface{} `json:"options"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			ctx := c.Request.Context()
			msg, err := r.app.EnhancedDialogueService.SendMessageWithTools(ctx, req.DialogueID, req.UserID, req.Content, req.ModelID, req.Options)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, msg)
		})
		chat.POST("/route", func(c *gin.Context) {
			var req struct {
				UserID     string                 `json:"user_id"`
				DialogueID string                 `json:"dialogue_id"`
				Content    string                 `json:"content" binding:"required"`
				ModelID    string                 `json:"model_id"`
				Options    map[string]interface{} `json:"options"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			ctx := c.Request.Context()
			chunkChan, err := r.app.EnhancedDialogueService.SendMessageStreamRouted(ctx, req.DialogueID, req.UserID, req.Content, req.ModelID, req.Options)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			r.writeSSEStream(c, chunkChan)
		})
		chat.GET("/route-info", func(c *gin.Context) {
			content := c.Query("content")
			if content == "" {
				response.BadRequest(c, "content parameter required")
				return
			}
			info := r.app.ModelRouter.GetRouteInfo(c.Request.Context(), content)
			response.OK(c, info)
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
				response.BadRequest(c, err.Error())
				return
			}
			ctx := c.Request.Context()
			result, err := r.app.EnhancedDialogueService.SendMessageWithPlan(ctx, req.DialogueID, req.UserID, req.Content, req.ModelID, req.Options)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, result)
		})
		chat.POST("", func(c *gin.Context) {
			var req struct {
				ModelID  string                 `json:"model_id"`
				Messages []services.ChatMessage `json:"messages" binding:"required"`
				Options  map[string]interface{} `json:"options"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			llmMessages := make([]llm.Message, len(req.Messages))
			for i, msg := range req.Messages {
				llmMessages[i] = llm.Message{Role: msg.Role, Content: msg.Content}
			}
			resp, err := r.app.ModelService.Chat(req.ModelID, llmMessages, req.Options)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, resp)
		})
		chat.POST("/stream", func(c *gin.Context) {
			var req struct {
				ModelID  string                 `json:"model_id"`
				Messages []services.ChatMessage `json:"messages" binding:"required"`
				Options  map[string]interface{} `json:"options"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			llmMessages := make([]llm.Message, len(req.Messages))
			for i, msg := range req.Messages {
				llmMessages[i] = llm.Message{Role: msg.Role, Content: msg.Content}
			}
			chunkChan, err := r.app.ModelService.ChatStream(req.ModelID, llmMessages, req.Options)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			r.writeSSEStream(c, chunkChan)
		})
	}
}

// registerModelRoutes 注册模型路由
func (r *Router) registerModelRoutes(api *gin.RouterGroup) {
	modelGroup := api.Group("/models")
	{
		modelGroup.GET("", func(c *gin.Context) {
			models, err := r.app.ModelService.ListModels()
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, models)
		})
		modelGroup.POST("", func(c *gin.Context) {
			var model models.Model
			if err := c.ShouldBindJSON(&model); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			err := r.app.ModelService.CreateModel(&model)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, model)
		})
		modelGroup.GET("/:id", func(c *gin.Context) {
			id := c.Param("id")
			model, err := r.app.ModelService.GetModel(id)
			if err != nil {
				response.NotFound(c, "Model not found")
				return
			}
			response.OK(c, model)
		})
		modelGroup.PUT("/:id", func(c *gin.Context) {
			id := c.Param("id")
			var model models.Model
			if err := c.ShouldBindJSON(&model); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			model.ID = id
			err := r.app.ModelService.UpdateModel(&model)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, model)
		})
		modelGroup.DELETE("/:id", func(c *gin.Context) {
			id := c.Param("id")
			err := r.app.ModelService.DeleteModel(id)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OKWithMessage(c, "Model deleted successfully", nil)
		})
		modelGroup.POST("/:id/enable", func(c *gin.Context) {
			id := c.Param("id")
			err := r.app.ModelService.EnableModel(id)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OKWithMessage(c, "Model enabled successfully", nil)
		})
		modelGroup.POST("/:id/disable", func(c *gin.Context) {
			id := c.Param("id")
			err := r.app.ModelService.DisableModel(id)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OKWithMessage(c, "Model disabled successfully", nil)
		})
		modelGroup.POST("/:id/instances", func(c *gin.Context) {
			id := c.Param("id")
			var req struct {
				Config map[string]interface{} `json:"config"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			instance, err := r.app.ModelService.CreateModelInstance(id, req.Config)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, instance)
		})
		modelGroup.POST("/instances/:id/execute", func(c *gin.Context) {
			id := c.Param("id")
			var req struct {
				Parameters map[string]interface{} `json:"parameters"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			execution, err := r.app.ModelService.ExecuteModel(id, req.Parameters)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, execution)
		})
	}
}

// registerPlanRoutes 注册规划路由
func (r *Router) registerPlanRoutes(api *gin.RouterGroup) {
	planGroup := api.Group("/plan")
	{
		planGroup.POST("/execute", func(c *gin.Context) {
			var req struct {
				SessionID string `json:"session_id" binding:"required"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			result, err := r.app.PlanService.ExecutePlan(c.Request.Context(), req.SessionID)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, result)
		})
		planGroup.GET("/:sessionId", func(c *gin.Context) {
			sessionID := c.Param("sessionId")
			result, err := r.app.PlanService.GetPlanStatus(c.Request.Context(), sessionID)
			if err != nil {
				response.NotFound(c, err.Error())
				return
			}
			response.OK(c, result)
		})
		planGroup.POST("/cancel", func(c *gin.Context) {
			var req struct {
				SessionID string `json:"session_id" binding:"required"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			err := r.app.PlanService.CancelPlan(c.Request.Context(), req.SessionID)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OKWithMessage(c, "plan cancelled", nil)
		})
	}
}

// registerFeedbackRoutes 注册反馈路由
func (r *Router) registerFeedbackRoutes(api *gin.RouterGroup) {
	feedback := api.Group("/feedback")
	{
		feedback.POST("", func(c *gin.Context) {
			var fb models.Feedback
			if err := c.ShouldBindJSON(&fb); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			err := r.app.FeedbackService.CreateFeedback(&fb)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, fb)
		})
		feedback.GET("/task/:id", func(c *gin.Context) {
			id := c.Param("id")
			feedbacks, err := r.app.FeedbackService.GetFeedbackByTask(id)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, feedbacks)
		})
		feedback.GET("/average/:type", func(c *gin.Context) {
			typeStr := c.Param("type")
			average, err := r.app.FeedbackService.GetAverageRating(typeStr)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, gin.H{"average_rating": average})
		})
	}
}

// registerLearningRoutes 注册学习路由
func (r *Router) registerLearningRoutes(api *gin.RouterGroup) {
	learning := api.Group("/learning")
	{
		learning.Use(func(c *gin.Context) {
			c.Set("db", r.app.DB)
			c.Next()
		})
		learning.POST("/learn", r.app.LearningHandler.LearnFromFeedbackHandler)
		learning.GET("/preferences", r.app.LearningHandler.GetUserPreferencesHandler)
		learning.POST("/preferences/learn", r.app.LearningHandler.LearnUserPreferencesHandler)
		learning.POST("/workflows/:workflow_id/optimize", r.app.LearningHandler.OptimizeWorkflowHandler)
		learning.POST("/evaluate", r.app.LearningHandler.EvaluateLearningEffectHandler)
		learning.GET("/prompts/recommended", r.app.LearningHandler.GetRecommendedPromptsHandler)
		learning.POST("/prompts/:optimization_id/apply", r.app.LearningHandler.ApplyPromptOptimizationHandler)
		learning.POST("/interactions", r.app.LearningHandler.RecordInteractionHandler)
		learning.GET("/insights", r.app.LearningHandler.GetLearningInsightsHandler)
		learning.GET("/records", r.app.LearningHandler.GetLearningRecordsHandler)
		learning.GET("/optimizations/prompts", r.app.LearningHandler.GetPromptOptimizationsHandler)
		learning.GET("/optimizations/workflows", r.app.LearningHandler.GetWorkflowOptimizationsHandler)
		learning.POST("/optimizations/workflows/:optimization_id/apply", r.app.LearningHandler.ApplyWorkflowOptimizationHandler)
		learning.GET("/interactions/history", r.app.LearningHandler.GetInteractionHistoryHandler)
	}
}

// registerEvolutionRoutes 注册进化路由
func (r *Router) registerEvolutionRoutes(api *gin.RouterGroup) {
	evolution := api.Group("/evolution")
	{
		evolution.GET("/reflections", func(c *gin.Context) {
			dialogueID := c.Query("dialogue_id")
			limit := 20
			if l, err := strconv.Atoi(c.DefaultQuery("limit", "20")); err == nil && l > 0 {
				limit = l
			}
			if dialogueID != "" {
				reflections, err := r.app.SelfReflectionService.GetReflectionsByDialogue(dialogueID)
				if err != nil {
					response.InternalError(c, err.Error())
					return
				}
				response.OK(c, reflections)
				return
			}
			reflections, err := r.app.SelfReflectionService.GetRecentReflections(limit)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, reflections)
		})
		evolution.POST("/reflect", func(c *gin.Context) {
			var req struct {
				DialogueID string `json:"dialogue_id"`
				UserID     string `json:"user_id"`
				Query      string `json:"query"`
				Response   string `json:"response"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			reflection, err := r.app.SelfReflectionService.ReflectOnResponse(c.Request.Context(), req.DialogueID, req.UserID, req.Query, req.Response)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, reflection)
		})
		evolution.POST("/reflections/:id/apply", func(c *gin.Context) {
			id := c.Param("id")
			if err := r.app.SelfReflectionService.ApplyReflection(id); err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OKWithMessage(c, "Reflection applied successfully", nil)
		})
		evolution.GET("/quality-trend", func(c *gin.Context) {
			days := 7
			if d, err := strconv.Atoi(c.DefaultQuery("days", "7")); err == nil && d > 0 {
				days = d
			}
			trend, err := r.app.SelfReflectionService.GetQualityTrend(days)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, trend)
		})
		evolution.GET("/patterns", func(c *gin.Context) {
			userID := c.Query("user_id")
			patternType := c.Query("type")
			patterns, err := r.app.PatternDetectorService.GetPatterns(userID, patternType)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, patterns)
		})
		evolution.POST("/patterns/detect", func(c *gin.Context) {
			userID := c.Query("user_id")
			patterns, err := r.app.PatternDetectorService.DetectPatterns(c.Request.Context(), userID)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, patterns)
		})
		evolution.POST("/patterns/:id/create-skill", func(c *gin.Context) {
			id := c.Param("id")
			skill, err := r.app.PatternDetectorService.CreateSkillFromPattern(c.Request.Context(), id)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OKWithMessage(c, "Skill created from pattern", gin.H{"skill": skill})
		})
		evolution.POST("/patterns/:id/ignore", func(c *gin.Context) {
			id := c.Param("id")
			if err := r.app.PatternDetectorService.IgnorePattern(id); err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OKWithMessage(c, "Pattern ignored", nil)
		})
		evolution.GET("/skill-evolutions", func(c *gin.Context) {
			skillID := c.Query("skill_id")
			if skillID != "" {
				evolutions, err := r.app.SkillEvolutionService.GetEvolutionHistory(skillID)
				if err != nil {
					response.InternalError(c, err.Error())
					return
				}
				response.OK(c, evolutions)
				return
			}
			evolutions, err := r.app.SkillEvolutionService.GetPendingEvolutions()
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, evolutions)
		})
		evolution.POST("/skill-evolutions/:id/apply", func(c *gin.Context) {
			id := c.Param("id")
			if err := r.app.SkillEvolutionService.ApplyEvolution(id); err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OKWithMessage(c, "Skill evolution applied successfully", nil)
		})
		evolution.POST("/skill-evolutions/:id/rollback", func(c *gin.Context) {
			id := c.Param("id")
			if err := r.app.SkillEvolutionService.RollbackEvolution(id); err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OKWithMessage(c, "Skill evolution rolled back successfully", nil)
		})
		evolution.POST("/skills/:skill_id/evolve", func(c *gin.Context) {
			skillID := c.Param("skill_id")
			evolution, err := r.app.SkillEvolutionService.EvolveSkillFromFeedback(c.Request.Context(), skillID)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			if evolution == nil {
				response.OKWithMessage(c, "No evolution needed", nil)
				return
			}
			response.OK(c, evolution)
		})
		evolution.GET("/gaps", func(c *gin.Context) {
			gapType := c.Query("type")
			severity := c.Query("severity")
			gaps, err := r.app.CapabilityGapService.GetGaps(gapType, severity)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, gaps)
		})
		evolution.POST("/gaps/detect", func(c *gin.Context) {
			userID := c.Query("user_id")
			gaps, err := r.app.CapabilityGapService.DetectGaps(c.Request.Context(), userID)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, gaps)
		})
		evolution.POST("/gaps/:id/create-skill", func(c *gin.Context) {
			id := c.Param("id")
			skill, err := r.app.CapabilityGapService.CreateSkillFromGap(c.Request.Context(), id)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OKWithMessage(c, "Skill created from capability gap", gin.H{"skill": skill})
		})
		evolution.POST("/gaps/:id/ignore", func(c *gin.Context) {
			id := c.Param("id")
			if err := r.app.CapabilityGapService.IgnoreGap(id); err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OKWithMessage(c, "Gap ignored", nil)
		})
	}
}

// registerMemoryRoutes 注册记忆路由
func (r *Router) registerMemoryRoutes(api *gin.RouterGroup) {
	memory := api.Group("/memory")
	{
		memory.POST("", func(c *gin.Context) {
			var mem models.Memory
			if err := c.ShouldBindJSON(&mem); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			err := r.app.MemoryService.CreateMemory(&mem)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			if r.app.MemoryEmbeddingService != nil {
				logger.SafeGo(func() {
					r.app.MemoryEmbeddingService.AutoEmbedNewMemories(mem.ID)
				})
			}
			response.OK(c, mem)
		})
		memory.GET("/user/:id", func(c *gin.Context) {
			userID := c.Param("id")
			memories, err := r.app.MemoryService.GetMemoriesByUser(userID)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, memories)
		})
		memory.GET("/search", func(c *gin.Context) {
			userID := c.Query("user_id")
			keyword := c.Query("keyword")
			memories, err := r.app.MemoryService.SearchMemories(userID, keyword)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, memories)
		})
		memory.POST("/semantic-search", func(c *gin.Context) {
			if r.app.MemoryEmbeddingService == nil {
				response.ServiceUnavailable(c, "semantic search not available")
				return
			}
			var req struct {
				UserID string `json:"user_id" binding:"required"`
				Query  string `json:"query" binding:"required"`
				Limit  int    `json:"limit"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			if req.Limit <= 0 {
				req.Limit = 5
			}
			results, err := r.app.MemoryEmbeddingService.SemanticSearch(c.Request.Context(), req.UserID, req.Query, req.Limit)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, gin.H{"data": results, "count": len(results)})
		})
		memory.POST("/hybrid-search", func(c *gin.Context) {
			if r.app.MemoryEmbeddingService == nil {
				response.ServiceUnavailable(c, "hybrid search not available")
				return
			}
			var req struct {
				UserID         string  `json:"user_id" binding:"required"`
				Query          string  `json:"query" binding:"required"`
				Limit          int     `json:"limit"`
				SemanticWeight float64 `json:"semantic_weight"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			if req.Limit <= 0 {
				req.Limit = 5
			}
			results, err := r.app.MemoryEmbeddingService.HybridSearch(c.Request.Context(), req.UserID, req.Query, req.Limit, req.SemanticWeight)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, gin.H{"data": results, "count": len(results)})
		})
		memory.POST("/batch-embed", func(c *gin.Context) {
			if r.app.MemoryEmbeddingService == nil {
				response.ServiceUnavailable(c, "embedding service not available")
				return
			}
			userID := c.Query("user_id")
			if userID == "" {
				response.BadRequest(c, "user_id is required")
				return
			}
			count, err := r.app.MemoryEmbeddingService.BatchEmbedUserMemories(userID)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OKWithMessage(c, "Embedded "+strconv.Itoa(count)+" memories", gin.H{"count": count})
		})
		memory.PUT("/:id", func(c *gin.Context) {
			id := c.Param("id")
			var mem models.Memory
			if err := c.ShouldBindJSON(&mem); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			mem.ID = id
			err := r.app.MemoryService.UpdateMemory(id, &mem)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			if r.app.MemoryEmbeddingService != nil {
				logger.SafeGo(func() {
					r.app.MemoryEmbeddingService.AutoEmbedNewMemories(id)
				})
			}
			response.OK(c, mem)
		})
		memory.DELETE("/:id", func(c *gin.Context) {
			id := c.Param("id")
			err := r.app.MemoryService.DeleteMemory(id)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OKWithMessage(c, "Memory deleted successfully", nil)
		})
		memory.POST("/adjust-priority", func(c *gin.Context) {
			err := r.app.MemoryService.AdjustPriority()
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OKWithMessage(c, "Memory priorities adjusted", nil)
		})
	}
}

// registerTaskRoutes 注册任务路由
func (r *Router) registerTaskRoutes(api *gin.RouterGroup) {
	tasks := api.Group("/tasks")
	{
		tasks.POST("", r.app.TaskHandler.CreateTaskHandler)
		tasks.POST("/decompose", r.app.TaskHandler.DecomposeTaskHandler)
		tasks.GET("", r.app.TaskHandler.ListTasksHandler)
		tasks.GET("/:id", r.app.TaskHandler.GetTaskHandler)
		tasks.PUT("/:id", r.app.TaskHandler.UpdateTaskHandler)
		tasks.DELETE("/:id", r.app.TaskHandler.DeleteTaskHandler)
		tasks.PATCH("/:id/status", r.app.TaskHandler.UpdateTaskStatusHandler)
		tasks.PATCH("/subtasks/:id/status", r.app.TaskHandler.UpdateSubtaskStatusHandler)
		tasks.POST("/progress", r.app.TaskHandler.UpdateProgressHandler)
		tasks.GET("/:id/progress", r.app.TaskHandler.GetTaskProgressHandler)
		tasks.GET("/overview", r.app.TaskHandler.GetTaskOverviewHandler)
		tasks.POST("/:id/retry", r.app.TaskHandler.RetryFailedTaskHandler)
		tasks.POST("/:id/summary", r.app.TaskHandler.GenerateTaskSummaryHandler)
		tasks.POST("/:id/cancel", r.app.TaskHandler.CancelTaskHandler)
		tasks.POST("/:id/reassign", r.app.TaskHandler.ReassignTaskHandler)
		tasks.GET("/:id/can-start", r.app.TaskHandler.CanStartTaskHandler)

		members := api.Group("/members")
		{
			members.POST("", r.app.TaskHandler.CreateMemberHandler)
			members.GET("", r.app.TaskHandler.ListMembersHandler)
			members.GET("/:id", r.app.TaskHandler.GetMemberTasksHandler)
			members.PUT("/:id", r.app.TaskHandler.UpdateMemberHandler)
		}

		templates := api.Group("/task-templates")
		{
			templates.GET("", r.app.TaskHandler.ListTemplatesHandler)
			templates.GET("/:type", r.app.TaskHandler.GetTemplateHandler)
		}
	}
}

// registerKnowledgeRoutes 注册知识库路由
func (r *Router) registerKnowledgeRoutes(api *gin.RouterGroup) {
	knowledge := api.Group("/knowledge")
	{
		knowledge.GET("", r.app.KnowledgeHandler.ListKnowledge)
		knowledge.POST("", r.app.KnowledgeHandler.CreateKnowledge)
		knowledge.GET("/:id", r.app.KnowledgeHandler.GetKnowledge)
		knowledge.PUT("/:id", r.app.KnowledgeHandler.UpdateKnowledge)
		knowledge.DELETE("/:id", r.app.KnowledgeHandler.DeleteKnowledge)
		knowledge.POST("/search", r.app.KnowledgeHandler.SearchKnowledge)
		knowledge.POST("/hybrid-search", r.app.KnowledgeHandler.HybridSearchKnowledge)
	}

	categories := api.Group("/knowledge/categories")
	{
		categories.GET("", r.app.KnowledgeHandler.ListCategories)
		categories.POST("", r.app.KnowledgeHandler.CreateCategory)
		categories.GET("/:id", r.app.KnowledgeHandler.GetCategory)
		categories.DELETE("/:id", r.app.KnowledgeHandler.DeleteCategory)
	}

	documents := api.Group("/documents")
	{
		documents.GET("", r.app.KnowledgeHandler.ListDocuments)
		documents.POST("/import", r.app.KnowledgeHandler.ImportDocument)
		documents.GET("/:id", r.app.KnowledgeHandler.GetDocument)
		documents.DELETE("/:id", r.app.KnowledgeHandler.DeleteDocument)
	}

	rag := api.Group("/rag")
	{
		rag.POST("/query", r.app.KnowledgeHandler.RAGQuery)
		rag.POST("/stream", r.app.KnowledgeHandler.RAGStream)
	}
}

// registerContextRoutes 注册上下文管理路由
func (r *Router) registerContextRoutes(api *gin.RouterGroup) {
	contextGroup := api.Group("/context")
	{
		contextGroup.POST("/compress", r.app.ContextHandler.CompressContext)
		contextGroup.POST("/summarize", r.app.ContextHandler.SummarizeContext)
		contextGroup.GET("/metrics", r.app.ContextHandler.GetMetrics)
		contextGroup.DELETE("/expired", r.app.ContextHandler.ClearExpired)
		contextGroup.GET("/dialogue/:id", r.app.ContextHandler.GetDialogueContext)
	}
}

// registerExtractionRoutes 注册知识提取路由
func (r *Router) registerExtractionRoutes(api *gin.RouterGroup) {
	extractionGroup := api.Group("/extraction")
	{
		extractionGroup.POST("/extract", r.app.ExtractionHandler.ExtractFromDialogue)
		extractionGroup.POST("/auto", r.app.ExtractionHandler.AutoExtract)
		extractionGroup.POST("/batch", r.app.ExtractionHandler.BatchExtract)
		extractionGroup.GET("/config", r.app.ExtractionHandler.GetConfig)
		extractionGroup.PUT("/config", r.app.ExtractionHandler.UpdateConfig)
	}
}

// registerPermissionRoutes 注册权限路由
func (r *Router) registerPermissionRoutes(api *gin.RouterGroup) {
	permGroup := api.Group("/permissions")
	{
		permGroup.GET("/profiles", func(c *gin.Context) {
			profiles := r.app.PermissionService.ListProfiles()
			response.OK(c, profiles)
		})
		permGroup.GET("/profiles/:mode", func(c *gin.Context) {
			mode := services.AgentMode(c.Param("mode"))
			profile := r.app.PermissionService.GetProfile(mode)
			if profile == nil {
				response.NotFound(c, "profile not found")
				return
			}
			response.OK(c, profile)
		})
		permGroup.POST("/respond", func(c *gin.Context) {
			var req struct {
				AskID  string                   `json:"ask_id" binding:"required"`
				Action services.PermissionAction `json:"action" binding:"required"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			if err := r.app.PermissionService.RespondAsk(req.AskID, req.Action); err != nil {
				response.NotFound(c, err.Error())
				return
			}
			response.OKWithMessage(c, "ok", nil)
		})
		permGroup.POST("/rules", func(c *gin.Context) {
			var rules []services.PermissionRule
			if err := c.ShouldBindJSON(&rules); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			r.app.PermissionService.UpdateGlobalRules(rules)
			response.OKWithMessage(c, "updated", nil)
		})
		permGroup.DELETE("/session/:sessionID", func(c *gin.Context) {
			sessionID := c.Param("sessionID")
			r.app.PermissionService.ClearSessionApprovals(sessionID)
			response.OKWithMessage(c, "cleared", nil)
		})
	}
}

// registerAgentRoutingRoutes 注册 Agent 路由
func (r *Router) registerAgentRoutingRoutes(api *gin.RouterGroup) {
	routingGroup := api.Group("/agent-routing")
	{
		routingGroup.GET("/config", func(c *gin.Context) {
			config := r.app.AgentRouter.GetConfig()
			response.OK(c, config)
		})
		routingGroup.GET("/routes", func(c *gin.Context) {
			routes := r.app.AgentRouter.ListRoutes()
			response.OK(c, routes)
		})
		routingGroup.GET("/route/:agent", func(c *gin.Context) {
			agentType := c.Param("agent")
			modelID := r.app.AgentRouter.RouteModelID(agentType)
			response.OK(c, gin.H{"agent": agentType, "model": modelID})
		})
		routingGroup.POST("/config", func(c *gin.Context) {
			var config services.AgentRoutingConfig
			if err := c.ShouldBindJSON(&config); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			r.app.AgentRouter.UpdateConfig(config)
			response.OKWithMessage(c, "updated", nil)
		})
	}
}

// registerSlashCommandRoutes 注册 Slash 命令路由
func (r *Router) registerSlashCommandRoutes(api *gin.RouterGroup) {
	slashGroup := api.Group("/slash")
	{
		slashGroup.GET("/commands", func(c *gin.Context) {
			commands := r.app.SlashRegistry.ListCommands()
			response.OK(c, commands)
		})
		slashGroup.POST("/execute", func(c *gin.Context) {
			var req struct {
				Command   string `json:"command" binding:"required"`
				Args      string `json:"args"`
				SessionID string `json:"session_id"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			result, err := r.app.SlashRegistry.Execute(c.Request.Context(), req.Command, req.Args, req.SessionID)
			if err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			response.OK(c, gin.H{"result": result})
		})
	}
}

// registerEventRoutes 注册事件路由
func (r *Router) registerEventRoutes(api *gin.RouterGroup) {
	eventGroup := api.Group("/events")
	{
		eventGroup.GET("", func(c *gin.Context) {
			topic := c.Query("topic")
			events, err := r.app.EventBus.GetEvents(topic, 50)
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, events)
		})
		eventGroup.GET("/stats", func(c *gin.Context) {
			stats, err := r.app.EventBus.GetStats()
			if err != nil {
				response.InternalError(c, err.Error())
				return
			}
			response.OK(c, stats)
		})
		eventGroup.GET("/:id", func(c *gin.Context) {
			id := c.Param("id")
			event, err := r.app.EventBus.GetEvent(id)
			if err != nil {
				response.NotFound(c, "Event not found")
				return
			}
			response.OK(c, event)
		})
		eventGroup.POST("/publish", func(c *gin.Context) {
			var req struct {
				Topic  string                 `json:"topic" binding:"required"`
				Type   string                 `json:"type" binding:"required"`
				Source string                 `json:"source"`
				Data   map[string]interface{} `json:"data"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			r.app.EventBus.Publish(c.Request.Context(), req.Topic, req.Type, req.Source, req.Data)
			response.OKWithMessage(c, "event published", nil)
		})
	}
}

// registerFeishuRoutes 注册飞书路由
func (r *Router) registerFeishuRoutes(api *gin.RouterGroup) {
	feishu := api.Group("/feishu")
	{
		feishu.GET("/status", r.app.FeishuHandler.GetStatus)
		feishu.POST("/start", r.app.FeishuHandler.Start)
		feishu.POST("/stop", r.app.FeishuHandler.Stop)
		feishu.GET("/sessions", r.app.FeishuHandler.ListSessions)
		feishu.DELETE("/session/:key", r.app.FeishuHandler.DeleteSession)
		feishu.GET("/users", r.app.FeishuHandler.ListUsers)
		feishu.POST("/card-callback", r.app.FeishuHandler.HandleCardCallback)
	}
}

// registerWebSocketRoutes 注册 WebSocket 路由
func (r *Router) registerWebSocketRoutes(api *gin.RouterGroup, engine *gin.Engine) {
	engine.GET("/ws", r.app.WebSocketHandler.HandleWebSocket)
	ws := api.Group("/ws")
	{
		ws.GET("/stats", r.app.WebSocketHandler.HandleWebSocketStats)
		ws.POST("/broadcast", r.app.WebSocketHandler.HandleBroadcast)
		ws.POST("/send/:user_id", r.app.WebSocketHandler.HandleSendToUser)
		ws.POST("/notify/task/:id", r.app.WebSocketHandler.HandleNotifyTask)
		ws.GET("/dialogue/stream", r.app.WebSocketHandler.DialogueStreamHandler(r.app.DialogueService))
	}
}

// writeSSEStream 通用 SSE 流式响应写入
func (r *Router) writeSSEStream(c *gin.Context, chunkChan <-chan llm.ChatStreamChunk) {
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
		if chunk.ToolDone != nil {
			c.SSEvent("message", map[string]interface{}{
				"type":   "tool_done",
				"tool":   chunk.ToolDone.Tool,
				"result": chunk.ToolDone.Result,
			})
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
}

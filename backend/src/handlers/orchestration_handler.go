package handlers

import (
	"net/http"

	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

// OrchestrationHandler 智能编排处理器
type OrchestrationHandler struct {
	orchestrationService *services.OrchestrationService
}

// NewOrchestrationHandler 创建智能编排处理器
func NewOrchestrationHandler(orchestrationService *services.OrchestrationService) *OrchestrationHandler {
	return &OrchestrationHandler{
		orchestrationService: orchestrationService,
	}
}

// ProcessMessage 处理用户消息 - 智能编排主入口
// @Summary 处理用户消息进行智能编排
// @Description 接收用户消息，自动分析任务、规划团队、生成执行方案
// @Tags orchestration
// @Accept json
// @Produce json
// @Param request body services.OrchestrationRequest true "编排请求"
// @Success 200 {object} services.OrchestrationResponse "编排响应"
// @Router /api/orchestration/process [post]
func (h *OrchestrationHandler) ProcessMessage(c *gin.Context) {
	var req services.OrchestrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 从上下文获取用户 ID
	if userID, exists := c.Get("user_id"); exists {
		req.UserID = userID.(string)
	}

	response, err := h.orchestrationService.ProcessUserMessage(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// HandleAction 处理用户操作
// @Summary 处理用户对方案的操作
// @Description 用户确认、拒绝或调整执行方案
// @Tags orchestration
// @Accept json
// @Produce json
// @Param session_id path string true "会话ID"
// @Param request body object{action string, comment string} true "操作请求"
// @Success 200 {object} services.OrchestrationResponse "编排响应"
// @Router /api/orchestration/:session_id/action [post]
func (h *OrchestrationHandler) HandleAction(c *gin.Context) {
	sessionID := c.Param("session_id")

	var req struct {
		Action  string `json:"action" binding:"required"` // approve, reject, adjust
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	response, err := h.orchestrationService.HandleUserAction(c.Request.Context(), sessionID, req.Action, req.Comment)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetSession 获取会话状态
// @Summary 获取编排会话状态
// @Description 查询指定会话的详细状态和进度
// @Tags orchestration
// @Produce json
// @Param session_id path string true "会话ID"
// @Success 200 {object} services.OrchestrationSession "会话信息"
// @Router /api/orchestration/:session_id [get]
func (h *OrchestrationHandler) GetSession(c *gin.Context) {
	sessionID := c.Param("session_id")

	session, err := h.orchestrationService.GetSessionStatus(sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, session)
}

// GetProgress 获取执行进度
// @Summary 获取执行进度
// @Description 查询正在执行的任务进度
// @Tags orchestration
// @Produce json
// @Param session_id path string true "会话ID"
// @Success 200 {object} services.OrchestrationResponse "进度信息"
// @Router /api/orchestration/:session_id/progress [get]
func (h *OrchestrationHandler) GetProgress(c *gin.Context) {
	sessionID := c.Param("session_id")

	progress, err := h.orchestrationService.GetExecutionProgress(sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, progress)
}

// CancelSession 取消会话
// @Summary 取消编排会话
// @Description 取消正在执行的任务
// @Tags orchestration
// @Produce json
// @Param session_id path string true "会话ID"
// @Success 200 {object} map[string]interface{} "取消结果"
// @Router /api/orchestration/:session_id/cancel [post]
func (h *OrchestrationHandler) CancelSession(c *gin.Context) {
	sessionID := c.Param("session_id")

	if err := h.orchestrationService.CancelSession(sessionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Session cancelled",
		"session_id": sessionID,
	})
}

// ListSessions 列出用户的所有会话
// @Summary 列出用户会话
// @Description 获取当前用户的所有编排会话
// @Tags orchestration
// @Produce json
// @Param limit query int false "限制数量" default(20)
// @Success 200 {object} map[string]interface{} "会话列表"
// @Router /api/orchestration/sessions [get]
func (h *OrchestrationHandler) ListSessions(c *gin.Context) {
	userID, _ := c.Get("user_id")
	uid := ""
	if userID != nil {
		uid = userID.(string)
	}

	sessions := h.orchestrationService.ListSessions(uid)

	c.JSON(http.StatusOK, gin.H{
		"sessions": sessions,
		"count":    len(sessions),
	})
}

// AnalyzeTask 仅分析任务（不执行）
// @Summary 分析任务
// @Description 分析用户消息，返回任务类型、复杂度、所需技能等信息
// @Tags orchestration
// @Accept json
// @Produce json
// @Param request body services.OrchestrationRequest true "分析请求"
// @Success 200 {object} orchestration.TaskAnalysis "任务分析结果"
// @Router /api/orchestration/analyze [post]
func (h *OrchestrationHandler) AnalyzeTask(c *gin.Context) {
	var req services.OrchestrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if userID, exists := c.Get("user_id"); exists {
		req.UserID = userID.(string)
	}

	analysis, err := h.orchestrationService.AnalyzeTask(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, analysis)
}

// PlanTeam 仅生成团队方案（不执行）
// @Summary 生成团队方案
// @Description 根据任务分析结果生成团队配置和执行计划
// @Tags orchestration
// @Accept json
// @Produce json
// @Param request body orchestration.TeamPlannerAnalysis true "规划请求"
// @Success 200 {object} orchestration.TeamPlan "团队方案"
// @Router /api/orchestration/plan [post]
func (h *OrchestrationHandler) PlanTeam(c *gin.Context) {
	var req services.OrchestrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if userID, exists := c.Get("user_id"); exists {
		req.UserID = userID.(string)
	}

	plan, err := h.orchestrationService.PlanTeam(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, plan)
}

// GetTemplates 获取可用的团队模板
// @Summary 获取团队模板
// @Description 列出所有预定义的团队模板
// @Tags orchestration
// @Produce json
// @Success 200 {object} map[string]interface{} "模板列表"
// @Router /api/orchestration/templates [get]
func (h *OrchestrationHandler) GetTemplates(c *gin.Context) {
	templates := h.orchestrationService.GetTeamTemplates()
	c.JSON(http.StatusOK, gin.H{
		"templates": templates,
		"count":     len(templates),
	})
}

// GetTemplate 获取指定模板详情
// @Summary 获取团队模板详情
// @Description 获取指定团队模板的详细信息
// @Tags orchestration
// @Produce json
// @Param name path string true "模板名称"
// @Success 200 {object} orchestration.TeamTemplate "模板详情"
// @Router /api/orchestration/templates/:name [get]
func (h *OrchestrationHandler) GetTemplate(c *gin.Context) {
	name := c.Param("name")

	template, err := h.orchestrationService.GetTeamTemplate(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, template)
}

// RegisterRoutes 注册路由
func (h *OrchestrationHandler) RegisterRoutes(r *gin.RouterGroup) {
	orchestration := r.Group("/orchestration")
	{
		// 主流程
		orchestration.POST("/process", h.ProcessMessage)              // 处理消息并开始编排

		// 会话管理
		orchestration.GET("/sessions", h.ListSessions)                // 列出会话
		orchestration.GET("/:session_id", h.GetSession)               // 获取会话状态
		orchestration.GET("/:session_id/progress", h.GetProgress)     // 获取执行进度
		orchestration.POST("/:session_id/cancel", h.CancelSession)    // 取消会话
		orchestration.POST("/:session_id/action", h.HandleAction)     // 处理用户操作

		// 分析和规划（独立功能）
		orchestration.POST("/analyze", h.AnalyzeTask)                 // 仅分析任务
		orchestration.POST("/plan", h.PlanTeam)                      // 仅规划团队

		// 模板管理
		orchestration.GET("/templates", h.GetTemplates)               // 列出模板
		orchestration.GET("/templates/:name", h.GetTemplate)          // 获取模板详情
	}
}

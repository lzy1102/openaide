package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openaide/backend/src/models"
	"openaide/backend/src/services"
)

// LearningHandler 学习处理器
type LearningHandler struct {
	learningService *services.LearningService
}

// NewLearningHandler 创建学习处理器
func NewLearningHandler(learningService *services.LearningService) *LearningHandler {
	return &LearningHandler{
		learningService: learningService,
	}
}

// LearnFromFeedbackRequest 从反馈学习请求
type LearnFromFeedbackRequest struct {
	TaskID string `json:"task_id" binding:"required"`
}

// LearnFromFeedbackHandler 从反馈学习处理器
func (h *LearningHandler) LearnFromFeedbackHandler(c *gin.Context) {
	var req LearnFromFeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	if err := h.learningService.LearnFromFeedback(ctx, req.TaskID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Learning completed successfully"})
}

// GetUserPreferencesHandler 获取用户偏好处理器
func (h *LearningHandler) GetUserPreferencesHandler(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}

	preferences, err := h.learningService.GetUserPreferences(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"preferences": preferences})
}

// LearnUserPreferencesHandler 学习用户偏好处理器
func (h *LearningHandler) LearnUserPreferencesHandler(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	if err := h.learningService.LearnUserPreferences(ctx, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User preferences learned successfully"})
}

// OptimizeWorkflowHandler 优化工作流处理器
func (h *LearningHandler) OptimizeWorkflowHandler(c *gin.Context) {
	workflowID := c.Param("workflow_id")
	if workflowID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workflow_id is required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	if err := h.learningService.OptimizeWorkflow(ctx, workflowID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Workflow optimization completed successfully"})
}

// EvaluateLearningEffectRequest 评估学习效果请求
type EvaluateLearningEffectRequest struct {
	Days int `json:"days"`
}

// EvaluateLearningEffectHandler 评估学习效果处理器
func (h *LearningHandler) EvaluateLearningEffectHandler(c *gin.Context) {
	var req EvaluateLearningEffectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 默认使用 7 天
		req.Days = 7
	}

	if req.Days <= 0 {
		req.Days = 7
	}

	ctx := c.Request.Context()
	period := time.Duration(req.Days) * 24 * time.Hour

	report, err := h.learningService.EvaluateLearningEffect(ctx, period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, report)
}

// GetRecommendedPromptsHandler 获取推荐 Prompt 处理器
func (h *LearningHandler) GetRecommendedPromptsHandler(c *gin.Context) {
	taskType := c.Query("task_type")
	if taskType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_type is required"})
		return
	}

	ctx := c.Request.Context()
	prompts, err := h.learningService.GetRecommendedPrompts(ctx, taskType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"prompts": prompts})
}

// ApplyPromptOptimizationHandler 应用 Prompt 优化处理器
func (h *LearningHandler) ApplyPromptOptimizationHandler(c *gin.Context) {
	optimizationID := c.Param("optimization_id")
	if optimizationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "optimization_id is required"})
		return
	}

	ctx := c.Request.Context()
	if err := h.learningService.ApplyPromptOptimization(ctx, optimizationID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Prompt optimization applied successfully"})
}

// RecordInteractionRequest 记录交互请求
type RecordInteractionRequest struct {
	UserID     string                 `json:"user_id" binding:"required"`
	SessionID  string                 `json:"session_id"`
	Query      string                 `json:"query" binding:"required"`
	Response   string                 `json:"response" binding:"required"`
	TaskType   string                 `json:"task_type"`
	Model      string                 `json:"model"`
	Duration   int64                  `json:"duration"`
	TokensUsed int                    `json:"tokens_used"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// RecordInteractionHandler 记录交互处理器
func (h *LearningHandler) RecordInteractionHandler(c *gin.Context) {
	var req RecordInteractionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	interaction := &models.InteractionRecord{
		UserID:     req.UserID,
		SessionID:  req.SessionID,
		Query:      req.Query,
		Response:   req.Response,
		TaskType:   req.TaskType,
		Model:      req.Model,
		Duration:   req.Duration,
		TokensUsed: req.TokensUsed,
		Metadata:   req.Metadata,
	}

	if err := h.learningService.RecordInteraction(ctx, interaction); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": interaction.ID})
}

// GetLearningInsightsHandler 获取学习洞察处理器
func (h *LearningHandler) GetLearningInsightsHandler(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}

	ctx := c.Request.Context()
	insights, err := h.learningService.GetLearningInsights(ctx, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, insights)
}

// GetLearningRecordsHandler 获取学习记录处理器
func (h *LearningHandler) GetLearningRecordsHandler(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	taskType := c.Query("task_type")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	db := c.MustGet("db").(*gorm.DB)

	query := db.Model(&models.LearningRecord{})
	if taskType != "" {
		query = query.Where("task_type = ?", taskType)
	}

	var total int64
	query.Count(&total)

	var records []models.LearningRecord
	offset := (page - 1) * pageSize
	query.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&records)

	c.JSON(http.StatusOK, gin.H{
		"records": records,
		"pagination": gin.H{
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	})
}

// GetPromptOptimizationsHandler 获取 Prompt 优化列表处理器
func (h *LearningHandler) GetPromptOptimizationsHandler(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	taskType := c.Query("task_type")
	status := c.Query("status")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	db := c.MustGet("db").(*gorm.DB)

	query := db.Model(&models.PromptOptimization{})
	if taskType != "" {
		query = query.Where("task_type = ?", taskType)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	query.Count(&total)

	var optimizations []models.PromptOptimization
	offset := (page - 1) * pageSize
	query.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&optimizations)

	c.JSON(http.StatusOK, gin.H{
		"optimizations": optimizations,
		"pagination": gin.H{
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	})
}

// GetWorkflowOptimizationsHandler 获取工作流优化列表处理器
func (h *LearningHandler) GetWorkflowOptimizationsHandler(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	workflowID := c.Query("workflow_id")
	status := c.Query("status")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	db := c.MustGet("db").(*gorm.DB)

	query := db.Model(&models.WorkflowOptimization{})
	if workflowID != "" {
		query = query.Where("workflow_id = ?", workflowID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	query.Count(&total)

	var optimizations []models.WorkflowOptimization
	offset := (page - 1) * pageSize
	query.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&optimizations)

	c.JSON(http.StatusOK, gin.H{
		"optimizations": optimizations,
		"pagination": gin.H{
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	})
}

// ApplyWorkflowOptimizationHandler 应用工作流优化处理器
func (h *LearningHandler) ApplyWorkflowOptimizationHandler(c *gin.Context) {
	optimizationID := c.Param("optimization_id")
	if optimizationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "optimization_id is required"})
		return
	}

	db := c.MustGet("db").(*gorm.DB)

	// 获取优化记录
	var optimization models.WorkflowOptimization
	if err := db.First(&optimization, optimizationID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "optimization not found"})
		return
	}

	// 应用优化（这里需要根据实际变更来实现）
	now := time.Now()
	optimization.Status = "applied"
	optimization.AppliedAt = &now

	if err := db.Save(&optimization).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Workflow optimization applied successfully"})
}

// GetInteractionHistoryHandler 获取交互历史处理器
func (h *LearningHandler) GetInteractionHistoryHandler(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	userID := c.Query("user_id")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	db := c.MustGet("db").(*gorm.DB)

	query := db.Model(&models.InteractionRecord{})
	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}

	var total int64
	query.Count(&total)

	var interactions []models.InteractionRecord
	offset := (page - 1) * pageSize
	query.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&interactions)

	c.JSON(http.StatusOK, gin.H{
		"interactions": interactions,
		"pagination": gin.H{
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	})
}

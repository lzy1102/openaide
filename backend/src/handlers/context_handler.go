package handlers

import (
	"net/http"
	"time"

	"openaide/backend/src/services"
	"github.com/gin-gonic/gin"
)

// ContextHandler 上下文管理 API 处理器
type ContextHandler struct {
	contextManager  services.ContextManager
	dialogueService *services.DialogueService
	logger          *services.LoggerService
}

// NewContextHandler 创建上下文管理处理器
func NewContextHandler(
	contextManager services.ContextManager,
	dialogueService *services.DialogueService,
	logger *services.LoggerService,
) *ContextHandler {
	return &ContextHandler{
		contextManager:  contextManager,
		dialogueService: dialogueService,
		logger:          logger,
	}
}

// RegisterRoutes 注册路由
func (h *ContextHandler) RegisterRoutes(r *gin.RouterGroup) {
	context := r.Group("/context")
	{
		context.POST("/compress", h.CompressContext)
		context.POST("/summarize", h.SummarizeContext)
		context.GET("/metrics", h.GetMetrics)
		context.DELETE("/expired", h.ClearExpired)
		context.GET("/dialogue/:id", h.GetDialogueContext)
	}
}

// CompressRequest 压缩请求
type CompressRequest struct {
	DialogueID string `json:"dialogue_id" binding:"required"`
}

// CompressContext 压缩对话上下文
// @Summary 压缩对话上下文
// @Description 压缩指定对话的历史记录，生成摘要并提取重要信息
// @Tags Context
// @Accept json
// @Produce json
// @Param request body CompressRequest true "压缩请求"
// @Success 200 {object} services.CompressedContext
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/context/compress [post]
func (h *ContextHandler) CompressContext(c *gin.Context) {
	var req CompressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	result, err := h.contextManager.Compress(req.DialogueID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// SummarizeRequest 摘要请求
type SummarizeRequest struct {
	DialogueID string `json:"dialogue_id" binding:"required"`
}

// SummarizeContext 生成对话摘要
// @Summary 生成对话摘要
// @Description 为指定对话生成摘要
// @Tags Context
// @Accept json
// @Produce json
// @Param request body SummarizeRequest true "摘要请求"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/context/summarize [post]
func (h *ContextHandler) SummarizeContext(c *gin.Context) {
	var req SummarizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	summary, err := h.contextManager.Summarize(req.DialogueID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"dialogue_id": req.DialogueID,
			"summary":     summary,
		},
	})
}

// GetMetrics 获取上下文指标
// @Summary 获取上下文指标
// @Description 获取当前上下文使用情况的统计指标
// @Tags Context
// @Produce json
// @Success 200 {object} services.ContextMetrics
// @Router /api/context/metrics [get]
func (h *ContextHandler) GetMetrics(c *gin.Context) {
	metrics := h.contextManager.GetMetrics()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    metrics,
	})
}

// ClearExpiredRequest 清理过期请求
type ClearExpiredRequest struct {
	Before string `json:"before"` // ISO 8601 格式时间，默认为 24 小时前
}

// ClearExpired 清理过期上下文
// @Summary 清理过期上下文
// @Description 清理超过指定时间的上下文数据
// @Tags Context
// @Accept json
// @Produce json
// @Param request body ClearExpiredRequest false "清理请求"
// @Success 200 {object} map[string]interface{}
// @Router /api/context/expired [delete]
func (h *ContextHandler) ClearExpired(c *gin.Context) {
	before := time.Now().Add(-24 * time.Hour) // 默认 24 小时前

	var req ClearExpiredRequest
	if c.ShouldBindJSON(&req) == nil && req.Before != "" {
		if parsedTime, err := time.Parse(time.RFC3339, req.Before); err == nil {
			before = parsedTime
		}
	}

	if err := h.contextManager.ClearExpired(before); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Expired contexts cleared successfully",
		"before":  before.Format(time.RFC3339),
	})
}

// GetDialogueContext 获取对话上下文
// @Summary 获取对话上下文
// @Description 获取指定对话的压缩上下文信息
// @Tags Context
// @Produce json
// @Param id path string true "对话 ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/context/dialogue/{id} [get]
func (h *ContextHandler) GetDialogueContext(c *gin.Context) {
	dialogueID := c.Param("id")

	// 获取重要信息
	importantInfo, err := h.contextManager.ExtractImportantInfo(dialogueID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Dialogue context not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"dialogue_id":    dialogueID,
			"important_info": importantInfo,
		},
	})
}

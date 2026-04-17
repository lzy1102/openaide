package handlers

import (
	"net/http"

	"openaide/backend/src/models"
	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

// ConfirmationHandler 确认处理器
type ConfirmationHandler struct {
	confirmationService *services.ConfirmationService
}

// NewConfirmationHandler 创建确认处理器
func NewConfirmationHandler(confirmationService *services.ConfirmationService) *ConfirmationHandler {
	return &ConfirmationHandler{confirmationService: confirmationService}
}

// ListConfirmations 列出所有确认请求
func (h *ConfirmationHandler) ListConfirmations(c *gin.Context) {
	confirmations, err := h.confirmationService.ListConfirmations()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, confirmations)
}

// CreateConfirmation 创建确认请求
func (h *ConfirmationHandler) CreateConfirmation(c *gin.Context) {
	var confirmation models.Confirmation
	if err := c.ShouldBindJSON(&confirmation); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.confirmationService.CreateConfirmation(&confirmation); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, confirmation)
}

// GetConfirmation 获取确认请求详情
func (h *ConfirmationHandler) GetConfirmation(c *gin.Context) {
	id := c.Param("id")
	confirmation, err := h.confirmationService.GetConfirmation(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Confirmation not found"})
		return
	}
	c.JSON(http.StatusOK, confirmation)
}

// DeleteConfirmation 删除确认请求
func (h *ConfirmationHandler) DeleteConfirmation(c *gin.Context) {
	id := c.Param("id")
	if err := h.confirmationService.DeleteConfirmation(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Confirmation deleted successfully"})
}

// ConfirmTask 确认任务
func (h *ConfirmationHandler) ConfirmTask(c *gin.Context) {
	id := c.Param("id")
	confirmation, err := h.confirmationService.ConfirmTask(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, confirmation)
}

// RejectTask 拒绝任务
func (h *ConfirmationHandler) RejectTask(c *gin.Context) {
	id := c.Param("id")
	confirmation, err := h.confirmationService.RejectTask(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, confirmation)
}

// RegisterRoutes 注册路由
func (h *ConfirmationHandler) RegisterRoutes(r *gin.RouterGroup) {
	confirmations := r.Group("/confirmations")
	{
		confirmations.GET("", h.ListConfirmations)
		confirmations.POST("", h.CreateConfirmation)
		confirmations.GET("/:id", h.GetConfirmation)
		confirmations.DELETE("/:id", h.DeleteConfirmation)
		confirmations.POST("/:id/confirm", h.ConfirmTask)
		confirmations.POST("/:id/reject", h.RejectTask)
	}
}

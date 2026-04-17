package handlers

import (
	"net/http"

	"openaide/backend/src/models"
	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

// AutomationHandler 自动化处理器
type AutomationHandler struct {
	automationService *services.AutomationService
}

// NewAutomationHandler 创建自动化处理器
func NewAutomationHandler(automationService *services.AutomationService) *AutomationHandler {
	return &AutomationHandler{automationService: automationService}
}

// ListExecutions 列出自动化执行记录
func (h *AutomationHandler) ListExecutions(c *gin.Context) {
	executions, err := h.automationService.ListAutomationExecutions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, executions)
}

// CreateExecution 创建自动化执行
func (h *AutomationHandler) CreateExecution(c *gin.Context) {
	var execution models.AutomationExecution
	if err := c.ShouldBindJSON(&execution); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.automationService.CreateAutomationExecution(&execution); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, execution)
}

// GetExecution 获取自动化执行详情
func (h *AutomationHandler) GetExecution(c *gin.Context) {
	id := c.Param("id")
	execution, err := h.automationService.GetAutomationExecution(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Automation execution not found"})
		return
	}
	c.JSON(http.StatusOK, execution)
}

// DeleteExecution 删除自动化执行
func (h *AutomationHandler) DeleteExecution(c *gin.Context) {
	id := c.Param("id")
	if err := h.automationService.DeleteAutomationExecution(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Automation execution deleted successfully"})
}

// ExecuteExecution 执行自动化
func (h *AutomationHandler) ExecuteExecution(c *gin.Context) {
	id := c.Param("id")
	execution, err := h.automationService.ExecuteAutomationExecution(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, execution)
}

// RegisterRoutes 注册路由
func (h *AutomationHandler) RegisterRoutes(r *gin.RouterGroup) {
	automation := r.Group("/automation")
	{
		automation.GET("/executions", h.ListExecutions)
		automation.POST("/executions", h.CreateExecution)
		automation.GET("/executions/:id", h.GetExecution)
		automation.DELETE("/executions/:id", h.DeleteExecution)
		automation.POST("/executions/:id/execute", h.ExecuteExecution)
	}
}

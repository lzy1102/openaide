package handlers

import (
	"net/http"

	"openaide/backend/src/models"
	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

// CodeHandler 代码执行处理器
type CodeHandler struct {
	codeService *services.CodeService
}

// NewCodeHandler 创建代码执行处理器
func NewCodeHandler(codeService *services.CodeService) *CodeHandler {
	return &CodeHandler{codeService: codeService}
}

// ListExecutions 列出代码执行记录
func (h *CodeHandler) ListExecutions(c *gin.Context) {
	executions, err := h.codeService.ListCodeExecutions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, executions)
}

// CreateExecution 创建代码执行
func (h *CodeHandler) CreateExecution(c *gin.Context) {
	var execution models.CodeExecution
	if err := c.ShouldBindJSON(&execution); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.codeService.CreateCodeExecution(&execution); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, execution)
}

// GetExecution 获取代码执行详情
func (h *CodeHandler) GetExecution(c *gin.Context) {
	id := c.Param("id")
	execution, err := h.codeService.GetCodeExecution(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Code execution not found"})
		return
	}
	c.JSON(http.StatusOK, execution)
}

// DeleteExecution 删除代码执行
func (h *CodeHandler) DeleteExecution(c *gin.Context) {
	id := c.Param("id")
	if err := h.codeService.DeleteCodeExecution(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Code execution deleted successfully"})
}

// ExecuteExecution 执行代码
func (h *CodeHandler) ExecuteExecution(c *gin.Context) {
	id := c.Param("id")
	execution, err := h.codeService.ExecuteCodeExecution(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, execution)
}

// RegisterRoutes 注册路由
func (h *CodeHandler) RegisterRoutes(r *gin.RouterGroup) {
	code := r.Group("/code")
	{
		code.GET("/executions", h.ListExecutions)
		code.POST("/executions", h.CreateExecution)
		code.GET("/executions/:id", h.GetExecution)
		code.DELETE("/executions/:id", h.DeleteExecution)
		code.POST("/executions/:id/execute", h.ExecuteExecution)
	}
}

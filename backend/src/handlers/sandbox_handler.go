package handlers

import (
	"net/http"

	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

// SandboxHandler 沙箱处理器
type SandboxHandler struct {
	sandboxService *services.SandboxService
}

// NewSandboxHandler 创建沙箱处理器
func NewSandboxHandler(sandboxService *services.SandboxService) *SandboxHandler {
	return &SandboxHandler{sandboxService: sandboxService}
}

// GetStatus 获取沙箱状态
func (h *SandboxHandler) GetStatus(c *gin.Context) {
	c.JSON(http.StatusOK, h.sandboxService.GetStatus())
}

// GetLanguages 获取支持的语言列表
func (h *SandboxHandler) GetLanguages(c *gin.Context) {
	c.JSON(http.StatusOK, h.sandboxService.SupportedLanguages())
}

// Execute 在沙箱中执行代码
func (h *SandboxHandler) Execute(c *gin.Context) {
	var req struct {
		Language string `json:"language" binding:"required"`
		Code     string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := services.ValidateCodeSafety(req.Code); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.sandboxService.Execute(c.Request.Context(), req.Language, req.Code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// RegisterRoutes 注册路由
func (h *SandboxHandler) RegisterRoutes(r *gin.RouterGroup) {
	sandbox := r.Group("/sandbox")
	{
		sandbox.GET("/status", h.GetStatus)
		sandbox.GET("/languages", h.GetLanguages)
		sandbox.POST("/execute", h.Execute)
	}
}

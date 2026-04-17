package handlers

import (
	"net/http"

	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

// MultiAgentHandler 多Agent协作处理器
type MultiAgentHandler struct {
	multiAgentService *services.MultiAgentService
}

// NewMultiAgentHandler 创建多Agent处理器
func NewMultiAgentHandler(multiAgentService *services.MultiAgentService) *MultiAgentHandler {
	return &MultiAgentHandler{multiAgentService: multiAgentService}
}

// Collaborate 协作处理请求
func (h *MultiAgentHandler) Collaborate(c *gin.Context) {
	var req struct {
		Request   string `json:"request" binding:"required"`
		MaxRounds int    `json:"max_rounds"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.MaxRounds <= 0 {
		req.MaxRounds = 2
	}
	result, err := h.multiAgentService.CollaborativeProcess(c.Request.Context(), req.Request, req.MaxRounds)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetRoles 获取Agent角色配置
func (h *MultiAgentHandler) GetRoles(c *gin.Context) {
	c.JSON(http.StatusOK, h.multiAgentService.DefaultAgentConfigs())
}

// RunAgent 运行单个Agent
func (h *MultiAgentHandler) RunAgent(c *gin.Context) {
	var req struct {
		Role        string `json:"role" binding:"required"`
		Input       string `json:"input" binding:"required"`
		CustomPrompt string `json:"custom_prompt"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.multiAgentService.RunSingleAgent(c.Request.Context(), services.AgentRole(req.Role), req.Input, req.CustomPrompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// RegisterRoutes 注册路由
func (h *MultiAgentHandler) RegisterRoutes(r *gin.RouterGroup) {
	agents := r.Group("/agents")
	{
		agents.POST("/collaborate", h.Collaborate)
		agents.GET("/roles", h.GetRoles)
		agents.POST("/run", h.RunAgent)
	}
}

package handlers

import (
	"encoding/json"
	"net/http"

	"openaide/backend/src/models"
	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

// ToolHandler 工具处理器
type ToolHandler struct {
	toolService *services.ToolService
}

// NewToolHandler 创建工具处理器
func NewToolHandler(toolService *services.ToolService) *ToolHandler {
	return &ToolHandler{
		toolService: toolService,
	}
}

// ListTools 列出所有工具
func (h *ToolHandler) ListTools(c *gin.Context) {
	tools, err := h.toolService.ListTools()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tools": tools,
		"count": len(tools),
	})
}

// GetTool 获取工具详情
func (h *ToolHandler) GetTool(c *gin.Context) {
	name := c.Param("name")

	tool, err := h.toolService.GetTool(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, tool)
}

// CreateTool 创建工具
func (h *ToolHandler) CreateTool(c *gin.Context) {
	var tool models.Tool
	if err := c.ShouldBindJSON(&tool); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.toolService.RegisterTool(&tool); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, tool)
}

// ExecuteTool 执行工具
func (h *ToolHandler) ExecuteTool(c *gin.Context) {
	var req struct {
		Name       string                 `json:"name" binding:"required"`
		Arguments  map[string]interface{} `json:"arguments"`
		DialogueID string                 `json:"dialogue_id"`
		MessageID  string                 `json:"message_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 将参数序列化为 JSON 字符串
	argsJSON, _ := json.Marshal(req.Arguments)

	toolCall := &models.ToolCall{
		ID:        "",
		Name:      req.Name,
		Arguments: string(argsJSON),
	}

	userID, _ := c.Get("user_id")
	result, err := h.toolService.ExecuteTool(
		c.Request.Context(),
		toolCall,
		req.DialogueID,
		req.MessageID,
		userID.(string),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetToolDefinitions 获取工具定义列表 (供 LLM 使用)
func (h *ToolHandler) GetToolDefinitions(c *gin.Context) {
	definitions := h.toolService.GetToolDefinitions()
	c.JSON(http.StatusOK, gin.H{
		"tools": definitions,
		"count": len(definitions),
	})
}

// ListPendingConfirmations 列出待确认的危险命令
func (h *ToolHandler) ListPendingConfirmations(c *gin.Context) {
	pending := h.toolService.ListPendingConfirmations()
	c.JSON(http.StatusOK, gin.H{
		"confirmations": pending,
		"count":        len(pending),
	})
}

// ApproveCommand 确认批准危险命令
func (h *ToolHandler) ApproveCommand(c *gin.Context) {
	id := c.Param("id")

	if err := h.toolService.ApproveCommand(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Command approved",
		"id":      id,
	})
}

// RejectCommand 拒绝危险命令
func (h *ToolHandler) RejectCommand(c *gin.Context) {
	id := c.Param("id")

	if err := h.toolService.RejectCommand(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Command rejected",
		"id":      id,
	})
}

// RegisterRoutes 注册路由
func (h *ToolHandler) RegisterRoutes(r *gin.RouterGroup) {
	tools := r.Group("/tools")
	{
		tools.GET("", h.ListTools)
		tools.GET("/definitions", h.GetToolDefinitions)
		tools.GET("/:name", h.GetTool)
		tools.POST("", h.CreateTool)
		tools.POST("/execute", h.ExecuteTool)

		// 危险命令确认
		tools.GET("/confirmations", h.ListPendingConfirmations)
		tools.POST("/confirmations/:id/approve", h.ApproveCommand)
		tools.POST("/confirmations/:id/reject", h.RejectCommand)
	}
}

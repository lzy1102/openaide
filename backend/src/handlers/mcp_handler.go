package handlers

import (
	"net/http"

	"openaide/backend/src/services/mcp"

	"github.com/gin-gonic/gin"
)

// MCPHandler MCP 管理 API 处理器
type MCPHandler struct {
	mcpManager *mcp.MCPManager
}

// NewMCPHandler 创建 MCP 处理器
func NewMCPHandler(mcpManager *mcp.MCPManager) *MCPHandler {
	return &MCPHandler{
		mcpManager: mcpManager,
	}
}

// ListServers 列出所有 MCP Server
func (h *MCPHandler) ListServers(c *gin.Context) {
	servers := h.mcpManager.ListServers()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    servers,
		"count":   len(servers),
	})
}

// AddServerRequest 添加 MCP Server 请求
type AddServerRequest struct {
	Name      string `json:"name" binding:"required"`
	Transport string `json:"transport" binding:"required"` // stdio, sse
	Command   string `json:"command"`                   // stdio 模式
	Args      string `json:"args"`                      // JSON array string
	URL       string `json:"url"`                       // SSE 模式
	Env       string `json:"env"`                       // JSON map string
}

// AddServer 添加 MCP Server
func (h *MCPHandler) AddServer(c *gin.Context) {
	var req AddServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	config := mcp.MCPServerConfig{
		Name:      req.Name,
		Transport: req.Transport,
		Command:   req.Command,
		Args:      req.Args,
		URL:       req.URL,
		Env:       req.Env,
		Enabled:   true,
	}

	client, err := h.mcpManager.AddServer(c.Request.Context(), config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data": gin.H{
			"id":         client.ID,
			"name":       client.Name,
			"tools":      len(client.Tools),
			"connected":  client.IsConnected(),
		},
	})
}

// RemoveServer 移除 MCP Server
func (h *MCPHandler) RemoveServer(c *gin.Context) {
	id := c.Param("id")

	if err := h.mcpManager.RemoveServer(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "MCP server removed",
	})
}

// ListServerTools 列出 Server 的工具
func (h *MCPHandler) ListServerTools(c *gin.Context) {
	id := c.Param("id")

	client, err := h.mcpManager.GetServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	tools := client.GetTools()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"server":  client.Name,
		"data":    tools,
		"count":   len(tools),
	})
}

// RefreshServerTools 重新发现工具
func (h *MCPHandler) RefreshServerTools(c *gin.Context) {
	id := c.Param("id")

	tools, err := h.mcpManager.RefreshTools(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tools,
		"count":   len(tools),
	})
}

// ReconnectServer 重新连接 Server
func (h *MCPHandler) ReconnectServer(c *gin.Context) {
	id := c.Param("id")

	if err := h.mcpManager.Reconnect(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "MCP server reconnected",
	})
}

// RegisterRoutes 注册路由
func (h *MCPHandler) RegisterRoutes(r *gin.RouterGroup) {
	mcpGroup := r.Group("/mcp")
	{
		mcpGroup.GET("/servers", h.ListServers)
		mcpGroup.POST("/servers", h.AddServer)
		mcpGroup.DELETE("/servers/:id", h.RemoveServer)
		mcpGroup.GET("/servers/:id/tools", h.ListServerTools)
		mcpGroup.POST("/servers/:id/refresh", h.RefreshServerTools)
		mcpGroup.POST("/servers/:id/reconnect", h.ReconnectServer)
	}
}

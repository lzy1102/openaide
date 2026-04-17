package handlers

import (
	"net/http"

	"openaide/backend/src/models"
	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

// PluginHandler 插件处理器
type PluginHandler struct {
	pluginService *services.PluginService
}

// NewPluginHandler 创建插件处理器
func NewPluginHandler(pluginService *services.PluginService) *PluginHandler {
	return &PluginHandler{pluginService: pluginService}
}

// ListPlugins 列出所有插件
func (h *PluginHandler) ListPlugins(c *gin.Context) {
	plugins, err := h.pluginService.ListPlugins()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, plugins)
}

// CreatePlugin 创建插件
func (h *PluginHandler) CreatePlugin(c *gin.Context) {
	var plugin models.Plugin
	if err := c.ShouldBindJSON(&plugin); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.pluginService.CreatePlugin(&plugin); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, plugin)
}

// GetPlugin 获取插件详情
func (h *PluginHandler) GetPlugin(c *gin.Context) {
	id := c.Param("id")
	plugin, err := h.pluginService.GetPlugin(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Plugin not found"})
		return
	}
	c.JSON(http.StatusOK, plugin)
}

// UpdatePlugin 更新插件
func (h *PluginHandler) UpdatePlugin(c *gin.Context) {
	id := c.Param("id")
	var plugin models.Plugin
	if err := c.ShouldBindJSON(&plugin); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	plugin.ID = id
	if err := h.pluginService.UpdatePlugin(&plugin); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, plugin)
}

// DeletePlugin 删除插件
func (h *PluginHandler) DeletePlugin(c *gin.Context) {
	id := c.Param("id")
	if err := h.pluginService.DeletePlugin(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Plugin deleted successfully"})
}

// InstallPlugin 安装插件
func (h *PluginHandler) InstallPlugin(c *gin.Context) {
	var plugin models.Plugin
	if err := c.ShouldBindJSON(&plugin); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.pluginService.InstallPlugin(&plugin); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, plugin)
}

// EnablePlugin 启用插件
func (h *PluginHandler) EnablePlugin(c *gin.Context) {
	id := c.Param("id")
	if err := h.pluginService.EnablePlugin(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Plugin enabled successfully"})
}

// DisablePlugin 禁用插件
func (h *PluginHandler) DisablePlugin(c *gin.Context) {
	id := c.Param("id")
	if err := h.pluginService.DisablePlugin(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Plugin disabled successfully"})
}

// CreatePluginInstance 创建插件实例
func (h *PluginHandler) CreatePluginInstance(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Config map[string]interface{} `json:"config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	instance, err := h.pluginService.CreatePluginInstance(id, req.Config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, instance)
}

// ExecutePluginInstance 执行插件实例
func (h *PluginHandler) ExecutePluginInstance(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Parameters map[string]interface{} `json:"parameters"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	execution, err := h.pluginService.ExecutePlugin(id, req.Parameters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, execution)
}

// RegisterRoutes 注册路由
func (h *PluginHandler) RegisterRoutes(r *gin.RouterGroup) {
	plugins := r.Group("/plugins")
	{
		plugins.GET("", h.ListPlugins)
		plugins.POST("", h.CreatePlugin)
		plugins.GET("/:id", h.GetPlugin)
		plugins.PUT("/:id", h.UpdatePlugin)
		plugins.DELETE("/:id", h.DeletePlugin)
		plugins.POST("/install", h.InstallPlugin)
		plugins.POST("/:id/enable", h.EnablePlugin)
		plugins.POST("/:id/disable", h.DisablePlugin)
		plugins.POST("/:id/instances", h.CreatePluginInstance)
		plugins.POST("/instances/:id/execute", h.ExecutePluginInstance)
	}
}

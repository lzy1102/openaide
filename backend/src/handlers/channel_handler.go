package handlers

import (
	"net/http"

	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

// ChannelHandler 渠道处理器
type ChannelHandler struct {
	registry *services.ChannelRegistry
}

// NewChannelHandler 创建渠道处理器
func NewChannelHandler(registry *services.ChannelRegistry) *ChannelHandler {
	return &ChannelHandler{registry: registry}
}

// ListChannels 获取渠道状态
func (h *ChannelHandler) ListChannels(c *gin.Context) {
	c.JSON(http.StatusOK, h.registry.GetStatus())
}

// ListEnabledChannels 获取已启用的渠道
func (h *ChannelHandler) ListEnabledChannels(c *gin.Context) {
	channels := h.registry.ListEnabled()
	result := make([]map[string]interface{}, 0, len(channels))
	for _, ch := range channels {
		result = append(result, map[string]interface{}{
			"type":    string(ch.Type()),
			"enabled": true,
		})
	}
	c.JSON(http.StatusOK, result)
}

// RegisterRoutes 注册路由
func (h *ChannelHandler) RegisterRoutes(r *gin.RouterGroup) {
	channels := r.Group("/channels")
	{
		channels.GET("", h.ListChannels)
		channels.GET("/enabled", h.ListEnabledChannels)
	}
}

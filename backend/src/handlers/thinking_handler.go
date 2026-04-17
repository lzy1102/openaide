package handlers

import (
	"net/http"

	"openaide/backend/src/models"
	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

// ThinkingHandler 思考处理器
type ThinkingHandler struct {
	thinkingService *services.ThinkingService
}

// NewThinkingHandler 创建思考处理器
func NewThinkingHandler(thinkingService *services.ThinkingService) *ThinkingHandler {
	return &ThinkingHandler{thinkingService: thinkingService}
}

// ListThoughts 列出所有思考
func (h *ThinkingHandler) ListThoughts(c *gin.Context) {
	thoughts, err := h.thinkingService.ListThoughts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, thoughts)
}

// CreateThought 创建思考
func (h *ThinkingHandler) CreateThought(c *gin.Context) {
	var thought models.Thought
	if err := c.ShouldBindJSON(&thought); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.thinkingService.CreateThought(&thought); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, thought)
}

// GetThought 获取思考详情
func (h *ThinkingHandler) GetThought(c *gin.Context) {
	id := c.Param("id")
	thought, err := h.thinkingService.GetThought(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Thought not found"})
		return
	}
	c.JSON(http.StatusOK, thought)
}

// DeleteThought 删除思考
func (h *ThinkingHandler) DeleteThought(c *gin.Context) {
	id := c.Param("id")
	if err := h.thinkingService.DeleteThought(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Thought deleted successfully"})
}

// CreateCorrection 创建纠正
func (h *ThinkingHandler) CreateCorrection(c *gin.Context) {
	id := c.Param("id")
	var correction models.Correction
	if err := c.ShouldBindJSON(&correction); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	correction.ThoughtID = id
	if err := h.thinkingService.CreateCorrection(&correction); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, correction)
}

// ListCorrections 列出纠正
func (h *ThinkingHandler) ListCorrections(c *gin.Context) {
	id := c.Param("id")
	corrections, err := h.thinkingService.ListCorrections(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, corrections)
}

// ResolveCorrection 解决纠正
func (h *ThinkingHandler) ResolveCorrection(c *gin.Context) {
	id := c.Param("id")
	correction, err := h.thinkingService.ResolveCorrection(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, correction)
}

// DeleteCorrection 删除纠正
func (h *ThinkingHandler) DeleteCorrection(c *gin.Context) {
	id := c.Param("id")
	if err := h.thinkingService.DeleteCorrection(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Correction deleted successfully"})
}

// ChainOfThought 思维链推理
func (h *ThinkingHandler) ChainOfThought(c *gin.Context) {
	var req services.ChainOfThoughtRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.thinkingService.ChainOfThought(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// MultiStepReasoning 多步推理
func (h *ThinkingHandler) MultiStepReasoning(c *gin.Context) {
	var req services.MultiStepReasoningRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.thinkingService.MultiStepReasoning(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// TreeOfThought 思维树推理
func (h *ThinkingHandler) TreeOfThought(c *gin.Context) {
	var req services.TreeOfThoughtRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.thinkingService.TreeOfThought(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetVisualization 获取思考可视化
func (h *ThinkingHandler) GetVisualization(c *gin.Context) {
	id := c.Param("id")
	vizType := c.DefaultQuery("type", "tree")
	result, err := h.thinkingService.GenerateVisualization(id, vizType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetTimeline 获取思考时间线
func (h *ThinkingHandler) GetTimeline(c *gin.Context) {
	id := c.Param("id")
	result, err := h.thinkingService.GenerateTimelineVisualization(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// RegisterRoutes 注册路由
func (h *ThinkingHandler) RegisterRoutes(r *gin.RouterGroup) {
	thinking := r.Group("/thinking")
	{
		// 思考 CRUD
		thinking.GET("/thoughts", h.ListThoughts)
		thinking.POST("/thoughts", h.CreateThought)
		thinking.GET("/thoughts/:id", h.GetThought)
		thinking.DELETE("/thoughts/:id", h.DeleteThought)

		// 纠正 CRUD
		thinking.POST("/thoughts/:id/corrections", h.CreateCorrection)
		thinking.GET("/thoughts/:id/corrections", h.ListCorrections)
		thinking.POST("/corrections/:id/resolve", h.ResolveCorrection)
		thinking.DELETE("/corrections/:id", h.DeleteCorrection)

		// 推理模式
		thinking.POST("/cot", h.ChainOfThought)
		thinking.POST("/multi-step", h.MultiStepReasoning)
		thinking.POST("/tree-of-thought", h.TreeOfThought)

		// 可视化
		thinking.GET("/visualizations/:id", h.GetVisualization)
		thinking.GET("/timelines/:id", h.GetTimeline)
	}
}

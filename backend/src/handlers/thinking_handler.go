package handlers

import (
	"net/http"

	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

// ThinkingHandler 思考服务 API 处理器
type ThinkingHandler struct {
	thinkingSvc *services.ThinkingService
}

// NewThinkingHandler 创建思考处理器
func NewThinkingHandler(thinkingSvc *services.ThinkingService) *ThinkingHandler {
	return &ThinkingHandler{
		thinkingSvc: thinkingSvc,
	}
}

// ChainOfThought 思维链推理
// @Summary 思维链推理
// @Description 使用 Chain-of-Thought 方法进行逐步推理
// @Tags thinking
// @Accept json
// @Produce json
// @Param request body services.ChainOfThoughtRequest true "推理请求"
// @Success 200 {object} services.ChainOfThoughtResponse "推理结果"
// @Router /api/thinking/chain-of-thought [post]
func (h *ThinkingHandler) ChainOfThought(c *gin.Context) {
	var req services.ChainOfThoughtRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}

	resp, err := h.thinkingSvc.ChainOfThought(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// MultiStepReasoning 多步推理
// @Summary 多步推理
// @Description 使用多步推理策略（linear/parallel/recursive/tree_search）
// @Tags thinking
// @Accept json
// @Produce json
// @Param request body services.MultiStepReasoningRequest true "推理请求"
// @Success 200 {object} services.MultiStepReasoningResponse "推理结果"
// @Router /api/thinking/multi-step [post]
func (h *ThinkingHandler) MultiStepReasoning(c *gin.Context) {
	var req services.MultiStepReasoningRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}

	resp, err := h.thinkingSvc.MultiStepReasoning(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// TreeOfThought 思维树推理
// @Summary 思维树推理
// @Description 使用 Tree-of-Thought 方法进行多分支探索
// @Tags thinking
// @Accept json
// @Produce json
// @Param request body services.TreeOfThoughtRequest true "推理请求"
// @Success 200 {object} services.TreeOfThoughtResponse "推理结果"
// @Router /api/thinking/tree-of-thought [post]
func (h *ThinkingHandler) TreeOfThought(c *gin.Context) {
	var req services.TreeOfThoughtRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}

	resp, err := h.thinkingSvc.TreeOfThought(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// ListThoughts 列出所有思考记录
// @Summary 列出思考记录
// @Description 获取所有思考记录列表
// @Tags thinking
// @Produce json
// @Success 200 {object} map[string]interface{} "思考记录列表"
// @Router /api/thinking/thoughts [get]
func (h *ThinkingHandler) ListThoughts(c *gin.Context) {
	thoughts, err := h.thinkingSvc.ListThoughts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"thoughts": thoughts,
		"count":    len(thoughts),
	})
}

// ListThoughtsByUser 列出用户的思考记录
// @Summary 列出用户思考记录
// @Description 获取指定用户的思考记录
// @Tags thinking
// @Produce json
// @Param user_id query string true "用户ID"
// @Success 200 {object} map[string]interface{} "思考记录列表"
// @Router /api/thinking/thoughts/user [get]
func (h *ThinkingHandler) ListThoughtsByUser(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}

	thoughts, err := h.thinkingSvc.ListThoughtsByUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"thoughts": thoughts,
		"count":    len(thoughts),
	})
}

// GetThought 获取思考详情
// @Summary 获取思考详情
// @Description 获取指定思考记录的详细信息
// @Tags thinking
// @Produce json
// @Param id path string true "思考ID"
// @Success 200 {object} models.Thought "思考详情"
// @Router /api/thinking/thoughts/:id [get]
func (h *ThinkingHandler) GetThought(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	thought, err := h.thinkingSvc.GetThought(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, thought)
}

// DeleteThought 删除思考记录
// @Summary 删除思考记录
// @Description 删除指定的思考记录
// @Tags thinking
// @Produce json
// @Param id path string true "思考ID"
// @Success 200 {object} map[string]interface{} "删除结果"
// @Router /api/thinking/thoughts/:id [delete]
func (h *ThinkingHandler) DeleteThought(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	if err := h.thinkingSvc.DeleteThought(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GenerateVisualization 生成思考可视化
// @Summary 生成思考可视化
// @Description 为指定思考记录生成可视化数据
// @Tags thinking
// @Produce json
// @Param id path string true "思考ID"
// @Param type query string false "可视化类型" default(tree)
// @Success 200 {object} services.VisualizationData "可视化数据"
// @Router /api/thinking/thoughts/:id/visualization [get]
func (h *ThinkingHandler) GenerateVisualization(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	vizType := c.DefaultQuery("type", "tree")

	viz, err := h.thinkingSvc.GenerateVisualization(id, vizType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, viz)
}

// GenerateTimelineVisualization 生成时间线可视化
// @Summary 生成时间线可视化
// @Description 为指定思考记录生成时间线可视化数据
// @Tags thinking
// @Produce json
// @Param id path string true "思考ID"
// @Success 200 {object} services.VisualizationData "时间线可视化数据"
// @Router /api/thinking/thoughts/:id/timeline [get]
func (h *ThinkingHandler) GenerateTimelineVisualization(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	viz, err := h.thinkingSvc.GenerateTimelineVisualization(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, viz)
}

// RegisterRoutes 注册路由
func (h *ThinkingHandler) RegisterRoutes(r *gin.RouterGroup) {
	thinking := r.Group("/thinking")
	{
		// 推理接口
		thinking.POST("/chain-of-thought", h.ChainOfThought)
		thinking.POST("/multi-step", h.MultiStepReasoning)
		thinking.POST("/tree-of-thought", h.TreeOfThought)

		// 思考记录管理
		thinking.GET("/thoughts", h.ListThoughts)
		thinking.GET("/thoughts/user", h.ListThoughtsByUser)
		thinking.GET("/thoughts/:id", h.GetThought)
		thinking.DELETE("/thoughts/:id", h.DeleteThought)

		// 可视化
		thinking.GET("/thoughts/:id/visualization", h.GenerateVisualization)
		thinking.GET("/thoughts/:id/timeline", h.GenerateTimelineVisualization)
	}
}

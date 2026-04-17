package handlers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"openaide/backend/src/models"
	"openaide/backend/src/services"
)

// FeishuHandler 飞书管理接口处理器
type FeishuHandler struct {
	feishuService *services.FeishuService
	learningSvc   *services.LearningService
	feedbackSvc   *services.FeedbackService
}

// NewFeishuHandler 创建飞书处理器
func NewFeishuHandler(feishuService *services.FeishuService, learningSvc *services.LearningService, feedbackSvc *services.FeedbackService) *FeishuHandler {
	return &FeishuHandler{
		feishuService: feishuService,
		learningSvc:   learningSvc,
		feedbackSvc:   feedbackSvc,
	}
}

// GetStatus 获取飞书服务状态
func (h *FeishuHandler) GetStatus(c *gin.Context) {
	c.JSON(http.StatusOK, h.feishuService.GetStatus())
}

// Start 手动启动飞书服务
func (h *FeishuHandler) Start(c *gin.Context) {
	if h.feishuService.IsRunning() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "feishu service is already running"})
		return
	}
	if err := h.feishuService.Start(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "feishu service started"})
}

// Stop 手动停止飞书服务
func (h *FeishuHandler) Stop(c *gin.Context) {
	h.feishuService.Stop()
	c.JSON(http.StatusOK, gin.H{"message": "feishu service stopped"})
}

// ListSessions 列出飞书会话
func (h *FeishuHandler) ListSessions(c *gin.Context) {
	sessions := h.feishuService.ListSessions()
	c.JSON(http.StatusOK, sessions)
}

// DeleteSession 重置飞书会话
func (h *FeishuHandler) DeleteSession(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session key is required"})
		return
	}
	if err := h.feishuService.DeleteSession(key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "session deleted"})
}

// ListUsers 列出飞书用户
func (h *FeishuHandler) ListUsers(c *gin.Context) {
	users := h.feishuService.ListUsers()
	c.JSON(http.StatusOK, users)
}

// HandleCardCallback 处理飞书卡片按钮回调
func (h *FeishuHandler) HandleCardCallback(c *gin.Context) {
	var req struct {
		Action struct {
			Value struct {
				Action    string `json:"action"`
				DialogueID string `json:"dialogue_id"`
				Rating    int    `json:"rating"`
			} `json:"value"`
		} `json:"action"`
		Operator struct {
			OpenID string `json:"open_id"`
		} `json:"operator"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[Feishu] card callback parse error: %v", err)
		c.JSON(http.StatusOK, gin.H{})
		return
	}

	action := req.Action.Value.Action
	dialogueID := req.Action.Value.DialogueID
	rating := req.Action.Value.Rating
	userID := req.Operator.OpenID

	if action == "" || dialogueID == "" {
		c.JSON(http.StatusOK, gin.H{})
		return
	}

	// 创建 Feedback 记录
	feedback := &models.Feedback{
		ID:        uuid.New().String(),
		TaskID:    dialogueID,
		TaskType:  "dialogue",
		UserID:    userID,
		Rating:    rating,
		Comment:   "",
		CreatedAt: time.Now(),
	}
	if err := h.feedbackSvc.CreateFeedback(feedback); err != nil {
		log.Printf("[Feishu] failed to create feedback: %v", err)
	}

	// 触发自动分析和应用
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := h.learningSvc.AnalyzeAndAutoApply(ctx, dialogueID); err != nil {
			log.Printf("[Feishu] auto-apply failed: %v", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{})
}

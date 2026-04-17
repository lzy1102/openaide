package handlers

import (
	"net/http"

	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

// VoiceHandler 语音处理器
type VoiceHandler struct {
	voiceService *services.VoiceService
}

// NewVoiceHandler 创建语音处理器
func NewVoiceHandler(voiceService *services.VoiceService) *VoiceHandler {
	return &VoiceHandler{voiceService: voiceService}
}

// GetStatus 获取语音服务状态
func (h *VoiceHandler) GetStatus(c *gin.Context) {
	c.JSON(http.StatusOK, h.voiceService.GetStatus())
}

// TextToSpeech 文本转语音
func (h *VoiceHandler) TextToSpeech(c *gin.Context) {
	var req struct {
		Text string `json:"text" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, format, err := h.voiceService.TextToSpeechBase64(c.Request.Context(), req.Text)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"audio": result, "format": format})
}

// SpeechToText 语音转文本
func (h *VoiceHandler) SpeechToText(c *gin.Context) {
	var req struct {
		Audio  string `json:"audio" binding:"required"`
		Format string `json:"format"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Format == "" {
		req.Format = "wav"
	}
	result, err := h.voiceService.SpeechToTextBase64(c.Request.Context(), req.Audio, req.Format)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// RegisterRoutes 注册路由
func (h *VoiceHandler) RegisterRoutes(r *gin.RouterGroup) {
	voice := r.Group("/voice")
	{
		voice.GET("/status", h.GetStatus)
		voice.POST("/tts", h.TextToSpeech)
		voice.POST("/stt", h.SpeechToText)
	}
}

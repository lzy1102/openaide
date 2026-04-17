package handlers

import (
	"net/http"
	"sync"

	"openaide/backend/src/models"
	"openaide/backend/src/services"
	"github.com/gin-gonic/gin"
)

// ExtractionHandler 知识提取 API 处理器
type ExtractionHandler struct {
	extractionService services.KnowledgeExtractionService
	dialogueService   *services.DialogueService
	knowledgeService  services.KnowledgeService
	logger            *services.LoggerService
	config            ExtractionConfig
	configMu          sync.RWMutex
}

// NewExtractionHandler 创建知识提取处理器
func NewExtractionHandler(
	extractionService services.KnowledgeExtractionService,
	dialogueService *services.DialogueService,
	knowledgeService services.KnowledgeService,
	logger *services.LoggerService,
) *ExtractionHandler {
	return &ExtractionHandler{
		extractionService: extractionService,
		dialogueService:   dialogueService,
		knowledgeService:  knowledgeService,
		logger:            logger,
		config: ExtractionConfig{
			MinMessageCount:    3,
			MinRelevanceScore:  0.7,
			AutoExtractEnabled: true,
			MaxExtractionCount: 10,
		},
	}
}

// RegisterRoutes 注册路由
func (h *ExtractionHandler) RegisterRoutes(r *gin.RouterGroup) {
	extraction := r.Group("/extraction")
	{
		extraction.POST("/extract", h.ExtractFromDialogue)
		extraction.POST("/auto", h.AutoExtract)
		extraction.POST("/batch", h.BatchExtract)
		extraction.GET("/config", h.GetConfig)
		extraction.PUT("/config", h.UpdateConfig)
	}
}

// ExtractRequest 从对话提取知识请求
type ExtractRequest struct {
	DialogueID string `json:"dialogue_id" binding:"required"`
	AutoSave   bool   `json:"auto_save"` // 是否自动保存到知识库
	UserID     string `json:"user_id"`    // 用户 ID，自动保存时需要
}

// ExtractFromDialogue 从对话中提取知识
// @Summary 从对话提取知识
// @Description 分析对话内容，自动提取有价值的知识点
// @Tags Extraction
// @Accept json
// @Produce json
// @Param request body ExtractRequest true "提取请求"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/extraction/extract [post]
func (h *ExtractionHandler) ExtractFromDialogue(c *gin.Context) {
	var req ExtractRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// 提取知识
	extracted, err := h.extractionService.ExtractFromDialogue(c.Request.Context(), req.DialogueID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// 如果需要自动保存
	if req.AutoSave && req.UserID != "" && len(extracted) > 0 {
		if err := h.extractionService.AutoSave(c.Request.Context(), extracted, req.UserID); err != nil {
			h.logger.Error(c.Request.Context(), "Failed to auto save extracted knowledge: %v", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"dialogue_id": req.DialogueID,
			"extracted":   extracted,
			"count":       len(extracted),
			"auto_saved":  req.AutoSave,
		},
	})
}

// AutoExtractRequest 自动提取请求
type AutoExtractRequest struct {
	UserID      string `json:"user_id" binding:"required"`
	MinMessages int    `json:"min_messages"` // 最少消息数才触发提取
}

// AutoExtract 自动从用户对话中提取知识
// @Summary 自动提取知识
// @Description 自动扫描用户的对话并提取有价值的知识
// @Tags Extraction
// @Accept json
// @Produce json
// @Param request body AutoExtractRequest true "自动提取请求"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/extraction/auto [post]
func (h *ExtractionHandler) AutoExtract(c *gin.Context) {
	var req AutoExtractRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if req.MinMessages <= 0 {
		req.MinMessages = 5 // 默认最少 5 条消息
	}

	// 获取用户对话列表
	dialogues := h.dialogueService.ListDialoguesByUser(req.UserID)

	totalExtracted := 0
	extractedKnowledge := make([]services.ExtractedKnowledge, 0)

	for _, dialogue := range dialogues {
		// 检查消息数量
		if len(dialogue.Messages) < req.MinMessages {
			continue
		}

		// 提取知识
		extracted, err := h.extractionService.ExtractFromDialogue(c.Request.Context(), dialogue.ID)
		if err != nil {
			h.logger.Error(c.Request.Context(), "Failed to extract from dialogue %s: %v", dialogue.ID, err)
			continue
		}

		// 自动保存到知识库
		if len(extracted) > 0 {
			if err := h.extractionService.AutoSave(c.Request.Context(), extracted, req.UserID); err != nil {
				h.logger.Error(c.Request.Context(), "Failed to auto save knowledge: %v", err)
				continue
			}
			totalExtracted += len(extracted)
		}

		extractedKnowledge = append(extractedKnowledge, extracted...)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"user_id":           req.UserID,
			"dialogues_scanned": len(dialogues),
			"total_extracted":   totalExtracted,
			"knowledge":         extractedKnowledge,
		},
	})
}

// BatchExtractRequest 批量提取请求
type BatchExtractRequest struct {
	DialogueIDs []string `json:"dialogue_ids" binding:"required"`
	AutoSave    bool     `json:"auto_save"`
	UserID      string   `json:"user_id"` // 用户 ID，自动保存时需要
}

// BatchExtract 批量从多个对话中提取知识
// @Summary 批量提取知识
// @Description 从多个对话中批量提取知识
// @Tags Extraction
// @Accept json
// @Produce json
// @Param request body BatchExtractRequest true "批量提取请求"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/extraction/batch [post]
func (h *ExtractionHandler) BatchExtract(c *gin.Context) {
	var req BatchExtractRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	results := make([]map[string]interface{}, 0)
	totalExtracted := 0

	for _, dialogueID := range req.DialogueIDs {
		// 检查对话是否存在
		_, exists := h.dialogueService.GetDialogue(dialogueID)
		if !exists {
			results = append(results, gin.H{
				"dialogue_id": dialogueID,
				"success":     false,
				"error":       "Dialogue not found",
			})
			continue
		}

		extracted, err := h.extractionService.ExtractFromDialogue(c.Request.Context(), dialogueID)
		if err != nil {
			results = append(results, gin.H{
				"dialogue_id": dialogueID,
				"success":     false,
				"error":       err.Error(),
			})
			continue
		}

		// 如果需要自动保存
		saved := 0
		if req.AutoSave && req.UserID != "" && len(extracted) > 0 {
			if err := h.extractionService.AutoSave(c.Request.Context(), extracted, req.UserID); err != nil {
				h.logger.Error(c.Request.Context(), "Failed to auto save knowledge: %v", err)
			} else {
				saved = len(extracted)
			}
		}

		totalExtracted += len(extracted)
		results = append(results, gin.H{
			"dialogue_id": dialogueID,
			"success":     true,
			"extracted":   len(extracted),
			"saved":       saved,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"total_dialogues": len(req.DialogueIDs),
			"total_extracted": totalExtracted,
			"results":         results,
		},
	})
}

// ExtractionConfig 提取配置
type ExtractionConfig struct {
	MinMessageCount    int     `json:"min_message_count"`
	MinRelevanceScore  float64 `json:"min_relevance_score"`
	AutoExtractEnabled bool    `json:"auto_extract_enabled"`
	MaxExtractionCount int     `json:"max_extraction_count"`
}

// GetConfig 获取提取配置
// @Summary 获取提取配置
// @Description 获取当前的知识提取配置
// @Tags Extraction
// @Produce json
// @Success 200 {object} ExtractionConfig
// @Router /api/extraction/config [get]
func (h *ExtractionHandler) GetConfig(c *gin.Context) {
	h.configMu.RLock()
	defer h.configMu.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    h.config,
	})
}

// UpdateConfigRequest 更新配置请求
type UpdateConfigRequest struct {
	MinMessageCount    int     `json:"min_message_count"`
	MinRelevanceScore  float64 `json:"min_relevance_score"`
	AutoExtractEnabled bool    `json:"auto_extract_enabled"`
	MaxExtractionCount int     `json:"max_extraction_count"`
}

// UpdateConfig 更新提取配置
// @Summary 更新提取配置
// @Description 更新知识提取的配置参数
// @Tags Extraction
// @Accept json
// @Produce json
// @Param request body UpdateConfigRequest true "配置请求"
// @Success 200 {object} map[string]interface{}
// @Router /api/extraction/config [put]
func (h *ExtractionHandler) UpdateConfig(c *gin.Context) {
	var req UpdateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	h.configMu.Lock()
	if req.MinMessageCount > 0 {
		h.config.MinMessageCount = req.MinMessageCount
	}
	if req.MinRelevanceScore > 0 {
		h.config.MinRelevanceScore = req.MinRelevanceScore
	}
	h.config.AutoExtractEnabled = req.AutoExtractEnabled
	if req.MaxExtractionCount > 0 {
		h.config.MaxExtractionCount = req.MaxExtractionCount
	}
	h.configMu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Configuration updated successfully",
		"data":    h.config,
	})
}

// convertToKnowledgeModel 将 ExtractedKnowledge 转换为 models.Knowledge
func convertToKnowledgeModel(extracted services.ExtractedKnowledge, userID string) *models.Knowledge {
	return &models.Knowledge{
		Title:       extracted.Title,
		Content:     extracted.Content,
		Summary:     extracted.Summary,
		Source:      extracted.Source,
		Confidence:  extracted.Confidence,
		UserID:      userID,
	}
}

package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"openaide/backend/src/models"
	"openaide/backend/src/services"
	"github.com/gin-gonic/gin"
)

// KnowledgeHandler 知识库 API 处理器
type KnowledgeHandler struct {
	knowledgeService  services.KnowledgeService
	documentService   services.DocumentService
	embeddingService  *services.OpenAIEmbeddingService
	contextManager    services.ContextManager
	ragService        services.RAGService
	logger            *services.LoggerService
}

// NewKnowledgeHandler 创建知识库处理器
func NewKnowledgeHandler(
	knowledgeService services.KnowledgeService,
	documentService services.DocumentService,
	ragService services.RAGService,
	logger *services.LoggerService,
) *KnowledgeHandler {
	return &KnowledgeHandler{
		knowledgeService: knowledgeService,
		documentService:  documentService,
		ragService:       ragService,
		logger:           logger,
	}
}

// RegisterRoutes 注册路由
func (h *KnowledgeHandler) RegisterRoutes(r *gin.RouterGroup) {
	knowledge := r.Group("/knowledge")
	{
		knowledge.GET("", h.ListKnowledge)
		knowledge.POST("", h.CreateKnowledge)
		knowledge.GET("/:id", h.GetKnowledge)
		knowledge.PUT("/:id", h.UpdateKnowledge)
		knowledge.DELETE("/:id", h.DeleteKnowledge)
		knowledge.POST("/search", h.SearchKnowledge)
		knowledge.POST("/hybrid-search", h.HybridSearchKnowledge)
	}

	categories := r.Group("/knowledge/categories")
	{
		categories.GET("", h.ListCategories)
		categories.POST("", h.CreateCategory)
		categories.GET("/:id", h.GetCategory)
		categories.DELETE("/:id", h.DeleteCategory)
	}

	documents := r.Group("/documents")
	{
		documents.GET("", h.ListDocuments)
		documents.POST("/import", h.ImportDocument)
		documents.GET("/:id", h.GetDocument)
		documents.DELETE("/:id", h.DeleteDocument)
	}

	rag := r.Group("/rag")
	{
		rag.POST("/query", h.RAGQuery)
		rag.POST("/stream", h.RAGStream)
	}
}

// ============ 知识条目 ============

// ListKnowledge 列出知识条目
func (h *KnowledgeHandler) ListKnowledge(c *gin.Context) {
	userID := c.Query("user_id")
	categoryID := c.Query("category_id")
	limit := parseIntQuery(c, "limit", 100)
	offset := parseIntQuery(c, "offset", 0)

	knowledges, err := h.knowledgeService.ListKnowledge(userID, categoryID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    knowledges,
		"count":   len(knowledges),
	})
}

// CreateKnowledgeRequest 创建知识请求
type CreateKnowledgeRequest struct {
	Title       string `json:"title" binding:"required"`
	Content     string `json:"content" binding:"required"`
	Summary     string `json:"summary"`
	CategoryID  string `json:"category_id"`
	Source      string `json:"source"`
	SourceID    string `json:"source_id"`
	UserID      string `json:"user_id" binding:"required"`
}

// CreateKnowledge 创建知识条目
func (h *KnowledgeHandler) CreateKnowledge(c *gin.Context) {
	var req CreateKnowledgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 创建知识并生成向量
	knowledge, err := h.knowledgeService.CreateKnowledgeWithEmbedding(
		c.Request.Context(),
		req.Title,
		req.Content,
		req.Summary,
		req.CategoryID,
		req.Source,
		req.SourceID,
		req.UserID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    knowledge,
	})
}

// GetKnowledge 获取知识条目
func (h *KnowledgeHandler) GetKnowledge(c *gin.Context) {
	id := c.Param("id")

	knowledge, err := h.knowledgeService.GetKnowledge(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Knowledge not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    knowledge,
	})
}

// UpdateKnowledgeRequest 更新知识请求
type UpdateKnowledgeRequest struct {
	Title      string `json:"title"`
	Content    string `json:"content"`
	Summary    string `json:"summary"`
	CategoryID string `json:"category_id"`
}

// UpdateKnowledge 更新知识条目
func (h *KnowledgeHandler) UpdateKnowledge(c *gin.Context) {
	id := c.Param("id")

	var req UpdateKnowledgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	knowledge, err := h.knowledgeService.GetKnowledge(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Knowledge not found"})
		return
	}

	if req.Title != "" {
		knowledge.Title = req.Title
	}
	if req.Content != "" {
		knowledge.Content = req.Content
	}
	if req.Summary != "" {
		knowledge.Summary = req.Summary
	}
	if req.CategoryID != "" {
		knowledge.CategoryID = req.CategoryID
	}

	if err := h.knowledgeService.UpdateKnowledge(knowledge); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    knowledge,
	})
}

// DeleteKnowledge 删除知识条目
func (h *KnowledgeHandler) DeleteKnowledge(c *gin.Context) {
	id := c.Param("id")

	if err := h.knowledgeService.DeleteKnowledge(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Knowledge deleted successfully",
	})
}

// SearchKnowledgeRequest 搜索知识请求
type SearchKnowledgeRequest struct {
	Query string `json:"query" binding:"required"`
	Limit int    `json:"limit"`
}

// SearchKnowledge 语义搜索知识
func (h *KnowledgeHandler) SearchKnowledge(c *gin.Context) {
	var req SearchKnowledgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Limit <= 0 {
		req.Limit = 10
	}

	results, err := h.knowledgeService.SearchKnowledge(c.Request.Context(), req.Query, req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    results,
		"query":   req.Query,
		"count":   len(results),
	})
}

// HybridSearchKnowledge 混合搜索知识
func (h *KnowledgeHandler) HybridSearchKnowledge(c *gin.Context) {
	var req SearchKnowledgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Limit <= 0 {
		req.Limit = 10
	}

	results, err := h.knowledgeService.HybridSearchKnowledge(c.Request.Context(), req.Query, req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    results,
		"query":   req.Query,
		"count":   len(results),
	})
}

// ============ 分类管理 ============

// ListCategories 列出分类
func (h *KnowledgeHandler) ListCategories(c *gin.Context) {
	userID := c.Query("user_id")

	categories, err := h.knowledgeService.ListCategories(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    categories,
	})
}

// CreateCategoryRequest 创建分类请求
type CreateCategoryRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	ParentID    string `json:"parent_id"`
	UserID      string `json:"user_id" binding:"required"`
}

// CreateCategory 创建分类
func (h *KnowledgeHandler) CreateCategory(c *gin.Context) {
	var req CreateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	category := &models.KnowledgeCategory{
		Name:        req.Name,
		Description: req.Description,
		ParentID:    req.ParentID,
		UserID:      req.UserID,
	}

	if err := h.knowledgeService.CreateCategory(category); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    category,
	})
}

// GetCategory 获取分类
func (h *KnowledgeHandler) GetCategory(c *gin.Context) {
	id := c.Param("id")

	category, err := h.knowledgeService.GetCategory(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    category,
	})
}

// DeleteCategory 删除分类
func (h *KnowledgeHandler) DeleteCategory(c *gin.Context) {
	id := c.Param("id")
	if err := h.knowledgeService.DeleteCategory(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Category deleted",
	})
}

// ============ 文档管理 ============

// ListDocuments 列出文档
func (h *KnowledgeHandler) ListDocuments(c *gin.Context) {
	userID := c.Query("user_id")

	documents, err := h.knowledgeService.ListDocuments(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    documents,
	})
}

// ImportDocumentRequest 导入文档请求
type ImportDocumentRequest struct {
	Title    string `json:"title" binding:"required"`
	FileType string `json:"file_type" binding:"required"` // txt, md, pdf, docx
	Content  string `json:"content" binding:"required"`
	UserID   string `json:"user_id" binding:"required"`
}

// ImportDocument 导入文档
func (h *KnowledgeHandler) ImportDocument(c *gin.Context) {
	var req ImportDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	importReq := services.ImportRequest{
		Title:    req.Title,
		FileType: req.FileType,
		Content:  []byte(req.Content),
		UserID:   req.UserID,
	}

	result, err := h.documentService.ImportDocument(c.Request.Context(), importReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetDocument 获取文档
func (h *KnowledgeHandler) GetDocument(c *gin.Context) {
	id := c.Param("id")

	document, err := h.knowledgeService.GetDocument(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    document,
	})
}

// DeleteDocument 删除文档
func (h *KnowledgeHandler) DeleteDocument(c *gin.Context) {
	id := c.Param("id")

	if err := h.documentService.DeleteDocument(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Document deleted successfully",
	})
}

// ============ RAG 查询 ============

// RAGQueryRequest RAG 查询请求
type RAGQueryRequest struct {
	Query           string  `json:"query" binding:"required"`
	TopK            int     `json:"top_k"`
	MaxContextTokens int    `json:"max_context_tokens"`
	MinScore        float64 `json:"min_score"`
	Temperature     float64 `json:"temperature"`
	IncludeSources  bool    `json:"include_sources"`
}

// RAGQuery RAG 查询
func (h *KnowledgeHandler) RAGQuery(c *gin.Context) {
	var req RAGQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	options := services.RAGOptions{
		TopK:            req.TopK,
		MaxContextTokens: req.MaxContextTokens,
		MinScore:        req.MinScore,
		Temperature:     req.Temperature,
		IncludeSources:  req.IncludeSources,
	}

	response, err := h.ragService.RetrieveAndGenerate(c.Request.Context(), req.Query, options)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
	})
}

// RAGStream RAG 流式查询
func (h *KnowledgeHandler) RAGStream(c *gin.Context) {
	var req RAGQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	options := services.RAGOptions{
		TopK:            req.TopK,
		MaxContextTokens: req.MaxContextTokens,
		MinScore:        req.MinScore,
		Temperature:     req.Temperature,
		IncludeSources:  req.IncludeSources,
	}

	stream, err := h.ragService.RetrieveAndGenerateStream(c.Request.Context(), req.Query, options)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 设置 SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	for chunk := range stream {
		if chunk.Error != nil {
			c.SSEvent("error", gin.H{"error": chunk.Error.Error()})
			break
		}

		data, _ := json.Marshal(chunk)
		c.SSEvent("message", string(data))
		c.Writer.Flush()
	}
}

// parseIntQuery 解析整数查询参数
func parseIntQuery(c *gin.Context, key string, defaultValue int) int {
	if val := c.Query(key); val != "" {
		var result int
		if _, err := fmt.Sscanf(val, "%d", &result); err == nil {
			return result
		}
	}
	return defaultValue
}

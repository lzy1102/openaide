package handlers

import (
	"net/http"

	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

// CodeHandler 代码服务 API 处理器
type CodeHandler struct {
	codeSvc *services.CodeService
}

// NewCodeHandler 创建代码处理器
func NewCodeHandler(codeSvc *services.CodeService) *CodeHandler {
	return &CodeHandler{
		codeSvc: codeSvc,
	}
}

// AnalyzeCode 代码分析
// @Summary 代码分析
// @Description 分析代码质量、复杂度、安全性和潜在问题
// @Tags code
// @Accept json
// @Produce json
// @Param request body services.CodeAnalysisRequest true "分析请求"
// @Success 200 {object} services.CodeAnalysisResponse "分析结果"
// @Router /api/code/analyze [post]
func (h *CodeHandler) AnalyzeCode(c *gin.Context) {
	var req services.CodeAnalysisRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.codeSvc.AnalyzeCode(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// GenerateCode 代码生成
// @Summary 代码生成
// @Description 根据描述生成代码
// @Tags code
// @Accept json
// @Produce json
// @Param request body services.CodeGenerationRequest true "生成请求"
// @Success 200 {object} services.CodeGenerationResponse "生成结果"
// @Router /api/code/generate [post]
func (h *CodeHandler) GenerateCode(c *gin.Context) {
	var req services.CodeGenerationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.codeSvc.GenerateCode(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// RefactorCode 代码重构
// @Summary 代码重构
// @Description 对代码进行重构优化
// @Tags code
// @Accept json
// @Produce json
// @Param request body services.CodeRefactorRequest true "重构请求"
// @Success 200 {object} services.CodeRefactorResponse "重构结果"
// @Router /api/code/refactor [post]
func (h *CodeHandler) RefactorCode(c *gin.Context) {
	var req services.CodeRefactorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.codeSvc.RefactorCode(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// ReviewCode 代码审查
// @Summary 代码审查
// @Description 对代码进行审查评分
// @Tags code
// @Accept json
// @Produce json
// @Param request body services.CodeReviewRequest true "审查请求"
// @Success 200 {object} services.CodeReviewResponse "审查结果"
// @Router /api/code/review [post]
func (h *CodeHandler) ReviewCode(c *gin.Context) {
	var req services.CodeReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.codeSvc.ReviewCode(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// ExecuteCode 代码执行
// @Summary 代码执行
// @Description 执行代码并返回结果（支持 Python/JS/Go/Bash）
// @Tags code
// @Accept json
// @Produce json
// @Param request body services.CodeExecutionRequest true "执行请求"
// @Success 200 {object} services.CodeExecutionResponse "执行结果"
// @Router /api/code/execute [post]
func (h *CodeHandler) ExecuteCode(c *gin.Context) {
	var req services.CodeExecutionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.codeSvc.ExecuteCode(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// ExplainCode 代码解释
// @Summary 代码解释
// @Description 解释代码的功能和逻辑
// @Tags code
// @Accept json
// @Produce json
// @Param request body services.ExplainCodeRequest true "解释请求"
// @Success 200 {object} services.ExplainCodeResponse "解释结果"
// @Router /api/code/explain [post]
func (h *CodeHandler) ExplainCode(c *gin.Context) {
	var req services.ExplainCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.codeSvc.ExplainCode(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// RegisterRoutes 注册路由
func (h *CodeHandler) RegisterRoutes(r *gin.RouterGroup) {
	code := r.Group("/code")
	{
		code.POST("/analyze", h.AnalyzeCode)
		code.POST("/generate", h.GenerateCode)
		code.POST("/refactor", h.RefactorCode)
		code.POST("/review", h.ReviewCode)
		code.POST("/execute", h.ExecuteCode)
		code.POST("/explain", h.ExplainCode)
	}
}

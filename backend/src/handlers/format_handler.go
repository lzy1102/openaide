package handlers

import (
	"net/http"

	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

type FormatHandler struct {
	formatSvc *services.FormatService
}

func NewFormatHandler(formatSvc *services.FormatService) *FormatHandler {
	return &FormatHandler{
		formatSvc: formatSvc,
	}
}

func (h *FormatHandler) FormatCode(c *gin.Context) {
	var req services.FormatCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.formatSvc.FormatCode(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *FormatHandler) LintCode(c *gin.Context) {
	var req services.LintCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.formatSvc.LintCode(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *FormatHandler) FixStyleIssues(c *gin.Context) {
	var req services.FixStyleIssuesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.formatSvc.FixStyleIssues(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *FormatHandler) DetectCodeSmells(c *gin.Context) {
	var req services.DetectCodeSmellsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.formatSvc.DetectCodeSmells(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *FormatHandler) RegisterRoutes(r *gin.RouterGroup) {
	format := r.Group("/format")
	{
		format.POST("/format", h.FormatCode)
		format.POST("/lint", h.LintCode)
		format.POST("/fix", h.FixStyleIssues)
		format.POST("/code-smells", h.DetectCodeSmells)
	}
}

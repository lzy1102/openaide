package handlers

import (
	"net/http"

	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

type CodeSearchHandler struct {
	codeSearchSvc *services.CodeSearchService
}

func NewCodeSearchHandler(codeSearchSvc *services.CodeSearchService) *CodeSearchHandler {
	return &CodeSearchHandler{
		codeSearchSvc: codeSearchSvc,
	}
}

func (h *CodeSearchHandler) SearchCode(c *gin.Context) {
	var req services.SearchCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.codeSearchSvc.SearchCode(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *CodeSearchHandler) FindSimilarCode(c *gin.Context) {
	var req services.FindSimilarCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.codeSearchSvc.FindSimilarCode(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *CodeSearchHandler) AnalyzeCodeStructure(c *gin.Context) {
	var req services.AnalyzeCodeStructureRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.codeSearchSvc.AnalyzeCodeStructure(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *CodeSearchHandler) FindDeadCode(c *gin.Context) {
	var req services.FindDeadCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.codeSearchSvc.FindDeadCode(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *CodeSearchHandler) RegisterRoutes(r *gin.RouterGroup) {
	codeSearch := r.Group("/code-search")
	{
		codeSearch.POST("/search", h.SearchCode)
		codeSearch.POST("/similar", h.FindSimilarCode)
		codeSearch.POST("/structure", h.AnalyzeCodeStructure)
		codeSearch.POST("/dead-code", h.FindDeadCode)
	}
}

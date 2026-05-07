package handlers

import (
	"net/http"

	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

type DependencyHandler struct {
	dependencySvc *services.DependencyService
}

func NewDependencyHandler(dependencySvc *services.DependencyService) *DependencyHandler {
	return &DependencyHandler{
		dependencySvc: dependencySvc,
	}
}

func (h *DependencyHandler) AnalyzeDependencies(c *gin.Context) {
	var req services.DependencyAnalysisRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.dependencySvc.AnalyzeDependencies(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *DependencyHandler) CheckSecurityVulnerabilities(c *gin.Context) {
	var req services.SecurityCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.dependencySvc.CheckSecurityVulnerabilities(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *DependencyHandler) SuggestUpdates(c *gin.Context) {
	var req services.UpdateSuggestionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.dependencySvc.SuggestUpdates(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *DependencyHandler) GenerateDependencyGraph(c *gin.Context) {
	var req services.DependencyGraphRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.dependencySvc.GenerateDependencyGraph(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *DependencyHandler) RegisterRoutes(r *gin.RouterGroup) {
	dependencies := r.Group("/dependencies")
	{
		dependencies.POST("/analyze", h.AnalyzeDependencies)
		dependencies.POST("/check-security", h.CheckSecurityVulnerabilities)
		dependencies.POST("/suggest-updates", h.SuggestUpdates)
		dependencies.POST("/graph", h.GenerateDependencyGraph)
	}
}

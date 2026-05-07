package handlers

import (
	"net/http"

	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

type DocGenHandler struct {
	docGenSvc *services.DocGenService
}

func NewDocGenHandler(docGenSvc *services.DocGenService) *DocGenHandler {
	return &DocGenHandler{
		docGenSvc: docGenSvc,
	}
}

func (h *DocGenHandler) GenerateAPIDoc(c *gin.Context) {
	var req services.GenerateAPIDocRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.docGenSvc.GenerateAPIDoc(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *DocGenHandler) GenerateReadme(c *gin.Context) {
	var req services.GenerateReadmeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.docGenSvc.GenerateReadme(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *DocGenHandler) GenerateChangelog(c *gin.Context) {
	var req services.GenerateChangelogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.docGenSvc.GenerateChangelog(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *DocGenHandler) GenerateCodeComments(c *gin.Context) {
	var req services.GenerateCodeCommentsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.docGenSvc.GenerateCodeComments(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *DocGenHandler) RegisterRoutes(r *gin.RouterGroup) {
	docGen := r.Group("/doc-gen")
	{
		docGen.POST("/api-doc", h.GenerateAPIDoc)
		docGen.POST("/readme", h.GenerateReadme)
		docGen.POST("/changelog", h.GenerateChangelog)
		docGen.POST("/comments", h.GenerateCodeComments)
	}
}

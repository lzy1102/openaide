package handlers

import (
	"net/http"

	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

type TestGenHandler struct {
	testGenSvc *services.TestGenService
}

func NewTestGenHandler(testGenSvc *services.TestGenService) *TestGenHandler {
	return &TestGenHandler{
		testGenSvc: testGenSvc,
	}
}

func (h *TestGenHandler) GenerateTests(c *gin.Context) {
	var req services.GenerateTestsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.testGenSvc.GenerateTests(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *TestGenHandler) GenerateIntegrationTests(c *gin.Context) {
	var req services.GenerateIntegrationTestsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.testGenSvc.GenerateIntegrationTests(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *TestGenHandler) AnalyzeCoverage(c *gin.Context) {
	var req services.AnalyzeCoverageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.testGenSvc.AnalyzeCoverage(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *TestGenHandler) GenerateMutationTests(c *gin.Context) {
	var req services.GenerateMutationTestsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.testGenSvc.GenerateMutationTests(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *TestGenHandler) RegisterRoutes(r *gin.RouterGroup) {
	testGen := r.Group("/test-gen")
	{
		testGen.POST("/generate", h.GenerateTests)
		testGen.POST("/generate-integration", h.GenerateIntegrationTests)
		testGen.POST("/analyze-coverage", h.AnalyzeCoverage)
		testGen.POST("/generate-mutation", h.GenerateMutationTests)
	}
}

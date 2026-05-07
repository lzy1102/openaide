package handlers

import (
	"net/http"

	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

type DeployHandler struct {
	deploySvc *services.DeployService
}

func NewDeployHandler(deploySvc *services.DeployService) *DeployHandler {
	return &DeployHandler{
		deploySvc: deploySvc,
	}
}

func (h *DeployHandler) GenerateDockerfile(c *gin.Context) {
	var req services.DockerfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.deploySvc.GenerateDockerfile(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *DeployHandler) GenerateCICD(c *gin.Context) {
	var req services.CICDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.deploySvc.GenerateCICD(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *DeployHandler) AnalyzeDeployReadiness(c *gin.Context) {
	var req services.DeployReadinessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.deploySvc.AnalyzeDeployReadiness(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *DeployHandler) GenerateK8sManifest(c *gin.Context) {
	var req services.K8sManifestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.deploySvc.GenerateK8sManifest(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *DeployHandler) RegisterRoutes(r *gin.RouterGroup) {
	deploy := r.Group("/deploy")
	{
		deploy.POST("/dockerfile", h.GenerateDockerfile)
		deploy.POST("/cicd", h.GenerateCICD)
		deploy.POST("/readiness", h.AnalyzeDeployReadiness)
		deploy.POST("/k8s", h.GenerateK8sManifest)
	}
}

package handlers

import (
	"net/http"
	"strings"

	"openaide/backend/src/models"
	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

// PromptTemplateHandler 提示词模板处理器
type PromptTemplateHandler struct {
	templateService *services.PromptTemplateService
}

// NewPromptTemplateHandler 创建提示词模板处理器
func NewPromptTemplateHandler(templateService *services.PromptTemplateService) *PromptTemplateHandler {
	return &PromptTemplateHandler{
		templateService: templateService,
	}
}

// ListTemplates 列出模板
func (h *PromptTemplateHandler) ListTemplates(c *gin.Context) {
	category := c.Query("category")
	tagsStr := c.Query("tags")

	var tags []string
	if tagsStr != "" {
		tags = strings.Split(tagsStr, ",")
	}

	templates, err := h.templateService.ListTemplates(category, tags)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"templates": templates,
		"count":     len(templates),
	})
}

// GetTemplate 获取模板
func (h *PromptTemplateHandler) GetTemplate(c *gin.Context) {
	id := c.Param("id")

	template, err := h.templateService.GetTemplate(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
		return
	}

	c.JSON(http.StatusOK, template)
}

// GetTemplateByName 通过名称获取模板
func (h *PromptTemplateHandler) GetTemplateByName(c *gin.Context) {
	name := c.Param("name")

	template, err := h.templateService.GetTemplateByName(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
		return
	}

	c.JSON(http.StatusOK, template)
}

// CreateTemplate 创建模板
func (h *PromptTemplateHandler) CreateTemplate(c *gin.Context) {
	var template models.PromptTemplate
	if err := c.ShouldBindJSON(&template); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("user_id")
	template.CreatedBy = userID.(string)

	if err := h.templateService.CreateTemplate(&template); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, template)
}

// UpdateTemplate 更新模板
func (h *PromptTemplateHandler) UpdateTemplate(c *gin.Context) {
	id := c.Param("id")

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.templateService.UpdateTemplate(id, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "template updated",
		"id":      id,
	})
}

// DeleteTemplate 删除模板
func (h *PromptTemplateHandler) DeleteTemplate(c *gin.Context) {
	id := c.Param("id")

	if err := h.templateService.DeleteTemplate(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "template deleted",
		"id":      id,
	})
}

// RenderTemplate 渲染模板
func (h *PromptTemplateHandler) RenderTemplate(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		Variables map[string]interface{} `json:"variables"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rendered, err := h.templateService.RenderTemplate(id, req.Variables)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"rendered_content": rendered,
	})
}

// RenderByName 通过名称渲染模板
func (h *PromptTemplateHandler) RenderByName(c *gin.Context) {
	name := c.Param("name")

	var req struct {
		Variables map[string]interface{} `json:"variables"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rendered, err := h.templateService.RenderByName(name, req.Variables)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"template_name":    name,
		"rendered_content": rendered,
	})
}

// CreateVersion 创建新版本
func (h *PromptTemplateHandler) CreateVersion(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		Template  string                   `json:"template" binding:"required"`
		Variables []models.PromptVariable  `json:"variables"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	newVersion, err := h.templateService.CreateVersion(id, req.Template, req.Variables)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, newVersion)
}

// GetVersions 获取版本历史
func (h *PromptTemplateHandler) GetVersions(c *gin.Context) {
	name := c.Param("name")

	versions, err := h.templateService.GetVersions(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"versions": versions,
		"count":    len(versions),
	})
}

// ExtractVariables 从模板提取变量
func (h *PromptTemplateHandler) ExtractVariables(c *gin.Context) {
	var req struct {
		Template string `json:"template" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	variables := h.templateService.ExtractVariables(req.Template)

	c.JSON(http.StatusOK, gin.H{
		"variables": variables,
		"count":     len(variables),
	})
}

// ExportTemplates 导出模板
func (h *PromptTemplateHandler) ExportTemplates(c *gin.Context) {
	var req struct {
		IDs []string `json:"ids"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	data, err := h.templateService.ExportTemplates(req.IDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Data(http.StatusOK, "application/json", data)
}

// ImportTemplates 导入模板
func (h *PromptTemplateHandler) ImportTemplates(c *gin.Context) {
	var req struct {
		Data      []byte `json:"data" binding:"required"`
		Overwrite bool   `json:"overwrite"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	count, err := h.templateService.ImportTemplates(req.Data, req.Overwrite)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "templates imported",
		"imported_count": count,
	})
}

// RegisterRoutes 注册路由
func (h *PromptTemplateHandler) RegisterRoutes(r *gin.RouterGroup) {
	templates := r.Group("/prompt-templates")
	{
		templates.GET("", h.ListTemplates)
		templates.POST("", h.CreateTemplate)
		templates.GET("/:id", h.GetTemplate)
		templates.PUT("/:id", h.UpdateTemplate)
		templates.DELETE("/:id", h.DeleteTemplate)
		templates.POST("/:id/render", h.RenderTemplate)
		templates.POST("/:id/versions", h.CreateVersion)

		// 通过名称操作
		templates.GET("/name/:name", h.GetTemplateByName)
		templates.POST("/name/:name/render", h.RenderByName)
		templates.GET("/name/:name/versions", h.GetVersions)

		// 工具接口
		templates.POST("/extract-variables", h.ExtractVariables)
		templates.POST("/export", h.ExportTemplates)
		templates.POST("/import", h.ImportTemplates)
	}
}

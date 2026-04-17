package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"openaide/backend/src/models"
	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SkillHandler 技能管理 API 处理器
type SkillHandler struct {
	skillSvc       *services.SkillService
	importSvc      *services.SkillImportService
}

// NewSkillHandler 创建技能处理器
func NewSkillHandler(skillSvc *services.SkillService) *SkillHandler {
	return &SkillHandler{
		skillSvc:       skillSvc,
	}
}

// SetImportService 设置导入服务（需要在初始化后调用）
func (h *SkillHandler) SetImportService(importSvc *services.SkillImportService) {
	h.importSvc = importSvc
}

// ListSkills 列出所有技能（支持分类过滤）
func (h *SkillHandler) ListSkills(c *gin.Context) {
	category := c.Query("category")

	if category != "" {
		skills, err := h.skillSvc.ListSkillsByCategory(category)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    skills,
			"count":   len(skills),
		})
		return
	}

	skills, err := h.skillSvc.ListSkills()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    skills,
		"count":   len(skills),
	})
}

// CreateSkill 创建技能
func (h *SkillHandler) CreateSkill(c *gin.Context) {
	var req struct {
		Name                 string      `json:"name" binding:"required"`
		Description          string      `json:"description"`
		Category             string      `json:"category"`
		Version              string      `json:"version"`
		Author               string      `json:"author"`
		Triggers             interface{} `json:"triggers"`
		SystemPromptOverride string      `json:"system_prompt_override"`
		Tools                interface{} `json:"tools"`
		ModelPreference      string      `json:"model_preference"`
		Config               interface{} `json:"config"`
		Enabled              *bool       `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 使用 SkillService.CreateSkill 处理 JSON 字段的序列化
	result, err := h.skillSvc.CreateSkillFromMap(map[string]interface{}{
		"name":                   req.Name,
		"description":            req.Description,
		"category":               req.Category,
		"version":                req.Version,
		"author":                 req.Author,
		"triggers":               req.Triggers,
		"system_prompt_override": req.SystemPromptOverride,
		"tools":                  req.Tools,
		"model_preference":       req.ModelPreference,
		"config":                 req.Config,
		"enabled":                req.Enabled,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetSkill 获取技能详情
func (h *SkillHandler) GetSkill(c *gin.Context) {
	id := c.Param("id")
	skill, err := h.skillSvc.GetSkill(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Skill not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    skill,
	})
}

// UpdateSkill 更新技能
func (h *SkillHandler) UpdateSkill(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Name                 string      `json:"name"`
		Description          string      `json:"description"`
		Category             string      `json:"category"`
		Version              string      `json:"version"`
		Author               string      `json:"author"`
		Triggers             interface{} `json:"triggers"`
		SystemPromptOverride string      `json:"system_prompt_override"`
		Tools                interface{} `json:"tools"`
		ModelPreference      string      `json:"model_preference"`
		Config               interface{} `json:"config"`
		Enabled              *bool       `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.skillSvc.UpdateSkillFromMap(id, map[string]interface{}{
		"name":                   req.Name,
		"description":            req.Description,
		"category":               req.Category,
		"version":                req.Version,
		"author":                 req.Author,
		"triggers":               req.Triggers,
		"system_prompt_override": req.SystemPromptOverride,
		"tools":                  req.Tools,
		"model_preference":       req.ModelPreference,
		"config":                 req.Config,
		"enabled":                req.Enabled,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// DeleteSkill 删除技能
func (h *SkillHandler) DeleteSkill(c *gin.Context) {
	id := c.Param("id")
	if err := h.skillSvc.DeleteSkill(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Skill deleted successfully",
	})
}

// EnableSkill 启用技能
func (h *SkillHandler) EnableSkill(c *gin.Context) {
	id := c.Param("id")
	if err := h.skillSvc.SetEnabled(id, true); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Skill enabled",
	})
}

// DisableSkill 禁用技能
func (h *SkillHandler) DisableSkill(c *gin.Context) {
	id := c.Param("id")
	if err := h.skillSvc.SetEnabled(id, false); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Skill disabled",
	})
}

// CreateSkillParameter 创建技能参数
func (h *SkillHandler) CreateSkillParameter(c *gin.Context) {
	skillID := c.Param("id")
	var req struct {
		Name        string      `json:"name" binding:"required"`
		Description string      `json:"description"`
		Type        string      `json:"type"`
		Required    bool        `json:"required"`
		Default     interface{} `json:"default"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	param := &models.SkillParameter{
		SkillID:     skillID,
		Name:        req.Name,
		Description: req.Description,
		Type:        req.Type,
		Required:    req.Required,
	}
	if req.Default != nil {
		param.Default = &models.JSONAny{Data: req.Default}
	}

	if err := h.skillSvc.CreateSkillParameter(param); err != nil {
		h.handleSkillError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    param,
	})
}

// UpdateSkillParameter 更新技能参数
func (h *SkillHandler) UpdateSkillParameter(c *gin.Context) {
	skillID := c.Param("id")
	paramID := c.Param("paramId")
	var payload map[string]interface{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := make(map[string]interface{})
	if value, ok := payload["name"]; ok {
		updates["name"] = value
	}
	if value, ok := payload["description"]; ok {
		updates["description"] = value
	}
	if value, ok := payload["type"]; ok {
		updates["type"] = value
	}
	if value, ok := payload["required"]; ok {
		if boolValue, ok := value.(bool); ok {
			updates["required"] = &boolValue
		}
	}
	if value, ok := payload["default"]; ok {
		updates["default"] = value
	}

	param, err := h.skillSvc.UpdateSkillParameter(skillID, paramID, updates)
	if err != nil {
		h.handleSkillError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    param,
	})
}

// DeleteSkillParameter 删除技能参数
func (h *SkillHandler) DeleteSkillParameter(c *gin.Context) {
	skillID := c.Param("id")
	paramID := c.Param("paramId")
	if err := h.skillSvc.DeleteSkillParameter(skillID, paramID); err != nil {
		h.handleSkillError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Skill parameter deleted successfully",
	})
}

// ExecuteSkill 执行技能
func (h *SkillHandler) ExecuteSkill(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Parameters map[string]interface{} `json:"parameters"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	execution, err := h.skillSvc.ExecuteSkill(id, req.Parameters)
	if err != nil {
		h.handleSkillError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    execution,
	})
}

// MatchSkill 匹配技能
func (h *SkillHandler) MatchSkill(c *gin.Context) {
	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	match := h.skillSvc.MatchSkill(req.Content)
	if match == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"matched": false,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"matched":    true,
		"skill":      match.Skill,
		"confidence": match.Confidence,
		"trigger":    match.MatchedTrigger,
	})
}

// ExecuteMatchedSkill 匹配并执行技能
func (h *SkillHandler) ExecuteMatchedSkill(c *gin.Context) {
	var req struct {
		Content string `json:"content" binding:"required"`
		UserID  string `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	execution, err := h.skillSvc.ExecuteMatchedSkill(c.Request.Context(), req.Content, req.UserID)
	if err != nil {
		h.handleSkillError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    execution,
	})
}

// GetSkillParameters 获取技能参数定义
func (h *SkillHandler) GetSkillParameters(c *gin.Context) {
	id := c.Param("id")
	params, err := h.skillSvc.GetSkillParameters(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    params,
		"count":   len(params),
	})
}

// GetSkillExecutions 获取技能执行历史
func (h *SkillHandler) GetSkillExecutions(c *gin.Context) {
	id := c.Param("id")
	limitStr := c.DefaultQuery("limit", "20")
	limit, _ := strconv.Atoi(limitStr)

	executions, err := h.skillSvc.GetSkillExecutions(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if limit > 0 && len(executions) > limit {
		executions = executions[:limit]
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    executions,
		"count":   len(executions),
	})
}

// ListCategories 列出所有技能分类
func (h *SkillHandler) ListCategories(c *gin.Context) {
	categories, err := h.skillSvc.ListCategories()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    categories,
		"count":   len(categories),
	})
}

// ImportSkillFromContent 从内容导入技能
func (h *SkillHandler) ImportSkillFromContent(c *gin.Context) {
	if h.importSvc == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "import service not initialized"})
		return
	}

	var req struct {
		Content    string            `json:"content" binding:"required"`
		References map[string]string `json:"references"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	skill, params, err := h.importSvc.ImportFromContent(req.Content, req.References)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"skill":      skill,
		"parameters": params,
		"message":    fmt.Sprintf("Skill '%s' imported successfully", skill.Name),
	})
}

// ImportSkillFromURL 从 URL 导入技能
func (h *SkillHandler) ImportSkillFromURL(c *gin.Context) {
	if h.importSvc == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "import service not initialized"})
		return
	}

	var req struct {
		URL string `json:"url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	skill, params, err := h.importSvc.ImportFromURL(req.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"skill":      skill,
		"parameters": params,
		"message":    fmt.Sprintf("Skill '%s' imported from URL successfully", skill.Name),
	})
}

// ValidateSkillMD 验证 SKILL.md 内容
func (h *SkillHandler) ValidateSkillMD(c *gin.Context) {
	if h.importSvc == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "import service not initialized"})
		return
	}

	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.importSvc.ValidateSKILLMD(req.Content); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"valid": false,
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":   true,
		"message": "SKILL.md is valid",
	})
}

// ExportSkillToSKILLMD 导出技能为 SKILL.md 格式
func (h *SkillHandler) ExportSkillToSKILLMD(c *gin.Context) {
	if h.importSvc == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "import service not initialized"})
		return
	}

	id := c.Param("id")
	content, err := h.importSvc.ExportToSKILLMD(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"content": content,
	})
}

// RegisterRoutes 注册路由
func (h *SkillHandler) RegisterRoutes(r *gin.RouterGroup) {
	skills := r.Group("/skills")
	{
		skills.GET("", h.ListSkills)
		skills.POST("", h.CreateSkill)
		skills.GET("/categories", h.ListCategories)
		skills.POST("/match", h.MatchSkill)
		skills.POST("/execute-matched", h.ExecuteMatchedSkill)
		
		// SKILL.md 导入/导出
		skills.POST("/import", h.ImportSkillFromContent)
		skills.POST("/import-from-url", h.ImportSkillFromURL)
		skills.POST("/validate", h.ValidateSkillMD)
		skills.GET("/:id/export", h.ExportSkillToSKILLMD)
		
		skills.GET("/:id", h.GetSkill)
		skills.PUT("/:id", h.UpdateSkill)
		skills.DELETE("/:id", h.DeleteSkill)
		skills.POST("/:id/enable", h.EnableSkill)
		skills.POST("/:id/disable", h.DisableSkill)
		skills.POST("/:id/execute", h.ExecuteSkill)
		skills.GET("/:id/parameters", h.GetSkillParameters)
		skills.POST("/:id/parameters", h.CreateSkillParameter)
		skills.PUT("/:id/parameters/:paramId", h.UpdateSkillParameter)
		skills.DELETE("/:id/parameters/:paramId", h.DeleteSkillParameter)
		skills.GET("/:id/executions", h.GetSkillExecutions)
	}
}

func (h *SkillHandler) handleSkillError(c *gin.Context, err error) {
	var paramErr *services.SkillParameterError
	switch {
	case errors.As(err, &paramErr):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

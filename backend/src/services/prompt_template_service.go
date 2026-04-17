package services

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	ttemplate "text/template"
	"time"

	"openaide/backend/src/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PromptTemplateService 提示词模板服务
type PromptTemplateService struct {
	db     *gorm.DB
	cache  *CacheService
	logger *LoggerService
}

// NewPromptTemplateService 创建提示词模板服务
func NewPromptTemplateService(db *gorm.DB, cache *CacheService, logger *LoggerService) *PromptTemplateService {
	s := &PromptTemplateService{
		db:     db,
		cache:  cache,
		logger: logger,
	}

	// 初始化默认模板
	s.initDefaultTemplates()

	return s
}

// initDefaultTemplates 初始化默认模板
func (s *PromptTemplateService) initDefaultTemplates() {
	defaultTemplates := []struct {
		Name        string
		Description string
		Category    string
		Template    string
		Variables   []models.PromptVariable
		Tags        []string
		IsDefault   bool
	}{
		{
			Name:        "default_system",
			Description: "默认系统提示词模板",
			Category:    "system",
			Template: `你是一个智能助手，具备以下能力：
1. 回答问题和提供建议
2. 分析和推理复杂问题
3. 编写和解释代码
4. 帮助用户完成各种任务

当前日期: {{.current_date}}
当前时间: {{.current_time}}
{{if .user_name}}用户: {{.user_name}}{{end}}
{{if .custom_instructions}}
特殊指令:
{{.custom_instructions}}
{{end}}`,
			Variables: []models.PromptVariable{
				{Name: "current_date", Type: "string", Description: "当前日期", DefaultValue: "", Required: false},
				{Name: "current_time", Type: "string", Description: "当前时间", DefaultValue: "", Required: false},
				{Name: "user_name", Type: "string", Description: "用户名称", DefaultValue: "", Required: false},
				{Name: "custom_instructions", Type: "string", Description: "自定义指令", DefaultValue: "", Required: false},
			},
			Tags:      []string{"system", "default"},
			IsDefault: true,
		},
		{
			Name:        "thinking_system",
			Description: "思考推理系统提示词",
			Category:    "system",
			Template: `你是一个具备深度思考能力的AI助手。

当面对复杂问题时，请按照以下步骤思考：
1. 理解问题的核心
2. 分析相关因素
3. 考虑多种可能性
4. 逐步推理得出结论

思考风格: {{.thinking_style}}
{{if .domain_expertise}}
专业领域: {{.domain_expertise}}
{{end}}
`,
			Variables: []models.PromptVariable{
				{Name: "thinking_style", Type: "string", Description: "思考风格 (analytical, creative, balanced)", DefaultValue: "balanced", Required: false},
				{Name: "domain_expertise", Type: "string", Description: "专业领域", DefaultValue: "", Required: false},
			},
			Tags:      []string{"system", "thinking"},
			IsDefault: false,
		},
		{
			Name:        "code_assistant",
			Description: "代码助手系统提示词",
			Category:    "system",
			Template: `你是一个专业的编程助手，精通多种编程语言和框架。

你的职责是：
1. 编写高质量、可维护的代码
2. 解释代码逻辑和最佳实践
3. 帮助调试和优化代码
4. 提供架构设计建议

编程语言偏好: {{.preferred_language}}
代码风格: {{.code_style}}
{{if .project_context}}
项目上下文:
{{.project_context}}
{{end}}`,
			Variables: []models.PromptVariable{
				{Name: "preferred_language", Type: "string", Description: "偏好的编程语言", DefaultValue: "Python", Required: false},
				{Name: "code_style", Type: "string", Description: "代码风格", DefaultValue: "clean", Required: false},
				{Name: "project_context", Type: "string", Description: "项目上下文信息", DefaultValue: "", Required: false},
			},
			Tags:      []string{"system", "code"},
			IsDefault: false,
		},
		{
			Name:        "task_decompose",
			Description: "任务分解提示词",
			Category:    "function",
			Template: `请将以下任务分解为可执行的子任务：

任务: {{.task_description}}
{{if .constraints}}
约束条件:
{{.constraints}}
{{end}}
{{if .context}}
上下文:
{{.context}}
{{end}}

请按照以下格式输出分解结果：
1. 子任务列表
2. 任务之间的依赖关系
3. 预计完成时间
4. 所需资源`,
			Variables: []models.PromptVariable{
				{Name: "task_description", Type: "string", Description: "任务描述", DefaultValue: "", Required: true},
				{Name: "constraints", Type: "string", Description: "约束条件", DefaultValue: "", Required: false},
				{Name: "context", Type: "string", Description: "上下文信息", DefaultValue: "", Required: false},
			},
			Tags:      []string{"function", "task"},
			IsDefault: false,
		},
		{
			Name:        "summarize",
			Description: "文本摘要提示词",
			Category:    "function",
			Template: `请对以下内容进行摘要：

{{.content}}

摘要要求:
- 长度: {{.max_length}} 字以内
- 风格: {{.style}}
{{if .focus_points}}
重点关注: {{.focus_points}}
{{end}}`,
			Variables: []models.PromptVariable{
				{Name: "content", Type: "string", Description: "待摘要内容", DefaultValue: "", Required: true},
				{Name: "max_length", Type: "number", Description: "最大长度", DefaultValue: "200", Required: false},
				{Name: "style", Type: "string", Description: "摘要风格 (concise, detailed, bullet)", DefaultValue: "concise", Required: false},
				{Name: "focus_points", Type: "string", Description: "关注重点", DefaultValue: "", Required: false},
			},
			Tags:      []string{"function", "summarize"},
			IsDefault: false,
		},
	}

	for _, t := range defaultTemplates {
		// 检查是否已存在
		var count int64
		s.db.Model(&models.PromptTemplate{}).Where("name = ?", t.Name).Count(&count)
		if count > 0 {
			continue
		}

		template := &models.PromptTemplate{
			ID:          uuid.New().String(),
			Name:        t.Name,
			Description: t.Description,
			Category:    t.Category,
			Template:    t.Template,
			Variables:   t.Variables,
			Tags:        t.Tags,
			Version:     "1.0.0",
			IsActive:    true,
			IsDefault:   t.IsDefault,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		if err := s.db.Create(template).Error; err != nil {
			s.logger.Error(context.Background(), "Failed to create default template: %s - %v", t.Name, err)
		}
	}
}

// CreateTemplate 创建模板
func (s *PromptTemplateService) CreateTemplate(template *models.PromptTemplate) error {
	// 验证模板语法
	if err := s.validateTemplate(template.Template); err != nil {
		return fmt.Errorf("invalid template syntax: %w", err)
	}

	// 设置版本
	if template.Version == "" {
		template.Version = "1.0.0"
	}

	template.IsActive = true
	template.CreatedAt = time.Now()
	template.UpdatedAt = time.Now()

	if err := s.db.Create(template).Error; err != nil {
		return fmt.Errorf("failed to create template: %w", err)
	}

	s.logger.Info(context.Background(), "Template created: %s - %s", template.ID, template.Name)
	return nil
}

// GetTemplate 获取模板
func (s *PromptTemplateService) GetTemplate(id string) (*models.PromptTemplate, error) {
	// 先从缓存获取
	cacheKey := fmt.Sprintf("prompt_template:%s", id)
	if cached, found := s.cache.Get(cacheKey); found {
		if template, ok := cached.(*models.PromptTemplate); ok {
			return template, nil
		}
	}

	var template models.PromptTemplate
	if err := s.db.Where("id = ? AND is_active = ?", id, true).First(&template).Error; err != nil {
		return nil, fmt.Errorf("template not found: %s", id)
	}

	// 缓存模板
	s.cache.Set(cacheKey, &template, 10*time.Minute)

	return &template, nil
}

// GetTemplateByName 通过名称获取模板
func (s *PromptTemplateService) GetTemplateByName(name string) (*models.PromptTemplate, error) {
	var template models.PromptTemplate
	if err := s.db.Where("name = ? AND is_active = ?", name, true).First(&template).Error; err != nil {
		return nil, fmt.Errorf("template not found: %s", name)
	}
	return &template, nil
}

// ListTemplates 列出模板
func (s *PromptTemplateService) ListTemplates(category string, tags []string) ([]models.PromptTemplate, error) {
	query := s.db.Where("is_active = ?", true)

	if category != "" {
		query = query.Where("category = ?", category)
	}

	var templates []models.PromptTemplate
	if err := query.Order("use_count DESC, created_at DESC").Find(&templates).Error; err != nil {
		return nil, err
	}

	// SQLite 不支持 JSON 查询，使用应用层过滤
	if len(tags) > 0 {
		filtered := make([]models.PromptTemplate, 0)
		for _, tmpl := range templates {
			for _, tag := range tmpl.Tags {
				contains := false
				for _, filterTag := range tags {
					if tag == filterTag {
						contains = true
						break
					}
				}
				if contains {
					filtered = append(filtered, tmpl)
					break
				}
			}
		}
		templates = filtered
	}

	return templates, nil
}

// UpdateTemplate 更新模板
func (s *PromptTemplateService) UpdateTemplate(id string, updates map[string]interface{}) error {
	updates["updated_at"] = time.Now()

	if err := s.db.Model(&models.PromptTemplate{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update template: %w", err)
	}

	// 清除缓存
	s.cache.Delete(fmt.Sprintf("prompt_template:%s", id))

	s.logger.Info(context.Background(), "Template updated: %s", id)
	return nil
}

// DeleteTemplate 删除模板 (软删除)
func (s *PromptTemplateService) DeleteTemplate(id string) error {
	if err := s.db.Model(&models.PromptTemplate{}).Where("id = ?", id).Update("is_active", false).Error; err != nil {
		return fmt.Errorf("failed to delete template: %w", err)
	}

	// 清除缓存
	s.cache.Delete(fmt.Sprintf("prompt_template:%s", id))

	s.logger.Info(context.Background(), "Template deleted: %s", id)
	return nil
}

// RenderTemplate 渲染模板
func (s *PromptTemplateService) RenderTemplate(templateID string, variables map[string]interface{}) (string, error) {
	template, err := s.GetTemplate(templateID)
	if err != nil {
		return "", err
	}

	return s.Render(template, variables)
}

// RenderByName 通过名称渲染模板
func (s *PromptTemplateService) RenderByName(name string, variables map[string]interface{}) (string, error) {
	template, err := s.GetTemplateByName(name)
	if err != nil {
		return "", err
	}

	return s.Render(template, variables)
}

// Render 渲染模板
func (s *PromptTemplateService) Render(tmpl *models.PromptTemplate, variables map[string]interface{}) (string, error) {
	// 合并默认值
	mergedVars := s.mergeWithDefaults(tmpl.Variables, variables)

	// 创建模板
	t, err := ttemplate.New("prompt").Parse(tmpl.Template)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	// 执行模板
	var buf strings.Builder
	if err := t.Execute(&buf, mergedVars); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	// 更新使用统计（异步，带错误处理）
	go func() {
		now := time.Now()
		if err := s.db.Model(&models.PromptTemplate{}).
			Where("id = ?", tmpl.ID).
			Updates(map[string]interface{}{
				"use_count":    gorm.Expr("use_count + 1"),
				"last_used_at": &now,
			}).Error; err != nil {
			s.logger.Error(context.Background(), "Failed to update template use count: %v", err)
		}
	}()

	return buf.String(), nil
}

// mergeWithDefaults 合并变量与默认值
func (s *PromptTemplateService) mergeWithDefaults(variables []models.PromptVariable, provided map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// 设置默认值
	for _, v := range variables {
		if v.DefaultValue != "" {
			result[v.Name] = v.DefaultValue
		}
	}

	// 覆盖提供的值
	for k, v := range provided {
		result[k] = v
	}

	// 添加内置变量
	now := time.Now()
	result["current_date"] = now.Format("2006-01-02")
	result["current_time"] = now.Format("15:04:05")
	result["current_datetime"] = now.Format("2006-01-02 15:04:05")

	return result
}

// validateTemplate 验证模板语法
func (s *PromptTemplateService) validateTemplate(templateStr string) error {
	// 检查模板语法
	_, err := ttemplate.New("validation").Parse(templateStr)
	if err != nil {
		return err
	}

	return nil
}

// CreateVersion 创建模板新版本
func (s *PromptTemplateService) CreateVersion(templateID string, newTemplate string, variables []models.PromptVariable) (*models.PromptTemplate, error) {
	// 获取原模板
	original, err := s.GetTemplate(templateID)
	if err != nil {
		return nil, err
	}

	// 解析版本号
	versionParts := strings.Split(original.Version, ".")
	if len(versionParts) != 3 {
		versionParts = []string{"1", "0", "0"}
	}

	// 增加版本号
	minor := 0
	fmt.Sscanf(versionParts[1], "%d", &minor)
	newVersion := fmt.Sprintf("%s.%d.%s", versionParts[0], minor+1, versionParts[2])

	// 创建新版本
	newTmpl := &models.PromptTemplate{
		ID:          uuid.New().String(),
		Name:        original.Name,
		Description: original.Description,
		Category:    original.Category,
		Template:    newTemplate,
		Variables:   variables,
		Tags:        original.Tags,
		Version:     newVersion,
		ParentID:    original.ID,
		IsActive:    true,
		IsDefault:   false,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.db.Create(newTmpl).Error; err != nil {
		return nil, fmt.Errorf("failed to create new version: %w", err)
	}

	// 将原版本设为非默认
	s.db.Model(&models.PromptTemplate{}).
		Where("id = ?", original.ID).
		Update("is_default", false)

	s.logger.Info(context.Background(), "Template version created: %s - %s", newTmpl.ID, newVersion)
	return newTmpl, nil
}

// GetVersions 获取模板版本历史
func (s *PromptTemplateService) GetVersions(templateName string) ([]models.PromptTemplate, error) {
	var versions []models.PromptTemplate
	if err := s.db.Where("name = ?", templateName).
		Order("created_at DESC").
		Find(&versions).Error; err != nil {
		return nil, err
	}
	return versions, nil
}

// ExtractVariables 从模板中提取变量
func (s *PromptTemplateService) ExtractVariables(templateStr string) []string {
	re := regexp.MustCompile(`\{\{\s*\.(\w+)\s*\}\}`)
	matches := re.FindAllStringSubmatch(templateStr, -1)

	variables := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 {
			variables[match[1]] = true
		}
	}

	result := make([]string, 0, len(variables))
	for v := range variables {
		result = append(result, v)
	}
	return result
}

// ExportTemplates 导出模板
func (s *PromptTemplateService) ExportTemplates(ids []string) ([]byte, error) {
	var templates []models.PromptTemplate
	if err := s.db.Where("id IN ?", ids).Find(&templates).Error; err != nil {
		return nil, err
	}

	return json.MarshalIndent(templates, "", "  ")
}

// ImportTemplates 导入模板
func (s *PromptTemplateService) ImportTemplates(data []byte, overwrite bool) (int, error) {
	var templates []models.PromptTemplate
	if err := json.Unmarshal(data, &templates); err != nil {
		return 0, fmt.Errorf("invalid import data: %w", err)
	}

	imported := 0
	for _, t := range templates {
		// 检查是否存在
		var existing models.PromptTemplate
		err := s.db.Where("name = ?", t.Name).First(&existing).Error

		if err == gorm.ErrRecordNotFound {
			// 创建新模板
			t.ID = uuid.New().String()
			t.IsActive = true
			t.CreatedAt = time.Now()
			t.UpdatedAt = time.Now()
			if err := s.db.Create(&t).Error; err == nil {
				imported++
			}
		} else if overwrite {
			// 覆盖现有模板
			t.ID = existing.ID
			t.UpdatedAt = time.Now()
			if err := s.db.Save(&t).Error; err == nil {
				imported++
			}
		}
	}

	return imported, nil
}

package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// SkillService 技能服务
type SkillService struct {
	db         *gorm.DB
	modelSvc   *ModelService
	logger     *LoggerService
	skillExecs map[string]SkillExecutor // name -> executor
}

// SkillExecutor 技能执行器接口
type SkillExecutor interface {
	Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

// SkillMatchResult 技能匹配结果
type SkillMatchResult struct {
	Skill          *models.Skill `json:"skill"`
	Confidence     float64       `json:"confidence"`
	MatchedTrigger string        `json:"matched_trigger"`
}

// SkillParameterError 技能参数错误
type SkillParameterError struct {
	Message string
}

func (e *SkillParameterError) Error() string {
	return e.Message
}

func IsSkillParameterError(err error) bool {
	var paramErr *SkillParameterError
	return errors.As(err, &paramErr)
}

// NewSkillService 创建技能服务实例
func NewSkillService(db *gorm.DB, modelSvc *ModelService, logger *LoggerService) *SkillService {
	svc := &SkillService{
		db:         db,
		modelSvc:   modelSvc,
		logger:     logger,
		skillExecs: make(map[string]SkillExecutor),
	}

	// 注册内置技能执行器
	svc.registerBuiltinExecutors()

	return svc
}

// ==================== CRUD 操作 ====================

// CreateSkill 创建技能
func (s *SkillService) CreateSkill(skill *models.Skill) error {
	skill.ID = uuid.New().String()
	skill.CreatedAt = time.Now()
	skill.UpdatedAt = time.Now()
	return s.db.Create(skill).Error
}

// UpdateSkill 更新技能
func (s *SkillService) UpdateSkill(skill *models.Skill) error {
	skill.UpdatedAt = time.Now()
	return s.db.Save(skill).Error
}

// SetEnabled 启用/禁用技能
func (s *SkillService) SetEnabled(id string, enabled bool) error {
	return s.db.Model(&models.Skill{}).Where("id = ?", id).Update("enabled", enabled).Error
}

// CreateSkillFromMap 从 map 创建技能（处理 JSON 字段）
func (s *SkillService) CreateSkillFromMap(data map[string]interface{}) (*models.Skill, error) {
	skill := &models.Skill{
		ID:        uuid.New().String(),
		Name:      getString(data, "name"),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Enabled:   true,
	}
	if desc, ok := data["description"].(string); ok {
		skill.Description = desc
	}
	if cat, ok := data["category"].(string); ok {
		skill.Category = cat
	}
	if ver, ok := data["version"].(string); ok {
		skill.Version = ver
	}
	if author, ok := data["author"].(string); ok {
		skill.Author = author
	}
	if prompt, ok := data["system_prompt_override"].(string); ok {
		skill.SystemPromptOverride = prompt
	}
	if pref, ok := data["model_preference"].(string); ok {
		skill.ModelPreference = pref
	}
	if v, ok := data["triggers"]; ok {
		skill.Triggers = toJSONSlice(v)
	}
	if v, ok := data["tools"]; ok {
		skill.Tools = toJSONSlice(v)
	}
	if v, ok := data["config"]; ok {
		skill.Config = toJSONMap(v)
	}
	if enabled, ok := data["enabled"].(*bool); ok {
		skill.Enabled = *enabled
	}

	if err := s.db.Create(skill).Error; err != nil {
		return nil, err
	}
	return skill, nil
}

// UpdateSkillFromMap 从 map 更新技能（仅更新非空字段）
func (s *SkillService) UpdateSkillFromMap(id string, data map[string]interface{}) (*models.Skill, error) {
	skill, err := s.GetSkill(id)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{"updated_at": time.Now()}
	if v, ok := data["name"].(string); ok && v != "" {
		updates["name"] = v
	}
	if v, ok := data["description"].(string); ok {
		updates["description"] = v
	}
	if v, ok := data["category"].(string); ok {
		updates["category"] = v
	}
	if v, ok := data["version"].(string); ok {
		updates["version"] = v
	}
	if v, ok := data["author"].(string); ok {
		updates["author"] = v
	}
	if v, ok := data["system_prompt_override"].(string); ok {
		updates["system_prompt_override"] = v
	}
	if v, ok := data["model_preference"].(string); ok {
		updates["model_preference"] = v
	}
	if v, ok := data["triggers"]; ok {
		updates["triggers"] = toJSONSlice(v)
	}
	if v, ok := data["tools"]; ok {
		updates["tools"] = toJSONSlice(v)
	}
	if v, ok := data["config"]; ok {
		updates["config"] = toJSONMap(v)
	}
	if enabled, ok := data["enabled"].(*bool); ok {
		updates["enabled"] = *enabled
	}

	if err := s.db.Model(skill).Updates(updates).Error; err != nil {
		return nil, err
	}

	return s.GetSkill(id)
}

// DeleteSkill 删除技能
func (s *SkillService) DeleteSkill(id string) error {
	return s.db.Where("id = ?", id).Delete(&models.Skill{}).Error
}

// GetSkill 获取技能
func (s *SkillService) GetSkill(id string) (*models.Skill, error) {
	var skill models.Skill
	err := s.db.First(&skill, id).Error
	return &skill, err
}

// GetSkillByName 按名称获取技能
func (s *SkillService) GetSkillByName(name string) (*models.Skill, error) {
	var skill models.Skill
	err := s.db.Where("name = ?", name).First(&skill).Error
	return &skill, err
}

// ListSkills 列出所有技能
func (s *SkillService) ListSkills() ([]models.Skill, error) {
	var skills []models.Skill
	err := s.db.Find(&skills).Error
	return skills, err
}

// ListEnabledSkills 列出已启用的技能
func (s *SkillService) ListEnabledSkills() ([]models.Skill, error) {
	var skills []models.Skill
	err := s.db.Where("enabled = ?", true).Find(&skills).Error
	return skills, err
}

// GetSkillLevel0 获取技能 Level 0 概要 (仅用于初始匹配, 节省token)
func (s *SkillService) GetSkillLevel0(id string) (*models.Skill, error) {
	var skill models.Skill
	err := s.db.Select("id", "name", "description", "category", "version", "author", "level0_summary", "triggers", "model_preference", "enabled").First(&skill, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &skill, nil
}

// GetSkillLevel1 获取技能 Level 1 完整内容 (用于执行时加载)
func (s *SkillService) GetSkillLevel1(id string) (*models.Skill, error) {
	var skill models.Skill
	err := s.db.First(&skill, "id = ?", id).Error
	return &skill, err
}

// GetSkillLevel2 获取技能 Level 2 参考材料
func (s *SkillService) GetSkillLevel2(id string) ([]string, error) {
	var skill models.Skill
	err := s.db.Select("level2_references").First(&skill, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	if skill.Level2References != nil {
		return skill.Level2References, nil
	}
	return []string{}, nil
}

// IncrementSkillUsage 增加技能使用次数
func (s *SkillService) IncrementSkillUsage(id string) error {
	return s.db.Model(&models.Skill{}).Where("id = ?", id).Updates(map[string]interface{}{
		"usage_count": gorm.Expr("usage_count + 1"),
	}).Error
}

// UpdateSkillSuccessRate 更新技能成功率
func (s *SkillService) UpdateSkillSuccessRate(id string, success bool) error {
	skill, err := s.GetSkill(id)
	if err != nil {
		return err
	}

	total := skill.UsageCount
	if total == 0 {
		total = 1
	}

	var successCount int
	if skill.SuccessRate > 0 {
		successCount = int(skill.SuccessRate * float64(total))
	}
	if success {
		successCount++
	}

	newRate := float64(successCount) / float64(total+1)
	return s.db.Model(&models.Skill{}).Where("id = ?", id).Update("success_rate", newRate).Error
}

// ListSkillsByCategory 按分类列出技能
func (s *SkillService) ListSkillsByCategory(category string) ([]models.Skill, error) {
	var skills []models.Skill
	err := s.db.Where("category = ?", category).Find(&skills).Error
	return skills, err
}

// ListCategories 列出所有技能分类
func (s *SkillService) ListCategories() ([]string, error) {
	var categories []string
	err := s.db.Model(&models.Skill{}).Distinct("category").Pluck("category", &categories).Error
	return categories, err
}

// ==================== 参数管理 ====================

// CreateSkillParameter 创建技能参数
func (s *SkillService) CreateSkillParameter(param *models.SkillParameter) error {
	param.SkillID = strings.TrimSpace(param.SkillID)
	param.Name = strings.TrimSpace(param.Name)
	param.Type = normalizeParameterType(param.Type)
	if param.Type == "" {
		param.Type = "string"
	}
	if param.SkillID == "" {
		return &SkillParameterError{Message: "skill_id is required"}
	}
	if param.Name == "" {
		return &SkillParameterError{Message: "parameter name is required"}
	}

	param.ID = uuid.New().String()
	param.CreatedAt = time.Now()
	param.UpdatedAt = time.Now()
	return s.db.Create(param).Error
}

// GetSkillParameter 获取单个技能参数
func (s *SkillService) GetSkillParameter(skillID, paramID string) (*models.SkillParameter, error) {
	var param models.SkillParameter
	err := s.db.Where("skill_id = ? AND id = ?", skillID, paramID).First(&param).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("skill parameter not found: %w", gorm.ErrRecordNotFound)
		}
		return nil, err
	}
	return &param, nil
}

// UpdateSkillParameter 更新技能参数
func (s *SkillService) UpdateSkillParameter(skillID, paramID string, data map[string]interface{}) (*models.SkillParameter, error) {
	param, err := s.GetSkillParameter(skillID, paramID)
	if err != nil {
		return nil, err
	}

	if v, ok := data["name"].(string); ok {
		name := strings.TrimSpace(v)
		if name == "" {
			return nil, &SkillParameterError{Message: "parameter name is required"}
		}
		param.Name = name
	}
	if v, ok := data["description"].(string); ok {
		param.Description = v
	}
	if v, ok := data["type"].(string); ok {
		normalizedType := normalizeParameterType(v)
		if normalizedType == "" {
			normalizedType = "string"
		}
		param.Type = normalizedType
	}
	if required, ok := data["required"].(*bool); ok {
		param.Required = *required
	}
	if defaultValue, exists := data["default"]; exists {
		if defaultValue == nil {
			param.Default = nil
		} else {
			param.Default = &models.JSONAny{Data: defaultValue}
		}
	}

	param.UpdatedAt = time.Now()
	if err := s.db.Save(param).Error; err != nil {
		return nil, err
	}
	return param, nil
}

// DeleteSkillParameter 删除技能参数
func (s *SkillService) DeleteSkillParameter(skillID, paramID string) error {
	param, err := s.GetSkillParameter(skillID, paramID)
	if err != nil {
		return err
	}
	return s.db.Delete(param).Error
}

// GetSkillParameters 获取技能参数
func (s *SkillService) GetSkillParameters(skillID string) ([]models.SkillParameter, error) {
	var params []models.SkillParameter
	err := s.db.Where("skill_id = ?", skillID).Order("created_at ASC").Find(&params).Error
	return params, err
}

// ==================== 技能匹配引擎 ====================

// MatchSkill 根据用户输入匹配最佳技能
// 返回置信度最高的技能，如果没有匹配返回 nil
func (s *SkillService) MatchSkill(content string) *SkillMatchResult {
	if content == "" {
		return nil
	}

	contentLower := strings.ToLower(content)

	skills, err := s.ListEnabledSkills()
	if err != nil {
		return nil
	}

	var bestMatch *SkillMatchResult
	bestScore := 0.0

	for i := range skills {
		skill := &skills[i]
		triggers := skill.Triggers
		if len(triggers) == 0 {
			continue
		}

		for _, trigger := range triggers {
			triggerLower := strings.ToLower(trigger)

			if !strings.Contains(contentLower, triggerLower) {
				continue
			}

			// 计算置信度：触发词长度 / 内容长度，加权
			score := float64(len(trigger)) / float64(len(content))
			if score > 1.0 {
				score = 1.0
			}
			// 长触发词匹配权重更高
			score *= (1.0 + float64(len(trigger))/20.0)
			if score > 1.0 {
				score = 1.0
			}

			if score > bestScore {
				bestScore = score
				bestMatch = &SkillMatchResult{
					Skill:          skill,
					Confidence:     score,
					MatchedTrigger: trigger,
				}
			}
		}
	}

	return bestMatch
}

// NeedsSkillExecution 判断是否需要使用技能
func (s *SkillService) NeedsSkillExecution(content string) bool {
	return s.MatchSkill(content) != nil
}

// ExecuteMatchedSkill 匹配并执行技能
func (s *SkillService) ExecuteMatchedSkill(ctx context.Context, content string, userID string) (*models.SkillExecution, error) {
	match := s.MatchSkill(content)
	if match == nil {
		return nil, fmt.Errorf("no matching skill found")
	}
	return s.ExecuteSkillWithContent(ctx, match.Skill, content, userID)
}

// ExecuteSkillWithContent 使用技能处理用户内容
func (s *SkillService) ExecuteSkillWithContent(ctx context.Context, skill *models.Skill, content, userID string) (*models.SkillExecution, error) {
	finalParams, err := s.buildExecutionParameters(ctx, skill, content, userID, nil, true)
	if err != nil {
		return nil, err
	}

	execution := &models.SkillExecution{
		ID:         uuid.New().String(),
		SkillID:    skill.ID,
		SkillName:  skill.Name,
		Parameters: finalParams,
		Status:     "running",
		StartedAt:  time.Now(),
	}

	result, err := s.executeSkillLogic(ctx, skill, finalParams)
	if err != nil {
		execution.Status = "failed"
		execution.Error = err.Error()
		execution.EndedAt = time.Now()
		s.db.Create(execution)
		return execution, err
	}

	execution.Status = "completed"
	if resultMap, ok := result.(map[string]interface{}); ok {
		execution.Result = &models.JSONAny{Data: resultMap}
	} else if str, ok := result.(string); ok {
		execution.Result = &models.JSONAny{Data: map[string]interface{}{"output": str}}
	}
	execution.EndedAt = time.Now()
	s.db.Create(execution)
	return execution, nil
}

// ==================== 技能执行 ====================

// ExecuteSkill 执行技能（兼容旧接口）
func (s *SkillService) ExecuteSkill(skillID string, parameters map[string]interface{}) (*models.SkillExecution, error) {
	skill, err := s.GetSkill(skillID)
	if err != nil {
		return nil, err
	}

	content, _ := parameters["content"].(string)
	userID, _ := parameters["user_id"].(string)
	finalParams, err := s.buildExecutionParameters(context.Background(), skill, content, userID, parameters, false)
	if err != nil {
		return nil, err
	}

	execution := &models.SkillExecution{
		ID:         uuid.New().String(),
		SkillID:    skillID,
		SkillName:  skill.Name,
		Parameters: finalParams,
		Status:     "running",
		StartedAt:  time.Now(),
	}

	result, err := s.executeSkillLogic(context.Background(), skill, finalParams)
	if err != nil {
		execution.Status = "failed"
		execution.Error = err.Error()
		execution.EndedAt = time.Now()
		s.db.Create(execution)
		return execution, err
	}

	execution.Status = "completed"
	if resultMap, ok := result.(map[string]interface{}); ok {
		execution.Result = &models.JSONAny{Data: resultMap}
	} else if str, ok := result.(string); ok {
		execution.Result = &models.JSONAny{Data: map[string]interface{}{"output": str}}
	}
	execution.EndedAt = time.Now()
	s.db.Create(execution)
	return execution, nil
}

// executeSkillLogic 执行技能逻辑
func (s *SkillService) executeSkillLogic(ctx context.Context, skill *models.Skill, parameters map[string]interface{}) (interface{}, error) {
	// 优先使用注册的执行器
	if exec, ok := s.skillExecs[skill.Name]; ok {
		return exec.Execute(ctx, parameters)
	}

	// 如果有 system_prompt_override，使用 LLM 执行
	if skill.SystemPromptOverride != "" && s.modelSvc != nil {
		return s.executeLLMBasedSkill(ctx, skill, parameters)
	}

	return nil, fmt.Errorf("skill %s has no executor or system prompt", skill.Name)
}

// executeLLMBasedSkill 基于 LLM 的技能执行
func (s *SkillService) executeLLMBasedSkill(ctx context.Context, skill *models.Skill, parameters map[string]interface{}) (interface{}, error) {
	content, _ := parameters["content"].(string)

	defs, err := s.GetSkillParameters(skill.ID)
	if err == nil && len(defs) > 0 {
		structuredParams := filterDeclaredParameters(defs, parameters)
		if len(structuredParams) > 0 {
			payload, marshalErr := json.MarshalIndent(structuredParams, "", "  ")
			if marshalErr == nil {
				var parts []string
				if strings.TrimSpace(content) != "" {
					parts = append(parts, content)
				}
				parts = append(parts, "已提取的技能参数(JSON)：\n"+string(payload))
				content = strings.Join(parts, "\n\n")
			}
		}
	}
	if strings.TrimSpace(content) == "" {
		content = "请根据技能参数完成任务。"
	}

	// 选择模型：优先按 ModelPreference 标签匹配
	modelID := ""
	if skill.ModelPreference != "" {
		var model models.Model
		err := s.modelSvc.db.Where("status = ? AND type = ? AND tags LIKE ?",
			"enabled", "llm", "%\""+skill.ModelPreference+"\"%").
			Order("priority DESC").First(&model).Error
		if err == nil {
			modelID = model.ID
		}
	}
	// 回退到默认模型
	if modelID == "" {
		defaultModel, err := s.modelSvc.GetDefaultModel()
		if err != nil {
			return nil, fmt.Errorf("no model available: %w", err)
		}
		modelID = defaultModel.ID
	}

	// 构建消息
	messages := []llm.Message{
		{Role: "user", Content: content},
	}
	options := map[string]interface{}{
		"system":      skill.SystemPromptOverride,
		"temperature": 0.7,
	}

	resp, err := s.modelSvc.Chat(modelID, messages, options)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	if len(resp.Choices) > 0 {
		return resp.Choices[0].Message.Content, nil
	}
	return "", nil
}

// GetSkillExecutions 获取技能执行历史
func (s *SkillService) GetSkillExecutions(skillID string) ([]models.SkillExecution, error) {
	var executions []models.SkillExecution
	err := s.db.Where("skill_id = ?", skillID).Order("started_at DESC").Find(&executions).Error
	return executions, err
}

// RegisterExecutor 注册技能执行器
func (s *SkillService) RegisterExecutor(name string, executor SkillExecutor) {
	s.skillExecs[name] = executor
}

// ResolveModelID 根据 ModelPreference 解析模型 ID
// 如果偏好标签匹配成功返回对应模型 ID，否则返回默认模型 ID
func (s *SkillService) ResolveModelID(ctx context.Context, modelPreference string) (string, error) {
	if modelPreference == "" {
		defaultModel, err := s.modelSvc.GetDefaultModel()
		if err != nil {
			return "", err
		}
		return defaultModel.ID, nil
	}

	// 按标签匹配模型
	var model models.Model
	err := s.modelSvc.db.Where("status = ? AND type = ? AND tags LIKE ?",
		"enabled", "llm", "%\""+modelPreference+"\"%").
		Order("priority DESC").First(&model).Error
	if err == nil {
		return model.ID, nil
	}

	// 回退到默认模型
	defaultModel, err := s.modelSvc.GetDefaultModel()
	if err != nil {
		return "", fmt.Errorf("no model available for preference '%s': %w", modelPreference, err)
	}
	return defaultModel.ID, nil
}

// TrackSkillExecution 记录技能执行（不实际执行，仅写入 DB 用于追踪）
func (s *SkillService) TrackSkillExecution(skillID, skillName string, parameters map[string]interface{}, status string) *models.SkillExecution {
	execution := &models.SkillExecution{
		ID:         uuid.New().String(),
		SkillID:    skillID,
		SkillName:  skillName,
		Parameters: cloneStringAnyMap(parameters),
		Status:     status,
		StartedAt:  time.Now(),
		EndedAt:    time.Now(),
	}
	if err := s.db.Create(execution).Error; err != nil {
		log.Printf("[SkillService] failed to track execution: %v", err)
	}
	return execution
}

func (s *SkillService) buildExecutionParameters(
	ctx context.Context,
	skill *models.Skill,
	content, userID string,
	providedParams map[string]interface{},
	allowExtraction bool,
) (map[string]interface{}, error) {
	params := cloneStringAnyMap(providedParams)
	if content != "" {
		params["content"] = content
	}
	if userID != "" {
		params["user_id"] = userID
	}

	defs, err := s.GetSkillParameters(skill.ID)
	if err != nil {
		return nil, err
	}
	if len(defs) == 0 {
		return params, nil
	}

	if allowExtraction && strings.TrimSpace(content) != "" {
		extracted, err := s.ExtractParametersFromContent(ctx, skill, defs, content)
		if err != nil {
			return nil, err
		}
		for key, value := range extracted {
			if _, exists := params[key]; !exists {
				params[key] = value
			}
		}
	}

	return normalizeParameters(defs, params)
}

// ExtractParametersFromContent 从自然语言中提取技能参数
func (s *SkillService) ExtractParametersFromContent(
	ctx context.Context,
	skill *models.Skill,
	defs []models.SkillParameter,
	content string,
) (map[string]interface{}, error) {
	if len(defs) == 0 || strings.TrimSpace(content) == "" {
		return map[string]interface{}{}, nil
	}
	if s.modelSvc == nil {
		return nil, fmt.Errorf("model service not available")
	}

	modelID, err := s.ResolveModelID(ctx, skill.ModelPreference)
	if err != nil {
		return nil, err
	}

	resp, err := s.modelSvc.Chat(modelID, []llm.Message{{Role: llm.RoleUser, Content: content}}, map[string]interface{}{
		"system":      buildParameterExtractionPrompt(skill, defs),
		"temperature": 0.1,
		"max_tokens":  800,
	})
	if err != nil {
		return nil, fmt.Errorf("parameter extraction failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from parameter extractor")
	}

	return parseParameterExtractionResponse(resp.Choices[0].Message.Content)
}

// ==================== 内置技能 ====================

// registerBuiltinExecutors 注册内置技能执行器
func (s *SkillService) registerBuiltinExecutors() {
	s.skillExecs["translation"] = &translationSkillExecutor{}
	s.skillExecs["code_review"] = &codeReviewSkillExecutor{}
	s.skillExecs["summarization"] = &summarizationSkillExecutor{}
}

// InitBuiltinSkills 初始化内置技能（如果不存在则创建）
func (s *SkillService) InitBuiltinSkills() {
	builtins := []models.Skill{
		{
			Name:                 "translation",
			Description:          "多语言翻译技能，支持中英日韩等多种语言互译",
			Category:             "language",
			Version:              "1.0",
			Author:               "system",
			Enabled:              true,
			Builtin:              true,
			Triggers:             models.JSONSlice{"翻译", "translate", "译成", "翻译成", "translate to"},
			SystemPromptOverride: "你是一个专业翻译。请将用户输入的内容翻译为目标语言。保持原文的格式、语气和专业术语。如果用户没有指定目标语言，默认翻译为英语。",
			ModelPreference:      "",
		},
		{
			Name:                 "code_review",
			Description:          "代码审查技能，分析代码质量、安全性和最佳实践",
			Category:             "development",
			Version:              "1.0",
			Author:               "system",
			Enabled:              true,
			Builtin:              true,
			Triggers:             models.JSONSlice{"代码审查", "code review", "审查代码", "review code", "代码评审"},
			SystemPromptOverride: "你是一个资深代码审查专家。请从以下维度审查用户提交的代码：\n1. 代码质量（命名、结构、可读性）\n2. 潜在 Bug 和边界情况\n3. 安全漏洞（注入、XSS 等）\n4. 性能问题\n5. 最佳实践建议\n\n请用清晰的中文给出审查意见，按严重程度分级（严重/建议/优化）。",
			ModelPreference:      "code",
		},
		{
			Name:                 "summarization",
			Description:          "文本摘要技能，生成简洁准确的内容摘要",
			Category:             "content",
			Version:              "1.0",
			Author:               "system",
			Enabled:              true,
			Builtin:              true,
			Triggers:             models.JSONSlice{"总结", "摘要", "summarize", "summary", "概括", "归纳"},
			SystemPromptOverride: "你是一个专业的文本摘要专家。请为用户提供简洁准确的摘要。要求：\n1. 保留核心信息和关键数据\n2. 使用简洁清晰的语言\n3. 按逻辑顺序组织内容\n4. 如果内容较长，使用分点列表",
			ModelPreference:      "",
		},
		{
			Name:                 "data_analysis",
			Description:          "数据分析技能，帮助用户分析数据并提供洞察",
			Category:             "analytics",
			Version:              "1.0",
			Author:               "system",
			Enabled:              true,
			Builtin:              true,
			Triggers:             models.JSONSlice{"数据分析", "data analysis", "分析数据", "统计"},
			SystemPromptOverride: "你是一个数据分析专家。请帮助用户分析数据、识别模式并提供洞察。使用清晰的结构化格式展示分析结果，包括：\n1. 数据概览\n2. 关键发现\n3. 趋势分析\n4. 建议和结论",
			ModelPreference:      "reasoning",
		},
		{
			Name:                 "daily_report",
			Description:          "日报生成技能，根据工作内容生成结构化日报",
			Category:             "productivity",
			Version:              "1.0",
			Author:               "system",
			Enabled:              true,
			Builtin:              true,
			Triggers:             models.JSONSlice{"日报", "daily report", "工作日报", "写日报", "生成日报"},
			SystemPromptOverride: "你是一个日报生成助手。请根据用户提供的工作内容，生成结构化的日报。格式如下：\n\n## 今日工作\n- [完成的具体工作项]\n\n## 遇到的问题\n- [问题描述及解决方案]\n\n## 明日计划\n- [计划的工作项]\n\n## 备注\n- [其他需要记录的事项]",
			ModelPreference:      "",
		},
	}

	for _, skill := range builtins {
		var existing models.Skill
		err := s.db.Where("name = ? AND builtin = ?", skill.Name, true).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			if err := s.CreateSkill(&skill); err != nil {
				log.Printf("[SkillService] failed to create builtin skill %s: %v", skill.Name, err)
			} else {
				log.Printf("[SkillService] created builtin skill: %s", skill.Name)
			}
		}
	}
}

// ==================== 内置技能执行器 ====================

// translationSkillExecutor 翻译技能执行器
type translationSkillExecutor struct{}

func (e *translationSkillExecutor) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	content, _ := params["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("翻译内容不能为空")
	}
	// 翻译由 LLM 的 system_prompt_override 处理，执行器只做参数校验
	return map[string]interface{}{
		"output": content,
		"skill":  "translation",
		"status": "llm_processing",
	}, nil
}

// codeReviewSkillExecutor 代码审查执行器
type codeReviewSkillExecutor struct{}

func (e *codeReviewSkillExecutor) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	content, _ := params["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("代码内容不能为空")
	}
	return map[string]interface{}{
		"output": content,
		"skill":  "code_review",
		"status": "llm_processing",
	}, nil
}

// summarizationSkillExecutor 摘要执行器
type summarizationSkillExecutor struct{}

func (e *summarizationSkillExecutor) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	content, _ := params["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("摘要内容不能为空")
	}
	return map[string]interface{}{
		"output": content,
		"skill":  "summarization",
		"status": "llm_processing",
	}, nil
}

// ==================== 辅助函数 ====================

func buildParameterExtractionPrompt(skill *models.Skill, defs []models.SkillParameter) string {
	definitions := make([]map[string]interface{}, 0, len(defs))
	for _, def := range defs {
		definition := map[string]interface{}{
			"name":        def.Name,
			"description": def.Description,
			"type":        normalizeParameterType(def.Type),
			"required":    def.Required,
		}
		if def.Default != nil {
			definition["default"] = def.Default.Data
		}
		definitions = append(definitions, definition)
	}
	payload, _ := json.Marshal(definitions)

	return fmt.Sprintf(`你是技能参数提取器。请从用户输入中提取技能 "%s" 所需的结构化参数。

规则：
1. 只能返回 JSON 对象，不要 Markdown，不要解释，不要代码块。
2. 只返回参数名到参数值的映射。
3. 无法确定的可选参数不要输出。
4. 无法确定的必填参数也不要猜测。
5. 布尔值返回 true/false，数值返回 JSON number，数组返回 JSON array，对象返回 JSON object。

参数定义：%s`, skill.Name, string(payload))
}

func parseParameterExtractionResponse(content string) (map[string]interface{}, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return map[string]interface{}{}, nil
	}
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse extracted parameters: %w", err)
	}
	if paramsRaw, ok := result["parameters"]; ok {
		if paramsMap, ok := paramsRaw.(map[string]interface{}); ok {
			return paramsMap, nil
		}
	}
	return result, nil
}

func normalizeParameters(defs []models.SkillParameter, raw map[string]interface{}) (map[string]interface{}, error) {
	normalized := cloneStringAnyMap(raw)
	for _, def := range defs {
		value, exists := raw[def.Name]
		if !exists || isMissingParameterValue(value) {
			if def.Default != nil {
				value = def.Default.Data
				exists = true
			} else if def.Required {
				return nil, &SkillParameterError{Message: fmt.Sprintf("parameter '%s' is required", def.Name)}
			} else {
				delete(normalized, def.Name)
				continue
			}
		}

		coerced, err := coerceParameterValue(def, value)
		if err != nil {
			return nil, &SkillParameterError{Message: fmt.Sprintf("parameter '%s': %v", def.Name, err)}
		}
		if exists {
			normalized[def.Name] = coerced
		}
	}
	return normalized, nil
}

func filterDeclaredParameters(defs []models.SkillParameter, parameters map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for _, def := range defs {
		if value, ok := parameters[def.Name]; ok {
			result[def.Name] = value
		}
	}
	return result
}

func cloneStringAnyMap(input map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for key, value := range input {
		result[key] = value
	}
	return result
}

func isMissingParameterValue(value interface{}) bool {
	if value == nil {
		return true
	}
	if str, ok := value.(string); ok {
		return strings.TrimSpace(str) == ""
	}
	return false
}

func normalizeParameterType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "string", "text":
		return strings.ToLower(strings.TrimSpace(value))
	case "int", "integer":
		return "integer"
	case "float", "double", "number":
		return "number"
	case "bool", "boolean":
		return "boolean"
	case "array", "list":
		return "array"
	case "object", "map", "json":
		return "object"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func coerceParameterValue(def models.SkillParameter, value interface{}) (interface{}, error) {
	switch normalizeParameterType(def.Type) {
	case "", "string":
		return coerceStringValue(value), nil
	case "integer":
		return coerceIntegerValue(value)
	case "number":
		return coerceNumberValue(value)
	case "boolean":
		return coerceBooleanValue(value)
	case "array":
		return coerceArrayValue(value)
	case "object":
		return coerceObjectValue(value)
	default:
		return value, nil
	}
}

func coerceStringValue(value interface{}) string {
	if str, ok := value.(string); ok {
		return str
	}
	return fmt.Sprintf("%v", value)
}

func coerceIntegerValue(value interface{}) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int8:
		return int(v), nil
	case int16:
		return int(v), nil
	case int32:
		return int(v), nil
	case int64:
		return int(v), nil
	case float32:
		if math.Trunc(float64(v)) != float64(v) {
			return 0, fmt.Errorf("expected integer")
		}
		return int(v), nil
	case float64:
		if math.Trunc(v) != v {
			return 0, fmt.Errorf("expected integer")
		}
		return int(v), nil
	case json.Number:
		parsed, err := v.Int64()
		if err != nil {
			return 0, fmt.Errorf("expected integer")
		}
		return int(parsed), nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, fmt.Errorf("expected integer")
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("expected integer")
	}
}

func coerceNumberValue(value interface{}) (float64, error) {
	switch v := value.(type) {
	case int:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	case json.Number:
		parsed, err := v.Float64()
		if err != nil {
			return 0, fmt.Errorf("expected number")
		}
		return parsed, nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, fmt.Errorf("expected number")
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("expected number")
	}
}

func coerceBooleanValue(value interface{}) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes", "y", "on":
			return true, nil
		case "false", "0", "no", "n", "off":
			return false, nil
		default:
			return false, fmt.Errorf("expected boolean")
		}
	case int:
		return v != 0, nil
	case int64:
		return v != 0, nil
	case float64:
		return v != 0, nil
	default:
		return false, fmt.Errorf("expected boolean")
	}
}

func coerceArrayValue(value interface{}) ([]interface{}, error) {
	switch v := value.(type) {
	case []interface{}:
		return v, nil
	case []string:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = item
		}
		return result, nil
	case string:
		trimmed := strings.TrimSpace(v)
		if strings.HasPrefix(trimmed, "[") {
			var result []interface{}
			if err := json.Unmarshal([]byte(trimmed), &result); err == nil {
				return result, nil
			}
		}
		parts := strings.Split(trimmed, ",")
		result := make([]interface{}, 0, len(parts))
		for _, part := range parts {
			item := strings.TrimSpace(part)
			if item != "" {
				result = append(result, item)
			}
		}
		return result, nil
	default:
		data, err := json.Marshal(v)
		if err == nil {
			var result []interface{}
			if unmarshalErr := json.Unmarshal(data, &result); unmarshalErr == nil {
				return result, nil
			}
		}
		return nil, fmt.Errorf("expected array")
	}
}

func coerceObjectValue(value interface{}) (map[string]interface{}, error) {
	switch v := value.(type) {
	case map[string]interface{}:
		return v, nil
	case string:
		trimmed := strings.TrimSpace(v)
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
			return nil, fmt.Errorf("expected object")
		}
		return result, nil
	default:
		data, err := json.Marshal(v)
		if err == nil {
			var result map[string]interface{}
			if unmarshalErr := json.Unmarshal(data, &result); unmarshalErr == nil {
				return result, nil
			}
		}
		return nil, fmt.Errorf("expected object")
	}
}

// getString 安全获取 map 中的字符串值
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// toJSONSlice 将 interface{} 转为 models.JSONSlice
func toJSONSlice(v interface{}) models.JSONSlice {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case models.JSONSlice:
		return val
	case []string:
		return val
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// toJSONMap 将 interface{} 转为 models.JSONMap
func toJSONMap(v interface{}) models.JSONMap {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case models.JSONMap:
		return val
	case map[string]interface{}:
		return val
	}
	return nil
}

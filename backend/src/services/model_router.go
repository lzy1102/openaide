package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"openaide/backend/src/models"
)

// RouteInfo 路由决策详情
type RouteInfo struct {
	TaskType      string  `json:"task_type"`
	SelectedModel string  `json:"selected_model"`
	Confidence    float64 `json:"confidence"`
	Reason        string  `json:"reason"`
}

// 任务类型及其关键词
var taskKeywords = map[string][]string{
	"code": {
		"写代码", "编程", "函数", "实现", "debug", "修复", "重构", "开发",
		"code", "function", "implement", "program", "script", "class", "method",
		"api", "接口", "模块", "组件", "bug", "error", "compile",
	},
	"reasoning": {
		"分析", "推理", "逻辑", "证明", "计算", "思考", "比较", "评估",
		"analyze", "reason", "logic", "calculate", "compare", "evaluate",
		"optimize", "优化", "方案", "策略", "design",
	},
	"creative": {
		"写", "创作", "翻译", "故事", "诗歌", "文案", "总结", "续写",
		"write", "create", "translate", "story", "poem", "compose",
		"summarize", "rewrite", "润色", "draft",
	},
	"knowledge": {
		"是什么", "为什么", "解释", "什么是", "如何",
		"what is", "why", "explain", "define", "describe",
		"介绍", "概念", "原理", "区别",
	},
}

// 任务类型优先级（平分时按此顺序选择）
var taskTypePriority = map[string]int{
	"code":      4,
	"reasoning": 3,
	"creative":  2,
	"knowledge": 1,
	"chat":      0,
}

// ModelRouter 模型路由服务
type ModelRouter struct {
	db       *gorm.DB
	modelSvc *ModelService
	logger   *LoggerService
}

// NewModelRouter 创建模型路由服务
func NewModelRouter(db *gorm.DB, modelSvc *ModelService, logger *LoggerService) *ModelRouter {
	return &ModelRouter{
		db:       db,
		modelSvc: modelSvc,
		logger:   logger,
	}
}

// Route 根据用户输入内容自动选择最合适的模型
func (r *ModelRouter) Route(ctx context.Context, content string, requiredCapabilities map[string]bool) (*models.Model, error) {
	taskType := r.classifyTask(content)

	var matchedModels []models.Model

	// 按标签查询匹配的模型（使用 LIKE 匹配 JSON 数组中的标签）
	if taskType != "chat" {
		r.db.Raw(`
			SELECT * FROM models
			WHERE status = 'enabled' AND type = 'llm'
			AND (
				tags LIKE ? OR tags LIKE ? OR tags LIKE ? OR
				tags LIKE ? OR tags LIKE ? OR tags = ?
			)
			ORDER BY priority DESC, created_at ASC
		`,
			"%\""+taskType+"\"%",
			"%\""+taskType+",%",
			"%,"+taskType+"\"%",
			"%,"+taskType+",%",
			"[\""+taskType+"\"]",
			"\""+taskType+"\"",
		).Scan(&matchedModels)

		// 过滤能力
		if len(requiredCapabilities) > 0 {
			matchedModels = r.filterByCapabilities(matchedModels, requiredCapabilities)
		}
	}

	// 无匹配，获取默认模型
	if len(matchedModels) == 0 {
		defaultModel, err := r.modelSvc.GetDefaultModel()
		if err != nil {
			return nil, fmt.Errorf("no suitable model found: %w", err)
		}

		// 默认模型也需要能力过滤
		if len(requiredCapabilities) > 0 && !r.hasCapabilities(defaultModel, requiredCapabilities) {
			allModels, err := r.modelSvc.ListEnabledModels()
			if err == nil {
				filtered := r.filterByCapabilities(allModels, requiredCapabilities)
				if len(filtered) > 0 {
					return &filtered[0], nil
				}
			}
			return nil, fmt.Errorf("no model with required capabilities: %v", requiredCapabilities)
		}

		return defaultModel, nil
	}

	return &matchedModels[0], nil
}

// RouteWithFallback 路由并支持降级
// triedModelIDs 是已尝试过并失败的模型 ID 列表
func (r *ModelRouter) RouteWithFallback(ctx context.Context, content string, requiredCapabilities map[string]bool, triedModelIDs []string) (*models.Model, error) {
	taskType := r.classifyTask(content)

	var candidates []models.Model

	if taskType != "chat" {
		r.db.Raw(`
			SELECT * FROM models
			WHERE status = 'enabled' AND type = 'llm'
			AND (
				tags LIKE ? OR tags LIKE ? OR tags LIKE ? OR
				tags LIKE ? OR tags LIKE ? OR tags = ?
			)
			ORDER BY priority DESC, created_at ASC
		`,
			"%\""+taskType+"\"%",
			"%\""+taskType+",%",
			"%,"+taskType+"\"%",
			"%,"+taskType+",%",
			"[\""+taskType+"\"]",
			"\""+taskType+"\"",
		).Scan(&candidates)
	}

	// 能力过滤
	if len(requiredCapabilities) > 0 {
		candidates = r.filterByCapabilities(candidates, requiredCapabilities)
	}

	// 排除已尝试的模型
	triedSet := make(map[string]bool)
	for _, id := range triedModelIDs {
		triedSet[id] = true
	}

	for _, m := range candidates {
		if !triedSet[m.ID] {
			return &m, nil
		}
	}

	// 从所有 enabled 模型中找
	allModels, err := r.modelSvc.ListEnabledModels()
	if err == nil {
		if len(requiredCapabilities) > 0 {
			allModels = r.filterByCapabilities(allModels, requiredCapabilities)
		}
		for _, m := range allModels {
			if !triedSet[m.ID] {
				return &m, nil
			}
		}
	}

	return nil, fmt.Errorf("no fallback model available (tried %d)", len(triedModelIDs))
}

// GetRouteInfo 返回路由决策详情
func (r *ModelRouter) GetRouteInfo(ctx context.Context, content string) RouteInfo {
	taskType := r.classifyTask(content)
	confidence := r.calculateConfidence(content, taskType)

	model, err := r.Route(ctx, content, nil)
	selectedModel := ""
	reason := ""
	if err != nil {
		reason = err.Error()
	} else {
		selectedModel = model.Name
		reason = fmt.Sprintf("task type '%s' matched model '%s' (priority=%d)", taskType, model.Name, model.Priority)
	}

	return RouteInfo{
		TaskType:      taskType,
		SelectedModel: selectedModel,
		Confidence:    confidence,
		Reason:        reason,
	}
}

// classifyTask 分析用户输入，返回任务类型
func (r *ModelRouter) classifyTask(content string) string {
	contentLower := strings.ToLower(content)

	bestType := "chat"
	bestScore := 0
	bestPriority := 0

	for taskType, keywords := range taskKeywords {
		score := 0
		for _, kw := range keywords {
			if strings.Contains(contentLower, strings.ToLower(kw)) {
				score++
			}
		}

		priority := taskTypePriority[taskType]

		// 更高分数，或同分更高优先级
		if score > bestScore || (score == bestScore && score > 0 && priority > bestPriority) {
			bestType = taskType
			bestScore = score
			bestPriority = priority
		}
	}

	return bestType
}

// calculateConfidence 计算分类置信度
func (r *ModelRouter) calculateConfidence(content, taskType string) float64 {
	if taskType == "chat" {
		return 0.3 // 默认类型，低置信度
	}

	contentLower := strings.ToLower(content)
	keywords := taskKeywords[taskType]
	hitCount := 0
	for _, kw := range keywords {
		if strings.Contains(contentLower, strings.ToLower(kw)) {
			hitCount++
		}
	}

	confidence := float64(hitCount) / float64(len(keywords))
	if confidence > 0.95 {
		confidence = 0.95
	}
	if confidence < 0.1 && hitCount > 0 {
		confidence = 0.1
	}
	return confidence
}

// filterByCapabilities 按能力过滤模型列表
func (r *ModelRouter) filterByCapabilities(modelList []models.Model, required map[string]bool) []models.Model {
	var result []models.Model
	for _, m := range modelList {
		if r.hasCapabilities(&m, required) {
			result = append(result, m)
		}
	}
	return result
}

// hasCapabilities 检查模型是否具有所需能力
func (r *ModelRouter) hasCapabilities(model *models.Model, required map[string]bool) bool {
	if len(required) == 0 {
		return true
	}

	capBytes, err := json.Marshal(model.Capabilities)
	if err != nil {
		return false
	}

	var caps map[string]interface{}
	if err := json.Unmarshal(capBytes, &caps); err != nil {
		return false
	}

	for capName, needed := range required {
		if !needed {
			continue
		}
		val, ok := caps[capName]
		if !ok {
			return false
		}
		boolVal, ok := val.(bool)
		if !ok || !boolVal {
			return false
		}
	}

	return true
}

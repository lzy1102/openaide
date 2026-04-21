package services

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// CostOptimizer 成本优化服务
// 根据任务类型和token消耗自动选择最优模型
type CostOptimizer struct {
	modelService   *ModelService
	usageService   *UsageService
	tokenEstimator *TokenEstimator
	logger         *LoggerService

	// 模型成本历史（用于动态调整）
	modelCostHistory map[string]*ModelCostStats
	historyMutex     sync.RWMutex
}

// ModelCostStats 模型成本统计
type ModelCostStats struct {
	ModelID       string    `json:"model_id"`
	AvgCostPer1K  float64   `json:"avg_cost_per_1k"`  // 每1K token的平均成本
	SuccessRate   float64   `json:"success_rate"`     // 成功率
	AvgLatency    float64   `json:"avg_latency"`      // 平均延迟（毫秒）
	TotalCalls    int64     `json:"total_calls"`
	TotalTokens   int64     `json:"total_tokens"`
	TotalCost     float64   `json:"total_cost"`
	LastUpdated   time.Time `json:"last_updated"`
}

// ModelPricing 模型定价信息
type ModelPricing struct {
	ModelID        string  `json:"model_id"`
	InputPrice     float64 `json:"input_price"`     // 每1K输入token的价格（美元）
	OutputPrice    float64 `json:"output_price"`    // 每1K输出token的价格（美元）
	ContextLimit   int     `json:"context_limit"`   // 上下文长度限制
	QualityScore   float64 `json:"quality_score"`   // 质量评分（0-1）
	SpeedScore     float64 `json:"speed_score"`     // 速度评分（0-1）
}

// TaskType 任务类型
type TaskType string

const (
	TaskTypeCode      TaskType = "code"      // 代码生成
	TaskTypeReasoning TaskType = "reasoning" // 推理分析
	TaskTypeCreative  TaskType = "creative"  // 创意写作
	TaskTypeKnowledge TaskType = "knowledge" // 知识问答
	TaskTypeSimple    TaskType = "simple"    // 简单对话
	TaskTypeComplex   TaskType = "complex"   // 复杂任务
)

// NewCostOptimizer 创建成本优化服务
func NewCostOptimizer(modelService *ModelService, usageService *UsageService, logger *LoggerService) *CostOptimizer {
	return &CostOptimizer{
		modelService:     modelService,
		usageService:     usageService,
		tokenEstimator:   NewTokenEstimator(),
		logger:           logger,
		modelCostHistory: make(map[string]*ModelCostStats),
	}
}

// GetOptimalModel 获取最优模型
// 根据任务类型、预估token数和成本预算选择最合适的模型
func (c *CostOptimizer) GetOptimalModel(ctx context.Context, query string, preferredModel string, budget float64) (string, error) {
	// 识别任务类型
	taskType := c.identifyTaskType(query)

	// 获取预估token数
	estimatedTokens := c.tokenEstimator.EstimateTokens(query, preferredModel)

	// 获取可用模型列表
	models, err := c.modelService.ListModels()
	if err != nil {
		return preferredModel, fmt.Errorf("failed to list models: %w", err)
	}

	// 获取模型定价
	pricings := c.getModelPricings()

	// 计算每个模型的得分
	bestModel := preferredModel
	bestScore := -1.0

	for _, model := range models {
		pricing, ok := pricings[model.ID]
		if !ok {
			continue
		}

		// 检查上下文限制
		if estimatedTokens > pricing.ContextLimit {
			continue
		}

		// 计算预估成本
		estimatedCost := c.estimateCost(estimatedTokens, pricing)

		// 检查预算
		if budget > 0 && estimatedCost > budget {
			continue
		}

		// 计算模型得分（综合考虑质量、速度、成本）
		score := c.calculateModelScore(taskType, pricing, model.ID)

		if score > bestScore {
			bestScore = score
			bestModel = model.ID
		}
	}

	if bestModel != preferredModel {
		c.logger.Info(ctx, "Cost optimization: switched from %s to %s for task type %s (score: %.2f)",
			preferredModel, bestModel, taskType, bestScore)
	}

	return bestModel, nil
}

// GetBudgetFriendlyModel 获取预算友好的模型
// 当接近预算限制时，自动降级到更便宜的模型
func (c *CostOptimizer) GetBudgetFriendlyModel(ctx context.Context, userID string, preferredModel string) (string, error) {
	// 获取用户今日使用量
	if c.usageService == nil {
		return preferredModel, nil
	}

	today := time.Now().Format("2006-01-02")
	dailyUsage, err := c.usageService.GetDailyUsage(userID, today)
	if err != nil {
		return preferredModel, nil
	}

	// 获取用户预算（简化处理，实际应从用户配置获取）
	dailyBudget := 1000.0 // 默认每日预算$1

	// 计算已使用预算比例
	usedRatio := dailyUsage.TotalCost / dailyBudget

	// 根据预算使用情况选择模型
	if usedRatio >= 0.9 {
		// 已使用90%以上，强制使用最便宜的模型
		return c.getCheapestModel(preferredModel)
	} else if usedRatio >= 0.7 {
		// 已使用70%以上，优先使用便宜模型
		return c.getCheaperModel(preferredModel)
	}

	return preferredModel, nil
}

// RecordModelPerformance 记录模型性能数据
func (c *CostOptimizer) RecordModelPerformance(modelID string, tokens int64, cost float64, latency int64, success bool) {
	c.historyMutex.Lock()
	defer c.historyMutex.Unlock()

	stats, ok := c.modelCostHistory[modelID]
	if !ok {
		stats = &ModelCostStats{
			ModelID: modelID,
		}
		c.modelCostHistory[modelID] = stats
	}

	// 更新统计
	stats.TotalCalls++
	stats.TotalTokens += tokens
	stats.TotalCost += cost
	stats.LastUpdated = time.Now()

	// 计算平均成本
	if stats.TotalTokens > 0 {
		stats.AvgCostPer1K = stats.TotalCost / float64(stats.TotalTokens) * 1000
	}

	// 计算成功率
	if success {
		// 使用指数移动平均
		stats.SuccessRate = stats.SuccessRate*0.9 + 0.1
	} else {
		stats.SuccessRate = stats.SuccessRate * 0.9
	}

	// 计算平均延迟
	if stats.AvgLatency == 0 {
		stats.AvgLatency = float64(latency)
	} else {
		stats.AvgLatency = stats.AvgLatency*0.9 + float64(latency)*0.1
	}
}

// GetCostStats 获取成本统计
func (c *CostOptimizer) GetCostStats() map[string]interface{} {
	c.historyMutex.RLock()
	defer c.historyMutex.RUnlock()

	stats := make(map[string]interface{})
	for modelID, modelStats := range c.modelCostHistory {
		stats[modelID] = map[string]interface{}{
			"avg_cost_per_1k": modelStats.AvgCostPer1K,
			"success_rate":    fmt.Sprintf("%.2f%%", modelStats.SuccessRate*100),
			"avg_latency_ms":  fmt.Sprintf("%.0f", modelStats.AvgLatency),
			"total_calls":     modelStats.TotalCalls,
			"total_tokens":    modelStats.TotalTokens,
			"total_cost":      fmt.Sprintf("$%.4f", modelStats.TotalCost),
		}
	}

	return stats
}

// identifyTaskType 识别任务类型
func (c *CostOptimizer) identifyTaskType(query string) TaskType {
	lowerQuery := strings.ToLower(query)

	// 代码相关
	codePatterns := []string{"代码", "code", "编程", "programming", "函数", "function", "bug", "debug", "错误"}
	for _, pattern := range codePatterns {
		if strings.Contains(lowerQuery, pattern) {
			return TaskTypeCode
		}
	}

	// 推理分析
	reasoningPatterns := []string{"分析", "analyze", "为什么", "why", "原因", "reason", "推理", "逻辑"}
	for _, pattern := range reasoningPatterns {
		if strings.Contains(lowerQuery, pattern) {
			return TaskTypeReasoning
		}
	}

	// 创意写作
	creativePatterns := []string{"写", "write", "创作", "create", "故事", "story", "诗歌", "poem", "文章"}
	for _, pattern := range creativePatterns {
		if strings.Contains(lowerQuery, pattern) {
			return TaskTypeCreative
		}
	}

	// 简单对话
	simplePatterns := []string{"你好", "hello", "hi", "谢谢", "thanks", "再见", "bye"}
	for _, pattern := range simplePatterns {
		if strings.Contains(lowerQuery, pattern) && len(lowerQuery) < 50 {
			return TaskTypeSimple
		}
	}

	// 复杂任务（包含多个子任务）
	if strings.Count(lowerQuery, "，")+strings.Count(lowerQuery, ",") > 3 {
		return TaskTypeComplex
	}

	// 默认知识问答
	return TaskTypeKnowledge
}

// getModelPricings 获取模型定价
func (c *CostOptimizer) getModelPricings() map[string]*ModelPricing {
	// 内置定价表（实际应从配置或数据库获取）
	return map[string]*ModelPricing{
		"gpt-4": {
			ModelID:      "gpt-4",
			InputPrice:   0.03,
			OutputPrice:  0.06,
			ContextLimit: 8192,
			QualityScore: 0.95,
			SpeedScore:   0.70,
		},
		"gpt-4-turbo": {
			ModelID:      "gpt-4-turbo",
			InputPrice:   0.01,
			OutputPrice:  0.03,
			ContextLimit: 128000,
			QualityScore: 0.93,
			SpeedScore:   0.75,
		},
		"gpt-3.5-turbo": {
			ModelID:      "gpt-3.5-turbo",
			InputPrice:   0.0005,
			OutputPrice:  0.0015,
			ContextLimit: 16385,
			QualityScore: 0.80,
			SpeedScore:   0.95,
		},
		"gpt-4o": {
			ModelID:      "gpt-4o",
			InputPrice:   0.005,
			OutputPrice:  0.015,
			ContextLimit: 128000,
			QualityScore: 0.94,
			SpeedScore:   0.90,
		},
		"gpt-4o-mini": {
			ModelID:      "gpt-4o-mini",
			InputPrice:   0.00015,
			OutputPrice:  0.0006,
			ContextLimit: 128000,
			QualityScore: 0.82,
			SpeedScore:   0.95,
		},
		"claude-3-haiku": {
			ModelID:      "claude-3-haiku",
			InputPrice:   0.00025,
			OutputPrice:  0.00125,
			ContextLimit: 200000,
			QualityScore: 0.78,
			SpeedScore:   0.95,
		},
		"deepseek-chat": {
			ModelID:      "deepseek-chat",
			InputPrice:   0.00014,
			OutputPrice:  0.00028,
			ContextLimit: 64000,
			QualityScore: 0.85,
			SpeedScore:   0.85,
		},
	}
}

// estimateCost 估算成本
func (c *CostOptimizer) estimateCost(estimatedTokens int, pricing *ModelPricing) float64 {
	// 假设输入输出比例 1:1
	inputTokens := float64(estimatedTokens) * 0.5
	outputTokens := float64(estimatedTokens) * 0.5

	cost := (inputTokens/1000)*pricing.InputPrice + (outputTokens/1000)*pricing.OutputPrice
	return cost
}

// calculateModelScore 计算模型得分
// 综合考虑质量、速度、成本
func (c *CostOptimizer) calculateModelScore(taskType TaskType, pricing *ModelPricing, modelID string) float64 {
	// 任务类型权重
	qualityWeight := 0.4
	speedWeight := 0.2
	costWeight := 0.4

	switch taskType {
	case TaskTypeCode:
		qualityWeight = 0.6
		speedWeight = 0.1
		costWeight = 0.3
	case TaskTypeReasoning:
		qualityWeight = 0.7
		speedWeight = 0.1
		costWeight = 0.2
	case TaskTypeCreative:
		qualityWeight = 0.6
		speedWeight = 0.2
		costWeight = 0.2
	case TaskTypeSimple:
		qualityWeight = 0.2
		speedWeight = 0.3
		costWeight = 0.5
	}

	// 质量得分（直接使用质量评分）
	qualityScore := pricing.QualityScore

	// 速度得分
	speedScore := pricing.SpeedScore

	// 成本得分（越便宜得分越高）
	avgPrice := (pricing.InputPrice + pricing.OutputPrice) / 2
	costScore := 1.0 / (1.0 + avgPrice*100) // 归一化

	// 历史性能调整
	c.historyMutex.RLock()
	if stats, ok := c.modelCostHistory[modelID]; ok {
		// 根据成功率调整质量得分
		qualityScore = qualityScore*0.7 + stats.SuccessRate*0.3
		// 根据延迟调整速度得分
		if stats.AvgLatency > 0 {
			latencyScore := 1.0 / (1.0 + stats.AvgLatency/1000)
			speedScore = speedScore*0.7 + latencyScore*0.3
		}
	}
	c.historyMutex.RUnlock()

	// 综合得分
	score := qualityScore*qualityWeight + speedScore*speedWeight + costScore*costWeight

	return score
}

// getCheapestModel 获取最便宜的模型
func (c *CostOptimizer) getCheapestModel(fallback string) (string, error) {
	pricings := c.getModelPricings()

	cheapest := fallback
	minPrice := -1.0

	for modelID, pricing := range pricings {
		avgPrice := (pricing.InputPrice + pricing.OutputPrice) / 2
		if minPrice < 0 || avgPrice < minPrice {
			minPrice = avgPrice
			cheapest = modelID
		}
	}

	return cheapest, nil
}

// getCheaperModel 获取更便宜的模型（但不是最便宜的）
func (c *CostOptimizer) getCheaperModel(current string) (string, error) {
	pricings := c.getModelPricings()

	currentPricing, ok := pricings[current]
	if !ok {
		return current, nil
	}

	currentPrice := (currentPricing.InputPrice + currentPricing.OutputPrice) / 2

	// 找一个便宜但至少保持80%质量的模型
	bestAlternative := current
	bestScore := -1.0

	for modelID, pricing := range pricings {
		if modelID == current {
			continue
		}

		avgPrice := (pricing.InputPrice + pricing.OutputPrice) / 2
		if avgPrice >= currentPrice {
			continue
		}

		// 质量不能下降太多
		if pricing.QualityScore < currentPricing.QualityScore*0.8 {
			continue
		}

		// 计算性价比
		score := pricing.QualityScore / avgPrice
		if score > bestScore {
			bestScore = score
			bestAlternative = modelID
		}
	}

	return bestAlternative, nil
}

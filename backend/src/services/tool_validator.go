package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// ToolResultValidator 工具结果验证器
type ToolResultValidator struct {
	maxRetries    int
	retryDelay    time.Duration
	maxResultSize int
}

// NewToolResultValidator 创建验证器
func NewToolResultValidator() *ToolResultValidator {
	return &ToolResultValidator{
		maxRetries:    3,
		retryDelay:    1 * time.Second,
		maxResultSize: 8000,
	}
}

// ValidationResult 验证结果
type ValidationResult struct {
	Valid       bool   `json:"valid"`
	Error       string `json:"error,omitempty"`
	FixedResult string `json:"fixed_result,omitempty"`
}

// ValidateToolResult 验证工具结果
func (v *ToolResultValidator) ValidateToolResult(toolName string, result interface{}) *ValidationResult {
	if result == nil {
		return &ValidationResult{
			Valid: false,
			Error: "tool returned nil result",
		}
	}

	// 序列化结果
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return &ValidationResult{
			Valid: false,
			Error: fmt.Sprintf("failed to serialize result: %v", err),
		}
	}

	resultStr := string(resultJSON)

	// 检查空结果
	if resultStr == "" || resultStr == "null" || resultStr == "{}" || resultStr == "[]" {
		return &ValidationResult{
			Valid: false,
			Error: "tool returned empty result",
		}
	}

	// 检查错误结果
	if strings.Contains(resultStr, "error") || strings.Contains(resultStr, "Error") {
		// 进一步检查是否是真正的错误
		var resultMap map[string]interface{}
		if json.Unmarshal(resultJSON, &resultMap) == nil {
			if errVal, ok := resultMap["error"]; ok && errVal != nil && errVal != "" {
				return &ValidationResult{
					Valid: false,
					Error: fmt.Sprintf("tool returned error: %v", errVal),
				}
			}
		}
	}

	// 截断过长结果
	if len(resultStr) > v.maxResultSize {
		resultStr = resultStr[:v.maxResultSize] + "\n...(result truncated, original length: " + fmt.Sprintf("%d", len(resultStr)) + ")"
	}

	return &ValidationResult{
		Valid:       true,
		FixedResult: resultStr,
	}
}

// ToolExecutorWithRetry 带重试的工具执行器
type ToolExecutorWithRetry struct {
	validator  *ToolResultValidator
	toolSvc    *ToolService
	logger     *LoggerService
}

// NewToolExecutorWithRetry 创建带重试的执行器
func NewToolExecutorWithRetry(toolSvc *ToolService, logger *LoggerService) *ToolExecutorWithRetry {
	return &ToolExecutorWithRetry{
		validator: NewToolResultValidator(),
		toolSvc:   toolSvc,
		logger:    logger,
	}
}

// ExecuteWithRetry 执行工具调用，支持重试
func (e *ToolExecutorWithRetry) ExecuteWithRetry(
	ctx context.Context,
	tc llm.ToolCall,
	dialogueID string,
	maxRetries int,
) (string, error) {
	if maxRetries <= 0 {
		maxRetries = e.validator.maxRetries
	}

	var lastErr error
	var lastResult string

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			e.logger.Info(ctx, "[ToolRetry] Retrying tool %s (attempt %d/%d)",
				tc.Function.Name, attempt+1, maxRetries)
			time.Sleep(e.validator.retryDelay * time.Duration(attempt))
		}

		// 执行工具
		result, err := e.executeSingle(ctx, tc, dialogueID)
		if err != nil {
			lastErr = err
			// 检查是否为需要确认的错误（不重试）
			var confErr *ConfirmationRequiredError
			if errors.As(err, &confErr) {
				return "", err // 直接返回确认错误
			}
			continue // 其他错误重试
		}

		// 验证结果
		validation := e.validator.ValidateToolResult(tc.Function.Name, result)
		if validation.Valid {
			return validation.FixedResult, nil
		}

		// 验证失败，记录错误并重试
		lastErr = fmt.Errorf("validation failed: %s", validation.Error)
		lastResult = validation.FixedResult
		e.logger.Warn(ctx, "[ToolRetry] Tool %s validation failed: %s",
			tc.Function.Name, validation.Error)
	}

	// 所有重试都失败了
	if lastResult != "" {
		return lastResult, fmt.Errorf("tool %s failed after %d attempts: %w",
			tc.Function.Name, maxRetries, lastErr)
	}

	return "", fmt.Errorf("tool %s failed after %d attempts: %w",
		tc.Function.Name, maxRetries, lastErr)
}

// executeSingle 执行单个工具调用
func (e *ToolExecutorWithRetry) executeSingle(
	ctx context.Context,
	tc llm.ToolCall,
	dialogueID string,
) (interface{}, error) {
	// 这里需要调用实际的工具执行逻辑
	// 由于 ToolService.ExecuteTool 需要 models.ToolCall，我们进行转换
	toolCall := &models.ToolCall{
		ID:        tc.ID,
		Name:      tc.Function.Name,
		Arguments: tc.Function.Arguments,
	}

	result, err := e.toolSvc.ExecuteTool(ctx, toolCall, "", "", "")
	if err != nil {
		return nil, err
	}

	if result.Content == nil {
		return nil, fmt.Errorf("tool returned nil content")
	}

	return result.Content, nil
}

// ToolCallMetrics 工具调用指标
type ToolCallMetrics struct {
	TotalCalls    int            `json:"total_calls"`
	SuccessCalls  int            `json:"success_calls"`
	FailedCalls   int            `json:"failed_calls"`
	RetriedCalls  int            `json:"retried_calls"`
	AvgDuration   time.Duration  `json:"avg_duration_ms"`
	ToolBreakdown map[string]int `json:"tool_breakdown"`
}

// ToolMetricsCollector 工具指标收集器
type ToolMetricsCollector struct {
	metrics   ToolCallMetrics
	mu        sync.RWMutex
}

// NewToolMetricsCollector 创建指标收集器
func NewToolMetricsCollector() *ToolMetricsCollector {
	return &ToolMetricsCollector{
		metrics: ToolCallMetrics{
			ToolBreakdown: make(map[string]int),
		},
	}
}

// RecordCall 记录工具调用
func (c *ToolMetricsCollector) RecordCall(toolName string, success bool, retried bool, duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.metrics.TotalCalls++
	c.metrics.ToolBreakdown[toolName]++

	if success {
		c.metrics.SuccessCalls++
	} else {
		c.metrics.FailedCalls++
	}

	if retried {
		c.metrics.RetriedCalls++
	}

	// 更新平均耗时
	if c.metrics.AvgDuration == 0 {
		c.metrics.AvgDuration = duration
	} else {
		c.metrics.AvgDuration = (c.metrics.AvgDuration + duration) / 2
	}
}

// GetMetrics 获取指标
func (c *ToolMetricsCollector) GetMetrics() ToolCallMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 返回副本
	m := c.metrics
	m.ToolBreakdown = make(map[string]int)
	for k, v := range c.metrics.ToolBreakdown {
		m.ToolBreakdown[k] = v
	}
	return m
}

// ResetMetrics 重置指标
func (c *ToolMetricsCollector) ResetMetrics() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.metrics = ToolCallMetrics{
		ToolBreakdown: make(map[string]int),
	}
}

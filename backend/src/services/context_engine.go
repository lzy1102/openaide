package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"openaide/backend/src/models"
	"gorm.io/gorm"
)

// CompressionMode 压缩模式类型 (参考 Hermes Agent 5种压缩策略)
type CompressionMode string

const (
	CompressionModeAggressive CompressionMode = "aggressive"
	CompressionModeBalanced   CompressionMode = "balanced"
	CompressionModeLossless   CompressionMode = "lossless"
	CompressionModeSmart      CompressionMode = "smart"
)

// CompressionConfig 压缩配置
type CompressionConfig struct {
	Mode               CompressionMode
	MaxTokens          int
	KeepLastN          int
	PreserveToolCalls  bool
	FallbackToSummary  bool
}

// DefaultCompressionConfig 默认压缩配置
var DefaultCompressionConfig = CompressionConfig{
	Mode:              CompressionModeBalanced,
	MaxTokens:         8000,
	KeepLastN:         4,
	PreserveToolCalls: true,
	FallbackToSummary: true,
}

// ContextEngine 可插拔上下文引擎接口
type ContextEngine interface {
	Compress(ctx context.Context, dialogueID string) (*CompressedContext, error)
	Summarize(ctx context.Context, dialogueID string) (string, error)
	ExtractImportantInfo(ctx context.Context, dialogueID string) (map[string]interface{}, error)
	ClearExpired(before time.Time) error
	GetMetrics() *ContextMetrics
	Name() string
}

// DefaultContextEngine 默认上下文引擎实现 (兼容原有 contextManager)
type DefaultContextEngine struct {
	db                 *gorm.DB
	dialogueService    *DialogueService
	cache              *CacheService
	logger             *LoggerService
	mu                 sync.RWMutex
	config             CompressionConfig
	compressionEnabled bool
}

func NewDefaultContextEngine(db *gorm.DB, dialogueService *DialogueService, cache *CacheService, logger *LoggerService, config CompressionConfig, compressionEnabled bool) *DefaultContextEngine {
	return &DefaultContextEngine{
		db:                 db,
		dialogueService:    dialogueService,
		cache:              cache,
		logger:             logger,
		config:             config,
		compressionEnabled: compressionEnabled,
	}
}

func (e *DefaultContextEngine) Name() string {
	return "default"
}

func (e *DefaultContextEngine) Compress(ctx context.Context, dialogueID string) (*CompressedContext, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	var dialogue models.Dialogue
	if err := e.db.Preload("Messages").First(&dialogue, "id = ?", dialogueID).Error; err != nil {
		return nil, fmt.Errorf("dialogue not found: %w", err)
	}

	if len(dialogue.Messages) == 0 {
		return nil, fmt.Errorf("no messages to compress")
	}

	var summary string
	var err error

	switch e.config.Mode {
	case CompressionModeAggressive:
		summary, err = e.compressAggressive(dialogue)
	case CompressionModeBalanced:
		summary, err = e.compressBalanced(dialogue)
	case CompressionModeLossless:
		summary, err = e.compressLossless(dialogue)
	case CompressionModeSmart:
		summary, err = e.compressSmart(dialogue)
	default:
		summary, err = e.compressBalanced(dialogue)
	}

	if err != nil && e.config.FallbackToSummary {
		summary = e.fallbackSummary(dialogue)
	} else if err != nil {
		return nil, fmt.Errorf("compression failed: %w", err)
	}

	importantInfo, _ := e.extractImportantInfo(&dialogue)

	compressedCtx := &CompressedContext{
		DialogueID:    dialogueID,
		Title:         dialogue.Title,
		Summary:       summary,
		ImportantInfo: importantInfo,
		MessageCount:  len(dialogue.Messages),
		TokenCount:    e.estimateTokenCount(dialogue.Messages),
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(24 * time.Hour),
	}

	if err := e.db.Create(compressedCtx).Error; err != nil {
		return nil, fmt.Errorf("failed to save compressed context: %w", err)
	}

	go e.ClearExpired(time.Now().Add(-24 * time.Hour))

	return compressedCtx, nil
}

func (e *DefaultContextEngine) compressAggressive(dialogue models.Dialogue) (string, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", dialogue.Title))
	sb.WriteString(fmt.Sprintf("**消息总数**: %d\n\n", len(dialogue.Messages)))

	keyPoints := e.extractKeyPoints(dialogue.Messages, 3)
	if len(keyPoints) > 0 {
		sb.WriteString("## 关键要点\n")
		for _, point := range keyPoints {
			sb.WriteString(fmt.Sprintf("- %s\n", point))
		}
	}

	return sb.String(), nil
}

func (e *DefaultContextEngine) compressBalanced(dialogue models.Dialogue) (string, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", dialogue.Title))
	sb.WriteString(fmt.Sprintf("**消息总数**: %d\n\n", len(dialogue.Messages)))

	messages := dialogue.Messages
	if len(messages) > e.config.KeepLastN {
		compressEnd := len(messages) - e.config.KeepLastN
		if e.config.PreserveToolCalls {
			compressEnd = e.findSafeCompressionBoundary(messages, compressEnd)
		}

		sb.WriteString("## 历史对话摘要\n")
		summarized := messages[:compressEnd]
		keyPoints := e.extractKeyPoints(summarized, 5)
		for _, point := range keyPoints {
			sb.WriteString(fmt.Sprintf("- %s\n", point))
		}

		if len(messages) > compressEnd {
			sb.WriteString("\n## 最近对话\n")
			for _, msg := range messages[compressEnd:] {
				sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Sender, truncate(msg.Content, 200)))
			}
		}
	} else {
		for _, msg := range messages {
			sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Sender, truncate(msg.Content, 300)))
		}
	}

	return sb.String(), nil
}

func (e *DefaultContextEngine) compressLossless(dialogue models.Dialogue) (string, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", dialogue.Title))

	messages := dialogue.Messages
	processed := make(map[int]bool)

	for i, msg := range messages {
		if processed[i] {
			continue
		}

		if isToolCall(msg) {
			sb.WriteString(fmt.Sprintf("[TOOL_CALL] %s\n", msg.Content))
			if i+1 < len(messages) && isToolResult(messages[i+1]) {
				sb.WriteString(fmt.Sprintf("[TOOL_RESULT] %s\n", truncate(messages[i+1].Content, 500)))
				processed[i+1] = true
			} else {
				sb.WriteString("[TOOL_RESULT] (结果缺失)\n")
			}
		} else if isToolResult(msg) {
			prevIndex := e.findPreviousToolCall(messages, i)
			if prevIndex == -1 {
				sb.WriteString("[TOOL_RESULT] (孤立结果，已清理)\n")
			}
		} else {
			sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Sender, truncate(msg.Content, 400)))
		}
		processed[i] = true
	}

	return sb.String(), nil
}

func (e *DefaultContextEngine) compressSmart(dialogue models.Dialogue) (string, error) {
	messages := dialogue.Messages

	hasToolCalls := false
	for _, msg := range messages {
		if isToolCall(msg) {
			hasToolCalls = true
			break
		}
	}

	if hasToolCalls {
		return e.compressLossless(dialogue)
	}

	if len(messages) > 20 {
		return e.compressAggressive(dialogue)
	}

	return e.compressBalanced(dialogue)
}

func (e *DefaultContextEngine) fallbackSummary(dialogue models.Dialogue) string {
	return fmt.Sprintf("[以下内容已被压缩以节省空间] 对话: %s, 包含 %d 条消息", dialogue.Title, len(dialogue.Messages))
}

func (e *DefaultContextEngine) extractKeyPoints(messages []models.Message, limit int) []string {
	keyPoints := make([]string, 0, limit)
	seen := make(map[string]bool)

	keyEntities := []string{"用户", "项目", "任务", "错误", "解决方案", "日期", "时间", "状态", "原因", "结果", "方法", "函数", "类", "接口", "配置", "API"}

	for _, msg := range messages {
		content := strings.ToLower(msg.Content)
		for _, keyword := range keyEntities {
			if strings.Contains(content, strings.ToLower(keyword)) {
				point := fmt.Sprintf("[%s] %s", keyword, truncate(msg.Content, 150))
				if !seen[point] && len(keyPoints) < limit {
					keyPoints = append(keyPoints, point)
					seen[point] = true
				}
			}
		}
	}

	return keyPoints
}

func (e *DefaultContextEngine) findSafeCompressionBoundary(messages []models.Message, targetIndex int) int {
	if targetIndex <= 0 || targetIndex >= len(messages) {
		return targetIndex
	}

	for i := targetIndex; i > 0; i-- {
		if !isToolCall(messages[i]) && !isToolResult(messages[i]) {
			return i
		}
	}

	return 0
}

func (e *DefaultContextEngine) findPreviousToolCall(messages []models.Message, resultIndex int) int {
	for i := resultIndex - 1; i >= 0; i-- {
		if isToolCall(messages[i]) {
			return i
		}
	}
	return -1
}

func isToolCall(msg models.Message) bool {
	return strings.Contains(strings.ToLower(msg.Content), "tool_call") || strings.Contains(strings.ToLower(msg.Content), "function_call")
}

func isToolResult(msg models.Message) bool {
	return strings.Contains(strings.ToLower(msg.Content), "tool_result") || strings.Contains(strings.ToLower(msg.Content), "function_result")
}



func (e *DefaultContextEngine) Summarize(ctx context.Context, dialogueID string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	var dialogue models.Dialogue
	if err := e.db.Preload("Messages").First(&dialogue, "id = ?", dialogueID).Error; err != nil {
		return "", fmt.Errorf("dialogue not found: %w", err)
	}

	return fmt.Sprintf("对话摘要: %s\n包含 %d 条消息", dialogue.Title, len(dialogue.Messages)), nil
}

func (e *DefaultContextEngine) ExtractImportantInfo(ctx context.Context, dialogueID string) (map[string]interface{}, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	var dialogue models.Dialogue
	if err := e.db.Preload("Messages").First(&dialogue, "id = ?", dialogueID).Error; err != nil {
		return nil, fmt.Errorf("dialogue not found: %w", err)
	}
	return e.extractImportantInfo(&dialogue)
}

func (e *DefaultContextEngine) extractImportantInfo(dialogue *models.Dialogue) (map[string]interface{}, error) {
	importantInfo := make(map[string]interface{})

	keyEntities := []string{"用户", "项目", "任务", "错误", "解决方案", "日期", "时间", "状态", "原因", "结果", "方法", "函数", "类", "接口", "配置", "API"}
	for _, msg := range dialogue.Messages {
		content := strings.ToLower(msg.Content)
		for _, keyword := range keyEntities {
			if strings.Contains(content, strings.ToLower(keyword)) {
				if _, exists := importantInfo[keyword]; !exists {
					importantInfo[keyword] = extractValue(content, keyword)
				}
			}
		}
	}
	return importantInfo, nil
}

func (e *DefaultContextEngine) GetMetrics() *ContextMetrics {
	e.mu.Lock()
	defer e.mu.Unlock()
	var metrics ContextMetrics

	e.db.Model(&CompressedContext{}).Count(&metrics.TotalContexts)
	e.db.Model(&models.Dialogue{}).Where("status = ?", "active").Count(&metrics.ActiveDialogues)

	var compressedCtxs []CompressedContext
	e.db.Find(&compressedCtxs)
	metrics.TotalMessages = 0
	metrics.TotalTokens = 0
	for _, ctx := range compressedCtxs {
		metrics.TotalMessages += int64(ctx.TokenCount)
		if ctx.CreatedAt.Before(metrics.OldestContext) || metrics.OldestContext.IsZero() {
			metrics.OldestContext = ctx.CreatedAt
		}
		if ctx.CreatedAt.After(metrics.NewestContext) {
			metrics.NewestContext = ctx.CreatedAt
		}
	}
	if metrics.TotalMessages > 0 {
		metrics.AvgTokensPerMsg = int(metrics.TotalTokens / metrics.TotalMessages)
	}
	return &metrics
}

func (e *DefaultContextEngine) ClearExpired(before time.Time) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	var expired []CompressedContext
	expiresAt := before
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(-24 * time.Hour)
	}
	if err := e.db.Where("expires_at < ?", expiresAt).Find(&expired).Error; err != nil {
		return err
	}
	for _, ctx := range expired {
		e.cache.Delete("context:" + ctx.DialogueID)
	}
	return e.db.Delete(&expired).Error
}

func (e *DefaultContextEngine) estimateTokenCount(messages []models.Message) int {
	return len(messages) * 10
}

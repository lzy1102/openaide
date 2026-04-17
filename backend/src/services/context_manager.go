package services

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"openaide/backend/src/models"
	"gorm.io/gorm"
)

// ContextManager 上下文管理服务
type ContextManager interface {
	// Compress 壓缩对话历史
	Compress(dialogueID string) (*CompressedContext, error)
	// Summarize 生成对话摘要
	Summarize(dialogueID string) (string, error)
	// ExtractImportantInfo 提取重要信息
	ExtractImportantInfo(dialogueID string) (map[string]interface{}, error)
	// ClearExpired 清理过期上下文
	ClearExpired(before time.Time) error
	// GetMetrics 获取上下文指标
	GetMetrics() *ContextMetrics
}

// CompressedContext 压缩后的的上下文
type CompressedContext struct {
	DialogueID    string    `json:"dialogue_id"`
	Title         string    `json:"title"`
	Summary       string    `json:"summary"`
	ImportantInfo map[string]interface{} `json:"important_info"`
	MessageCount  int       `json:"message_count"`
	TokenCount   int       `json:"token_count"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

// ContextMetrics 上下文指标
type ContextMetrics struct {
	TotalContexts   int64     `json:"total_contexts"`
	ActiveDialogues int64     `json:"active_dialogues"`
	TotalMessages   int64     `json:"total_messages"`
	TotalTokens     int64     `json:"total_tokens"`
	AvgTokensPerMsg int       `json:"avg_tokens_per_msg"`
	OldestContext   time.Time `json:"oldest_context"`
	NewestContext   time.Time `json:"newest_context"`
}

// contextManager 上下文管理服务实现
type contextManager struct {
	db                *gorm.DB
	dialogueService   *DialogueService
	cache             *CacheService
	logger            *LoggerService
	mu                sync.RWMutex
	maxContexts       int
	maxTokensPerCtx    int
	contextTTL        time.Duration
	compressionEnabled bool
}

// NewContextManager 创建上下文管理服务
func NewContextManager(db *gorm.DB, dialogueService *DialogueService, cache *CacheService, logger *LoggerService, maxContexts int, maxTokensPerCtx int, contextTTL time.Duration, compressionEnabled bool) ContextManager {
	return &contextManager{
		db:                db,
		dialogueService:   dialogueService,
		cache:             cache,
		logger:            logger,
		maxContexts:       maxContexts,
		maxTokensPerCtx:    maxTokensPerCtx,
		contextTTL:        contextTTL,
		compressionEnabled: compressionEnabled,
	}
}

// Compress 压缩对话历史
func (m *contextManager) Compress(dialogueID string) (*CompressedContext, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否存在
	var dialogue models.Dialogue
	if err := m.db.First(&dialogue, "id = ?", dialogueID).Error; err != nil {
		return nil, fmt.Errorf("dialogue not found")
	}

	// 生成摘要（简化版本，直接使用对话标题和内容生成）
	summary := fmt.Sprintf("对话: %s\n消息数: %d", dialogue.Title, len(dialogue.Messages))

	// 提取重要信息
	importantInfo, err := m.extractImportantInfo(dialogue)
	if err != nil {
		return nil, fmt.Errorf("failed to extract important info: %w", err)
	}

	// 创建压缩上下文
	ctx := &CompressedContext{
		DialogueID:    dialogueID,
		Title:         dialogue.Title,
		Summary:       summary,
		ImportantInfo: importantInfo,
		MessageCount:  len(dialogue.Messages),
		TokenCount:    m.estimateTokenCount(dialogue.Messages),
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(m.contextTTL),
	}

	// 保存到数据库
	if err := m.db.Create(ctx).Error; err != nil {
		return nil, fmt.Errorf("failed to save compressed context: %w", err)
	}

	// 清理过期上下文
	go m.ClearExpired(time.Now().Add(-m.contextTTL))

	return ctx, nil
}

// buildDialogueText 构建对话文本
func (m *contextManager) buildDialogueText(dialogue models.Dialogue) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", dialogue.Title))
	for _, msg := range dialogue.Messages {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Sender, msg.Content))
	}
	return sb.String()
}

// estimateTokenCount 估算 Token 数量
func (m *contextManager) estimateTokenCount(messages []models.Message) int {
	// 简单估算：每条消息约 10 tokens
	return len(messages) * 10
}

// extractImportantInfo 提取重要信息
func (m *contextManager) extractImportantInfo(dialogue models.Dialogue) (map[string]interface{}, error) {
	// 提取关键信息
	importantInfo := make(map[string]interface{})

	// 关键实体
	keyEntities := []string{
		"用户", "项目", "任务", "错误", "解决方案",
		"日期", "时间", "状态",
		"原因",
		"结果",
		"方法",
		"函数",
		"类",
		"接口",
		"配置",
		"API",
	}
	for _, msg := range dialogue.Messages {
		content := strings.ToLower(msg.Content)
		for _, keyword := range keyEntities {
			if strings.Contains(content, keyword) {
				if _, exists := importantInfo[keyword]; !exists {
					importantInfo[keyword] = extractValue(content, keyword)
				}
			}
		}
	}
	return importantInfo, nil
}
// extractValue 提取值
func extractValue(content, keyword string) string {
	start := strings.Index(content, keyword)
	if start == -1 {
		return ""
	}
	end := start + len(keyword)
	return strings.TrimSpace(content[start:end])
}

// Summarize 生成对话摘要
func (m *contextManager) Summarize(dialogueID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 加载对话
	var dialogue models.Dialogue
	if err := m.db.Preload("Messages").First(&dialogue, "id = ?", dialogueID).Error; err != nil {
		return "", fmt.Errorf("dialogue not found")
	}

	// 简化摘要生成
	return fmt.Sprintf("对话摘要: %s\n包含 %d 条消息", dialogue.Title, len(dialogue.Messages)), nil
}

// ExtractImportantInfo 提取重要信息
func (m *contextManager) ExtractImportantInfo(dialogueID string) (map[string]interface{}, error) {
	var dialogue models.Dialogue
	if err := m.db.Preload("Messages").First(&dialogue, "id = ?", dialogueID).Error; err != nil {
		return nil, fmt.Errorf("dialogue not found")
	}
	return m.extractImportantInfo(dialogue)
}

// GetMetrics 获取上下文指标
func (m *contextManager) GetMetrics() *ContextMetrics {
	m.mu.Lock()
	defer m.mu.Unlock()
	var metrics ContextMetrics
	// 统计上下文
	m.db.Model(&CompressedContext{}).Count(&metrics.TotalContexts)
	// 统计活跃对话
	m.db.Model(&models.Dialogue{}).Where("status = ?", "active").Count(&metrics.ActiveDialogues)
	// 统计消息数
	var compressedCtxs []CompressedContext
	m.db.Find(&compressedCtxs)
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
	// 计算平均值
	if metrics.TotalMessages > 0 {
		metrics.AvgTokensPerMsg = int(metrics.TotalTokens / metrics.TotalMessages)
	}
	return &metrics
}
// ClearExpired 清理过期上下文
func (m *contextManager) ClearExpired(before time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var expired []CompressedContext
	expiresAt := before
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(-m.contextTTL)
	}
	if err := m.db.Where("expires_at < ?", expiresAt).Find(&expired).Error; err != nil {
		return err
	}
	for _, ctx := range expired {
		m.cache.Delete("context:" + ctx.DialogueID)
	}
	return m.db.Delete(&expired).Error
}

package models

import "time"

// CompressedContext 压缩后的上下文存储 (参考 Hermes Agent 会话持久化)
type CompressedContext struct {
	ID            string                 `json:"id" gorm:"primaryKey;type:varchar(255)"`
	DialogueID    string                 `json:"dialogue_id" gorm:"index;type:varchar(255)"`
	Title         string                 `json:"title"`
	Summary       string                 `json:"summary" gorm:"type:text"`
	ImportantInfo JSONMap                `json:"important_info" gorm:"type:json"`
	MessageCount  int                    `json:"message_count"`
	TokenCount    int                    `json:"token_count"`
	CreatedAt     time.Time              `json:"created_at"`
	ExpiresAt     time.Time              `json:"expires_at" gorm:"index"`
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

// ContextConfig 上下文管理配置 (参考 Hermes Agent CompressionConfig)
type ContextConfig struct {
	MaxTokens          int           `json:"max_tokens"`
	KeepLastN          int           `json:"keep_last_n"`
	CompressionEnabled bool          `json:"compression_enabled"`
	CompressionMode    string        `json:"compression_mode"`
	ExpiryDuration     time.Duration `json:"expiry_duration"`
}

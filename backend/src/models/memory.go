package models

import "time"

// Memory 长期记忆模型
type Memory struct {
	ID           string    `json:"id" gorm:"primaryKey"`
	UserID       string    `json:"user_id"`
	Content      string    `json:"content"`         // 记忆内容
	Category     string    `json:"category"`        // 记忆类别
	MemoryType   string    `json:"memory_type" gorm:"default:context"` // fact, preference, procedure, context
	Importance   int       `json:"importance"`      // 重要性级别（1-5）
	AccessCount  int       `json:"access_count" gorm:"default:0"` // 访问次数
	LastAccessed time.Time `json:"last_accessed"`   // 最后访问时间
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"` // 过期时间（nil 表示永不过期）
	Tags         JSONSlice `json:"tags" gorm:"type:json"` // 记忆标签
	
	// 向量嵌入字段（用于语义搜索）
	Embedding    string    `json:"embedding,omitempty" gorm:"type:text"` // 向量嵌入 JSON
	EmbeddingModel string `json:"embedding_model"`                      // 使用的嵌入模型
}

// 记忆类型常量
const (
	MemoryTypeFact       = "fact"       // 事实信息
	MemoryTypePreference = "preference" // 用户偏好
	MemoryTypeProcedure  = "procedure"  // 操作步骤
	MemoryTypeContext    = "context"    // 项目背景
)

// 记忆类别常量
const (
	MemoryCategoryPersonal  = "personal"
	MemoryCategoryProject   = "project"
	MemoryCategoryTechnical = "technical"
	MemoryCategoryHabit     = "habit"
)

// ShortTermMemory 短期记忆（对话摘要）
type ShortTermMemory struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	UserID    string    `json:"user_id"`
	Summary   string    `json:"summary" gorm:"type:text"` // 对话摘要
	MessageCount int  `json:"message_count"`               // 原始消息数
	DialogueID string    `json:"dialogue_id"`               // 关联对话
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

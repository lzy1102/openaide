package models

import (
	"time"
)

// Dialogue 对话模型
type Dialogue struct {
	ID                string    `json:"id" gorm:"primaryKey"`
	UserID            string    `json:"user_id"`
	Title             string    `json:"title"`
	Status            string    `json:"status" gorm:"default:active"`       // active, completed
	MessagesExtracted bool      `json:"messages_extracted" gorm:"default:false"` // 是否已提取记忆
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	Messages          []Message `json:"messages"`
}

// Message 消息模型
type Message struct {
	ID               string    `json:"id" gorm:"primaryKey"`
	DialogueID       string    `json:"dialogue_id"`
	Sender           string    `json:"sender"`
	Content          string    `json:"content"`
	ReasoningContent string    `json:"reasoning_content,omitempty"`
	ToolCallID       string    `json:"tool_call_id,omitempty"`
	ModelID          string    `json:"model_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

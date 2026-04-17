package models

import (
	"time"
)

// Dialogue 对话模型
type Dialogue struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`
}

// Message 消息模型
type Message struct {
	ID               string    `json:"id"`
	DialogueID       string    `json:"dialogue_id"`
	Sender           string    `json:"sender"` // user or assistant
	Content          string    `json:"content"`
	ReasoningContent string    `json:"reasoning_content,omitempty"` // AI 思考过程
	CreatedAt        time.Time `json:"created_at"`
}

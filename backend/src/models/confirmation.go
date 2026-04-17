package models

import "time"

// Confirmation 确认模型
type Confirmation struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id"`
	TaskType    string    `json:"task_type"` // skill, plugin, model, etc.
	Title       string    `json:"title"`
	Description string    `json:"description"`
	UserID      string    `json:"user_id"`
	Status      string    `json:"status"` // pending, confirmed, rejected
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

package models

import "time"

// CodeExecution 代码执行模型
type CodeExecution struct {
	ID            string    `json:"id" gorm:"primaryKey"`
	DialogueID    string    `json:"dialogue_id" gorm:"index"`
	MessageID     string    `json:"message_id" gorm:"index"`
	UserID        string    `json:"user_id" gorm:"index"`
	Language      string    `json:"language" gorm:"size:20"` // python, javascript, go, etc.
	Code          string    `json:"code" gorm:"type:text"`
	Parameters    JSONMap   `json:"parameters" gorm:"type:json"`
	Status        string    `json:"status" gorm:"size:20"` // pending, running, completed, failed, timeout
	Output        string    `json:"output" gorm:"type:text"`
	Error         string    `json:"error" gorm:"type:text"`
	ExecutionTime float64   `json:"execution_time"` // in seconds
	MemoryUsed    int64     `json:"memory_used"`    // in bytes
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

package models

import (
	"time"
)

// AutomationExecution 自动化执行模型
type AutomationExecution struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Type        string    `json:"type"` // scheduled, event-based, etc.
	Trigger     string    `json:"trigger"`
	Actions     string    `json:"actions" gorm:"type:text"` // JSON string of []AutomationAction
	Status      string    `json:"status"` // pending, running, completed, failed, paused
	Schedule    string    `json:"schedule,omitempty"` // cron expression
	LastRunAt   time.Time `json:"last_run_at,omitempty"`
	NextRunAt   time.Time `json:"next_run_at,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AutomationActionData 自动化动作数据（用于 JSON 序列化）
type AutomationActionData struct {
	ID          string                 `json:"id"`
	ExecutionID string                 `json:"execution_id"`
	Type        string                 `json:"type"` // skill, plugin, model, etc.
	TargetID    string                 `json:"target_id"`
	Parameters  map[string]interface{} `json:"parameters"`
	Status      string                 `json:"status"` // pending, running, completed, failed
	Result      interface{}            `json:"result,omitempty"`
	Error       string                 `json:"error,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

package models

import "time"

// Feedback 反馈模型
type Feedback struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id"`
	TaskType    string    `json:"task_type"` // skill, plugin, model, etc.
	UserID      string    `json:"user_id"`
	Rating      int       `json:"rating"` // 1-5
	Comment     string    `json:"comment"`
	CreatedAt   time.Time `json:"created_at"`
}

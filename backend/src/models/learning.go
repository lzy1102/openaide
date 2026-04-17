package models

import "time"

// LearningRecord 学习记录模型
type LearningRecord struct {
	ID         string    `json:"id" gorm:"primaryKey"`
	Type       string    `json:"type" gorm:"index"` // feedback_analysis, model_update, workflow_optimization, etc.
	TaskType   string    `json:"task_type" gorm:"index"`
	ModelID    string    `json:"model_id" gorm:"index"`
	Data       JSONMap   `json:"data" gorm:"type:json"`
	Confidence float64   `json:"confidence"`
	Status     string    `json:"status" gorm:"default:'pending'"` // pending, applied, rejected
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// UserPreference 用户偏好模型
type UserPreference struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	UserID    string    `json:"user_id" gorm:"index"`
	Key       string    `json:"key" gorm:"index"` // task_type.code.avg_rating, response_style, etc.
	Value     *JSONAny  `json:"value" gorm:"type:json"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PromptOptimization Prompt 优化记录
type PromptOptimization struct {
	ID          string     `json:"id" gorm:"primaryKey"`
	DialogueID  string     `json:"dialogue_id" gorm:"index"`
	TaskType    string     `json:"task_type" gorm:"index"`
	Original    string     `json:"original"`       // 原始 Prompt
	Suggestions string     `json:"suggestions"`    // 优化建议
	Status      string     `json:"status" gorm:"default:'pending'"` // pending, applied, rejected
	Confidence  float64    `json:"confidence"`     // 置信度 0-1
	AppliedAt   *time.Time `json:"applied_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// WorkflowOptimization 工作流优化记录
type WorkflowOptimization struct {
	ID          string         `json:"id" gorm:"primaryKey"`
	WorkflowID  string         `json:"workflow_id" gorm:"index"`
	Analysis    string         `json:"analysis"`    // 分析结果
	Suggestions string         `json:"suggestions"` // 优化建议
	Changes     []ChangeItem   `json:"changes" gorm:"type:json"`
	Status      string         `json:"status" gorm:"default:'pending'"` // pending, applied, rejected
	AppliedAt   *time.Time     `json:"applied_at"`
	CreatedAt   time.Time      `json:"created_at"`
}

// ChangeItem 变更项
type ChangeItem struct {
	StepID    string `json:"step_id"`
	Attribute string `json:"attribute"`
	OldValue  string `json:"old_value"`
	NewValue  string `json:"new_value"`
}

// InteractionRecord 交互记录
type InteractionRecord struct {
	ID           string    `json:"id" gorm:"primaryKey"`
	UserID       string    `json:"user_id" gorm:"index"`
	SessionID    string    `json:"session_id" gorm:"index"`
	Query        string    `json:"query"`
	Response     string    `json:"response"`
	TaskType     string    `json:"task_type" gorm:"index"`
	Model        string    `json:"model"`
	Duration     int64     `json:"duration"` // 毫秒
	TokensUsed   int       `json:"tokens_used"`
	Metadata     JSONMap   `json:"metadata" gorm:"type:json"`
	FeedbackID   string    `json:"feedback_id" gorm:"index"`
	CreatedAt    time.Time `json:"created_at"`
}

// LearningMetrics 学习指标
type LearningMetrics struct {
	ID                string    `json:"id" gorm:"primaryKey"`
	Period            string    `json:"period" gorm:"index"` // daily, weekly, monthly
	StartDate         time.Time `json:"start_date"`
	EndDate           time.Time `json:"end_date"`
	TotalInteractions int       `json:"total_interactions"`
	TotalFeedbacks    int       `json:"total_feedbacks"`
	AverageRating     float64   `json:"average_rating"`
	SuccessRate       float64   `json:"success_rate"`
	ImprovementRate   float64   `json:"improvement_rate"`
	LearningActions   int       `json:"learning_actions"`
	CreatedAt         time.Time `json:"created_at"`
}

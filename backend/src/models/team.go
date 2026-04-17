package models

import "time"

// Team 团队模型
type Team struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"index"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled" gorm:"default:true"`
	Config      JSONMap   `json:"config" gorm:"type:json"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TeamEvent 团队事件模型
type TeamEvent struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	TeamID    string    `json:"team_id" gorm:"index"`
	Type      string    `json:"type" gorm:"index"` // task_assigned, task_completed, task_failed, message_sent, member_added, member_removed
	Source    string    `json:"source"` // member ID or system
	Target    string    `json:"target,omitempty"` // target member ID
	Data      JSONMap   `json:"data" gorm:"type:json"`
	CreatedAt time.Time `json:"created_at"`
}

// TeamMessage 团队消息模型
type TeamMessage struct {
	ID        string     `json:"id" gorm:"primaryKey"`
	TeamID    string     `json:"team_id" gorm:"index"`
	From      string     `json:"from"` // member ID
	ToMember  string     `json:"to" gorm:"column:to_member;index"` // member ID or "*" for broadcast
	Type      string     `json:"type"` // message, request, response, notification, task_assigned, task_completed
	Content   JSONMap    `json:"content" gorm:"type:json"`
	Status    string     `json:"status" gorm:"index"` // pending, delivered, read
	CreatedAt time.Time  `json:"created_at"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
}

// ProgressReport 进度报告模型
type ProgressReport struct {
	ID          string            `json:"id" gorm:"primaryKey"`
	TeamID      string            `json:"team_id" gorm:"index"`
	GeneratedBy string            `json:"generated_by"`
	PeriodStart time.Time         `json:"period_start"`
	PeriodEnd   time.Time         `json:"period_end"`
	Summary     string            `json:"summary" gorm:"type:text"`
	Statistics  JSONMap           `json:"statistics" gorm:"type:json"`
	TaskStatus  TaskStatusSummary `json:"task_status" gorm:"type:json"`
	MemberStats MemberStatsSlice `json:"member_stats" gorm:"type:json"` // JSON serialized
	CreatedAt   time.Time         `json:"created_at"`
}

// TaskStatusSummary 任务状态摘要
type TaskStatusSummary struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Completed  int `json:"completed"`
	Failed     int `json:"failed"`
	Cancelled  int `json:"cancelled"`
}

// MemberStat 成员统计
type MemberStat struct {
	MemberID        string  `json:"member_id"`
	MemberName      string  `json:"member_name"`
	TasksAssigned   int     `json:"tasks_assigned"`
	TasksCompleted  int     `json:"tasks_completed"`
	TasksFailed     int     `json:"tasks_failed"`
	AverageProgress float64 `json:"average_progress"`
}

// RetryConfig 重试配置
type RetryConfig struct {
	MaxAttempts     int      `json:"max_attempts"`
	InitialDelay    int64    `json:"initial_delay"` // milliseconds
	MaxDelay        int64    `json:"max_delay"`
	BackoffFactor   float64  `json:"backoff_factor"`
	RetryableErrors JSONSlice `json:"retryable_errors"`
}

// TeamConfig 团队配置（用于保存/恢复团队）
type TeamConfig struct {
	Team        Team         `json:"team"`
	Members     []TeamMember `json:"members"`
	Tasks       []Task       `json:"tasks,omitempty"`
	RetryConfig RetryConfig  `json:"retry_config"`
	Version     string       `json:"version"`
	ExportedAt  time.Time    `json:"exported_at"`
}

// TeamActivity 团队活动记录
type TeamActivity struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	TeamID      string    `json:"team_id" gorm:"index"`
	MemberID    string    `json:"member_id,omitempty" gorm:"index"`
	Action      string    `json:"action"` // task_created, task_completed, member_joined, etc.
	Description string    `json:"description"`
	Metadata    JSONMap   `json:"metadata,omitempty" gorm:"type:json"`
	CreatedAt   time.Time `json:"created_at"`
}

// TeamMetrics 团队指标
type TeamMetrics struct {
	ID              string    `json:"id" gorm:"primaryKey"`
	TeamID          string    `json:"team_id" gorm:"index"`
	Period          string    `json:"period"` // daily, weekly, monthly
	PeriodStart     time.Time `json:"period_start"`
	PeriodEnd       time.Time `json:"period_end"`
	TotalTasks      int       `json:"total_tasks"`
	CompletedTasks  int       `json:"completed_tasks"`
	FailedTasks     int       `json:"failed_tasks"`
	AverageTaskTime float64   `json:"average_task_time"` // minutes
	MemberCount     int       `json:"member_count"`
	ActiveMembers   int       `json:"active_members"`
	CreatedAt       time.Time `json:"created_at"`
}

// TeamRole 团队角色定义
type TeamRole struct {
	ID           string     `json:"id" gorm:"primaryKey"`
	TeamID       string     `json:"team_id" gorm:"index"`
	Name         string     `json:"name" gorm:"not null"`
	Description  string     `json:"description"`
	Capabilities JSONSlice  `json:"capabilities" gorm:"type:json"`
	Permissions  JSONSlice  `json:"permissions" gorm:"type:json"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// WorkflowTask 工作流任务关联
type WorkflowTask struct {
	ID         string    `json:"id" gorm:"primaryKey"`
	WorkflowID string    `json:"workflow_id" gorm:"index"`
	TaskID     string    `json:"task_id" gorm:"index"`
	Order      int       `json:"order"`
	Required   bool      `json:"required" gorm:"default:true"`
	CreatedAt  time.Time `json:"created_at"`
}

// TaskDependencyGraph 任务依赖图
type TaskDependencyGraph struct {
	ID         string    `json:"id" gorm:"primaryKey"`
	TeamID     string    `json:"team_id" gorm:"index"`
	Nodes      string    `json:"nodes" gorm:"type:json"` // JSON string of []GraphNode
	Edges      string    `json:"edges" gorm:"type:json"` // JSON string of []GraphEdge
	Metadata   JSONMap   `json:"metadata,omitempty" gorm:"type:json"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// GraphNode 图节点
type GraphNode struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Type     string `json:"type"`
	Metadata string `json:"metadata,omitempty"` // JSON string
}

// GraphEdge 图边
type GraphEdge struct {
	From     string  `json:"from"`
	To       string  `json:"to"`
	Type     string  `json:"type"`
	Weight   float64 `json:"weight"`
	Metadata string  `json:"metadata,omitempty"` // JSON string
}

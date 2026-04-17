package models

import (
	"time"
)

// Workflow 工作流模型
type Workflow struct {
	ID          string        `json:"id" gorm:"primaryKey"`
	Name        string        `json:"name" gorm:"index"`
	Description string        `json:"description"`
	Steps       []WorkflowStep `json:"steps" gorm:"foreignKey:WorkflowID"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`

	// 新增字段
	Version     string   `json:"version" gorm:"default:'1.0.0'"`
	Category    string   `json:"category" gorm:"index"`
	Enabled     bool     `json:"enabled" gorm:"default:true"`
	TemplateID  string   `json:"template_id" gorm:"index"`
	Metadata    JSONMap  `json:"metadata" gorm:"type:json"`
}

// WorkflowStep 工作流步骤模型
type WorkflowStep struct {
	ID          string   `json:"id" gorm:"primaryKey"`
	WorkflowID  string   `json:"workflow_id" gorm:"index"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        string   `json:"type"` // llm, code, plugin, automation, condition, parallel, etc.
	Parameters  JSONMap  `json:"parameters" gorm:"type:json"`
	Order       int      `json:"order"`

	// 新增字段
	Timeout      *time.Duration `json:"timeout,omitempty"`
	RetryPolicy  JSONMap        `json:"retry_policy,omitempty" gorm:"type:json"`
	Condition    JSONMap        `json:"condition,omitempty" gorm:"type:json"`
	Parallel     bool           `json:"parallel"`
	DependsOn    JSONSlice      `json:"depends_on,omitempty" gorm:"type:json"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`

	// 回滚配置
	RollbackOnFailure bool   `json:"rollback_on_failure"`
	RollbackStep      string `json:"rollback_step,omitempty"`
}

// WorkflowInstance 工作流实例模型
type WorkflowInstance struct {
	ID          string          `json:"id" gorm:"primaryKey"`
	WorkflowID  string          `json:"workflow_id" gorm:"index"`
	Status      string          `json:"status" gorm:"index"` // pending, running, completed, failed, cancelled, paused
	Steps       []StepInstance  `json:"steps" gorm:"foreignKey:WorkflowInstanceID"`
	Input       JSONMap         `json:"input" gorm:"type:json"`
	Output      JSONMap         `json:"output" gorm:"type:json"`
	Error       string          `json:"error,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`

	// 新增字段
	ParentInstanceID string   `json:"parent_instance_id,omitempty" gorm:"index"`
	Variables        JSONMap  `json:"variables" gorm:"type:json"`
	CurrentStep      string   `json:"current_step,omitempty"`
	Progress         float64  `json:"progress" gorm:"default:0"`
}

// StepInstance 步骤实例模型
type StepInstance struct {
	ID                 string      `json:"id" gorm:"primaryKey"`
	WorkflowInstanceID string      `json:"workflow_instance_id" gorm:"index"`
	StepID             string      `json:"step_id" gorm:"index"`
	Name               string      `json:"name"`
	Status             string      `json:"status" gorm:"index"` // pending, running, completed, failed, skipped, retrying
	Input              JSONMap     `json:"input" gorm:"type:json"`
	Output             JSONMap     `json:"output" gorm:"type:json"`
	Error              string      `json:"error,omitempty"`
	StartedAt          *time.Time  `json:"started_at,omitempty"`
	CompletedAt        *time.Time  `json:"completed_at,omitempty"`

	// 新增字段
	AttemptCount int          `json:"attempt_count" gorm:"default:0"`
	Duration     int64        `json:"duration,omitempty"` // 执行时长（毫秒）
	RetriedAt    JSONTimeSlice `json:"retried_at,omitempty" gorm:"type:json"`
	Logs         []StepLog    `json:"logs,omitempty" gorm:"foreignKey:StepInstanceID"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

// StepLog 步骤日志
type StepLog struct {
	ID             string    `json:"id" gorm:"primaryKey"`
	StepInstanceID string    `json:"step_instance_id" gorm:"index"`
	Level          string    `json:"level"` // info, warning, error, debug
	Message        string    `json:"message"`
	Data           JSONMap   `json:"data,omitempty" gorm:"type:json"`
	CreatedAt      time.Time `json:"created_at"`
}

// WorkflowTemplate 工作流模板模型
type WorkflowTemplate struct {
	ID          string             `json:"id" gorm:"primaryKey"`
	Name        string             `json:"name" gorm:"index"`
	Description string             `json:"description"`
	Category    string             `json:"category" gorm:"index"`
	Version     string             `json:"version"`
	Definition  JSONMap            `json:"definition" gorm:"type:json"` // 工作流定义
	Variables   []TemplateVariable `json:"variables" gorm:"foreignKey:TemplateID"`
	Enabled     bool               `json:"enabled" gorm:"default:true"`
	Builtin     bool               `json:"builtin" gorm:"default:false"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

// TemplateVariable 模板变量模型
type TemplateVariable struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	TemplateID  string    `json:"template_id" gorm:"index"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Default     *JSONAny  `json:"default" gorm:"type:json"`
	Required    bool      `json:"required"`
	Description string    `json:"description"`
	Validation  string    `json:"validation,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// WorkflowExecution 工作流执行记录
type WorkflowExecution struct {
	ID          string      `json:"id" gorm:"primaryKey"`
	WorkflowID  string      `json:"workflow_id" gorm:"index"`
	InstanceID  string      `json:"instance_id" gorm:"index"`
	Status      string      `json:"status"`
	TriggerType string      `json:"trigger_type"` // manual, api, schedule, event
	TriggeredBy string      `json:"triggered_by"`
	Duration    int64       `json:"duration"` // 执行时长（毫秒）
	Input       JSONMap     `json:"input" gorm:"type:json"`
	Output      JSONMap     `json:"output" gorm:"type:json"`
	Error       string      `json:"error,omitempty"`
	StartedAt   time.Time   `json:"started_at"`
	CompletedAt *time.Time  `json:"completed_at,omitempty"`
}

// WorkflowSchedule 工作流调度配置
type WorkflowSchedule struct {
	ID          string      `json:"id" gorm:"primaryKey"`
	WorkflowID  string      `json:"workflow_id" gorm:"index"`
	Name        string      `json:"name"`
	CronExpr    string      `json:"cron_expr"` // cron 表达式
	Enabled     bool        `json:"enabled" gorm:"default:true"`
	Parameters  JSONMap     `json:"parameters" gorm:"type:json"`
	NextRunAt   *time.Time  `json:"next_run_at"`
	LastRunAt   *time.Time  `json:"last_run_at"`
	RunCount    int         `json:"run_count" gorm:"default:0"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

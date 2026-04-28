package models

import "time"

// Task 任务模型
type Task struct {
	ID          string       `json:"id" gorm:"primaryKey"`
	TeamID      string       `json:"team_id" gorm:"index"`
	Title       string       `json:"title" gorm:"not null"`
	Description string       `json:"description"`
	Type        string       `json:"type" gorm:"index"` // coding, research, plan, testing, review, etc.
	Priority    string       `json:"priority" gorm:"index"` // low, medium, high, urgent
	Status      string       `json:"status" gorm:"index"` // pending, assigned, in_progress, completed, failed, cancelled
	Complexity  int          `json:"complexity"` // 1-10
	Estimated   int          `json:"estimated"` // estimated minutes
	ParentTaskID string      `json:"parent_task_id,omitempty" gorm:"index"`
	AssignedTo  string       `json:"assigned_to,omitempty" gorm:"index"`
	CreatedBy   string       `json:"created_by"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	StartedAt   *time.Time   `json:"started_at,omitempty"`
	CompletedAt *time.Time   `json:"completed_at,omitempty"`
	Subtasks    []Subtask    `json:"subtasks,omitempty" gorm:"foreignKey:ParentTaskID"`
	Dependencies []TaskDependency `json:"dependencies,omitempty" gorm:"foreignKey:TaskID"`
	Tags        []string     `json:"tags,omitempty" gorm:"-"`
	Context     TaskContext  `json:"context,omitempty" gorm:"embedded;embeddedPrefix:context_"`
	RetryCount  int          `json:"retry_count" gorm:"default:0"`
	MaxRetries  int          `json:"max_retries" gorm:"default:3"`
	Result      *TaskResult  `json:"result,omitempty" gorm:"embedded;embeddedPrefix:result_"`
}

// Subtask 子任务模型
type Subtask struct {
	ID           string       `json:"id" gorm:"primaryKey"`
	ParentTaskID string       `json:"parent_task_id" gorm:"index"`
	Title        string       `json:"title" gorm:"not null"`
	Description  string       `json:"description"`
	Type         string       `json:"type"` // coding, research, plan, testing, review
	Priority     string       `json:"priority"` // low, medium, high, urgent
	Status       string       `json:"status"` // pending, assigned, in_progress, completed, failed
	AssignedTo   string       `json:"assigned_to,omitempty" gorm:"index"`
	Order        int          `json:"order"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	StartedAt    *time.Time   `json:"started_at,omitempty"`
	CompletedAt  *time.Time   `json:"completed_at,omitempty"`
	Dependencies []SubtaskDependency `json:"dependencies,omitempty" gorm:"foreignKey:SubtaskID"`
	Estimated    int          `json:"estimated"` // estimated minutes
	Actual       int          `json:"actual"`    // actual minutes taken
}

// TaskContext 任务上下文
type TaskContext struct {
	ProjectID      string                 `json:"project_id,omitempty"`
	Module         string                 `json:"module,omitempty"`
	RelatedFiles   JSONSlice              `json:"related_files,omitempty" gorm:"type:json"`
	RelatedIssues  JSONSlice              `json:"related_issues,omitempty" gorm:"type:json"`
	Metadata       JSONMap                `json:"metadata,omitempty" gorm:"type:json"`
	Requirements   JSONSlice              `json:"requirements,omitempty" gorm:"type:json"`
	Constraints    JSONSlice              `json:"constraints,omitempty" gorm:"type:json"`
}

// TaskDependency 任务依赖关系
type TaskDependency struct {
	ID         string    `json:"id" gorm:"primaryKey"`
	TaskID     string    `json:"task_id" gorm:"index"`
	DependsOn  string    `json:"depends_on" gorm:"index"` // task_id that this task depends on
	Type       string    `json:"type"` // after, before, concurrent
	CreatedAt  time.Time `json:"created_at"`
}

// SubtaskDependency 子任务依赖关系
type SubtaskDependency struct {
	ID         string    `json:"id" gorm:"primaryKey"`
	SubtaskID  string    `json:"subtask_id" gorm:"index"`
	DependsOn  string    `json:"depends_on" gorm:"index"`
	Type       string    `json:"type"`
	CreatedAt  time.Time `json:"created_at"`
}

// TaskResult 任务结果
type TaskResult struct {
	Success    bool                   `json:"success"`
	Output     string                 `json:"output,omitempty"`
	Error      string                 `json:"error,omitempty"`
	Artifacts  JSONSlice              `json:"artifacts,omitempty" gorm:"type:json"`
	Metrics    JSONMap                `json:"metrics,omitempty" gorm:"type:json"`
	Summary    string                 `json:"summary,omitempty"`
}

// TeamMember 团队成员
type TeamMember struct {
	ID          string                 `json:"id" gorm:"primaryKey"`
	TeamID      string                 `json:"team_id" gorm:"index"`
	Name        string                 `json:"name" gorm:"not null"`
	Role        string                 `json:"role" gorm:"index"` // lead, developer, reviewer, researcher, planner
	Capabilities []Capability          `json:"capabilities,omitempty" gorm:"serializer:json"`
	Availability string                `json:"availability"` // available, busy, offline
	CurrentLoad int                    `json:"current_load" gorm:"default:0"` // number of active tasks
	MaxLoad     int                    `json:"max_load" gorm:"default:3"`
	Specialization []string            `json:"specialization,omitempty" gorm:"serializer:json"`
	Experience  map[string]int         `json:"experience,omitempty" gorm:"serializer:json"` // skill -> years
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	LastActiveAt *time.Time            `json:"last_active_at,omitempty"`
}

// Capability 能力
type Capability struct {
	Name        string  `json:"name"`
	Level       float64 `json:"level"` // 0.0-1.0
	ConfirmedAt time.Time `json:"confirmed_at"`
}

// TaskAssignment 任务分配记录
type TaskAssignment struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	TaskID      string    `json:"task_id" gorm:"index"`
	SubtaskID   string    `json:"subtask_id,omitempty" gorm:"index"`
	AssignedTo  string    `json:"assigned_to" gorm:"index"`
	AssignedBy  string    `json:"assigned_by"`
	Reason      string    `json:"reason,omitempty"`
	Score       float64   `json:"score"` // assignment confidence score
	Status      string    `json:"status"` // pending, accepted, rejected, completed
	AssignedAt  time.Time `json:"assigned_at"`
	RespondedAt *time.Time `json:"responded_at,omitempty"`
}

// TaskDecomposition 任务分解模板
type TaskDecomposition struct {
	ID          string                   `json:"id" gorm:"primaryKey"`
	Name        string                   `json:"name" gorm:"not null"`
	Description string                   `json:"description"`
	TaskType    string                   `json:"task_type" gorm:"index"`
	Template    []SubtaskTemplate        `json:"template" gorm:"serializer:json"`
	Variables   []TaskTemplateVariable   `json:"variables" gorm:"serializer:json"`
	CreatedAt   time.Time                `json:"created_at"`
	UpdatedAt   time.Time                `json:"updated_at"`
}

// SubtaskTemplate 子任务模板
type SubtaskTemplate struct {
	ID          string                 `json:"id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Type        string                 `json:"type"`
	Order       int                    `json:"order"`
	Required    bool                   `json:"required"`
	Default     bool                   `json:"default"` // included by default
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// TaskTemplateVariable 任务模板变量
type TaskTemplateVariable struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Default     interface{} `json:"default,omitempty"`
	Required    bool        `json:"required"`
	Description string      `json:"description"`
}

// TaskProgress 任务进度
type TaskProgress struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	TaskID      string    `json:"task_id" gorm:"index"`
	SubtaskID   string    `json:"subtask_id,omitempty" gorm:"index"`
	Stage       string    `json:"stage"` // analyzing, planning, implementing, testing, reviewing
	Percent     int       `json:"percent"`
	Message     string    `json:"message,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty" gorm:"serializer:json"`
	CreatedAt   time.Time `json:"created_at"`
}

// TaskSummary 任务总结
type TaskSummary struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	TaskID      string    `json:"task_id" gorm:"index"`
	Summary     string    `json:"summary" gorm:"type:text"`
	Lessons     string    `json:"lessons,omitempty" gorm:"type:text"`
	Suggestions string    `json:"suggestions,omitempty" gorm:"type:text"`
	GeneratedBy string    `json:"generated_by"`
	CreatedAt   time.Time `json:"created_at"`
}

// TaskStatusUpdate 任务状态更新
type TaskStatusUpdate struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	TaskID      string    `json:"task_id" gorm:"index"`
	OldStatus   string    `json:"old_status"`
	NewStatus   string    `json:"new_status"`
	UpdatedBy   string    `json:"updated_by"`
	Reason      string    `json:"reason,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// OrchestrationRecord 编排执行记录（持久化任务分解执行结果）
type OrchestrationRecord struct {
	ID           string    `json:"id" gorm:"primaryKey"`
	SessionID    string    `json:"session_id" gorm:"index"`
	UserID       string    `json:"user_id" gorm:"index"`
	UserMessage  string    `json:"user_message" gorm:"type:text"`
	Status       string    `json:"status" gorm:"index"` // analyzing, planning, executing, completed, failed, cancelled
	TaskType     string    `json:"task_type,omitempty"`
	Complexity   string    `json:"complexity,omitempty"`
	PlanJSON     string    `json:"plan_json,omitempty" gorm:"type:text"`       // StructuredPlan JSON
	CheckpointsJSON string `json:"checkpoints_json,omitempty" gorm:"type:text"` // ExecutionCheckpoint array JSON
	ResultJSON   string    `json:"result_json,omitempty" gorm:"type:text"`     // 执行结果汇总
	Error        string    `json:"error,omitempty" gorm:"type:text"`
	StartedAt    time.Time `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	DurationMs   int64     `json:"duration_ms"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// SubtaskExecutionRecord 子任务执行记录
type SubtaskExecutionRecord struct {
	ID              string    `json:"id" gorm:"primaryKey"`
	OrchestrationID string    `json:"orchestration_id" gorm:"index"`
	SessionID       string    `json:"session_id" gorm:"index"`
	SubtaskID       string    `json:"subtask_id"`
	PhaseIndex      int       `json:"phase_index"`
	Title           string    `json:"title"`
	Description     string    `json:"description" gorm:"type:text"`
	Type            string    `json:"type"`
	Status          string    `json:"status" gorm:"index"` // in_progress, completed, failed, skipped
	Output          string    `json:"output,omitempty" gorm:"type:text"`
	Error           string    `json:"error,omitempty" gorm:"type:text"`
	StartedAt       time.Time `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	DurationMs      int64     `json:"duration_ms"`
	CreatedAt       time.Time `json:"created_at"`
}

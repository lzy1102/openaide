package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ScheduledTask 定时任务
type ScheduledTask struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"size:100;not null"`
	Description string    `json:"description" gorm:"size:500"`
	UserID      string    `json:"user_id" gorm:"index;not null"`

	// 调度配置
	ScheduleType string     `json:"schedule_type" gorm:"size:20;not null"` // cron, interval, once
	CronExpr     string     `json:"cron_expr" gorm:"size:100"`             // cron 表达式
	Interval     int64      `json:"interval"`                              // 间隔 (秒)
	ExecuteAt    *time.Time `json:"execute_at"`                            // 一次性任务执行时间

	// 任务类型和参数
	TaskType   string   `json:"task_type" gorm:"size:50;not null"` // workflow, reminder, webhook, script
	TaskConfig JSONMap  `json:"task_config" gorm:"type:json"`

	// 状态
	Status    string `json:"status" gorm:"size:20;default:active"` // active, paused, completed, failed
	IsEnabled bool   `json:"is_enabled" gorm:"default:true"`
	LastError string `json:"last_error" gorm:"type:text"`

	// 执行统计
	RunCount     int64      `json:"run_count" gorm:"default:0"`
	SuccessCount int64      `json:"success_count" gorm:"default:0"`
	FailCount    int64      `json:"fail_count" gorm:"default:0"`
	LastRunAt    *time.Time `json:"last_run_at"`
	NextRunAt    *time.Time `json:"next_run_at"`

	// 限制
	MaxRuns   int64      `json:"max_runs" gorm:"default:0"` // 最大运行次数 (0=无限)
	ExpiresAt *time.Time `json:"expires_at"`                // 过期时间

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TaskExecution 任务执行记录
type TaskExecution struct {
	ID       string `json:"id" gorm:"primaryKey"`
	TaskID   string `json:"task_id" gorm:"index;not null"`
	TaskName string `json:"task_name" gorm:"size:100"`

	// 执行信息
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	Duration    int64      `json:"duration"` // 毫秒

	// 执行结果
	Status string   `json:"status" gorm:"size:20"` // running, success, failed, timeout
	Result JSONMap  `json:"result" gorm:"type:json"`
	Error  string   `json:"error" gorm:"type:text"`

	// 触发信息
	TriggerType string `json:"trigger_type" gorm:"size:20"` // schedule, manual
	TriggeredBy string `json:"triggered_by" gorm:"size:36"` // user_id or system

	CreatedAt time.Time `json:"created_at"`
}

// TaskReminder 任务提醒
type TaskReminder struct {
	ID         string `json:"id" gorm:"primaryKey"`
	UserID     string `json:"user_id" gorm:"index;not null"`
	DialogueID string `json:"dialogue_id" gorm:"index"`

	// 提醒内容
	Title   string `json:"title" gorm:"size:200;not null"`
	Content string `json:"content" gorm:"type:text"`

	// 提醒时间
	RemindAt     time.Time `json:"remind_at" gorm:"index"`
	RepeatType   string    `json:"repeat_type" gorm:"size:20;default:none"` // none, daily, weekly, monthly, yearly
	RepeatConfig JSONMap   `json:"repeat_config" gorm:"type:json"`

	// 状态
	Status      string     `json:"status" gorm:"size:20;default:pending"` // pending, sent, snoozed, cancelled
	SnoozedAt   *time.Time `json:"snoozed_at"`
	SnoozeCount int        `json:"snooze_count" gorm:"default:0"`

	// 通知方式
	NotifyType   string  `json:"notify_type" gorm:"size:20;default:websocket"` // websocket, email, webhook
	NotifyConfig JSONMap `json:"notify_config" gorm:"type:json"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BeforeCreate 创建前钩子
func (t *ScheduledTask) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	return nil
}

func (e *TaskExecution) BeforeCreate(tx *gorm.DB) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	return nil
}

func (r *TaskReminder) BeforeCreate(tx *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	return nil
}

// TableName 指定表名
func (ScheduledTask) TableName() string {
	return "scheduled_tasks"
}

func (TaskExecution) TableName() string {
	return "task_executions"
}

func (TaskReminder) TableName() string {
	return "task_reminders"
}

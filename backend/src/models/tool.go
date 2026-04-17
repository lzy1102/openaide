package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Tool 工具定义
type Tool struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"uniqueIndex;size:100;not null"`
	Description string    `json:"description" gorm:"size:500"`
	Type        string    `json:"type" gorm:"size:50;not null"` // function, api, script, builtin
	Category    string    `json:"category" gorm:"size:50"`      // search, file, web, database, etc.
	Enabled     bool      `json:"enabled" gorm:"default:true"`

	// 工具参数定义 (JSON Schema)
	ParametersSchema JSONMap `json:"parameters_schema" gorm:"type:json"`

	// 执行配置
	ExecutorType   string  `json:"executor_type" gorm:"size:50"` // http, script, builtin
	ExecutorConfig JSONMap `json:"executor_config" gorm:"type:json"`

	// 认证配置
	AuthType   string  `json:"auth_type" gorm:"size:50"` // none, api_key, oauth, basic
	AuthConfig JSONMap `json:"auth_config" gorm:"type:json"`

	// 元数据
	Tags       JSONSlice `json:"tags" gorm:"type:json"`
	Examples   string   `json:"examples" gorm:"type:text"` // JSON string
	Timeout    int      `json:"timeout" gorm:"default:30"`    // 超时时间(秒)
	RateLimit  int      `json:"rate_limit" gorm:"default:60"` // 每分钟调用限制

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ToolExample 工具使用示例
type ToolExample struct {
	Description string `json:"description"`
	Parameters  string `json:"parameters"` // JSON string
	Result      string `json:"result"`
}

// ToolExecution 工具执行记录
type ToolExecution struct {
	ID         string    `json:"id" gorm:"primaryKey"`
	ToolID     string    `json:"tool_id" gorm:"index;not null"`
	ToolName   string    `json:"tool_name" gorm:"size:100"`
	DialogueID string    `json:"dialogue_id" gorm:"index"`
	MessageID  string    `json:"message_id" gorm:"index"`
	UserID     string    `json:"user_id" gorm:"index"`

	// 执行参数和结果
	Parameters JSONMap   `json:"parameters" gorm:"type:json"`
	Result     JSONMap   `json:"result" gorm:"type:json"`
	Error      string    `json:"error" gorm:"type:text"`

	// 执行状态
	Status     string     `json:"status" gorm:"size:20"` // pending, running, success, failed
	Duration   int        `json:"duration"`              // 执行时间(毫秒)
	TokenUsage int        `json:"token_usage"`           // 消耗的 Token 数

	// 时间戳
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ToolCall LLM 发起的工具调用请求
type ToolCall struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ToolResult 工具执行结果
type ToolResult struct {
	ToolCallID string      `json:"tool_call_id"`
	Content    interface{} `json:"content"`
	IsError    bool        `json:"is_error,omitempty"`
}

// BeforeCreate 创建前钩子
func (t *Tool) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	return nil
}

// BeforeCreate 创建前钩子
func (e *ToolExecution) BeforeCreate(tx *gorm.DB) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	return nil
}

// TableName 指定表名
func (Tool) TableName() string {
	return "tools"
}

// TableName 指定表名
func (ToolExecution) TableName() string {
	return "tool_executions"
}

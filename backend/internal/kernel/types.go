package kernel

import (
	"context"
	"time"
)

// Message 内核消息（与 LLM 层解耦的通用格式）
type Message struct {
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
	Name             string `json:"name,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string `json:"tool_call_id,omitempty"`
}

// ToolCall 工具调用
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall 函数调用
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolResult 工具执行结果
type ToolResult struct {
	Content interface{} `json:"content"`
	Error   string      `json:"error,omitempty"`
}

// ToolDefinition 工具定义
type ToolDefinition struct {
	Type     string     `json:"type"`
	Function FunctionDef `json:"function"`
}

// FunctionDef 函数定义
type FunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// LLMResponse LLM 响应（内核通用格式）
type LLMResponse struct {
	ID       string   `json:"id"`
	Content  string   `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Usage    *TokenUsage `json:"usage,omitempty"`
	Model    string   `json:"model"`
}

// TokenUsage Token 使用统计
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// StreamChunk 流式响应块
type StreamChunk struct {
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	Done             bool   `json:"done"`
	Usage            *TokenUsage `json:"usage,omitempty"`
	Error            error  `json:"-"`
}

// Session 对话会话
type Session struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	UserID    string    `json:"user_id"`
	Messages  []Message `json:"messages"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Query 用户查询
type Query struct {
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
	UserID    string `json:"user_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Options   QueryOptions `json:"options,omitempty"`
}

// QueryOptions 查询选项
type QueryOptions struct {
	ModelID      string   `json:"model_id,omitempty"`
	Temperature  float64  `json:"temperature,omitempty"`
	MaxTokens    int      `json:"max_tokens,omitempty"`
	ToolFilter   []string `json:"tool_filter,omitempty"`
	EnableStream bool     `json:"enable_stream,omitempty"`
	SkillID      string   `json:"skill_id,omitempty"`
}

// Response 内核响应
type Response struct {
	Content    string        `json:"content"`
	ToolCalls  int           `json:"tool_calls"`
	TokensUsed int           `json:"tokens_used"`
	Duration   time.Duration `json:"duration"`
	Model      string        `json:"model"`
	Error      string        `json:"error,omitempty"`
}

// Event 内核事件
type Event struct {
	Type      string                 `json:"type"`
	Source    string                 `json:"source"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
}

// EventType 事件类型常量
const (
	EventQueryReceived    = "query.received"
	EventThinkingStarted  = "thinking.started"
	EventThinkingEnded    = "thinking.ended"
	EventToolCallStarted  = "toolcall.started"
	EventToolCallEnded    = "toolcall.ended"
	EventResponseStarted  = "response.started"
	EventResponseChunk    = "response.chunk"
	EventResponseEnded    = "response.ended"
	EventError            = "error"
	EventSessionCreated   = "session.created"
	EventSessionUpdated   = "session.updated"
)

// KernelState 内核状态
type KernelState int

const (
	StateIdle KernelState = iota
	StateThinking
	StateToolCalling
	StateResponding
	StateError
)

func (s KernelState) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateThinking:
		return "thinking"
	case StateToolCalling:
		return "tool_calling"
	case StateResponding:
		return "responding"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

package kernel

import (
	"time"
)

// Message 内核消息（与 LLM 层解耦的通用格式）
type Message struct {
	Role             string     `json:"role"`
	Content          string     `json:"content"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	Name             string     `json:"name,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
}

// ToolCall 工具调用
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall 函数调用
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolResult 工具执行结果
type ToolResult struct {
	Content     interface{} `json:"content"`
	Error       string      `json:"error,omitempty"`
	ErrorCode   string      `json:"error_code,omitempty"`   // NOT_FOUND, PERMISSION_DENIED, TIMEOUT, INVALID_ARGS, EXEC_FAILED
	IsRetryable bool        `json:"is_retryable,omitempty"` // 可以换参数重试
}

// ToolDefinition 工具定义
type ToolDefinition struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

// FunctionDef 函数定义
type FunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
	Strict      bool                   `json:"strict,omitempty"`
}

// LLMResponse LLM 响应（内核通用格式）
type LLMResponse struct {
	ID               string      `json:"id"`
	Content          string      `json:"content"`
	ReasoningContent string      `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall  `json:"tool_calls,omitempty"`
	Usage            *TokenUsage `json:"usage,omitempty"`
	Model            string      `json:"model"`
}

// TokenUsage Token 使用统计
type TokenUsage struct {
	PromptTokens          int `json:"prompt_tokens"`
	CompletionTokens      int `json:"completion_tokens"`
	TotalTokens           int `json:"total_tokens"`
	PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens,omitempty"`
	PromptCacheMissTokens int `json:"prompt_cache_miss_tokens,omitempty"`
}

// StreamChunkType 流式块类型
type StreamChunkType string

const (
	ChunkTypeContent  StreamChunkType = "content"   // 文本内容块
	ChunkTypeThinking StreamChunkType = "thinking"  // 推理内容块
	ChunkTypeToolCall StreamChunkType = "tool_call" // 工具调用
	ChunkTypeToolDone StreamChunkType = "tool_done" // 工具执行完成
	ChunkTypeProgress StreamChunkType = "progress"  // 多轮进度
	ChunkTypeDone     StreamChunkType = "done"      // 流结束
	ChunkTypeError    StreamChunkType = "error"     // 错误
)

// StreamChunk 流式响应块
type StreamChunk struct {
	Type             StreamChunkType `json:"type"`
	Content          string          `json:"content,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID       string          `json:"tool_call_id,omitempty"`
	ToolName         string          `json:"tool_name,omitempty"`
	ToolArgs         string          `json:"tool_args,omitempty"`
	ToolResult       *ToolResult     `json:"tool_result,omitempty"`
	Round            int             `json:"round,omitempty"`
	TotalRounds      int             `json:"total_rounds,omitempty"`
	Done             bool            `json:"done"`
	Usage            *TokenUsage     `json:"usage,omitempty"`
	Error            error           `json:"-"`
}

// Session 对话会话
type Session struct {
	ID        string                 `json:"id"`
	ProjectID string                 `json:"project_id"`
	UserID    string                 `json:"user_id"`
	Messages  []Message              `json:"messages"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// SafeCopy returns a deep copy of the session for concurrent-safe use
func (s *Session) SafeCopy() *Session {
	c := *s
	c.Messages = make([]Message, len(s.Messages))
	copy(c.Messages, s.Messages)
	if s.Metadata != nil {
		c.Metadata = make(map[string]interface{}, len(s.Metadata))
		for k, v := range s.Metadata {
			c.Metadata[k] = v
		}
	}
	return &c
}

// Query 用户查询
type Query struct {
	SessionID string       `json:"session_id"`
	Content   string       `json:"content"`
	UserID    string       `json:"user_id,omitempty"`
	ProjectID string       `json:"project_id,omitempty"`
	Options   QueryOptions `json:"options,omitempty"`
}

// ResponseFormat 结构化输出格式
type ResponseFormat struct {
	Type       string      `json:"type"`                  // "json_object" | "json_schema" | "text"
	JSONSchema *JSONSchema `json:"json_schema,omitempty"` // json_schema 模式的 Schema 定义
}

// JSONSchema JSON Schema 定义
type JSONSchema struct {
	Name   string                 `json:"name"`
	Schema map[string]interface{} `json:"schema"`
	Strict bool                   `json:"strict,omitempty"`
}

// QueryOptions 查询选项
type QueryOptions struct {
	ModelID        string          `json:"model_id,omitempty"`
	Temperature    float64         `json:"temperature,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	ToolFilter     []string        `json:"tool_filter,omitempty"`
	EnableStream   bool            `json:"enable_stream,omitempty"`
	ForcePlan      bool            `json:"force_plan,omitempty"` // 强制规划模式
	SkillID        string          `json:"skill_id,omitempty"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"` // 结构化输出格式

	// 交互回调（REPL 用 pterm 实现，内核 goroutine 中同步调用）
	OnBudgetExhausted func(round, maxRounds int) bool        // 预算用尽，返回 true 继续，false 合成
	OnSkillDistilled  func(skillName, skillDesc string) bool // 技能蒸馏通知，返回 true 创建
	WorkingDir        string                                 // 项目工作目录（Server 模式用，覆盖 CWD）
	LastReflection    *ReflectionResult                      // L5: 上次反思结果，注入提示词
	ProjectContext    string                                 // 项目知识（来自 ProjectMind，由编排器注入）
}

// Response 内核响应
type Response struct {
	Content    string        `json:"content"`
	ToolCalls  int           `json:"tool_calls"`
	TokensUsed int           `json:"tokens_used"`
	CacheHit   int           `json:"-"` // prompt 缓存命中
	CacheMiss  int           `json:"-"` // prompt 缓存未命中
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
	EventQueryReceived   = "query.received"
	EventThinkingStarted = "thinking.started"
	EventThinkingEnded   = "thinking.ended"
	EventToolCallStarted = "toolcall.started"
	EventToolCallEnded   = "toolcall.ended"
	EventResponseStarted = "response.started"
	EventResponseChunk   = "response.chunk"
	EventResponseEnded   = "response.ended"
	EventError           = "error"
	EventSessionCreated  = "session.created"
	EventSessionUpdated  = "session.updated"
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

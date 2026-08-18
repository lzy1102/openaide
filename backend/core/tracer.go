package kernel

import (
	"context"
	"time"
)

// TraceEventType 跟踪事件类型
type TraceEventType string

const (
	TraceLLM        TraceEventType = "llm_call"
	TraceTool       TraceEventType = "tool_call"
	TraceThink      TraceEventType = "thinking"
	TraceState      TraceEventType = "state_transition"
	TraceCheckpoint TraceEventType = "checkpoint"
	TraceError      TraceEventType = "error"
	TraceReflection TraceEventType = "reflection"
	TraceMemory     TraceEventType = "memory"
	TraceSession    TraceEventType = "session"
)

// TraceStatus 跟踪状态
type TraceStatus string

const (
	TraceStatusOK    TraceStatus = "ok"
	TraceStatusError TraceStatus = "error"
)

// TraceEvent 单次跟踪事件记录
type TraceEvent struct {
	ID        string            `json:"id"`
	ParentID  string            `json:"parent_id,omitempty"`
	SessionID string            `json:"session_id"`
	Type      TraceEventType    `json:"type"`
	Name      string            `json:"name"`
	StartTime time.Time         `json:"start_time"`
	EndTime   time.Time         `json:"end_time,omitempty"`
	Duration  time.Duration     `json:"duration_ms,omitempty"`
	Input     interface{}       `json:"input,omitempty"`
	Output    interface{}       `json:"output,omitempty"`
	Error     string            `json:"error,omitempty"`
	Tags      map[string]string `json:"tags,omitempty"`
	Status    TraceStatus       `json:"status"`
}

// Tracer 跟踪接口 - 可插拔，支持文件、网络等实现
type Tracer interface {
	// Record 记录一个已完结的事件
	Record(ctx context.Context, event *TraceEvent) error

	// StartSpan 开启一个跟踪跨度，返回带 span ID 的 context
	StartSpan(ctx context.Context, sessionID string, eventType TraceEventType, name string) context.Context

	// EndSpan 结束当前跨度
	EndSpan(ctx context.Context, output interface{}, err error)

	// Flush 刷出缓冲区
	Flush(ctx context.Context) error

	// Close 关闭跟踪器
	Close() error
}

type SpanKey struct{}

type SpanInfo struct {
	ID        string
	ParentID  string
	SessionID string
	StartTime time.Time
	EventType TraceEventType
	Name      string
}

// NoopTracer 空操作跟踪器 - 不记录任何事件
type NoopTracer struct{}

func (n *NoopTracer) Record(_ context.Context, _ *TraceEvent) error { return nil }
func (n *NoopTracer) StartSpan(ctx context.Context, _ string, _ TraceEventType, _ string) context.Context {
	return ctx
}
func (n *NoopTracer) EndSpan(_ context.Context, _ interface{}, _ error) {}
func (n *NoopTracer) Flush(_ context.Context) error                     { return nil }
func (n *NoopTracer) Close() error                                      { return nil }

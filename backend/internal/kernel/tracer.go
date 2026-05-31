package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// TraceEventType 跟踪事件类型
type TraceEventType string

const (
	TraceLLM         TraceEventType = "llm_call"
	TraceTool        TraceEventType = "tool_call"
	TraceThink       TraceEventType = "thinking"
	TraceState       TraceEventType = "state_transition"
	TraceCheckpoint  TraceEventType = "checkpoint"
	TraceError       TraceEventType = "error"
	TraceReflection  TraceEventType = "reflection"
	TraceMemory      TraceEventType = "memory"
	TraceSession     TraceEventType = "session"
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

type spanKey struct{}

type spanInfo struct {
	id        string
	parentID  string
	sessionID string
	startTime time.Time
	eventType TraceEventType
	name      string
}

// FileTracer 基于 JSONL 文件的跟踪器 — CSP actor
type FileTracer struct {
	actor   *Actor
	file    *os.File
	encoder *json.Encoder
	buffer  []*TraceEvent
	bufSize int
	nextID  int64
}

// FileTracerConfig 文件跟踪器配置
type FileTracerConfig struct {
	FilePath string // 输出路径，默认 ./data/traces.jsonl
	BufSize  int    // 缓冲区大小，达到后自动刷新，默认 10
}

// NewFileTracer 创建文件跟踪器
func NewFileTracer(cfg FileTracerConfig) (*FileTracer, error) {
	path := cfg.FilePath
	if path == "" {
		path = filepath.Join("data", "traces.jsonl")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create trace dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open trace file: %w", err)
	}
	bufSize := cfg.BufSize
	if bufSize <= 0 {
		bufSize = 10
	}
	return &FileTracer{
		actor:   NewActor(8),
		file:    f,
		encoder: json.NewEncoder(f),
		buffer:  make([]*TraceEvent, 0, bufSize),
		bufSize: bufSize,
	}, nil
}

func (t *FileTracer) genID() string {
	ch := make(chan string, 1)
	t.actor.Send(func() {
		id := t.nextID
		t.nextID++
		ch <- fmt.Sprintf("trace_%d", id)
	})
	return <-ch
}

func (t *FileTracer) Record(ctx context.Context, event *TraceEvent) error {
	ch := make(chan error, 1)
	t.actor.Send(func() {
		t.buffer = append(t.buffer, event)
		if len(t.buffer) >= t.bufSize {
			ch <- t.flushLocked()
		} else {
			ch <- nil
		}
	})
	return <-ch
}

func (t *FileTracer) StartSpan(ctx context.Context, sessionID string, eventType TraceEventType, name string) context.Context {
	spanID := t.genID()
	var parentID string
	if parent, ok := ctx.Value(spanKey{}).(*spanInfo); ok {
		parentID = parent.id
	}
	info := &spanInfo{
		id:        spanID,
		parentID:  parentID,
		sessionID: sessionID,
		startTime: time.Now(),
		eventType: eventType,
		name:      name,
	}
	return context.WithValue(ctx, spanKey{}, info)
}

func (t *FileTracer) EndSpan(ctx context.Context, output interface{}, err error) {
	info, ok := ctx.Value(spanKey{}).(*spanInfo)
	if !ok || info == nil {
		return
	}
	now := time.Now()
	status := TraceStatusOK
	var errStr string
	if err != nil {
		status = TraceStatusError
		errStr = err.Error()
	}
	event := &TraceEvent{
		ID:        info.id,
		ParentID:  info.parentID,
		SessionID: info.sessionID,
		Type:      info.eventType,
		Name:      info.name,
		StartTime: info.startTime,
		EndTime:   now,
		Duration:  now.Sub(info.startTime),
		Output:    output,
		Error:     errStr,
		Status:    status,
	}
	if err := t.Record(ctx, event); err != nil {
		slog.Debug("Tracer.Record failed", "event", info.name, "error", err)
	}
}

func (t *FileTracer) Flush(ctx context.Context) error {
	ch := make(chan error, 1)
	t.actor.Send(func() { ch <- t.flushLocked() })
	return <-ch
}

func (t *FileTracer) flushLocked() error {
	if len(t.buffer) == 0 {
		return nil
	}
	for _, event := range t.buffer {
		if err := t.encoder.Encode(event); err != nil {
			return fmt.Errorf("encode trace: %w", err)
		}
	}
	t.buffer = t.buffer[:0]
	return nil
}

func (t *FileTracer) Close() error {
	ch := make(chan error, 1)
	t.actor.Send(func() {
		if err := t.flushLocked(); err != nil {
			ch <- err
			return
		}
		ch <- t.file.Close()
	})
	err := <-ch
	t.actor.Stop()
	return err
}

// NoopTracer 空操作跟踪器 - 不记录任何事件
type NoopTracer struct{}

func (n *NoopTracer) Record(_ context.Context, _ *TraceEvent) error { return nil }
func (n *NoopTracer) StartSpan(ctx context.Context, _ string, _ TraceEventType, _ string) context.Context {
	return ctx
}
func (n *NoopTracer) EndSpan(_ context.Context, _ interface{}, _ error)          {}
func (n *NoopTracer) Flush(_ context.Context) error                             { return nil }
func (n *NoopTracer) Close() error                                              { return nil }

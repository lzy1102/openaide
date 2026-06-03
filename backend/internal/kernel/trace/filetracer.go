package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"openaide/backend/internal/kernel"
	"openaide/backend/internal/kernel/actor"
)

// FileTracer 基于 JSONL 文件的跟踪器 — CSP actor
type FileTracer struct {
	actor   *actor.Actor
	file    *os.File
	encoder *json.Encoder
	buffer  []*kernel.TraceEvent
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
		actor:   actor.NewActor(8),
		file:    f,
		encoder: json.NewEncoder(f),
		buffer:  make([]*kernel.TraceEvent, 0, bufSize),
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

func (t *FileTracer) Record(ctx context.Context, event *kernel.TraceEvent) error {
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

func (t *FileTracer) StartSpan(ctx context.Context, sessionID string, eventType kernel.TraceEventType, name string) context.Context {
	spanID := t.genID()
	var parentID string
	if parent, ok := ctx.Value(kernel.SpanKey{}).(*kernel.SpanInfo); ok {
		parentID = parent.ID
	}
	info := &kernel.SpanInfo{
		ID:        spanID,
		ParentID:  parentID,
		SessionID: sessionID,
		StartTime: time.Now(),
		EventType: eventType,
		Name:      name,
	}
	return context.WithValue(ctx, kernel.SpanKey{}, info)
}

func (t *FileTracer) EndSpan(ctx context.Context, output interface{}, err error) {
	info, ok := ctx.Value(kernel.SpanKey{}).(*kernel.SpanInfo)
	if !ok || info == nil {
		return
	}
	now := time.Now()
	status := kernel.TraceStatusOK
	var errStr string
	if err != nil {
		status = kernel.TraceStatusError
		errStr = err.Error()
	}
	event := &kernel.TraceEvent{
		ID:        info.ID,
		ParentID:  info.ParentID,
		SessionID: info.SessionID,
		Type:      info.EventType,
		Name:      info.Name,
		StartTime: info.StartTime,
		EndTime:   now,
		Duration:  now.Sub(info.StartTime),
		Output:    output,
		Error:     errStr,
		Status:    status,
	}
	if err := t.Record(ctx, event); err != nil {
		slog.Debug("Tracer.Record failed", "event", info.Name, "error", err)
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

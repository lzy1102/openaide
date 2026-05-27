package infra

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"
)

type contextKey string

const traceIDKey contextKey = "trace_id"

// NewTraceID generates a short unique trace identifier
func NewTraceID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// WithTraceID injects a trace ID into the context
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// TraceID extracts the trace ID from context
func TraceID(ctx context.Context) string {
	if id, ok := ctx.Value(traceIDKey).(string); ok {
		return id
	}
	return "-"
}

// TraceSpan logs the duration of a named operation
func TraceSpan(ctx context.Context, name string) func() {
	tid := TraceID(ctx)
	start := time.Now()
	return func() {
		slog.Debug("trace", "id", tid, "span", name, "duration_ms", time.Since(start).Milliseconds())
	}
}

// TraceLog logs a key-value event within the current trace
func TraceLog(ctx context.Context, msg string, attrs ...any) {
	tid := TraceID(ctx)
	args := []any{"id", tid}
	args = append(args, attrs...)
	slog.Debug("trace: "+msg, args...)
}

package kernel

import (
	"context"
	"testing"
)

func TestFileTracer_RecordAndFlush(t *testing.T) {
	ft, err := NewFileTracer(FileTracerConfig{FilePath: t.TempDir() + "/trace.jsonl"})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	event := &TraceEvent{
		ID:   "e1",
		Type: TraceLLM,
		Name: "test-llm-call",
	}
	if err := ft.Record(ctx, event); err != nil {
		t.Error("Record:", err)
	}
	if err := ft.Flush(ctx); err != nil {
		t.Error("Flush:", err)
	}
	ft.Close()
}

func TestFileTracer_StartEndSpan(t *testing.T) {
	ft, _ := NewFileTracer(FileTracerConfig{FilePath: t.TempDir() + "/span.jsonl"})
	defer ft.Close()

	ctx := context.Background()
	spanCtx := ft.StartSpan(ctx, "s1", TraceTool, "test-tool")
	ft.EndSpan(spanCtx, map[string]string{"result": "ok"}, nil)

	if err := ft.Flush(ctx); err != nil {
		t.Error("Flush after span:", err)
	}
}

func TestNoopTracer(t *testing.T) {
	n := &NoopTracer{}
	ctx := context.Background()
	if err := n.Record(ctx, &TraceEvent{}); err != nil {
		t.Error("noop Record should not error")
	}
	spanCtx := n.StartSpan(ctx, "s", TraceLLM, "test")
	n.EndSpan(spanCtx, nil, nil)
	if err := n.Flush(ctx); err != nil {
		t.Error("noop Flush should not error")
	}
	if err := n.Close(); err != nil {
		t.Error("noop Close should not error")
	}
}

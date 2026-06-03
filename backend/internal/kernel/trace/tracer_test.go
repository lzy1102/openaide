package trace

import (
	"context"
	"testing"

	"openaide/backend/internal/kernel"
)

func TestFileTracer_RecordAndFlush(t *testing.T) {
	ft, err := NewFileTracer(FileTracerConfig{FilePath: t.TempDir() + "/trace.jsonl"})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	event := &kernel.TraceEvent{ID: "e1", Type: kernel.TraceLLM, Name: "test-llm-call"}
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
	spanCtx := ft.StartSpan(ctx, "s1", kernel.TraceTool, "test-tool")
	ft.EndSpan(spanCtx, map[string]string{"result": "ok"}, nil)

	if err := ft.Flush(ctx); err != nil {
		t.Error("Flush after span:", err)
	}
}

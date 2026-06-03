package kernel

import (
	"context"
	"testing"
)

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

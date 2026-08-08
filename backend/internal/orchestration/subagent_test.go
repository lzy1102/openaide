package orchestration

import (
	"context"
	"testing"
	"time"

	"openaide/backend/internal/kernel"
)

// blockingLLM 挂起的 LLM:ChatStream 阻塞直到 ctx 取消。
type blockingLLM struct{ mockLLMProvider }

func (b *blockingLLM) ChatStream(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (<-chan kernel.StreamChunk, error) {
	<-ctx.Done() // 模拟 LLM 挂起,直到超时取消
	ch := make(chan kernel.StreamChunk, 1)
	ch <- kernel.StreamChunk{Type: kernel.ChunkTypeError, Error: ctx.Err(), Done: true}
	close(ch)
	return ch, nil
}

func TestRunSubAgent_TimeoutVisible(t *testing.T) {
	store := kernel.NewSessionStoreAdapter()
	o := NewOrchestrator(&mockKernel{}, &blockingLLM{}, &mockToolExecutor{}, &mockMemory{}, store)
	o.SetSubAgentTimeout(100 * time.Millisecond) // 短超时加速测试
	team := NewTeam(o)
	team.AddRole("analyst", "Analyst", "Analysis role", "You analyze", []string{"read_file"})
	o.SetTeam(team)

	var statuses []string
	start := time.Now()
	_, err := o.RunSubAgent(context.Background(), "u", "p", "analyst", "do work", nil, func(role string, round int, status string) {
		statuses = append(statuses, status)
	})

	if err == nil {
		t.Fatal("expected timeout error from blocking LLM")
	}
	hasTimeout := false
	for _, s := range statuses {
		if s == "timeout" {
			hasTimeout = true
		}
	}
	if !hasTimeout {
		t.Errorf("expected timeout status in progress callbacks, got %v", statuses)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("RunSubAgent took too long: %v", elapsed)
	}
}

func TestRunSubAgent_DoneVisible(t *testing.T) {
	store := kernel.NewSessionStoreAdapter()
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, store)
	team := NewTeam(o)
	team.AddRole("analyst", "Analyst", "Analysis role", "You analyze", []string{"read_file"})
	o.SetTeam(team)

	var lastStatus string
	result, err := o.RunSubAgent(context.Background(), "u", "p", "analyst", "analyze", nil, func(role string, round int, status string) {
		lastStatus = status
	})
	if err != nil {
		t.Fatalf("RunSubAgent: %v", err)
	}
	if lastStatus != "done" {
		t.Errorf("expected done status, got %q", lastStatus)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}

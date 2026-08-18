package tools

import (
	"context"
	"strings"
	"testing"

	"openaide/backend/core"
)

type mockMemoryManager struct {
	archiveContent string
	facts          []string
}

func (m *mockMemoryManager) ArchiveConversation(ctx context.Context, sessionID, summary string, messages []kernel.Message, importance float64) error {
	m.archiveContent = summary
	return nil
}

func (m *mockMemoryManager) RetrieveArchive(ctx context.Context, query string, limit int) ([]kernel.Message, float64, error) {
	if strings.Contains(query, "none") {
		return nil, 0, nil
	}
	return []kernel.Message{
		{Role: "user", Content: "fix login bug"},
		{Role: "assistant", Content: "fixed auth/service.go"},
	}, 0.85, nil
}

func (m *mockMemoryManager) StoreCoreFact(ctx context.Context, content string, importance float64) {
	m.facts = append(m.facts, content)
}

func (m *mockMemoryManager) GetCoreFacts(ctx context.Context, query string, limit int) []string {
	return m.facts
}

func TestHandleManageMemory_Archive(t *testing.T) {
	r := NewRegistry()
	mm := &mockMemoryManager{}
	r.RegisterMemory(mm)

	result, err := r.handleManageMemory(context.Background(), `{"action":"archive","content":"Fixed login bug in auth module","importance":0.9}`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	if mm.archiveContent != "Fixed login bug in auth module" {
		t.Errorf("expected archived content, got: %q", mm.archiveContent)
	}
}

func TestHandleManageMemory_Retrieve(t *testing.T) {
	r := NewRegistry()
	mm := &mockMemoryManager{}
	r.RegisterMemory(mm)

	result, _ := r.handleManageMemory(context.Background(), `{"action":"retrieve","content":"login bug"}`)
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	if !strings.Contains(result.Content.(string), "fixed auth/service.go") {
		t.Errorf("expected retrieve result, got: %v", result.Content)
	}
}

func TestHandleManageMemory_Remember(t *testing.T) {
	r := NewRegistry()
	mm := &mockMemoryManager{}
	r.RegisterMemory(mm)

	result, _ := r.handleManageMemory(context.Background(), `{"action":"remember","content":"Token validation is in middleware/token.go","importance":0.8}`)
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	if len(mm.facts) != 1 || mm.facts[0] != "Token validation is in middleware/token.go" {
		t.Errorf("expected fact stored, got: %v", mm.facts)
	}
}

func TestHandleManageMemory_Recall(t *testing.T) {
	r := NewRegistry()
	mm := &mockMemoryManager{}
	r.RegisterMemory(mm)
	mm.StoreCoreFact(context.Background(), "Important: always check middleware first", 0.9)

	result, _ := r.handleManageMemory(context.Background(), `{"action":"recall","content":"middleware"}`)
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	if !strings.Contains(result.Content.(string), "always check middleware first") {
		t.Errorf("expected recall, got: %v", result.Content)
	}
}

func TestHandleManageMemory_NoManager(t *testing.T) {
	r := NewRegistry() // 未注入 memory manager
	result, _ := r.handleManageMemory(context.Background(), `{"action":"archive","content":"test"}`)
	if result.Error != "" {
		t.Fatal(result.Error)
	}
}

func TestMemoryToolDefs(t *testing.T) {
	defs := memoryToolDefs()
	if len(defs) != 1 || defs[0].Function.Name != "manage_memory" {
		t.Error("expected manage_memory tool definition")
	}
}

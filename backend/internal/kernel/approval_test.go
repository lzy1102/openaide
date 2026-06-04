package kernel

import (
	"context"
	"testing"
)

type mockApprovalLLM struct{ risk string }

func (m *mockApprovalLLM) Chat(ctx context.Context, msgs []Message, tools []ToolDefinition, opts map[string]interface{}) (*LLMResponse, error) {
	return &LLMResponse{Content: m.risk}, nil
}
func (m *mockApprovalLLM) ChatStream(ctx context.Context, msgs []Message, tools []ToolDefinition, opts map[string]interface{}) (<-chan StreamChunk, error) {
	return nil, nil
}
func (m *mockApprovalLLM) GetModelID() string { return "mock" }
func (m *mockApprovalLLM) SetModelID(mdl string) {}

func TestAutoApprover_LowRisk(t *testing.T) {
	a := NewAutoApprover()
	result := a.RequestApproval(context.Background(), &ApprovalRequest{
		ID: "1", Tool: "read_file", Args: "{}", Reason: "read", Risk: "low",
	})
	if !result.Approved {
		t.Error("read_file should auto-approve (whitelist)")
	}
}

func TestAutoApprover_LLMSafe(t *testing.T) {
	a := NewAutoApprover()
	a.SetLLM(&mockApprovalLLM{risk: "safe"})
	result := a.RequestApproval(context.Background(), &ApprovalRequest{
		ID: "1", Tool: "execute_command", Args: `{"command":"ls"}`, Reason: "list", Risk: "high",
	})
	if !result.Approved {
		t.Error("LLM-assessed safe tools should auto-approve")
	}
}

func TestAutoApprover_LLMDangerous(t *testing.T) {
	a := NewAutoApprover()
	a.SetLLM(&mockApprovalLLM{risk: "dangerous"})
	result := a.RequestApproval(context.Background(), &ApprovalRequest{
		ID: "1", Tool: "execute_command", Args: `{"command":"rm -rf /"}`, Reason: "destroy", Risk: "high",
	})
	if result.Approved {
		t.Error("LLM-assessed dangerous tools should NOT auto-approve")
	}
}

func TestAutoApprover_UnsafeMode(t *testing.T) {
	a := NewAutoApprover()
	a.UnsafeMode = true
	result := a.RequestApproval(context.Background(), &ApprovalRequest{
		ID: "1", Tool: "execute_command", Args: `{"command":"rm -rf /"}`, Reason: "destroy", Risk: "high",
	})
	if !result.Approved {
		t.Error("UnsafeMode should auto-approve everything")
	}
}

func TestApprovalRequest(t *testing.T) {
	req := &ApprovalRequest{
		ID: "abc", Tool: "execute_command",
		Args: `{"command":"ls"}`, Reason: "execute", Risk: "high",
	}
	if req.ID != "abc" || req.Risk != "high" {
		t.Error("request fields mismatch")
	}
}

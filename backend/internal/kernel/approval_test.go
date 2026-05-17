package kernel

import (
	"context"
	"testing"
)

func TestAutoApprover_LowRisk(t *testing.T) {
	a := NewAutoApprover()
	result := a.RequestApproval(context.Background(), &ApprovalRequest{
		ID: "1", Tool: "read_file", Args: "{}", Reason: "read", Risk: "low",
	})
	if !result.Approved {
		t.Error("low risk tools should auto-approve")
	}
}

func TestAutoApprover_HighRisk(t *testing.T) {
	a := NewAutoApprover()
	result := a.RequestApproval(context.Background(), &ApprovalRequest{
		ID: "1", Tool: "execute_command", Args: `{"command":"rm -rf /"}`, Reason: "dangerous", Risk: "high",
	})
	if result.Approved {
		t.Error("high risk tools should NOT auto-approve")
	}
}

func TestAutoApprover_WriteFile(t *testing.T) {
	a := NewAutoApprover()
	result := a.RequestApproval(context.Background(), &ApprovalRequest{
		ID: "1", Tool: "write_file", Args: "{}", Reason: "write", Risk: "medium",
	})
	if result.Approved {
		t.Error("write_file should NOT auto-approve")
	}
}

func TestDangerousTools_List(t *testing.T) {
	if _, ok := DangerousTools["execute_command"]; !ok {
		t.Error("execute_command should be dangerous")
	}
	if _, ok := DangerousTools["write_file"]; !ok {
		t.Error("write_file should be dangerous")
	}
	if _, ok := DangerousTools["read_file"]; ok {
		t.Error("read_file should NOT be dangerous")
	}
}

func TestApprovalRequest(t *testing.T) {
	req := &ApprovalRequest{
		ID: "abc", Tool: "execute_command",
		Args: `{"command":"ls"}`, Reason: "执行系统命令", Risk: "high",
	}
	if req.ID != "abc" || req.Risk != "high" {
		t.Error("request fields mismatch")
	}
}

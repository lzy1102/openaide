package kernel

import (
	"context"
	"fmt"
	"strings"
)

// ApprovalRequest 审批请求
type ApprovalRequest struct {
	ID       string `json:"id"`
	Tool     string `json:"tool"`
	Args     string `json:"args"`
	Reason   string `json:"reason"`
	Risk     string `json:"risk"` // low, medium, high
}

// ApprovalResult 审批结果
type ApprovalResult struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason,omitempty"`
}

// Approver 审批接口
type Approver interface {
	RequestApproval(ctx context.Context, req *ApprovalRequest) *ApprovalResult
}

//   UnsafeMode=true:  放行所有工具
//   UnsafeMode=false: LLM 评估风险 + 白名单快速通道
type AutoApprover struct {
	ApprovedTools map[string]bool
	UnsafeMode    bool
	llm           LLMProvider // 可选：LLM 评估工具调用风险
}

// NewAutoApprover 创建自动审批器（默认安全模式）
func NewAutoApprover() *AutoApprover {
	return &AutoApprover{
		UnsafeMode: false,
		ApprovedTools: map[string]bool{
			"read_file":        true,
			"list_directory":   true,
			"search_files":     true,
			"search_symbols":   true,
			"search_knowledge": true,
			"git_status":       true,
			"git_diff":         true,
			"git_log":          true,
			"git_blame":        true,
			"add_knowledge":    true,
			"read_image":       true,
		},
	}
}

// SetLLM 注入 LLM 用于智能风险评估
func (a *AutoApprover) SetLLM(llm LLMProvider) { a.llm = llm }

func (a *AutoApprover) RequestApproval(ctx context.Context, req *ApprovalRequest) *ApprovalResult {
	if a.UnsafeMode {
		return &ApprovalResult{Approved: true, Reason: "auto-approved (unsafe mode)"}
	}

	// Read-only tools always safe — no LLM call needed
	if approved, ok := a.ApprovedTools[req.Tool]; ok && approved {
		return &ApprovalResult{Approved: true, Reason: "auto-approved"}
	}

	// LLM assesses risk based on specific arguments
	return a.assessWithLLM(ctx, req)
}

func (a *AutoApprover) assessWithLLM(ctx context.Context, req *ApprovalRequest) *ApprovalResult {
	prompt := fmt.Sprintf(`Assess the risk of this tool call. Consider the tool name AND its arguments.

Tool: %s
Arguments: %s

Risk levels:
- safe: read-only operations, harmless commands (ls, pwd, cat), legitimate code edits
- caution: file writes, package installs, git operations
- dangerous: destructive commands (rm, drop, format), suspicious URLs, privilege escalation

Reply with ONE word: safe, caution, or dangerous.`, req.Tool, req.Args)

	resp, err := a.llm.Chat(ctx, []Message{
		{Role: "user", Content: prompt},
	}, nil, map[string]interface{}{"max_tokens": 10, "temperature": 0})
	if err != nil {
		return &ApprovalResult{Approved: false, Reason: "risk assessment unavailable"}
	}

	risk := strings.TrimSpace(strings.ToLower(resp.Content))
	switch {
	case risk == "safe":
		return &ApprovalResult{Approved: true, Reason: "LLM assessed as safe"}
	case risk == "caution":
		return &ApprovalResult{Approved: false, Reason: fmt.Sprintf("LLM assessed as caution: check args '%s'", req.Args)}
	default:
		return &ApprovalResult{Approved: false, Reason: fmt.Sprintf("LLM assessed as dangerous: %s", risk)}
	}
}


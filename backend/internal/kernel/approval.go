package kernel

import (
	"context"
	"fmt"
	"sync"
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

// InteractiveApprover 交互式审批器 — 通过channel与CLI通信
type InteractiveApprover struct {
	mu       sync.Mutex
	pending  chan *ApprovalRequest
	response chan *ApprovalResult
}

// NewInteractiveApprover 创建交互式审批器
func NewInteractiveApprover() *InteractiveApprover {
	return &InteractiveApprover{
		pending:  make(chan *ApprovalRequest, 10),
		response: make(chan *ApprovalResult, 10),
	}
}

func (a *InteractiveApprover) RequestApproval(ctx context.Context, req *ApprovalRequest) *ApprovalResult {
	select {
	case a.pending <- req:
	case <-ctx.Done():
		return &ApprovalResult{Approved: false, Reason: "timeout"}
	}

	select {
	case result := <-a.response:
		return result
	case <-ctx.Done():
		return &ApprovalResult{Approved: false, Reason: "timeout"}
	}
}

// WaitForApproval 等待审批请求（CLI调用）
func (a *InteractiveApprover) WaitForApproval() *ApprovalRequest {
	return <-a.pending
}

// Respond 响应审批请求（CLI调用）
func (a *InteractiveApprover) Respond(id string, approved bool, reason string) {
	a.response <- &ApprovalResult{Approved: approved, Reason: reason}
}

// AutoApprover 自动审批器 — 低风险自动通过
type AutoApprover struct {
	ApprovedTools map[string]bool
}

// NewAutoApprover 创建自动审批器
func NewAutoApprover() *AutoApprover {
	return &AutoApprover{
		ApprovedTools: map[string]bool{
			"read_file":       true,
			"list_directory":  true,
			"search_files":    true,
			"search_symbols":  true,
			"search_knowledge": true,
			"git_status":      true,
			"git_diff":        true,
			"git_log":         true,
			"git_blame":       true,
			// 本地Agent默认放行
			"write_file":        true,
			"execute_command":   true,
			"add_knowledge":     true,
			"read_image":        true,
			"diff_edit":         true,
			"diff_edit_lines":   true,
		},
	}
}

func (a *AutoApprover) RequestApproval(ctx context.Context, req *ApprovalRequest) *ApprovalResult {
	if approved, ok := a.ApprovedTools[req.Tool]; ok && approved {
		return &ApprovalResult{Approved: true, Reason: "auto-approved (low risk)"}
	}
	return &ApprovalResult{Approved: false, Reason: fmt.Sprintf("tool '%s' requires manual approval", req.Tool)}
}

// DangerousTools 高危工具列表
var DangerousTools = map[string]string{
	"execute_command": "执行任意系统命令，可能造成不可逆损害",
	"write_file":      "写入文件可能覆盖重要内容",
	"diff_edit":       "搜索替换可能错误匹配",
	"diff_edit_lines": "按行替换可能破坏代码结构",
}

package kernel

import (
	"context"
	"fmt"
	"strings"
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

// AutoApprover 自动审批器
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

	// 白名单快速通道
	if approved, ok := a.ApprovedTools[req.Tool]; ok && approved {
		return &ApprovalResult{Approved: true, Reason: "auto-approved"}
	}

	// LLM 风险评估：根据具体参数判断风险
	if a.llm != nil {
		return a.assessWithLLM(ctx, req)
	}

	// 无 LLM 时的兜底：已知高危工具拒绝，其他需手动审批
	if _, dangerous := DangerousTools[req.Tool]; dangerous {
		return &ApprovalResult{Approved: false, Reason: fmt.Sprintf("高危工具 '%s': %s", req.Tool, DangerousTools[req.Tool])}
	}
	return &ApprovalResult{Approved: false, Reason: fmt.Sprintf("tool '%s' requires manual approval", req.Tool)}
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
	case strings.Contains(risk, "safe"):
		return &ApprovalResult{Approved: true, Reason: "LLM assessed as safe"}
	case strings.Contains(risk, "caution"):
		return &ApprovalResult{Approved: false, Reason: fmt.Sprintf("LLM assessed as caution: check args '%s'", req.Args)}
	default:
		return &ApprovalResult{Approved: false, Reason: fmt.Sprintf("LLM assessed as dangerous: %s", risk)}
	}
}

// DangerousTools 高危工具列表（无 LLM 时的兜底）
var DangerousTools = map[string]string{
	"execute_command": "执行任意系统命令，可能造成不可逆损害",
	"write_file":      "写入文件可能覆盖重要内容",
	"diff_edit":       "搜索替换可能错误匹配",
}

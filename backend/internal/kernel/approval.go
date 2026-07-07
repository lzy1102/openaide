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

// SafeCommandPrefixes — execute_command 匹配这些前缀时自动放行（免 LLM 评估）
var SafeCommandPrefixes = []string{
	"git log", "git diff", "git status", "git blame", "git show",
	"ls", "pwd", "cat", "head", "tail", "wc", "file",
	"go test", "go build", "go vet", "go fmt",
	"npm test", "npm run", "npx",
	"make", "cargo test", "cargo build",
	"python", "python3", "pip",
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
			"git_status":       true,
			"git_diff":         true,
			"git_log":          true,
			"git_blame":        true,
			"read_image":       true,
		},
	}
}

// isSafeCommand — execute_command 命令前缀匹配
func isSafeCommand(args string) bool {
	args = strings.TrimSpace(args)
	for _, prefix := range SafeCommandPrefixes {
		if strings.HasPrefix(args, prefix) {
			return true
		}
	}
	return false
}

// SetLLM 注入 LLM 用于智能风险评估
func (a *AutoApprover) SetLLM(llm LLMProvider) { a.llm = llm }

func (a *AutoApprover) RequestApproval(ctx context.Context, req *ApprovalRequest) *ApprovalResult {
	if a.UnsafeMode {
		return &ApprovalResult{Approved: true, Reason: "auto-approved (unsafe mode)"}
	}

	if approved, ok := a.ApprovedTools[req.Tool]; ok && approved {
		return &ApprovalResult{Approved: true, Reason: "auto-approved"}
	}

	if req.Tool == "execute_command" && isSafeCommand(req.Args) {
		return &ApprovalResult{Approved: true, Reason: "safe command prefix"}
	}

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


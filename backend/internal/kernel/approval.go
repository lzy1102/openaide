package kernel

import (
	"context"
	"fmt"
	"strings"
)

type ApprovalRequest struct {
	ID     string `json:"id"`
	Tool   string `json:"tool"`
	Args   string `json:"args"`
	Reason string `json:"reason"`
	Risk   string `json:"risk"`
}

type ApprovalResult struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason,omitempty"`
}

type Approver interface {
	RequestApproval(ctx context.Context, req *ApprovalRequest) *ApprovalResult
}

var SafeCommandPrefixes = []string{
	"git log", "git diff", "git status", "git blame", "git show",
	"git branch", "git remote", "git tag",
	"ls", "pwd", "cat", "head", "tail", "wc", "file", "which",
	"go test", "go build", "go vet", "go fmt", "go version",
	"npm test", "npm run", "npm list", "npm root",
	"make", "cargo test", "cargo build", "cargo check",
	"python", "python3", "pip list", "pip show",
	"docker ps", "docker images", "docker logs",
}

type AutoApprover struct {
	ApprovedTools map[string]bool
	UnsafeMode    bool
	llm           LLMProvider
}

func NewAutoApprover() *AutoApprover {
	return &AutoApprover{
		UnsafeMode: false,
		ApprovedTools: map[string]bool{
			"read_file":      true,
			"list_directory": true,
			"search_files":   true,
			"search_symbols": true,
			"git_status":     true,
			"git_diff":       true,
			"git_log":        true,
			"git_blame":      true,
			"read_image":     true,
			// 写操作默认 auto-approve:非交互模式(subagent/API)直接放行;
			// 交互模式(REPL)的 OnApproval 回调在 approver 之前执行,
			// 用户有机会预览和拒绝。
			"write_file":        true,
			"diff_edit":         true,
			"edit_files":        true,
			"git_commit":        true,
			"git_create_branch": true,
		},
	}
}

// SetLLM 注入 LLM 用于 execute_command 的风险二次评估。
// 配置后,execute_command 会先走 LLM 判断 safe/dangerous,
// 再回退到静态规则(危险命令黑名单 / 安全命令白名单)。
func (a *AutoApprover) SetLLM(llm LLMProvider) {
	a.llm = llm
}

// isDangerousCommand 委托给共享的 token 级检测,审批层与工具层使用同一实现,避免规则漂移导致绕过。
func isDangerousCommand(args string) bool {
	return IsDangerousCommand(args)
}

func isSafeCommand(args string) bool {
	args = strings.TrimSpace(args)
	for _, prefix := range SafeCommandPrefixes {
		if strings.HasPrefix(args, prefix) {
			return true
		}
	}
	return false
}

func (a *AutoApprover) RequestApproval(ctx context.Context, req *ApprovalRequest) *ApprovalResult {
	if a.UnsafeMode {
		return &ApprovalResult{Approved: true, Reason: "auto-approved (unsafe mode)"}
	}

	if approved, ok := a.ApprovedTools[req.Tool]; ok && approved {
		return &ApprovalResult{Approved: true, Reason: "auto-approved"}
	}

	if req.Tool == "execute_command" {
		// LLM 风险评估优先于静态规则
		if a.llm != nil {
			prompt := fmt.Sprintf(
				"Assess the risk of executing this command. Reply with exactly one word: safe or dangerous.\nCommand: %s",
				req.Args,
			)
			if resp, err := a.llm.Chat(ctx, []Message{{Role: "user", Content: prompt}}, nil, nil); err == nil && resp != nil {
				assessment := strings.ToLower(strings.TrimSpace(resp.Content))
				if strings.Contains(assessment, "dangerous") {
					return &ApprovalResult{Approved: false, Reason: "LLM assessed as dangerous"}
				}
				if strings.Contains(assessment, "safe") {
					return &ApprovalResult{Approved: true, Reason: "LLM assessed as safe"}
				}
			}
		}
		if isDangerousCommand(req.Args) {
			return &ApprovalResult{Approved: false, Reason: "dangerous command — needs approval"}
		}
		if isSafeCommand(req.Args) {
			return &ApprovalResult{Approved: true, Reason: "safe command"}
		}
		return &ApprovalResult{Approved: true, Reason: "auto-approved"}
	}

	return &ApprovalResult{Approved: true, Reason: "auto-approved"}
}

package kernel

import (
	"context"
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

var DangerousCommandPrefixes = []string{
	"rm -rf", "rm -r /", "rm -f /",
	"rmdir /", "mkfs", "format",
	"sudo rm", "sudo mkfs", "sudo format",
	"DROP TABLE", "DROP DATABASE", "DELETE FROM",
	"dd if=", "> /dev/sd",
	"shutdown", "reboot", "halt", "init 0",
	"chmod -R 777 /", "chown -R",
}

type AutoApprover struct {
	ApprovedTools map[string]bool
	UnsafeMode    bool
}

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

func isDangerousCommand(args string) bool {
	args = strings.TrimSpace(strings.ToLower(args))
	for _, prefix := range DangerousCommandPrefixes {
		if strings.HasPrefix(args, strings.ToLower(prefix)) {
			return true
		}
	}
	return false
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

func (a *AutoApprover) RequestApproval(_ context.Context, req *ApprovalRequest) *ApprovalResult {
	if a.UnsafeMode {
		return &ApprovalResult{Approved: true, Reason: "auto-approved (unsafe mode)"}
	}

	if approved, ok := a.ApprovedTools[req.Tool]; ok && approved {
		return &ApprovalResult{Approved: true, Reason: "auto-approved"}
	}

	if req.Tool == "execute_command" {
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

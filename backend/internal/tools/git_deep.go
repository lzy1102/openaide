package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"openaide/backend/internal/git"
	"openaide/backend/internal/kernel"
)

func gitToolDefs() []kernel.ToolDefinition {
	return []kernel.ToolDefinition{
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "git_status",
				Description: "获取 Git 仓库状态",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "仓库路径（可选，默认当前目录）",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "git_diff",
				Description: "获取Git工作区差异",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{"type": "string", "description": "文件路径（可选，默认所有文件）"},
						"staged": map[string]interface{}{"type": "boolean", "description": "是否只看暂存区"},
					},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "git_log",
				Description: "获取Git提交历史",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"limit": map[string]interface{}{"type": "integer", "description": "返回条数（默认10）"},
					},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "git_blame",
				Description: "追溯文件每行的最后修改者",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{"type": "string", "description": "文件路径"},
					},
					"required": []string{"path"},
				},
			},
		},
	}
}

// handleGitStatus 获取 Git 仓库状态
func handleGitStatus(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Path string `json:"path,omitempty"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Path == "" {
		args.Path = "."
	}

	absPath, err := safeAbsPath(args.Path)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	client := git.NewClient(absPath)
	if !client.IsRepo() {
		return &kernel.ToolResult{Error: fmt.Sprintf("not a git repository: %s", absPath)}, nil
	}

	status, err := client.Status()
	if err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("git status failed: %v", err)}, nil
	}

	root, _ := client.Root()
	var out strings.Builder
	out.WriteString(fmt.Sprintf("// Git: %s\n", root))
	out.WriteString(fmt.Sprintf("Branch: %s", status.Branch))
	if status.Ahead > 0 || status.Behind > 0 {
		out.WriteString(fmt.Sprintf(" (ahead=%d, behind=%d)", status.Ahead, status.Behind))
	}
	out.WriteString("\n")

	if status.IsClean {
		out.WriteString("Clean working tree\n")
		return &kernel.ToolResult{Content: out.String()}, nil
	}

	if len(status.Staged) > 0 {
		out.WriteString("\n[Staged]\n")
		for _, f := range status.Staged {
			out.WriteString(fmt.Sprintf("  %s  %s\n", f.Status, f.Path))
		}
	}
	if len(status.Unstaged) > 0 {
		out.WriteString("\n[Unstaged]\n")
		for _, f := range status.Unstaged {
			out.WriteString(fmt.Sprintf("  %s  %s\n", f.Status, f.Path))
		}
	}
	if len(status.Untracked) > 0 {
		out.WriteString("\n[Untracked]\n")
		for _, f := range status.Untracked {
			out.WriteString(fmt.Sprintf("  ?  %s\n", f))
		}
	}
	if len(status.Conflicts) > 0 {
		out.WriteString("\n[Conflicts]\n")
		for _, f := range status.Conflicts {
			out.WriteString(fmt.Sprintf("  X  %s\n", f))
		}
	}
	return &kernel.ToolResult{Content: out.String()}, nil
}

// handleGitDiff 获取文件差异
func handleGitDiff(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Path   string `json:"path,omitempty"`
		Staged bool   `json:"staged,omitempty"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Path == "" { args.Path = "." }

	absPath, err := safeAbsPath(args.Path)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	client := git.NewClient(absPath)
	if !client.IsRepo() {
		return &kernel.ToolResult{Error: fmt.Sprintf("not a git repo: %s", absPath)}, nil
	}

	if args.Path != "." {
		diff, err := client.DiffFile(args.Path, args.Staged)
		if err != nil {
			return &kernel.ToolResult{Error: err.Error()}, nil
		}
		return &kernel.ToolResult{Content: formatDiff(diff)}, nil
	}

	diffs, err := client.DiffUnstaged()
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	var out strings.Builder
	out.WriteString(fmt.Sprintf("// Unstaged changes (%d files)\n", len(diffs)))
	for _, d := range diffs {
		out.WriteString(fmt.Sprintf("--- %s (+%d/-%d)\n%s\n", d.Path, d.Additions, d.Deletions, d.Content))
	}
	return &kernel.ToolResult{Content: out.String()}, nil
}

// handleGitLog 获取提交历史
func handleGitLog(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Path  string `json:"path,omitempty"`
		Limit int    `json:"limit,omitempty"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Limit <= 0 { args.Limit = 10 }
	if args.Path == "" { args.Path = "." }

	absPath, err := safeAbsPath(args.Path)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	client := git.NewClient(absPath)
	if !client.IsRepo() {
		return &kernel.ToolResult{Error: fmt.Sprintf("not a git repo: %s", absPath)}, nil
	}

	commits, err := client.Log(args.Limit)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	var out strings.Builder
	out.WriteString(fmt.Sprintf("// Recent %d commits\n", len(commits)))
	for _, c := range commits {
		out.WriteString(fmt.Sprintf("%s %s — %s\n", c.Hash[:8], c.Author, c.Message))
	}
	return &kernel.ToolResult{Content: out.String()}, nil
}

// handleGitBlame 文件追溯
func handleGitBlame(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Path == "" {
		return &kernel.ToolResult{Error: "path is required"}, nil
	}

	absPath, err := safeAbsPath(args.Path)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	client := git.NewClient(absPath)
	if !client.IsRepo() {
		return &kernel.ToolResult{Error: fmt.Sprintf("not a git repo: %s", absPath)}, nil
	}

	// Execute git blame via os/exec
	cmd := execCommand(ctx, absPath, "git", "blame", "--line-porcelain", args.Path)
	data, err := cmd.Output()
	if err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("git blame failed: %v", err)}, nil
	}

	var out strings.Builder
	out.WriteString(fmt.Sprintf("// Git blame: %s\n", args.Path))
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "author ") {
			out.WriteString(strings.TrimPrefix(line, "author ") + ": ")
		} else if len(line) > 0 && line[0] == '\t' {
			out.WriteString(strings.TrimPrefix(line, "\t") + "\n")
		}
	}
	return &kernel.ToolResult{Content: out.String()}, nil
}

func formatDiff(d *git.Diff) string {
	return fmt.Sprintf("--- %s (+%d/-%d)\n%s", d.Path, d.Additions, d.Deletions, d.Content)
}

func execCommand(ctx context.Context, dir, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd
}

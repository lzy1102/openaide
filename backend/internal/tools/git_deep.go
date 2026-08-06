package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
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
				Description: "Get Git repository status",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Repository path (optional, defaults to current directory)",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "git_diff",
				Description: "Get Git working-tree diff",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path":   map[string]interface{}{"type": "string", "description": "File path (optional, defaults to all files)"},
						"staged": map[string]interface{}{"type": "boolean", "description": "Only show staged changes"},
					},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "git_log",
				Description: "Get Git commit history",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"limit": map[string]interface{}{"type": "integer", "description": "Number of commits to return (default 10)"},
					},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "git_blame",
				Description: "Trace the last modifier of each line in a file",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{"type": "string", "description": "File path"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name: "git_commit",
				Description: "Stage and commit files to Git. Automatically runs git add on the specified files (or all changes) before committing." +
					"Use to close the loop after edits are complete.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"message": map[string]interface{}{
							"type":        "string",
							"description": "Commit message",
						},
						"paths": map[string]interface{}{
							"type":        "array",
							"items":       map[string]interface{}{"type": "string"},
							"description": "File paths to stage (optional, defaults to all changes)",
						},
					},
					"required": []string{"message"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "git_create_branch",
				Description: "Create and switch to a new branch. Use to isolate work before making changes.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "New branch name",
						},
					},
					"required": []string{"name"},
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

	absPath, err := validateAndResolve(args.Path)
	if err != nil {
		return toolErrInvalidPath(err), nil
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
	if args.Path == "" {
		args.Path = "."
	}

	absPath, err := validateAndResolve(args.Path)
	if err != nil {
		return toolErrInvalidPath(err), nil
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
	if args.Limit <= 0 {
		args.Limit = 10
	}
	if args.Path == "" {
		args.Path = "."
	}

	absPath, err := validateAndResolve(args.Path)
	if err != nil {
		return toolErrInvalidPath(err), nil
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

	absPath, err := validateAndResolve(args.Path)
	if err != nil {
		return toolErrInvalidPath(err), nil
	}

	// git commands run from the file's directory, not the file itself.
	client := git.NewClient(filepath.Dir(absPath))
	if !client.IsRepo() {
		return &kernel.ToolResult{Error: fmt.Sprintf("not a git repo: %s", absPath)}, nil
	}

	// Execute git blame via os/exec
	cmd := execCommand(ctx, filepath.Dir(absPath), "git", "blame", "--line-porcelain", args.Path)
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

// handleGitCommit 暂存并提交文件
func handleGitCommit(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Message string   `json:"message"`
		Paths   []string `json:"paths,omitempty"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Message == "" {
		return &kernel.ToolResult{Error: "message is required"}, nil
	}

	cwd, _ := validatePath(".")
	client := git.NewClient(cwd)
	if !client.IsRepo() {
		return &kernel.ToolResult{Error: "not a git repository"}, nil
	}

	// 暂存文件:有 paths 就 add 指定文件,否则 add 全部
	if len(args.Paths) > 0 {
		if err := client.Add(args.Paths...); err != nil {
			return &kernel.ToolResult{Error: fmt.Sprintf("git add failed: %v", err)}, nil
		}
	} else {
		if err := client.Add("."); err != nil {
			return &kernel.ToolResult{Error: fmt.Sprintf("git add failed: %v", err)}, nil
		}
	}

	// 提交
	if err := client.Commit(args.Message); err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("git commit failed: %v", err)}, nil
	}

	// 读回最新 commit 确认
	commits, _ := client.Log(1)
	var hash, author string
	if len(commits) > 0 {
		hash = commits[0].Hash[:8]
		author = commits[0].Author
	}
	return &kernel.ToolResult{
		Content: fmt.Sprintf("✓ Committed: %s — %s\n  Author: %s\n  Message: %s",
			hash, args.Message, author, args.Message),
	}, nil
}

// handleGitCreateBranch 创建并切换到新分支
func handleGitCreateBranch(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Name == "" {
		return &kernel.ToolResult{Error: "name is required"}, nil
	}

	cwd, _ := validatePath(".")
	client := git.NewClient(cwd)
	if !client.IsRepo() {
		return &kernel.ToolResult{Error: "not a git repository"}, nil
	}

	// 创建并切换:git checkout -b <name>
	cmd := execCommand(ctx, cwd, "git", "checkout", "-b", args.Name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return &kernel.ToolResult{
			Error: fmt.Sprintf("git checkout -b failed: %v\n%s", err, strings.TrimSpace(string(output))),
		}, nil
	}

	return &kernel.ToolResult{
		Content: fmt.Sprintf("✓ Created and switched to branch: %s", args.Name),
	}, nil
}

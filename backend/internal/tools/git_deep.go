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

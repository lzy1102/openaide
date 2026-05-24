package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"openaide/backend/internal/kernel"
)

func fileSystemToolDefs() []kernel.ToolDefinition {
	return []kernel.ToolDefinition{
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "read_file",
				Description: "读取文件内容，支持 offset/limit 分页",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "文件路径",
						},
						"offset": map[string]interface{}{
							"type":        "integer",
							"description": "起始行号（0-based，默认0）",
						},
						"limit": map[string]interface{}{
							"type":        "integer",
							"description": "读取行数（默认全部）",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "write_file",
				Description: "写入文件内容",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "文件路径",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "文件内容",
						},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "execute_command",
				Description: "执行系统命令",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"command": map[string]interface{}{
							"type":        "string",
							"description": "要执行的命令",
						},
						"working_dir": map[string]interface{}{
							"type":        "string",
							"description": "工作目录（可选）",
						},
					},
					"required": []string{"command"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "list_directory",
				Description: "列出目录内容",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "目录路径",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "search_files",
				Description: "搜索文件内容",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"pattern": map[string]interface{}{
							"type":        "string",
							"description": "搜索模式",
						},
						"path": map[string]interface{}{
							"type":        "string",
							"description": "搜索路径",
						},
					},
					"required": []string{"pattern", "path"},
				},
			},
		},
	}
}

// handleReadFile 读取文件内容
func handleReadFile(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Path   string `json:"path"`
		Offset int    `json:"offset,omitempty"`
		Limit  int    `json:"limit,omitempty"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error(), ErrorCode: "INVALID_PATH", IsRetryable: false}, nil
	}
	if args.Path == "" {
		return &kernel.ToolResult{Error: "path is required"}, nil
	}

	absPath, err := safeAbsPath(args.Path)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error(), ErrorCode: "INVALID_PATH", IsRetryable: false}, nil
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("read failed: %v", err)}, nil
	}

	lines := strings.Split(string(data), "\n")
	if args.Offset > 0 || args.Limit > 0 {
		if args.Offset >= len(lines) {
			return &kernel.ToolResult{Content: ""}, nil
		}
		end := len(lines)
		if args.Limit > 0 && args.Offset+args.Limit < end {
			end = args.Offset + args.Limit
		}
		lines = lines[args.Offset:end]
	}

	var out strings.Builder
	out.WriteString(fmt.Sprintf("// %s (%d lines)\n", absPath, len(lines)))
	digits := len(fmt.Sprintf("%d", len(lines)))
	for i, line := range lines {
		fmt.Fprintf(&out, "%*d | %s\n", digits, i+1, line)
	}
	return &kernel.ToolResult{Content: out.String()}, nil
}

// handleWriteFile 写入文件
func handleWriteFile(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error(), ErrorCode: "INVALID_PATH", IsRetryable: false}, nil
	}
	if args.Path == "" {
		return &kernel.ToolResult{Error: "path is required"}, nil
	}

	absPath, err := safeAbsPath(args.Path)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error(), ErrorCode: "INVALID_PATH", IsRetryable: false}, nil
	}

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("mkdir failed: %v", err)}, nil
	}

	if err := os.WriteFile(absPath, []byte(args.Content), 0644); err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("write failed: %v", err)}, nil
	}
	return &kernel.ToolResult{Content: fmt.Sprintf("Wrote %d bytes to %s", len(args.Content), absPath)}, nil
}

// handleExecuteCommand 执行系统命令
func handleExecuteCommand(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Command    string `json:"command"`
		WorkingDir string `json:"working_dir,omitempty"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error(), ErrorCode: "INVALID_PATH", IsRetryable: false}, nil
	}
	if args.Command == "" {
		return &kernel.ToolResult{Error: "command is required"}, nil
	}

	timeout := 30 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline) - time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "sh", "-c", args.Command)
	if args.WorkingDir != "" {
		absDir, err := safeAbsPath(args.WorkingDir)
		if err != nil {
			return &kernel.ToolResult{Error: err.Error(), ErrorCode: "INVALID_PATH", IsRetryable: false}, nil
		}
		cmd.Dir = absDir
	} else {
		cmd.Dir, _ = os.Getwd()
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return &kernel.ToolResult{Error: fmt.Sprintf("exec failed: %v", err), ErrorCode: "EXEC_FAILED", IsRetryable: true}, nil
		}
	}

	out := stdout.String()
	if stderr.Len() > 0 {
		out += "\n[stderr]\n" + stderr.String()
	}
	if len(out) > 100*1024 {
		out = out[:100*1024] + "\n... (truncated)"
	}
	return &kernel.ToolResult{Content: fmt.Sprintf("[exit=%d]\n%s", exitCode, out)}, nil
}

// handleListDirectory 列出目录内容
func handleListDirectory(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error(), ErrorCode: "INVALID_PATH", IsRetryable: false}, nil
	}
	if args.Path == "" {
		args.Path = "."
	}

	absPath, err := safeAbsPath(args.Path)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error(), ErrorCode: "INVALID_PATH", IsRetryable: false}, nil
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("readdir failed: %v", err), ErrorCode: "NOT_FOUND", IsRetryable: true}, nil
	}

	var out strings.Builder
	var lines []string
	for _, e := range entries {
		if isIgnored(filepath.Join(absPath, e.Name())) {
			continue
		}
		info, _ := e.Info()
		size := ""
		modTime := ""
		if info != nil {
			size = formatBytes(info.Size())
			modTime = info.ModTime().Format("Jan 02 15:04")
		}
		typeChar := " "
		if e.IsDir() {
			typeChar = "/"
		}
		lines = append(lines, fmt.Sprintf("%s  %9s  %s%s", modTime, size, e.Name(), typeChar))
	}
	out.WriteString(fmt.Sprintf("// %s/ (%d entries)\n", absPath, len(lines)))
	for _, l := range lines {
		out.WriteString(l + "\n")
	}
	return &kernel.ToolResult{Content: out.String()}, nil
}

// handleSearchFiles 搜索文件内容
func handleSearchFiles(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error(), ErrorCode: "INVALID_PATH", IsRetryable: false}, nil
	}
	if args.Pattern == "" {
		return &kernel.ToolResult{Error: "pattern is required", ErrorCode: "INVALID_ARGS", IsRetryable: true}, nil
	}
	if args.Path == "" {
		args.Path = "."
	}

	absPath, err := safeAbsPath(args.Path)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error(), ErrorCode: "INVALID_PATH", IsRetryable: false}, nil
	}

	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("invalid regex: %v", err)}, nil
	}

	const maxResults = 200
	var results []string
	total := 0

	filepath.Walk(absPath, func(fpath string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || isIgnored(fpath) {
			return nil
		}
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}
		if info.Size() > 1*1024*1024 {
			return nil
		}
		data, err := os.ReadFile(fpath)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		relPath, _ := filepath.Rel(absPath, fpath)
		for i, line := range lines {
			if re.MatchString(line) && total < maxResults {
				results = append(results, fmt.Sprintf("%s:%d: %s", relPath, i+1, strings.TrimSpace(line)))
				total++
			}
		}
		return nil
	})

	var out strings.Builder
	out.WriteString(fmt.Sprintf("// Search '%s' in %s/ (%d matches)\n", args.Pattern, absPath, total))
	for _, r := range results {
		out.WriteString(r + "\n")
	}
	if total >= maxResults {
		out.WriteString("... (truncated)\n")
	}
	return &kernel.ToolResult{Content: out.String()}, nil
}

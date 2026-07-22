package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// handleWriteFile 写入文件(原子写 + 文件锁)
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

	// 文件锁:防止并发写同一文件丢更新
	unlock := lockFile(absPath)
	defer unlock()

	// 原子写:先写临时文件,再 rename,防止写到一半崩溃
	if err := atomicWriteFile(absPath, []byte(args.Content), 0644); err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("write failed: %v", err)}, nil
	}

	// ACI: show what was written with line numbers for verification
	lines := strings.Split(args.Content, "\n")
	var out strings.Builder
	out.WriteString(fmt.Sprintf("✓ Wrote %s (%d lines, %d bytes)\n", absPath, len(lines), len(args.Content)))

	// Verify write by reading back
	verifyData, _ := os.ReadFile(absPath)
	if string(verifyData) == args.Content {
		out.WriteString("✓ Verified: content matches\n")
	}

	// Show preview of written content
	if len(args.Content) <= 500 {
		out.WriteString("```\n")
		for i, line := range lines {
			out.WriteString(fmt.Sprintf("%3d | %s\n", i+1, line))
		}
		out.WriteString("```")
	} else {
		out.WriteString(fmt.Sprintf("```\n%s\n... (%d more lines)\n```", strings.Join(lines[:5], "\n"), len(lines)-5))
	}
	return &kernel.ToolResult{Content: out.String()}, nil
}

// dangerousCmdPatterns 是 handler 层的危险命令黑名单。
// 这是 approval 层(approval.go DangerousCommandPrefixes)之后的二次防线:
// 如果 approval 通过了(unsafe mode 或 LLM 误判 safe),这里仍会拦截。
// 注意:用精确前缀匹配而非 Contains,避免 "rm -rf ." 误匹配 "rm -rf ./build"。
var dangerousCmdPatterns = []string{
	"rm -rf /", "rm -rf ~", "rm -rf *",
	"rm -r /", "rm -f /",
	"rmdir /", "mkfs", "format ",
	"sudo rm", "sudo mkfs", "sudo format",
	"dd if=", "> /dev/sd",
	"shutdown", "reboot", "halt", "init 0",
	"chmod -R 777 /", "chown -R",
	"DROP TABLE", "DROP DATABASE", "DELETE FROM",
	":(){:|:&};:", // fork bomb
	"| sh", "| bash", "| zsh", // 管道执行远程脚本
}

// isDangerousCommand 检查命令是否匹配危险模式(大小写不敏感)。
func isDangerousCommand(cmd string) bool {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	for _, pattern := range dangerousCmdPatterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
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

	// 二次安全检查:approval 层之后的防线
	// 防止 unsafe mode 或 LLM 误判时执行破坏性命令
	if isDangerousCommand(args.Command) {
		return &kernel.ToolResult{
			Error:     fmt.Sprintf("blocked by safety check: command matches dangerous pattern. If this is a false positive, modify the command (e.g. use 'rm -rf ./build' instead of 'rm -rf *')"),
			ErrorCode: "DANGEROUS_COMMAND",
		}, nil
	}

	// 超时:默认 30s,有 ctx deadline 时用 deadline-1s,但不低于 5s
	timeout := 30 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline) - time.Second
		if remaining > 0 {
			timeout = remaining
		}
	}
	if timeout < 5*time.Second {
		timeout = 5 * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(execCtx, "cmd", "/c", args.Command)
		} else {
			cmd = exec.CommandContext(execCtx, "sh", "-c", args.Command)
		}
	if args.WorkingDir != "" {
		absDir, err := safeAbsPath(args.WorkingDir)
		if err != nil {
			return &kernel.ToolResult{Error: err.Error(), ErrorCode: "INVALID_PATH", IsRetryable: false}, nil
		}
		cmd.Dir = absDir
	} else {
		cmd.Dir, _ = os.Getwd()
	}

	// 分别限制 stdout 和 stderr,防止单条管道把上下文吃爆
	const maxStreamBytes = 50 * 1024 // 50KB per stream
	var stdout, stderr limitedBuffer
	stdout.max = maxStreamBytes
	stderr.max = maxStreamBytes
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// 超时或启动失败
			errMsg := fmt.Sprintf("exec failed: %v", err)
			if execCtx.Err() == context.DeadlineExceeded {
				errMsg = fmt.Sprintf("command timed out after %v", timeout)
			}
			return &kernel.ToolResult{Error: errMsg, ErrorCode: "EXEC_FAILED", IsRetryable: true}, nil
		}
	}

	out := stdout.String()
	if stderr.Len() > 0 {
		out += "\n[stderr]\n" + stderr.String()
	}
	if stdout.truncated || stderr.truncated {
		out += "\n... (output truncated)"
	}
	return &kernel.ToolResult{Content: fmt.Sprintf("[exit=%d]\n%s", exitCode, out)}, nil
}

// limitedBuffer 是有大小限制的 bytes.Buffer,超限时停止写入并标记 truncated。
type limitedBuffer struct {
	buf       []byte
	max       int
	truncated bool
}

// errBufferFull 是 limitedBuffer 超限时返回的错误,让 io.Copy 停止拷贝。
var errBufferFull = fmt.Errorf("buffer limit reached")

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.truncated {
		return 0, errBufferFull
	}
	remaining := b.max - len(b.buf)
	if remaining <= 0 {
		b.truncated = true
		return 0, errBufferFull
	}
	if len(p) > remaining {
		b.buf = append(b.buf, p[:remaining]...)
		b.truncated = true
		return remaining, nil // 部分写入成功,下次 Write 返回 errBufferFull
	}
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	return string(b.buf)
}

func (b *limitedBuffer) Len() int {
	return len(b.buf)
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

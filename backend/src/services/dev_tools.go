package services

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/glebarez/sqlite" // SQLite driver
)

// ========================================
// CodeFormatTool 代码格式化工具
// ========================================

// CodeFormatTool 代码格式化工具
type CodeFormatTool struct{}

func (t *CodeFormatTool) Name() string { return "code_format" }

func (t *CodeFormatTool) Definition() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "code_format",
			"description": "格式化代码文件。支持 go, python, javascript, typescript, rust, c/cpp",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path":     map[string]interface{}{"type": "string", "description": "文件路径"},
					"language": map[string]interface{}{"type": "string", "description": "编程语言（可选，会自动检测）"},
				},
				"required": []string{"path"},
			},
		},
	}
}

func (t *CodeFormatTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	path, _ := params["path"].(string)
	language, _ := params["language"].(string)

	if path == "" {
		return nil, fmt.Errorf("path parameter is required")
	}

	// 自动检测语言
	if language == "" {
		language = detectLanguage(path)
	}

	// 读取文件
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// 根据语言选择格式化器
	var cmd *exec.Cmd
	switch language {
	case "go":
		cmd = exec.CommandContext(ctx, "gofmt", "-w", path)
	case "python", "py":
		cmd = exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("python3 -m black %s 2>/dev/null || python3 -m autopep8 --in-place %s 2>/dev/null || echo 'no formatter available'", path, path))
	case "javascript", "js", "typescript", "ts":
		cmd = exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("npx prettier --write %s 2>/dev/null || echo 'prettier not available'", path))
	case "java":
		cmd = exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("google-java-format --replace %s 2>/dev/null || echo 'java formatter not available'", path))
	case "rust", "rs":
		cmd = exec.CommandContext(ctx, "rustfmt", path)
	case "c", "cpp", "h", "hpp":
		cmd = exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("clang-format -i %s 2>/dev/null || echo 'clang-format not available'", path))
	default:
		return map[string]interface{}{
			"path":   path,
			"status": "no_formatter",
			"note":   fmt.Sprintf("No formatter available for language: %s", language),
		}, nil
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err = cmd.Run()
	duration := time.Since(startTime)

	if err != nil {
		return map[string]interface{}{
			"path":   path,
			"error":  err.Error(),
			"stderr": stderr.String(),
		}, nil
	}

	// 读取格式化后的内容
	newContent, _ := os.ReadFile(path)

	return map[string]interface{}{
		"path":        path,
		"language":    language,
		"success":     true,
		"duration":    duration.String(),
		"content":     string(newContent),
		"changes":     string(content) != string(newContent),
		"old_size":    len(content),
		"new_size":    len(newContent),
	}, nil
}

// ========================================
// LintTool 语法检查工具
// ========================================

// LintTool 语法检查工具
type LintTool struct{}

func (t *LintTool) Name() string { return "lint" }

func (t *LintTool) Definition() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "lint",
			"description": "语法检查和代码审查。支持 go, python, javascript, typescript, rust",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path":     map[string]interface{}{"type": "string", "description": "文件或目录路径"},
					"language": map[string]interface{}{"type": "string", "description": "编程语言（可选）"},
				},
				"required": []string{"path"},
			},
		},
	}
}

func (t *LintTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	path, _ := params["path"].(string)
	language, _ := params["language"].(string)

	if path == "" {
		return nil, fmt.Errorf("path parameter is required")
	}

	if language == "" {
		language = detectLanguage(path)
	}

	var cmd *exec.Cmd
	var issues []LintIssue

	switch language {
	case "go":
		cmd = exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("golangci-lint run --out-format=json %s 2>/dev/null || go vet %s 2>&1 || golint %s 2>&1", path, path, path))
	case "python", "py":
		cmd = exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("pylint %s --output-format=json 2>/dev/null || flake8 %s --format=json 2>/dev/null || python3 -m py_compile %s 2>&1", path, path, path))
	case "javascript", "js":
		cmd = exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("npx eslint %s --format=json 2>/dev/null || echo 'no linter available'", path))
	case "typescript", "ts":
		cmd = exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("npx tsc --noEmit %s 2>&1 || npx eslint %s --format=json 2>/dev/null || echo 'no linter available'", path, path))
	case "rust", "rs":
		cmd = exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("cargo clippy --manifest-path=%s 2>&1 || rustc --emit=metadata %s 2>&1", filepath.Dir(path), path))
	default:
		// 通用语法检查：尝试编译/解析
		return map[string]interface{}{
			"path":   path,
			"status": "no_linter",
			"note":   fmt.Sprintf("No linter available for language: %s", language),
		}, nil
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	output := stdout.String()
	errOutput := stderr.String()

	// 解析输出为 issues
	hasErrors := err != nil || strings.Contains(output, "error") || strings.Contains(errOutput, "error")
	hasWarnings := strings.Contains(output, "warning") || strings.Contains(errOutput, "warning")

	return map[string]interface{}{
		"path":       path,
		"language":   language,
		"success":    !hasErrors,
		"has_errors": hasErrors,
		"has_warnings": hasWarnings,
		"output":     truncatePlanStr(output, 5000),
		"error":      truncatePlanStr(errOutput, 2000),
		"issues":     issues,
		"duration":   duration.String(),
	}, nil
}

// LintIssue 检查发现的问题
type LintIssue struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Level   string `json:"level"`   // error, warning, info
	Message string `json:"message"`
	Rule    string `json:"rule"`
}

// ========================================
// DatabaseTool 数据库连接工具
// ========================================

// DatabaseTool 数据库连接和查询工具
type DatabaseTool struct{}

func (t *DatabaseTool) Name() string { return "database" }

func (t *DatabaseTool) Definition() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "database",
			"description": "数据库连接和查询。支持 MySQL, PostgreSQL, SQLite",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action": map[string]interface{}{"type": "string", "description": "操作类型: query, execute, schema"},
					"driver": map[string]interface{}{"type": "string", "description": "数据库驱动: mysql, postgres, sqlite3"},
					"dsn":    map[string]interface{}{"type": "string", "description": "数据库连接字符串"},
					"query":  map[string]interface{}{"type": "string", "description": "SQL 查询语句"},
				},
				"required": []string{"action", "driver", "dsn"},
			},
		},
	}
}

func (t *DatabaseTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	action, _ := params["action"].(string)
	if action == "" {
		action = "query"
	}

	switch action {
	case "query":
		return t.executeQuery(ctx, params)
	case "execute":
		return t.executeCommand(ctx, params)
	case "schema":
		return t.getSchema(ctx, params)
	default:
		return nil, fmt.Errorf("unknown action: %s (supported: query, execute, schema)", action)
	}
}

func (t *DatabaseTool) executeQuery(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	driver, _ := params["driver"].(string)
	dsn, _ := params["dsn"].(string)
	query, _ := params["query"].(string)

	if driver == "" || dsn == "" || query == "" {
		return nil, fmt.Errorf("driver, dsn, and query parameters are required")
	}

	// 超时控制
	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// 测试连接
	if err := db.PingContext(execCtx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// 执行查询
	rows, err := db.QueryContext(execCtx, query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	// 获取列名
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// 读取结果
	var results []map[string]interface{}
	limit := 1000 // 限制结果数量

	for rows.Next() && len(results) < limit {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		results = append(results, row)
	}

	return map[string]interface{}{
		"columns": columns,
		"rows":    results,
		"count":   len(results),
	}, nil
}

func (t *DatabaseTool) executeCommand(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	driver, _ := params["driver"].(string)
	dsn, _ := params["dsn"].(string)
	query, _ := params["query"].(string)

	if driver == "" || dsn == "" || query == "" {
		return nil, fmt.Errorf("driver, dsn, and query parameters are required")
	}

	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(execCtx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	result, err := db.ExecContext(execCtx, query)
	if err != nil {
		return nil, fmt.Errorf("execution failed: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	lastInsertID, _ := result.LastInsertId()

	return map[string]interface{}{
		"success":        true,
		"rows_affected":  rowsAffected,
		"last_insert_id": lastInsertID,
	}, nil
}

func (t *DatabaseTool) getSchema(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	driver, _ := params["driver"].(string)
	dsn, _ := params["dsn"].(string)

	if driver == "" || dsn == "" {
		return nil, fmt.Errorf("driver and dsn parameters are required")
	}

	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(execCtx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	var tables []string

	switch driver {
	case "mysql":
		rows, err := db.QueryContext(execCtx, "SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE()")
		if err != nil {
			return nil, fmt.Errorf("failed to get tables: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			rows.Scan(&name)
			tables = append(tables, name)
		}

	case "postgres":
		rows, err := db.QueryContext(execCtx, "SELECT tablename FROM pg_tables WHERE schemaname = 'public'")
		if err != nil {
			return nil, fmt.Errorf("failed to get tables: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			rows.Scan(&name)
			tables = append(tables, name)
		}

	case "sqlite3", "sqlite":
		rows, err := db.QueryContext(execCtx, "SELECT name FROM sqlite_master WHERE type='table'")
		if err != nil {
			return nil, fmt.Errorf("failed to get tables: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			rows.Scan(&name)
			tables = append(tables, name)
		}
	}

	return map[string]interface{}{
		"tables": tables,
		"count":  len(tables),
	}, nil
}

// ========================================
// GitTool Git 操作工具
// ========================================

// GitTool Git 操作工具
type GitTool struct{}

func (t *GitTool) Name() string { return "git" }

func (t *GitTool) Definition() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "git",
			"description": "Git 版本控制操作。支持 status, log, diff, add, commit, push, pull, branch, checkout",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action":    map[string]interface{}{"type": "string", "description": "Git 操作"},
					"repo_path": map[string]interface{}{"type": "string", "description": "仓库路径"},
					"message":   map[string]interface{}{"type": "string", "description": "提交信息"},
					"branch":    map[string]interface{}{"type": "string", "description": "分支名"},
				},
				"required": []string{"action"},
			},
		},
	}
}

func (t *GitTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	action, _ := params["action"].(string)
	repoPath, _ := params["repo_path"].(string)

	if action == "" {
		return nil, fmt.Errorf("action parameter is required")
	}

	if repoPath == "" {
		cwd, _ := os.Getwd()
		repoPath = cwd
	}

	switch action {
	case "status":
		return t.gitStatus(ctx, repoPath)
	case "log":
		return t.gitLog(ctx, repoPath, params)
	case "diff":
		return t.gitDiff(ctx, repoPath, params)
	case "add":
		return t.gitAdd(ctx, repoPath, params)
	case "commit":
		return t.gitCommit(ctx, repoPath, params)
	case "push":
		return t.gitPush(ctx, repoPath)
	case "pull":
		return t.gitPull(ctx, repoPath)
	case "branch":
		return t.gitBranch(ctx, repoPath)
	case "checkout":
		return t.gitCheckout(ctx, repoPath, params)
	default:
		return nil, fmt.Errorf("unknown git action: %s", action)
	}
}

func (t *GitTool) gitStatus(ctx context.Context, repoPath string) (interface{}, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "status", "--short")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %s", string(output))
	}

	return map[string]interface{}{
		"status": string(output),
		"has_changes": strings.TrimSpace(string(output)) != "",
	}, nil
}

func (t *GitTool) gitLog(ctx context.Context, repoPath string, params map[string]interface{}) (interface{}, error) {
	limit := 10
	if n, ok := params["limit"].(float64); ok && int(n) > 0 {
		limit = int(n)
	}

	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "log", fmt.Sprintf("--oneline -%d", limit))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git log failed: %s", string(output))
	}

	return map[string]interface{}{
		"log": string(output),
	}, nil
}

func (t *GitTool) gitDiff(ctx context.Context, repoPath string, params map[string]interface{}) (interface{}, error) {
	file, _ := params["file"].(string)

	args := []string{"-C", repoPath, "diff"}
	if file != "" {
		args = append(args, "--", file)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %s", string(output))
	}

	return map[string]interface{}{
		"diff": string(output),
	}, nil
}

func (t *GitTool) gitAdd(ctx context.Context, repoPath string, params map[string]interface{}) (interface{}, error) {
	files, _ := params["files"].([]interface{})
	if len(files) == 0 {
		files = []interface{}{"."}
	}

	args := []string{"-C", repoPath, "add"}
	for _, f := range files {
		args = append(args, fmt.Sprintf("%v", f))
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git add failed: %s", string(output))
	}

	return map[string]interface{}{
		"success": true,
		"output":  string(output),
	}, nil
}

func (t *GitTool) gitCommit(ctx context.Context, repoPath string, params map[string]interface{}) (interface{}, error) {
	message, _ := params["message"].(string)
	if message == "" {
		return nil, fmt.Errorf("message parameter is required for commit")
	}

	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "commit", "-m", message)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git commit failed: %s", string(output))
	}

	return map[string]interface{}{
		"success": true,
		"output":  string(output),
	}, nil
}

func (t *GitTool) gitPush(ctx context.Context, repoPath string) (interface{}, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "push")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git push failed: %s", string(output))
	}

	return map[string]interface{}{
		"success": true,
		"output":  string(output),
	}, nil
}

func (t *GitTool) gitPull(ctx context.Context, repoPath string) (interface{}, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "pull")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git pull failed: %s", string(output))
	}

	return map[string]interface{}{
		"success": true,
		"output":  string(output),
	}, nil
}

func (t *GitTool) gitBranch(ctx context.Context, repoPath string) (interface{}, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "branch", "--list")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git branch failed: %s", string(output))
	}

	return map[string]interface{}{
		"branches": strings.Split(strings.TrimSpace(string(output)), "\n"),
	}, nil
}

func (t *GitTool) gitCheckout(ctx context.Context, repoPath string, params map[string]interface{}) (interface{}, error) {
	branch, _ := params["branch"].(string)
	if branch == "" {
		return nil, fmt.Errorf("branch parameter is required")
	}

	createNew, _ := params["create"].(bool)

	args := []string{"-C", repoPath, "checkout"}
	if createNew {
		args = append(args, "-b")
	}
	args = append(args, branch)

	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git checkout failed: %s", string(output))
	}

	return map[string]interface{}{
		"success": true,
		"output":  string(output),
	}, nil
}

// ========================================
// FileSearchTool 文件搜索工具
// ========================================

// FileSearchTool 文件搜索工具
type FileSearchTool struct{}

func (t *FileSearchTool) Name() string { return "file_search" }

func (t *FileSearchTool) Definition() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "file_search",
			"description": "文件搜索。支持按文件名查找（find）和按内容搜索（grep）",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action":      map[string]interface{}{"type": "string", "description": "搜索类型: find, grep"},
					"path":        map[string]interface{}{"type": "string", "description": "搜索路径"},
					"pattern":     map[string]interface{}{"type": "string", "description": "搜索模式"},
					"ignore_case": map[string]interface{}{"type": "boolean", "description": "忽略大小写"},
				},
				"required": []string{"action", "pattern"},
			},
		},
	}
}

func (t *FileSearchTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	action, _ := params["action"].(string)
	if action == "" {
		action = "find"
	}

	switch action {
	case "find":
		return t.findFile(ctx, params)
	case "grep":
		return t.grepContent(ctx, params)
	default:
		return nil, fmt.Errorf("unknown action: %s (supported: find, grep)", action)
	}
}

func (t *FileSearchTool) findFile(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	path, _ := params["path"].(string)
	pattern, _ := params["pattern"].(string)
	fileType, _ := params["type"].(string)

	if path == "" {
		path = "."
	}
	if pattern == "" {
		pattern = "*"
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	args := []string{"-C", path, "-name", pattern}
	if fileType != "" {
		args = append([]string{"-type", "f"}, args...)
	}

	cmd := exec.CommandContext(ctx, "find", args...)
	output, err := cmd.CombinedOutput()

	var files []string
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			if line != "" {
				files = append(files, line)
			}
		}
	}

	return map[string]interface{}{
		"files": files,
		"count": len(files),
		"error": err != nil,
	}, nil
}

func (t *FileSearchTool) grepContent(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	path, _ := params["path"].(string)
	pattern, _ := params["pattern"].(string)
	ignoreCase, _ := params["ignore_case"].(bool)

	if path == "" {
		path = "."
	}
	if pattern == "" {
		return nil, fmt.Errorf("pattern parameter is required")
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	args := []string{"-rn", pattern, path}
	if ignoreCase {
		args = append([]string{"-i"}, args...)
	}

	cmd := exec.CommandContext(ctx, "grep", args...)
	output, err := cmd.CombinedOutput()

	var matches []map[string]interface{}
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, ":", 3)
			if len(parts) == 3 {
				matches = append(matches, map[string]interface{}{
					"file":    parts[0],
					"line":    parts[1],
					"content": parts[2],
				})
			}
		}
	}

	return map[string]interface{}{
		"matches": matches,
		"count":   len(matches),
	}, nil
}

// ========================================
// DependencyTool 依赖安装工具
// ========================================

// DependencyTool 依赖安装工具
type DependencyTool struct{}

func (t *DependencyTool) Name() string { return "dependency" }

func (t *DependencyTool) Definition() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "dependency",
			"description": "依赖包管理。支持 npm, yarn, pip, go, apt, brew, cargo",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"package_manager": map[string]interface{}{"type": "string", "description": "包管理器名称"},
					"packages":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "要安装的包列表"},
				},
				"required": []string{"package_manager", "packages"},
			},
		},
	}
}

func (t *DependencyTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	packageManager, _ := params["package_manager"].(string)
	packages, _ := params["packages"].([]interface{})

	if packageManager == "" || len(packages) == 0 {
		return nil, fmt.Errorf("package_manager and packages parameters are required")
	}

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	var args []string
	switch packageManager {
	case "npm", "yarn", "pnpm":
		args = append([]string{packageManager, "install"}, convertToStrings(packages)...)
	case "pip", "pip3":
		args = append([]string{packageManager, "install"}, convertToStrings(packages)...)
	case "go":
		args = append([]string{"go", "get"}, convertToStrings(packages)...)
	case "apt", "apt-get":
		args = append([]string{packageManager, "install", "-y"}, convertToStrings(packages)...)
	case "brew":
		args = append([]string{"brew", "install"}, convertToStrings(packages)...)
	case "cargo":
		args = append([]string{"cargo", "install"}, convertToStrings(packages)...)
	default:
		return nil, fmt.Errorf("unsupported package manager: %s", packageManager)
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir, _ = os.Getwd()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	output := stdout.String()
	errOutput := stderr.String()

	return map[string]interface{}{
		"package_manager": packageManager,
		"packages":        packages,
		"success":         err == nil,
		"output":          truncatePlanStr(output, 2000),
		"error":           truncatePlanStr(errOutput, 1000),
		"duration":        duration.String(),
	}, nil
}

// ========================================
// 辅助函数
// ========================================

// detectLanguage 根据文件扩展名检测语言
func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".java":
		return "java"
	case ".rs":
		return "rust"
	case ".c":
		return "c"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	case ".h":
		return "c"
	case ".hpp", ".hxx":
		return "cpp"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".cs":
		return "csharp"
	default:
		return ""
	}
}

// convertToStrings 将 interface{} 切片转换为字符串切片
func convertToStrings(items []interface{}) []string {
	result := make([]string, len(items))
	for i, item := range items {
		result[i] = fmt.Sprintf("%v", item)
	}
	return result
}

// RegisterDevTools 注册开发工具到 ToolService
func RegisterDevTools(toolService *ToolService) {
	toolService.registry.builtin["code_format"] = &CodeFormatTool{}
	toolService.registry.builtin["lint"] = &LintTool{}
	toolService.registry.builtin["database"] = &DatabaseTool{}
	toolService.registry.builtin["git"] = &GitTool{}
	toolService.registry.builtin["file_search"] = &FileSearchTool{}
	toolService.registry.builtin["dependency"] = &DependencyTool{}
}

// GetDevToolDefinitions 获取开发工具定义（供 LLM 使用）
func GetDevToolDefinitions() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "code_format",
				"description": "格式化代码文件。支持 go, python, javascript, typescript, java, rust, c/cpp",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path":     map[string]interface{}{"type": "string", "description": "文件路径"},
						"language": map[string]interface{}{"type": "string", "description": "编程语言（可选，会自动检测）"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "lint",
				"description": "语法检查和代码审查。支持 go, python, javascript, typescript, rust",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path":     map[string]interface{}{"type": "string", "description": "文件或目录路径"},
						"language": map[string]interface{}{"type": "string", "description": "编程语言（可选）"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "database",
				"description": "数据库连接和查询。支持 MySQL, PostgreSQL, SQLite",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"action": map[string]interface{}{"type": "string", "description": "操作类型: query, execute, schema", "enum": []string{"query", "execute", "schema"}},
						"driver": map[string]interface{}{"type": "string", "description": "数据库驱动: mysql, postgres, sqlite3"},
						"dsn":    map[string]interface{}{"type": "string", "description": "数据库连接字符串"},
						"query":  map[string]interface{}{"type": "string", "description": "SQL 查询语句"},
					},
					"required": []string{"action", "driver", "dsn"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "git",
				"description": "Git 版本控制操作。支持 status, log, diff, add, commit, push, pull, branch, checkout",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"action":    map[string]interface{}{"type": "string", "description": "Git 操作"},
						"repo_path": map[string]interface{}{"type": "string", "description": "仓库路径（可选，默认当前目录）"},
						"message":   map[string]interface{}{"type": "string", "description": "提交信息（commit 时需要）"},
						"branch":    map[string]interface{}{"type": "string", "description": "分支名（checkout 时需要）"},
						"files":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "文件列表（add 时使用）"},
						"create":    map[string]interface{}{"type": "boolean", "description": "是否创建新分支（checkout 时使用）"},
						"limit":     map[string]interface{}{"type": "number", "description": "日志条数限制（log 时使用）"},
						"file":      map[string]interface{}{"type": "string", "description": "特定文件（diff 时使用）"},
					},
					"required": []string{"action"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "file_search",
				"description": "文件搜索。支持按文件名查找（find）和按内容搜索（grep）",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"action":      map[string]interface{}{"type": "string", "description": "搜索类型: find, grep", "enum": []string{"find", "grep"}},
						"path":        map[string]interface{}{"type": "string", "description": "搜索路径"},
						"pattern":     map[string]interface{}{"type": "string", "description": "搜索模式（文件名通配符或正则表达式）"},
						"type":        map[string]interface{}{"type": "string", "description": "文件类型（find 时使用，如 f 表示文件）"},
						"ignore_case": map[string]interface{}{"type": "boolean", "description": "是否忽略大小写（grep 时使用）"},
					},
					"required": []string{"action", "pattern"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "dependency",
				"description": "依赖包管理。支持 npm, yarn, pnpm, pip, pip3, go, apt, brew, cargo",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"package_manager": map[string]interface{}{"type": "string", "description": "包管理器名称"},
						"packages":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "要安装的包列表"},
					},
					"required": []string{"package_manager", "packages"},
				},
			},
		},
	}
}

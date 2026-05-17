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
	"sync"
	"time"

	"openaide/backend/internal/git"
	"openaide/backend/internal/index"
	"openaide/backend/internal/kernel"
)

// Registry 工具注册表
type Registry struct {
	definitions map[string]kernel.ToolDefinition
	handlers    map[string]kernel.ToolHandler
	mu          sync.RWMutex
}

// NewRegistry 创建工具注册表
func NewRegistry() *Registry {
	return &Registry{
		definitions: make(map[string]kernel.ToolDefinition),
		handlers:    make(map[string]kernel.ToolHandler),
	}
}

// Register 注册工具
func (r *Registry) Register(tool kernel.ToolDefinition, handler kernel.ToolHandler) error {
	name := tool.Function.Name
	if name == "" {
		return fmt.Errorf("tool name is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.definitions[name]; exists {
		return fmt.Errorf("tool already registered: %s", name)
	}

	r.definitions[name] = tool
	r.handlers[name] = handler
	return nil
}

// Unregister 注销工具
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.definitions[name]; !exists {
		return fmt.Errorf("tool not found: %s", name)
	}

	delete(r.definitions, name)
	delete(r.handlers, name)
	return nil
}

// GetDefinitions 获取所有工具定义
func (r *Registry) GetDefinitions() []kernel.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]kernel.ToolDefinition, 0, len(r.definitions))
	for _, def := range r.definitions {
		defs = append(defs, def)
	}
	return defs
}

// GetDefinitionsByNames 按名称获取工具定义
func (r *Registry) GetDefinitionsByNames(names []string) []kernel.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]kernel.ToolDefinition, 0, len(names))
	for _, name := range names {
		if def, ok := r.definitions[name]; ok {
			defs = append(defs, def)
		}
	}
	return defs
}

// Execute 执行工具调用
func (r *Registry) Execute(ctx context.Context, call kernel.ToolCall, sessionID string) (*kernel.ToolResult, error) {
	name := call.Function.Name

	r.mu.RLock()
	handler, exists := r.handlers[name]
	r.mu.RUnlock()

	if !exists {
		return &kernel.ToolResult{Error: fmt.Sprintf("tool not found: %s", name)}, nil
	}

	return handler(ctx, call.Function.Arguments)
}

// HasTool 检查工具是否存在
func (r *Registry) HasTool(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.definitions[name]
	return exists
}

// Count 获取工具数量
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.definitions)
}

// BuiltinTools 内置工具集合
func BuiltinTools() []kernel.ToolDefinition {
	return []kernel.ToolDefinition{
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "read_file",
				Description: "读取文件内容",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "文件路径",
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
				Name:        "search_knowledge",
				Description: "搜索知识库中的文档",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "搜索查询",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "add_knowledge",
				Description: "将有用的知识存入知识库，供未来使用",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"title": map[string]interface{}{
							"type":        "string",
							"description": "知识标题",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "知识内容",
						},
						"tags": map[string]interface{}{
							"type":        "string",
							"description": "标签，逗号分隔",
						},
					},
					"required": []string{"title", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "search_symbols",
				Description: "搜索代码符号（函数、类型、方法等）",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "符号名称或部分名称",
						},
						"path": map[string]interface{}{
							"type":        "string",
							"description": "搜索路径（默认当前目录）",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "read_image",
				Description: "读取图片文件，返回base64数据供多模态模型分析",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "图片路径",
						},
					},
					"required": []string{"path"},
				},
			},
		},

		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "diff_edit",
				Description: "精确搜索替换编辑文件（只修改匹配部分）",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{"type": "string", "description": "文件路径"},
						"search_text": map[string]interface{}{"type": "string", "description": "要搜索的文本（必须唯一）"},
						"replace_text": map[string]interface{}{"type": "string", "description": "替换后的文本"},
					},
					"required": []string{"path", "search_text", "replace_text"},
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
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "web_search",
				Description: "联网搜索，获取最新信息",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{"type": "string", "description": "搜索关键词"},
						"limit": map[string]interface{}{"type": "integer", "description": "结果数量（默认5）"},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "web_fetch",
				Description: "抓取网页内容，提取正文文本",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"url": map[string]interface{}{"type": "string", "description": "网页URL"},
						"max_length": map[string]interface{}{"type": "integer", "description": "最大返回长度"},
					},
					"required": []string{"url"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "ai_search",
				Description: "AI增强搜索：搜索+抓取+分析一步到位",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{"type": "string", "description": "搜索查询"},
						"fetch_pages": map[string]interface{}{"type": "boolean", "description": "是否抓取页面内容"},
					},
					"required": []string{"query"},
				},
			},
		},
	}
}

// BuiltinHandlers 内置工具处理函数
func BuiltinHandlers() map[string]kernel.ToolHandler {
	return map[string]kernel.ToolHandler{
		"read_file":        handleReadFile,
		"write_file":       handleWriteFile,
		"execute_command":  handleExecuteCommand,
		"list_directory":   handleListDirectory,
		"search_files":     handleSearchFiles,
		"git_status":       handleGitStatus,
		"search_knowledge": handleSearchKnowledge,
		"add_knowledge":    handleAddKnowledge,
		"search_symbols":   handleSearchSymbols,
		"read_image":       handleReadImage,
		"diff_edit":        handleDiffEdit,
		"git_diff":         handleGitDiff,
		"git_log":          handleGitLog,
		"git_blame":        handleGitBlame,
		"web_search":       handleWebSearch,
		"web_fetch":        handleWebFetch,
		"ai_search":        handleAISearch,
	}
}

// RegisterBuiltins 注册所有内置工具
func RegisterBuiltins(registry *Registry) error {
	tools := BuiltinTools()
	handlers := BuiltinHandlers()

	for _, tool := range tools {
		name := tool.Function.Name
		handler, ok := handlers[name]
		if !ok {
			continue
		}
		if err := registry.Register(tool, handler); err != nil {
			return err
		}
	}

	return nil
}

// ============ 工具 Handler 实现 ============

// handleReadFile 读取文件内容
func handleReadFile(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Path   string `json:"path"`
		Offset int    `json:"offset,omitempty"`
		Limit  int    `json:"limit,omitempty"`
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
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Path == "" {
		return &kernel.ToolResult{Error: "path is required"}, nil
	}

	absPath, err := safeAbsPath(args.Path)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
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
		return &kernel.ToolResult{Error: err.Error()}, nil
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
			return &kernel.ToolResult{Error: err.Error()}, nil
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
			return &kernel.ToolResult{Error: fmt.Sprintf("exec failed: %v", err)}, nil
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
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Path == "" {
		args.Path = "."
	}

	absPath, err := safeAbsPath(args.Path)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("readdir failed: %v", err)}, nil
	}

	var out strings.Builder
	out.WriteString(fmt.Sprintf("// %s/ (%d entries)\n", absPath, len(entries)))
	for _, e := range entries {
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
		fmt.Fprintf(&out, "%s  %9s  %s%s\n", modTime, size, e.Name(), typeChar)
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
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Pattern == "" {
		return &kernel.ToolResult{Error: "pattern is required"}, nil
	}
	if args.Path == "" {
		args.Path = "."
	}

	absPath, err := safeAbsPath(args.Path)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("invalid regex: %v", err)}, nil
	}

	const maxResults = 200
	var results []string
	total := 0

	filepath.Walk(absPath, func(fpath string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
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

// handleSearchKnowledge 搜索知识库
func handleSearchKnowledge(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Query == "" {
		return &kernel.ToolResult{Error: "query is required"}, nil
	}

	// 通过 context 获取知识库引用
	kb, ok := ctx.Value(knowledgeCtxKey).(KnowledgeAccessor)
	if !ok || kb == nil {
		return &kernel.ToolResult{Error: "knowledge base not available"}, nil
	}

	items, err := kb.SearchKnowledge(ctx, args.Query, 5)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	var out strings.Builder
	out.WriteString(fmt.Sprintf("// Knowledge search: '%s' (%d results)\n", args.Query, len(items)))
	for _, item := range items {
		tags := strings.Join(item.Tags, ", ")
		out.WriteString(fmt.Sprintf("## %s [%s]\n%s\n\n", item.Title, tags, item.Content))
	}
	return &kernel.ToolResult{Content: out.String()}, nil
}

// handleAddKnowledge 添加知识到知识库
func handleAddKnowledge(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Title   string `json:"title"`
		Content string `json:"content"`
		Tags    string `json:"tags,omitempty"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Title == "" || args.Content == "" {
		return &kernel.ToolResult{Error: "title and content are required"}, nil
	}

	kb, ok := ctx.Value(knowledgeCtxKey).(KnowledgeAccessor)
	if !ok || kb == nil {
		return &kernel.ToolResult{Error: "knowledge base not available"}, nil
	}

	var tags []string
	if args.Tags != "" {
		for _, t := range strings.Split(args.Tags, ",") {
			tags = append(tags, strings.TrimSpace(t))
		}
	}
	tags = append(tags, "agent-generated")

	id, err := kb.AddKnowledge(ctx, args.Title, args.Content, "agent", tags)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	return &kernel.ToolResult{Content: fmt.Sprintf("Knowledge stored: %s", id)}, nil
}

// ============ 辅助函数 ============

// KnowledgeAccessor 知识库访问接口（避免循环导入）
type KnowledgeAccessor interface {
	AddKnowledge(ctx context.Context, title, content, source string, tags []string) (string, error)
	SearchKnowledge(ctx context.Context, query string, limit int) ([]kernel.KnowledgeItem, error)
}

type contextKey string

const knowledgeCtxKey contextKey = "knowledge"

// WithKnowledge 将知识库注入 context
func WithKnowledge(ctx context.Context, kb KnowledgeAccessor) context.Context {
	return context.WithValue(ctx, knowledgeCtxKey, kb)
}

// safeAbsPath 路径安全检查
func safeAbsPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	return filepath.Clean(abs), nil
}

// formatBytes 可读文件大小
func formatBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// handleSearchSymbols 搜索代码符号
func handleSearchSymbols(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Query string `json:"query"`
		Path  string `json:"path,omitempty"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Query == "" {
		return &kernel.ToolResult{Error: "query is required"}, nil
	}
	if args.Path == "" {
		args.Path = "."
	}

	absPath, err := safeAbsPath(args.Path)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	// 使用代码索引器
	idx, err := index.NewIndexer(absPath + "/.index")
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	symbols := idx.SearchSymbols(args.Query)

	var out strings.Builder
	out.WriteString(fmt.Sprintf("// Symbols matching '%s' in %s/ (%d results)\n", args.Query, absPath, len(symbols)))
	for _, s := range symbols {
		out.WriteString(fmt.Sprintf("%s:%d  [%s] %s\n", s.File, s.Line, s.Type, s.Name))
		if s.Signature != "" {
			out.WriteString(fmt.Sprintf("  sig: %s\n", s.Signature))
		}
	}
	return &kernel.ToolResult{Content: out.String()}, nil
}

package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

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
		return fmt.Errorf("tool name is empty")
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
	for _, d := range r.definitions {
		defs = append(defs, d)
	}
	return defs
}

// GetDefinitionsByNames 按名称获取工具定义
func (r *Registry) GetDefinitionsByNames(names []string) []kernel.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]kernel.ToolDefinition, 0, len(names))
	for _, name := range names {
		if d, ok := r.definitions[name]; ok {
			defs = append(defs, d)
		}
	}
	return defs
}

// Execute 执行工具调用
func (r *Registry) Execute(ctx context.Context, call kernel.ToolCall, sessionID string) (*kernel.ToolResult, error) {
	r.mu.RLock()
	handler, ok := r.handlers[call.Function.Name]
	r.mu.RUnlock()

	if !ok {
		return &kernel.ToolResult{
			Error: fmt.Sprintf("unknown tool: %s", call.Function.Name),
		}, nil
	}

	return handler(ctx, call.Function.Arguments)
}

// HasTool 检查工具是否存在
func (r *Registry) HasTool(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.handlers[name]
	return ok
}

// Count 返回已注册工具数量
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.definitions)
}

// BuiltinTools 所有内置工具定义
func BuiltinTools() []kernel.ToolDefinition {
	var tools []kernel.ToolDefinition
	tools = append(tools, fileSystemToolDefs()...)
	tools = append(tools, knowledgeToolDefs()...)
	tools = append(tools, symbolToolDefs()...)
	tools = append(tools, fileEditToolDefs()...)
	tools = append(tools, gitToolDefs()...)
	tools = append(tools, webToolDefs()...)
	tools = append(tools, todoToolDefs()...)
	tools = append(tools, askToolDefs()...)
	tools = append(tools, browserToolDefs()...)
	tools = append(tools, browserExtendedDefs()...)
	tools = append(tools, desktopToolDefs()...)
	tools = append(tools, multimodalToolDefs()...)
	return tools
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
		"apply_patch":      handleApplyPatch,
		"git_diff":         handleGitDiff,
		"git_log":          handleGitLog,
		"git_blame":        handleGitBlame,
		"web_search":       handleWebSearch,
		"web_fetch":        handleWebFetch,
		"ai_search":          handleAISearch,
		"browser_navigate":   handleBrowserNavigate,
		"browser_extract":    handleBrowserExtract,
		"browser_screenshot": handleBrowserScreenshot,
		"browser_click":      handleBrowserClick,
		"browser_fill":       handleBrowserFill,
		"browser_click_at":  handleBrowserClickAt,
		"browser_scroll":    handleBrowserScroll,
		"browser_type":      handleBrowserType,
		"desktop_screenshot": handleDesktopScreenshot,
		"desktop_click":     handleDesktopClick,
		"desktop_type":      handleDesktopType,
		"desktop_key":       handleDesktopKey,
		"desktop_scroll":    handleDesktopScroll,
		"desktop_move":      handleDesktopMove,
		"todo_write":       handleTodoWrite,
		"todo_read":        handleTodoRead,
		"ask_user":         handleAskUser,
		"desktop_drag":      handleDesktopDrag,
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

// ============ 辅助函数 ============

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

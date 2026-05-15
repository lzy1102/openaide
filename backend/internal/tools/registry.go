package tools

import (
	"context"
	"encoding/json"
	"fmt"
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
	}
}

// BuiltinHandlers 内置工具处理函数
func BuiltinHandlers() map[string]kernel.ToolHandler {
	return map[string]kernel.ToolHandler{
		"read_file": func(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
			var args struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal([]byte(arguments), &args); err != nil {
				return &kernel.ToolResult{Error: err.Error()}, nil
			}
			// TODO: 实现文件读取
			return &kernel.ToolResult{Content: fmt.Sprintf("File content of %s", args.Path)}, nil
		},
		"write_file": func(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
			var args struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal([]byte(arguments), &args); err != nil {
				return &kernel.ToolResult{Error: err.Error()}, nil
			}
			// TODO: 实现文件写入
			return &kernel.ToolResult{Content: fmt.Sprintf("Written to %s", args.Path)}, nil
		},
		"execute_command": func(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
			var args struct {
				Command    string `json:"command"`
				WorkingDir string `json:"working_dir,omitempty"`
			}
			if err := json.Unmarshal([]byte(arguments), &args); err != nil {
				return &kernel.ToolResult{Error: err.Error()}, nil
			}
			// TODO: 实现命令执行（带权限检查）
			return &kernel.ToolResult{Content: fmt.Sprintf("Executed: %s", args.Command)}, nil
		},
		"list_directory": func(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
			var args struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal([]byte(arguments), &args); err != nil {
				return &kernel.ToolResult{Error: err.Error()}, nil
			}
			// TODO: 实现目录列表
			return &kernel.ToolResult{Content: fmt.Sprintf("Directory listing of %s", args.Path)}, nil
		},
		"search_files": func(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
			var args struct {
				Pattern string `json:"pattern"`
				Path    string `json:"path"`
			}
			if err := json.Unmarshal([]byte(arguments), &args); err != nil {
				return &kernel.ToolResult{Error: err.Error()}, nil
			}
			// TODO: 实现文件搜索
			return &kernel.ToolResult{Content: fmt.Sprintf("Search results for %s in %s", args.Pattern, args.Path)}, nil
		},
		"git_status": func(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
			var args struct {
				Path string `json:"path,omitempty"`
			}
			if err := json.Unmarshal([]byte(arguments), &args); err != nil {
				return &kernel.ToolResult{Error: err.Error()}, nil
			}
			// TODO: 实现 Git 状态获取
			return &kernel.ToolResult{Content: "Git status placeholder"}, nil
		},
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

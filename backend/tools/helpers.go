package tools

import (
	"fmt"
	"path/filepath"

	"openaide/backend/core"
)

// validateAndResolve combines validatePath + resolveSafePath.
// Returns a safe absolute path or an error ToolResult.
func validateAndResolve(raw string) (string, error) {
	p, err := validatePath(raw)
	if err != nil {
		return "", err
	}
	return resolveSafePath(p)
}

// toolErr creates a ToolResult with an error message and optional error code.
func toolErr(code string, format string, args ...interface{}) *kernel.ToolResult {
	return &kernel.ToolResult{
		Error:     fmt.Sprintf(format, args...),
		ErrorCode: code,
	}
}

// toolErrInvalidPath creates a path-related error ToolResult.
func toolErrInvalidPath(err error) *kernel.ToolResult {
	return &kernel.ToolResult{
		Error:     err.Error(),
		ErrorCode: "INVALID_PATH",
	}
}

// toolErrIO creates an I/O error ToolResult.
func toolErrIO(operation string, err error) *kernel.ToolResult {
	return &kernel.ToolResult{
		Error: fmt.Sprintf("%s failed: %v", operation, err),
	}
}

// RegisterBuiltins registers all built-in tools into the registry.
// 由各按类注册方法聚合而成，便于后续按能力裁剪。
func RegisterBuiltins(registry *Registry) error {
	for _, reg := range []func(*Registry) error{
		(*Registry).RegisterFileTools,
		(*Registry).RegisterEditTools,
		(*Registry).RegisterGitTools,
		(*Registry).RegisterWebTools,
		(*Registry).RegisterBrowserTools,
		(*Registry).RegisterDesktopTools,
		(*Registry).RegisterAskTools,
		(*Registry).RegisterMemoryTools,
		(*Registry).RegisterLSPSymbolTools,
		(*Registry).RegisterTodoTools,
		(*Registry).RegisterVerifyTools,
		(*Registry).RegisterMultimodalTools,
	} {
		if err := reg(registry); err != nil {
			return err
		}
	}
	return nil
}

// registerGroup 将一组工具定义及其匹配的 handler 注册进 Registry。
// handler 的唯一来源是 BuiltinHandlers，按名字配对，缺失即跳过。
func (r *Registry) registerGroup(defs []kernel.ToolDefinition) error {
	handlers := r.BuiltinHandlers()
	for _, tool := range defs {
		name := tool.Function.Name
		h, ok := handlers[name]
		if !ok {
			continue
		}
		if err := r.Register(tool, h); err != nil {
			return err
		}
	}
	return nil
}

// concatDefs 拼接多组工具定义。
func concatDefs(groups ...[]kernel.ToolDefinition) []kernel.ToolDefinition {
	var out []kernel.ToolDefinition
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// RegisterFileTools 注册文件系统类工具。
func (r *Registry) RegisterFileTools() error { return r.registerGroup(fileSystemToolDefs()) }

// RegisterEditTools 注册文件编辑与撤销类工具。
func (r *Registry) RegisterEditTools() error {
	return r.registerGroup(concatDefs(fileEditToolDefs(), multiFileEditToolDefs(), undoToolDefs()))
}

// RegisterGitTools 注册 Git 类工具。
func (r *Registry) RegisterGitTools() error { return r.registerGroup(gitToolDefs()) }

// RegisterWebTools 注册联网搜索与抓取类工具。
func (r *Registry) RegisterWebTools() error { return r.registerGroup(webToolDefs()) }

// RegisterBrowserTools 注册浏览器类工具（需 Chromium，未启用时跳过）。
func (r *Registry) RegisterBrowserTools() error {
	if !browserEnabled() {
		return nil
	}
	return r.registerGroup(concatDefs(browserToolDefs(), browserExtendedDefs()))
}

// RegisterDesktopTools 注册桌面自动化类工具。
func (r *Registry) RegisterDesktopTools() error { return r.registerGroup(desktopToolDefs()) }

// RegisterAskTools 注册 ask_user 类工具。
func (r *Registry) RegisterAskTools() error { return r.registerGroup(askToolDefs()) }

// RegisterMemoryTools 注册记忆管理类工具。
func (r *Registry) RegisterMemoryTools() error { return r.registerGroup(memoryToolDefs()) }

// RegisterLSPSymbolTools 注册 LSP 与符号检索类工具。
func (r *Registry) RegisterLSPSymbolTools() error {
	return r.registerGroup(concatDefs(lspToolDefs(), symbolToolDefs()))
}

// RegisterTodoTools 注册待办类工具。
func (r *Registry) RegisterTodoTools() error { return r.registerGroup(todoToolDefs()) }

// RegisterVerifyTools 注册校验类工具。
func (r *Registry) RegisterVerifyTools() error { return r.registerGroup(verifyToolDefs()) }

// RegisterMultimodalTools 注册多模态类工具。
func (r *Registry) RegisterMultimodalTools() error { return r.registerGroup(multimodalToolDefs()) }

// BuiltinTools returns all built-in tool definitions.
func BuiltinTools() []kernel.ToolDefinition {
	var defs []kernel.ToolDefinition
	defs = append(defs, fileSystemToolDefs()...)
	defs = append(defs, fileEditToolDefs()...)
	defs = append(defs, multiFileEditToolDefs()...)
	defs = append(defs, undoToolDefs()...)
	defs = append(defs, gitToolDefs()...)
	defs = append(defs, webToolDefs()...)
	// Browser tools require a Chromium install (~500MB) and are opt-in.
	// Keep them out of the schema when disabled so the LLM never picks a
	// tool that will fail at call time.
	if browserEnabled() {
		defs = append(defs, browserToolDefs()...)
		defs = append(defs, browserExtendedDefs()...)
	}
	defs = append(defs, desktopToolDefs()...)
	defs = append(defs, askToolDefs()...)
	defs = append(defs, memoryToolDefs()...)
	defs = append(defs, lspToolDefs()...)
	defs = append(defs, symbolToolDefs()...)
	defs = append(defs, todoToolDefs()...)
	defs = append(defs, verifyToolDefs()...)
	defs = append(defs, multimodalToolDefs()...)
	return defs
}

// BuiltinHandlers returns all built-in tool handlers keyed by tool name.
func (r *Registry) BuiltinHandlers() map[string]kernel.ToolHandler {
	handlers := map[string]kernel.ToolHandler{
		"read_file":             handleReadFile,
		"write_file":            handleWriteFile,
		"list_directory":        handleListDirectory,
		"search_files":          handleSearchFiles,
		"execute_command":       handleExecuteCommand,
		"diff_edit":             handleDiffEdit,
		"diff_edit_lines":       handleDiffEditLines,
		"apply_patch":           handleApplyPatch,
		"edit_files":            handleEditFiles,
		"undo_edit":             handleUndoEdit,
		"list_undo_checkpoints": handleListUndoCheckpoints,
		"git_diff":              handleGitDiff,
		"git_status":            handleGitStatus,
		"git_log":               handleGitLog,
		"git_blame":             handleGitBlame,
		"git_create_branch":     handleGitCreateBranch,
		"git_commit":            handleGitCommit,
		"web_search":            r.handleWebSearch,
		"web_fetch":             handleWebFetch,
		"ai_search":             r.handleAISearch,
		"manage_memory":         r.handleManageMemory,
		"search_symbols":        handleSearchSymbols,
		"lsp_definition":        handleLSPDefinition,
		"lsp_references":        handleLSPReferences,
		"lsp_hover":             handleLSPHover,
		"lsp_diagnostics":       handleLSPDiagnostics,
		"lsp_symbols":           handleLSPSymbols,
		"lsp_rename":            handleLSPRename,
		"read_image":            handleReadImage,
		"todo_write":            handleTodoWrite,
		"todo_read":             handleTodoRead,
		"verify_claim":          handleVerifyClaim,
		"trace_callers":         handleTraceCallers,
		"browser_navigate":      handleBrowserNavigate,
		"browser_extract":       handleBrowserExtract,
		"browser_screenshot":    handleBrowserScreenshot,
		"browser_click":         handleBrowserClick,
		"browser_fill":          handleBrowserFill,
		"browser_click_at":      handleBrowserClickAt,
		"browser_scroll":        handleBrowserScroll,
		"browser_type":          handleBrowserType,
		"desktop_click":         handleDesktopClick,
		"desktop_move":          handleDesktopMove,
		"desktop_drag":          handleDesktopDrag,
		"desktop_scroll":        handleDesktopScroll,
		"desktop_key":           handleDesktopKey,
		"desktop_screenshot":    handleDesktopScreenshot,
		"desktop_type":          handleDesktopType,
	}
	// Register ask_user handler with access to pendingQuestions channel.
	handlers["ask_user"] = r.handleAskUser
	return handlers
}

// safeRelPath returns the relative path from base to target, cleaned.
func safeRelPath(base, target string) (string, error) {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("invalid base path: %w", err)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("invalid target path: %w", err)
	}
	return filepath.Rel(absBase, absTarget)
}

// formatBytes formats bytes to human readable string.
func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1fG", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1fM", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1fK", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%dB", b)
	}
}

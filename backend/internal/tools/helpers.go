package tools

import (
	"fmt"
	"path/filepath"

	"openaide/backend/internal/kernel"
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
func RegisterBuiltins(registry *Registry) error {
	tools := BuiltinTools()
	handlers := registry.BuiltinHandlers()
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

// BuiltinTools returns all built-in tool definitions.
func BuiltinTools() []kernel.ToolDefinition {
	var defs []kernel.ToolDefinition
	defs = append(defs, fileSystemToolDefs()...)
	defs = append(defs, fileEditToolDefs()...)
	defs = append(defs, multiFileEditToolDefs()...)
	defs = append(defs, undoToolDefs()...)
	defs = append(defs, gitToolDefs()...)
	defs = append(defs, webToolDefs()...)
	defs = append(defs, browserToolDefs()...)
	defs = append(defs, browserExtendedDefs()...)
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
		"web_search":            handleWebSearch,
		"web_fetch":             handleWebFetch,
		"ai_search":             handleAISearch,
		"manage_memory":         handleManageMemory,
		"search_symbols":        handleSearchSymbols,
		"lsp_definition":        handleLSPDefinition,
		"lsp_references":        handleLSPReferences,
		"lsp_hover":             handleLSPHover,
		"lsp_diagnostics":       handleLSPDiagnostics,
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

package tools

import (
	"fmt"
	"path/filepath"

	"openaide/backend/internal/kernel"
)

// safeAbsPath converts a path to absolute and cleans it for safety.
func safeAbsPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	return filepath.Clean(abs), nil
}

// formatBytes formats a file size in human-readable form.
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

// ── Builtin tools ─────────────────────────────────────────────

// BuiltinTools returns all built-in tool definitions.
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
	tools = append(tools, lspToolDefs()...)
	tools = append(tools, verifyToolDefs()...)
tools = append(tools, memoryToolDefs()...)
tools = append(tools, feedbackToolDefs()...)
	return tools
}

// BuiltinHandlers returns all built-in tool handlers.
func BuiltinHandlers() map[string]kernel.ToolHandler {
	return map[string]kernel.ToolHandler{
		"read_file":          handleReadFile,
		"write_file":         handleWriteFile,
		"execute_command":    handleExecuteCommand,
		"list_directory":     handleListDirectory,
		"search_files":       handleSearchFiles,
		"git_status":         handleGitStatus,
		"search_knowledge":   handleSearchKnowledge,
		"add_knowledge":      handleAddKnowledge,
		"search_symbols":     handleSearchSymbols,
		"read_image":         handleReadImage,
		"diff_edit":          handleDiffEdit,
		"apply_patch":        handleApplyPatch,
		"git_diff":           handleGitDiff,
		"git_log":            handleGitLog,
		"git_blame":          handleGitBlame,
		"web_search":         handleWebSearch,
		"web_fetch":          handleWebFetch,
		"ai_search":          handleAISearch,
		"browser_navigate":   handleBrowserNavigate,
		"browser_extract":    handleBrowserExtract,
		"browser_screenshot": handleBrowserScreenshot,
		"browser_click":      handleBrowserClick,
		"browser_fill":       handleBrowserFill,
		"browser_click_at":   handleBrowserClickAt,
		"browser_scroll":     handleBrowserScroll,
		"browser_type":       handleBrowserType,
		"desktop_screenshot": handleDesktopScreenshot,
		"desktop_click":      handleDesktopClick,
		"desktop_type":       handleDesktopType,
		"desktop_key":        handleDesktopKey,
		"desktop_scroll":     handleDesktopScroll,
		"desktop_move":       handleDesktopMove,
		"desktop_drag":       handleDesktopDrag,
		"todo_write":         handleTodoWrite,
		"todo_read":          handleTodoRead,
		"ask_user":           handleAskUser,
		"lsp_definition":     handleLSPDefinition,
		"lsp_references":     handleLSPReferences,
		"lsp_hover":          handleLSPHover,
		"lsp_diagnostics":    handleLSPDiagnostics,
		"verify_claim":       handleVerifyClaim,
		"trace_callers":      handleTraceCallers,
		"manage_memory": handleManageMemory,
		"request_feedback": handleRequestFeedback,
	}
}

// RegisterBuiltins registers all built-in tools into the registry.
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

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"openaide/backend/internal/kernel"
	"openaide/backend/internal/lsp"
)

var (
	activeLSPClients   = make(map[string]*lsp.Client)
	activeLSPClientsMu sync.RWMutex
)

// SetLSPClient registers an LSP client (called from infra on startup).
func SetLSPClient(language string, c *lsp.Client) {
	activeLSPClientsMu.Lock()
	activeLSPClients[language] = c
	activeLSPClientsMu.Unlock()
}

// CloseAllLSPClients shuts down all running language servers (called from infra on shutdown).
func CloseAllLSPClients() {
	activeLSPClientsMu.Lock()
	defer activeLSPClientsMu.Unlock()
	for lang, c := range activeLSPClients {
		c.Close()
		delete(activeLSPClients, lang)
	}
}

// NotifyFileChanged pushes a didChange to the language server for the file's
// language, keeping diagnostics fresh after edits. Safe no-op when no server
// is running for the file type.
func NotifyFileChanged(filePath string) {
	c := clientForFile(filePath)
	if c == nil {
		return
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return
	}
	c.DidChange(filePath, string(data))
}

func lspToolDefs() []kernel.ToolDefinition {
	return []kernel.ToolDefinition{
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "lsp_definition",
				Description: "Jump to the definition of a symbol at a file location. Returns file path and line where the symbol is defined.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"file":      map[string]interface{}{"type": "string", "description": "File path"},
						"line":      map[string]interface{}{"type": "integer", "description": "Line number (0-based)"},
						"character": map[string]interface{}{"type": "integer", "description": "Character offset (0-based)"},
					},
					"required": []string{"file", "line", "character"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "lsp_references",
				Description: "Find all references to a symbol across the codebase.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"file":      map[string]interface{}{"type": "string", "description": "File path"},
						"line":      map[string]interface{}{"type": "integer", "description": "Line number (0-based)"},
						"character": map[string]interface{}{"type": "integer", "description": "Character offset (0-based)"},
					},
					"required": []string{"file", "line", "character"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "lsp_hover",
				Description: "Get type information and documentation for the symbol at a position.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"file":      map[string]interface{}{"type": "string", "description": "File path"},
						"line":      map[string]interface{}{"type": "integer", "description": "Line number (0-based)"},
						"character": map[string]interface{}{"type": "integer", "description": "Character offset (0-based)"},
					},
					"required": []string{"file", "line", "character"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "lsp_diagnostics",
				Description: "Get compiler/linter errors for a file. Returns ERROR/WARN messages with line numbers.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"file": map[string]interface{}{"type": "string", "description": "File path"},
					},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "lsp_symbols",
				Description: "List all symbols (functions, types, variables) defined in a file with their kinds and line numbers.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"file": map[string]interface{}{"type": "string", "description": "File path"},
					},
					"required": []string{"file"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "lsp_rename",
				Description: "Rename a symbol across the codebase. Returns a list of files and positions where edits must be applied. Use with edit_files or diff_edit to apply the changes.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"file":      map[string]interface{}{"type": "string", "description": "File path containing the symbol"},
						"line":      map[string]interface{}{"type": "integer", "description": "Line number (0-based)"},
						"character": map[string]interface{}{"type": "integer", "description": "Character offset (0-based)"},
						"new_name":  map[string]interface{}{"type": "string", "description": "New symbol name"},
					},
					"required": []string{"file", "line", "character", "new_name"},
				},
			},
		},
	}
}

func clientForFile(filePath string) *lsp.Client {
	lang := lsp.DetectLanguage(filePath)
	if lang == "" {
		return nil
	}
	activeLSPClientsMu.RLock()
	defer activeLSPClientsMu.RUnlock()
	return activeLSPClients[lang]
}

func noLSPServerError(filePath string) *kernel.ToolResult {
	lang := lsp.DetectLanguage(filePath)
	if lang == "" {
		return &kernel.ToolResult{Error: fmt.Sprintf("unsupported file type: %s", filepath.Base(filePath))}
	}
	return &kernel.ToolResult{
		Error: fmt.Sprintf(
			"no LSP server running for %s (%s). Supported languages: go, rust, c, cpp, zig, python, ruby, lua, php, java, kotlin, scala, typescript, javascript, html, css, csharp, swift, elixir, erlang, haskell, dart, r, julia",
			filepath.Base(filePath), lang),
	}
}

func handleLSPDefinition(ctx context.Context, args string) (*kernel.ToolResult, error) {
	var a struct {
		File      string `json:"file"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
	}
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if a.File == "" {
		return &kernel.ToolResult{Error: "file parameter required"}, nil
	}
	c := clientForFile(a.File)
	if c == nil {
		return noLSPServerError(a.File), nil
	}
	if data, err := os.ReadFile(a.File); err == nil {
		c.OpenDocument(a.File, string(data))
	}
	locs, err := c.Definition(a.File, a.Line, a.Character)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if len(locs) == 0 {
		return &kernel.ToolResult{Content: "No definition found"}, nil
	}
	var sb strings.Builder
	sb.WriteString("Definitions:\n")
	for _, loc := range locs {
		sb.WriteString(fmt.Sprintf("  %s:%d\n", uriToPath(loc.URI), loc.Range.Start.Line))
	}
	return &kernel.ToolResult{Content: sb.String()}, nil
}

func handleLSPReferences(ctx context.Context, args string) (*kernel.ToolResult, error) {
	var a struct {
		File      string `json:"file"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
	}
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if a.File == "" {
		return &kernel.ToolResult{Error: "file parameter required"}, nil
	}
	c := clientForFile(a.File)
	if c == nil {
		return noLSPServerError(a.File), nil
	}
	locs, err := c.References(a.File, a.Line, a.Character)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d references:\n", len(locs)))
	for _, loc := range locs {
		sb.WriteString(fmt.Sprintf("  %s:%d\n", uriToPath(loc.URI), loc.Range.Start.Line))
	}
	return &kernel.ToolResult{Content: sb.String()}, nil
}

func handleLSPHover(ctx context.Context, args string) (*kernel.ToolResult, error) {
	var a struct {
		File      string `json:"file"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
	}
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if a.File == "" {
		return &kernel.ToolResult{Error: "file parameter required"}, nil
	}
	c := clientForFile(a.File)
	if c == nil {
		return noLSPServerError(a.File), nil
	}
	hover, err := c.Hover(a.File, a.Line, a.Character)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	return &kernel.ToolResult{Content: hover.Contents.Value}, nil
}

func handleLSPDiagnostics(ctx context.Context, args string) (*kernel.ToolResult, error) {
	var a struct{ File string }
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	var sb strings.Builder
	sb.WriteString("Diagnostics:\n")
	count := 0
	activeLSPClientsMu.RLock()
	defer activeLSPClientsMu.RUnlock()
	for lang, c := range activeLSPClients {
		if a.File != "" {
			for _, d := range c.Diagnostics(a.File) {
				sev := "INFO"
				switch d.Severity {
				case 1:
					sev = "ERROR"
				case 2:
					sev = "WARN"
				}
				sb.WriteString(fmt.Sprintf("  [%s:%s] %s:%d — %s\n", lang, sev, a.File, d.Range.Start.Line, d.Message))
				count++
			}
		} else {
			_ = lang
		}
	}
	if count == 0 {
		sb.WriteString("  No diagnostics yet. Open a file first (use lsp_definition or lsp_hover).\n")
	}
	return &kernel.ToolResult{Content: sb.String()}, nil
}

func handleLSPSymbols(ctx context.Context, args string) (*kernel.ToolResult, error) {
	var a struct{ File string }
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if a.File == "" {
		return &kernel.ToolResult{Error: "file parameter required"}, nil
	}
	c := clientForFile(a.File)
	if c == nil {
		return noLSPServerError(a.File), nil
	}
	if data, err := os.ReadFile(a.File); err == nil {
		c.OpenDocument(a.File, string(data))
	}
	syms, err := c.Symbols(a.File)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if len(syms) == 0 {
		return &kernel.ToolResult{Content: "No symbols found"}, nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Symbols in %s:\n", a.File))
	var writeSym func([]lsp.DocumentSymbol, int)
	writeSym = func(list []lsp.DocumentSymbol, depth int) {
		indent := strings.Repeat("  ", depth)
		for _, s := range list {
			sb.WriteString(fmt.Sprintf("%s- %s (kind %d, line %d)\n",
				indent, s.Name, s.Kind, s.SelectionRange.Start.Line))
			if len(s.Children) > 0 {
				writeSym(s.Children, depth+1)
			}
		}
	}
	writeSym(syms, 0)
	return &kernel.ToolResult{Content: sb.String()}, nil
}

func handleLSPRename(ctx context.Context, args string) (*kernel.ToolResult, error) {
	var a struct {
		File      string `json:"file"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
		NewName   string `json:"new_name"`
	}
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if a.File == "" || a.NewName == "" {
		return &kernel.ToolResult{Error: "file and new_name parameters required"}, nil
	}
	c := clientForFile(a.File)
	if c == nil {
		return noLSPServerError(a.File), nil
	}
	if data, err := os.ReadFile(a.File); err == nil {
		c.OpenDocument(a.File, string(data))
	}
	edit, err := c.Rename(a.File, a.Line, a.Character, a.NewName)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if edit == nil || len(edit.Changes) == 0 {
		return &kernel.ToolResult{Content: "Rename returned no changes (symbol not found?)"}, nil
	}
	paths := make([]string, 0, len(edit.Changes))
	for uri := range edit.Changes {
		paths = append(paths, uriToPath(uri))
	}
	sort.Strings(paths)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Rename %q — apply these edits:\n", a.NewName))
	for _, p := range paths {
		edits := edit.Changes[toURI(p)]
		sb.WriteString(fmt.Sprintf("  %s (%d edit(s)):\n", p, len(edits)))
		for _, e := range edits {
			sb.WriteString(fmt.Sprintf("    line %d: %q\n", e.Range.Start.Line, e.NewText))
		}
	}
	return &kernel.ToolResult{Content: sb.String()}, nil
}

func uriToPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		return strings.TrimPrefix(uri, "file://")
	}
	return uri
}

func toURI(path string) string {
	abs, _ := filepath.Abs(path)
	return "file://" + abs
}

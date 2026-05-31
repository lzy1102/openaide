package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"openaide/backend/internal/kernel"
	"openaide/backend/internal/lsp"
)

var activeLSPClients = make(map[string]*lsp.Client)

// SetLSPClient registers an LSP client (called from infra on startup).
func SetLSPClient(language string, c *lsp.Client) {
	activeLSPClients[language] = c
}

func lspToolDefs() []kernel.ToolDefinition {
	return []kernel.ToolDefinition{
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "lsp_definition",
				Description: "Jump to the definition of a symbol at a file location. Returns file path and line where the symbol is defined.",
				Parameters: map[string]interface{}{
					"type":       "object",
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
					"type":       "object",
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
					"type":       "object",
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
					"type":       "object",
					"properties": map[string]interface{}{
						"file": map[string]interface{}{"type": "string", "description": "File path"},
					},
				},
			},
		},
	}
}

func clientForFile(filePath string) *lsp.Client {
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".go":
		return activeLSPClients["go"]
	case ".py":
		return activeLSPClients["python"]
	case ".ts", ".tsx":
		return activeLSPClients["typescript"]
	case ".js", ".jsx":
		return activeLSPClients["javascript"]
	}
	return nil
}

func handleLSPDefinition(ctx context.Context, args string) (*kernel.ToolResult, error) {
	var a struct {
		File      string `json:"file"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
	}
	json.Unmarshal([]byte(args), &a)
	if a.File == "" {
		return &kernel.ToolResult{Error: "file parameter required"}, nil
	}
	c := clientForFile(a.File)
	if c == nil {
		return &kernel.ToolResult{Error: "no LSP server for this file type. Supported: .go, .py, .ts, .js"}, nil
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
		sb.WriteString(fmt.Sprintf("  %s:%d\n", filepath.Base(loc.URI), loc.Range.Start.Line))
	}
	return &kernel.ToolResult{Content: sb.String()}, nil
}

func handleLSPReferences(ctx context.Context, args string) (*kernel.ToolResult, error) {
	var a struct {
		File      string `json:"file"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
	}
	json.Unmarshal([]byte(args), &a)
	c := clientForFile(a.File)
	if c == nil {
		return &kernel.ToolResult{Error: "no LSP server for this file type"}, nil
	}
	locs, err := c.References(a.File, a.Line, a.Character)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d references:\n", len(locs)))
	for _, loc := range locs {
		sb.WriteString(fmt.Sprintf("  %s:%d\n", filepath.Base(loc.URI), loc.Range.Start.Line))
	}
	return &kernel.ToolResult{Content: sb.String()}, nil
}

func handleLSPHover(ctx context.Context, args string) (*kernel.ToolResult, error) {
	var a struct {
		File      string `json:"file"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
	}
	json.Unmarshal([]byte(args), &a)
	c := clientForFile(a.File)
	if c == nil {
		return &kernel.ToolResult{Error: "no LSP server for this file type"}, nil
	}
	hover, err := c.Hover(a.File, a.Line, a.Character)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	return &kernel.ToolResult{Content: hover.Contents.Value}, nil
}

func handleLSPDiagnostics(ctx context.Context, args string) (*kernel.ToolResult, error) {
	var a struct{ File string }
	json.Unmarshal([]byte(args), &a)

	var sb strings.Builder
	sb.WriteString("Diagnostics:\n")
	count := 0
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
				sb.WriteString(fmt.Sprintf("  [%s:%s] %s:%d — %s\n", lang, sev, filepath.Base(a.File), d.Range.Start.Line, d.Message))
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

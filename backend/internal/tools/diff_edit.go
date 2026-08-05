package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"openaide/backend/internal/kernel"
)

func fileEditToolDefs() []kernel.ToolDefinition {
	return []kernel.ToolDefinition{
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "diff_edit",
				Description: "Precise search-and-replace file edit (only modifies the matching part).",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path":         map[string]interface{}{"type": "string", "description": "File path"},
						"search_text":  map[string]interface{}{"type": "string", "description": "Text to search for (must be unique in the file)"},
						"replace_text": map[string]interface{}{"type": "string", "description": "Replacement text"},
					},
					"required": []string{"path", "search_text", "replace_text"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "apply_patch",
				Description: "Batch-edit a file using SEARCH/REPLACE blocks. Apply multiple edits in one call.\nFormat: <<<<<<< SEARCH\nold code\n=======\nnew code\n>>>>>>> REPLACE",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path":    map[string]interface{}{"type": "string", "description": "File path"},
						"content": map[string]interface{}{"type": "string", "description": "SEARCH/REPLACE blocks, separated by blank lines"},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "diff_edit_lines",
				Description: "Edit a file by line-number range (replace the specified line interval). Good for structured replacements.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path":       map[string]interface{}{"type": "string", "description": "File path"},
						"start_line": map[string]interface{}{"type": "integer", "description": "Starting line number (1-based)"},
						"end_line":   map[string]interface{}{"type": "integer", "description": "Ending line number (inclusive); omit to replace only start_line"},
						"content":    map[string]interface{}{"type": "string", "description": "Replacement content"},
					},
					"required": []string{"path", "start_line", "content"},
				},
			},
		}}
}

// handleDiffEdit 精确搜索替换编辑 — 只修改匹配部分，保持其他内容不变
func handleDiffEdit(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Path        string `json:"path"`
		SearchText  string `json:"search_text"`
		ReplaceText string `json:"replace_text"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Path == "" || args.SearchText == "" {
		return &kernel.ToolResult{Error: "path and search_text are required"}, nil
	}

	absPath, err := validateAndResolve(args.Path)
	if err != nil {
		return toolErrInvalidPath(err), nil
	}

	// 文件锁:防止并发写丢更新(read-modify-write TOCTOU)
	unlock := lockFile(absPath)
	defer unlock()

	// Undo 检查点:写之前备份当前内容
	saveFileCheckpoint(absPath, "diff_edit")

	data, err := os.ReadFile(absPath)
	if err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("read failed: %v", err)}, nil
	}

	content := string(data)
	count := strings.Count(content, args.SearchText)

	switch {
	case count == 0:
		return &kernel.ToolResult{Error: fmt.Sprintf("search_text not found in %s. Hint: read the file first to verify exact content/whitespace.", absPath)}, nil
	case count > 1:
		return &kernel.ToolResult{Error: fmt.Sprintf("search_text found %d times in %s — include 2-3 lines of surrounding context for uniqueness.", count, absPath)}, nil
	}

	newContent := strings.Replace(content, args.SearchText, args.ReplaceText, 1)
	if err := atomicWriteFile(absPath, []byte(newContent), 0644); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	// ACI verification: show before/after context with line numbers
	lineNum := findLineNumber(content, args.SearchText)
	var result strings.Builder
	result.WriteString(fmt.Sprintf("✓ Modified %s (line %d)\n", absPath, lineNum))
	result.WriteString("--- before\n")
	if len(args.SearchText) > 500 {
		result.WriteString(args.SearchText[:500] + "\n... (truncated)\n")
	} else {
		result.WriteString(args.SearchText + "\n")
	}
	result.WriteString("+++ after\n")
	if len(args.ReplaceText) > 500 {
		result.WriteString(args.ReplaceText[:500] + "\n... (truncated)\n")
	} else {
		result.WriteString(args.ReplaceText + "\n")
	}

	// Verify the change was applied correctly
	verifyData, _ := os.ReadFile(absPath)
	if strings.Contains(string(verifyData), args.ReplaceText) {
		result.WriteString("✓ Verified: replacement applied correctly")
	} else {
		result.WriteString("✗ Warning: could not verify replacement — re-read file to confirm")
	}

	return &kernel.ToolResult{Content: result.String()}, nil
}

func findLineNumber(content, search string) int {
	idx := strings.Index(content, search)
	if idx < 0 {
		return 0
	}
	return strings.Count(content[:idx], "\n") + 1
}

// handleDiffEditLines 按行号精确替换
func handleDiffEditLines(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Path == "" {
		return &kernel.ToolResult{Error: "path is required"}, nil
	}

	absPath, err := validateAndResolve(args.Path)
	if err != nil {
		return toolErrInvalidPath(err), nil
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	lines := strings.Split(string(data), "\n")
	if args.StartLine < 1 || args.StartLine > len(lines) {
		return &kernel.ToolResult{Error: fmt.Sprintf("start_line %d out of range (1-%d)", args.StartLine, len(lines))}, nil
	}
	if args.EndLine == 0 {
		args.EndLine = args.StartLine
	}
	if args.EndLine > len(lines) {
		args.EndLine = len(lines)
	}

	replacement := strings.Split(args.Content, "\n")
	newLines := make([]string, 0, len(lines)-int(args.EndLine-args.StartLine+1)+len(replacement))
	newLines = append(newLines, lines[:args.StartLine-1]...)
	newLines = append(newLines, replacement...)
	newLines = append(newLines, lines[args.EndLine:]...)

	saveFileCheckpoint(absPath, "diff_edit_lines")
	if err := os.WriteFile(absPath, []byte(strings.Join(newLines, "\n")), 0644); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	return &kernel.ToolResult{
		Content: fmt.Sprintf("✓ %s: replaced lines %d-%d with %d lines", absPath, args.StartLine, args.EndLine, len(replacement)),
	}, nil
}

func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ── SEARCH/REPLACE Block Parser ─────────────────────────────
// Auto-converts Aider-style SEARCH/REPLACE blocks into diff_edit calls.
// This allows the LLM to use its most natural edit format.

// searchReplaceBlock represents one SEARCH/REPLACE pair
type searchReplaceBlock struct {
	Search  string
	Replace string
}

// parseSearchReplaceBlocks extracts SEARCH/REPLACE blocks from text
func parseSearchReplaceBlocks(content string) []searchReplaceBlock {
	var blocks []searchReplaceBlock
	re := regexp.MustCompile(`<<<<<<< SEARCH\n([\s\S]*?)=======\n([\s\S]*?)>>>>>>> REPLACE`)
	matches := re.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		if len(m) >= 3 {
			blocks = append(blocks, searchReplaceBlock{
				Search:  strings.TrimRight(m[1], "\n"),
				Replace: strings.TrimRight(m[2], "\n"),
			})
		}
	}
	return blocks
}

// applySearchReplacePatch parses SEARCH/REPLACE blocks and applies them to target files.
// Each block must include a file path marker or the path must be provided as context.
func applySearchReplacePatch(absPath, content string) (string, error) {
	blocks := parseSearchReplaceBlocks(content)
	if len(blocks) == 0 {
		return "", fmt.Errorf("no SEARCH/REPLACE blocks found. Format must be:\n<<<<<<< SEARCH\nold code\n=======\nnew code\n>>>>>>> REPLACE")
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	current := string(data)

	applied := 0
	var failed []string
	for i, block := range blocks {
		if !strings.Contains(current, block.Search) {
			// Provide context to help LLM fix the mismatch
			preview := block.Search
			if len(preview) > 60 {
				preview = preview[:57] + "..."
			}
			failed = append(failed, fmt.Sprintf("block %d: '%s' not found in file", i+1, preview))
			continue
		}
		current = strings.Replace(current, block.Search, block.Replace, 1)
		applied++
	}

	if applied == 0 {
		return "", fmt.Errorf("SEARCH/REPLACE failed — %d blocks, none matched.\n%s\n\nHint: read the file first to ensure exact whitespace/indentation matches.",
			len(blocks), strings.Join(failed, "\n"))
	}

	saveFileCheckpoint(absPath, "apply_patch")
	if err := os.WriteFile(absPath, []byte(current), 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	result := fmt.Sprintf("✓ %s: %d SEARCH/REPLACE blocks applied", absPath, applied)
	if len(failed) > 0 {
		result += fmt.Sprintf("\n⚠ %d blocks skipped (not found in file)", len(failed))
	}
	return result, nil
}

// handleApplyPatch parses SEARCH/REPLACE blocks and applies them to a file.
// The LLM outputs SEARCH/REPLACE blocks in its response content;
// this handler auto-detects them and applies them as edits.
func handleApplyPatch(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	json.Unmarshal([]byte(arguments), &args)
	if args.Path == "" || args.Content == "" {
		return &kernel.ToolResult{Error: "path and content are required"}, nil
	}

	absPath, err := validateAndResolve(args.Path)
	if err != nil {
		return toolErrInvalidPath(err), nil
	}

	result, err := applySearchReplacePatch(absPath, args.Content)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	return &kernel.ToolResult{Content: result}, nil
}

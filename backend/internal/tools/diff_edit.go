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
	}
}

// handleDiffEdit 精确搜索替换编辑 — 只修改匹配部分，保持其他内容不变
func handleDiffEdit(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Path       string `json:"path"`
		SearchText string `json:"search_text"`
		ReplaceText string `json:"replace_text"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Path == "" || args.SearchText == "" {
		return &kernel.ToolResult{Error: "path and search_text are required"}, nil
	}

	absPath, err := safeAbsPath(args.Path)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("read failed: %v", err)}, nil
	}

	content := string(data)
	count := strings.Count(content, args.SearchText)

	switch {
	case count == 0:
		return &kernel.ToolResult{Error: fmt.Sprintf("search_text not found in %s", absPath)}, nil
	case count > 1:
		return &kernel.ToolResult{Error: fmt.Sprintf("search_text found %d times — must be unique. Use more context.", count)}, nil
	}

	newContent := strings.Replace(content, args.SearchText, args.ReplaceText, 1)
	if err := os.WriteFile(absPath, []byte(newContent), 0644); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	// 生成简短diff
	oldLines := strings.Split(args.SearchText, "\n")
	newLines := strings.Split(args.ReplaceText, "\n")
	added := len(newLines) - len(oldLines)

	return &kernel.ToolResult{
		Content: fmt.Sprintf("✓ %s: %d line(s) modified (%+d lines)\n  search: %q\n  replace: %q",
			absPath, len(oldLines), added, truncStr(args.SearchText, 60), truncStr(args.ReplaceText, 60)),
	}, nil
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

	absPath, err := safeAbsPath(args.Path)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
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
		return "", fmt.Errorf("no SEARCH/REPLACE blocks found in content")
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	current := string(data)

	applied := 0
	for _, block := range blocks {
		if !strings.Contains(current, block.Search) {
			continue // skip non-matching blocks
		}
		// Only replace the first occurrence
		current = strings.Replace(current, block.Search, block.Replace, 1)
		applied++
	}

	if applied == 0 {
		return "", fmt.Errorf("no SEARCH blocks matched in file: %s", absPath)
	}

	if err := os.WriteFile(absPath, []byte(current), 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return fmt.Sprintf("✓ %s: %d SEARCH/REPLACE blocks applied", absPath, applied), nil
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

	absPath, err := safeAbsPath(args.Path)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	result, err := applySearchReplacePatch(absPath, args.Content)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	return &kernel.ToolResult{Content: result}, nil
}

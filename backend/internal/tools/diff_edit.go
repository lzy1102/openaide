package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"openaide/backend/internal/kernel"
)

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

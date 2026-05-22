package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"openaide/backend/internal/index"
	"openaide/backend/internal/kernel"
)

func symbolToolDefs() []kernel.ToolDefinition {
	return []kernel.ToolDefinition{
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "search_symbols",
				Description: "搜索代码符号（函数、类型、方法等）",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "符号名称或部分名称",
						},
						"path": map[string]interface{}{
							"type":        "string",
							"description": "搜索路径（默认当前目录）",
						},
					},
					"required": []string{"query"},
				},
			},
		},
	}
}

// handleSearchSymbols 搜索代码符号
func handleSearchSymbols(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Query string `json:"query"`
		Path  string `json:"path,omitempty"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Query == "" {
		return &kernel.ToolResult{Error: "query is required"}, nil
	}
	if args.Path == "" {
		args.Path = "."
	}

	absPath, err := safeAbsPath(args.Path)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	// 使用代码索引器
	idx, err := index.NewIndexer(absPath + "/.index")
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	symbols := idx.SearchSymbols(args.Query)

	var out strings.Builder
	out.WriteString(fmt.Sprintf("// Symbols matching '%s' in %s/ (%d results)\n", args.Query, absPath, len(symbols)))
	for _, s := range symbols {
		out.WriteString(fmt.Sprintf("%s:%d  [%s] %s\n", s.File, s.Line, s.Type, s.Name))
		if s.Signature != "" {
			out.WriteString(fmt.Sprintf("  sig: %s\n", s.Signature))
		}
	}
	return &kernel.ToolResult{Content: out.String()}, nil
}

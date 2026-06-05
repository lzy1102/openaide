package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"openaide/backend/internal/kernel"
)

func knowledgeToolDefs() []kernel.ToolDefinition {
	return []kernel.ToolDefinition{
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "search_knowledge",
				Description: "搜索知识库中的文档",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "搜索查询",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "add_knowledge",
				Description: "将有用的知识存入知识库，供未来使用",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"title": map[string]interface{}{
							"type":        "string",
							"description": "知识标题",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "知识内容",
						},
						"tags": map[string]interface{}{
							"type":        "string",
							"description": "标签，逗号分隔",
						},
					},
					"required": []string{"title", "content"},
				},
			},
		},
	}
}

// KnowledgeAccessor and WithKnowledge moved to kernel package

func handleSearchKnowledge(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Query == "" {
		return &kernel.ToolResult{Error: "query is required"}, nil
	}

	// 通过 context 获取知识库引用
	kb, ok := kernel.GetKnowledge(ctx)
	if !ok || kb == nil {
		return &kernel.ToolResult{Error: "knowledge base not available"}, nil
	}

	items, err := kb.SearchKnowledge(ctx, args.Query, 5)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	var out strings.Builder
	out.WriteString(fmt.Sprintf("// Knowledge search: '%s' (%d results)\n", args.Query, len(items)))
	for _, item := range items {
		tags := strings.Join(item.Tags, ", ")
		out.WriteString(fmt.Sprintf("## %s [%s]\n%s\n\n", item.Title, tags, item.Content))
	}
	return &kernel.ToolResult{Content: out.String()}, nil
}

// handleAddKnowledge 添加知识到知识库
func handleAddKnowledge(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Title   string `json:"title"`
		Content string `json:"content"`
		Tags    string `json:"tags,omitempty"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Title == "" || args.Content == "" {
		return &kernel.ToolResult{Error: "title and content are required"}, nil
	}

	kb, ok := kernel.GetKnowledge(ctx)
	if !ok || kb == nil {
		return &kernel.ToolResult{Error: "knowledge base not available"}, nil
	}

	var tags []string
	if args.Tags != "" {
		for _, t := range strings.Split(args.Tags, ",") {
			tags = append(tags, strings.TrimSpace(t))
		}
	}
	tags = append(tags, "agent-generated")

	id, err := kb.AddKnowledge(ctx, args.Title, args.Content, "agent", tags)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	return &kernel.ToolResult{Content: fmt.Sprintf("Knowledge stored: %s", id)}, nil
}

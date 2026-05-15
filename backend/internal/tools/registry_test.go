package tools

import (
	"context"
	"testing"

	"openaide/backend/internal/kernel"
)

func TestRegistry_Register(t *testing.T) {
	reg := NewRegistry()

	tool := kernel.ToolDefinition{
		Type: "function",
		Function: kernel.FunctionDef{
			Name:        "test_tool",
			Description: "测试工具",
			Parameters:  map[string]interface{}{},
		},
	}

	handler := func(ctx context.Context, args string) (*kernel.ToolResult, error) {
		return &kernel.ToolResult{Content: "ok"}, nil
	}

	if err := reg.Register(tool, handler); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if !reg.HasTool("test_tool") {
		t.Error("Tool not found after registration")
	}

	if reg.Count() != 1 {
		t.Errorf("Expected 1 tool, got %d", reg.Count())
	}
}

func TestRegistry_Execute(t *testing.T) {
	reg := NewRegistry()

	tool := kernel.ToolDefinition{
		Type: "function",
		Function: kernel.FunctionDef{
			Name: "echo",
		},
	}

	handler := func(ctx context.Context, args string) (*kernel.ToolResult, error) {
		return &kernel.ToolResult{Content: "echo: " + args}, nil
	}

	reg.Register(tool, handler)

	result, err := reg.Execute(context.Background(), kernel.ToolCall{
		Function: kernel.FunctionCall{Name: "echo", Arguments: "hello"},
	}, "session1")

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.Content != "echo: hello" {
		t.Errorf("Expected 'echo: hello', got '%s'", result.Content)
	}
}

func TestRegistry_GetDefinitions(t *testing.T) {
	reg := NewRegistry()

	reg.Register(kernel.ToolDefinition{
		Function: kernel.FunctionDef{Name: "tool1"},
	}, nil)
	reg.Register(kernel.ToolDefinition{
		Function: kernel.FunctionDef{Name: "tool2"},
	}, nil)

	defs := reg.GetDefinitions()
	if len(defs) != 2 {
		t.Errorf("Expected 2 definitions, got %d", len(defs))
	}

	filtered := reg.GetDefinitionsByNames([]string{"tool1"})
	if len(filtered) != 1 {
		t.Errorf("Expected 1 filtered definition, got %d", len(filtered))
	}
}

func TestBuiltinTools(t *testing.T) {
	tools := BuiltinTools()
	if len(tools) == 0 {
		t.Error("No builtin tools defined")
	}

	// 检查关键工具
	hasReadFile := false
	for _, tool := range tools {
		if tool.Function.Name == "read_file" {
			hasReadFile = true
			break
		}
	}
	if !hasReadFile {
		t.Error("Missing read_file tool")
	}
}

func TestRegisterBuiltins(t *testing.T) {
	reg := NewRegistry()
	if err := RegisterBuiltins(reg); err != nil {
		t.Fatalf("RegisterBuiltins failed: %v", err)
	}

	if reg.Count() == 0 {
		t.Error("No builtin tools registered")
	}
}

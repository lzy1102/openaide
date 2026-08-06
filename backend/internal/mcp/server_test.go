package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"openaide/backend/internal/kernel"
)

// mockExecutor implements kernel.ToolExecutor for protocol tests.
type mockExecutor struct {
	defs []kernel.ToolDefinition
	// capture the last executed call for assertion
	lastCall kernel.ToolCall
	result   *kernel.ToolResult
	err      error
}

func newMockExecutor(defs []kernel.ToolDefinition) *mockExecutor {
	return &mockExecutor{defs: defs}
}

func (m *mockExecutor) GetDefinitions() []kernel.ToolDefinition { return m.defs }
func (m *mockExecutor) GetDefinitionsByNames(names []string) []kernel.ToolDefinition {
	var out []kernel.ToolDefinition
	for _, n := range names {
		for _, d := range m.defs {
			if d.Function.Name == n {
				out = append(out, d)
			}
		}
	}
	return out
}
func (m *mockExecutor) Execute(ctx context.Context, call kernel.ToolCall, sessionID string) (*kernel.ToolResult, error) {
	m.lastCall = call
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}
func (m *mockExecutor) Register(tool kernel.ToolDefinition, handler kernel.ToolHandler) error {
	return nil
}

// runServer performs one request/response round-trip and returns the response.
func runServer(t *testing.T, ex kernel.ToolExecutor, request string) map[string]interface{} {
	t.Helper()
	var in, out bytes.Buffer
	in.WriteString(request)
	s := NewServerWithIO(ex, &in, &out)
	if err := s.Serve(); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	scanner := bufio.NewScanner(&out)
	scanner.Scan()
	line := scanner.Bytes()
	if len(bytes.TrimSpace(line)) == 0 {
		// no response (e.g. notifications) — return empty
		return nil
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(line, &resp); err != nil {
		t.Fatalf("unmarshal response %q: %v", line, err)
	}
	return resp
}

func TestServerInitialize(t *testing.T) {
	ex := newMockExecutor(nil)
	resp := runServer(t, ex, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)

	if resp["id"] != float64(1) {
		t.Errorf("id = %v, want 1", resp["id"])
	}
	result := resp["result"].(map[string]interface{})
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("protocolVersion = %v, want 2024-11-05", result["protocolVersion"])
	}
	info := result["serverInfo"].(map[string]interface{})
	if info["name"] != "openaide" {
		t.Errorf("serverInfo.name = %v, want openaide", info["name"])
	}
}

func TestServerPing(t *testing.T) {
	ex := newMockExecutor(nil)
	resp := runServer(t, ex, `{"jsonrpc":"2.0","id":7,"method":"ping"}`)
	if _, ok := resp["result"]; !ok {
		t.Errorf("ping should return a result, got %v", resp)
	}
	if resp["id"] != float64(7) {
		t.Errorf("id = %v, want 7", resp["id"])
	}
}

func TestServerListTools(t *testing.T) {
	ex := newMockExecutor([]kernel.ToolDefinition{
		{Type: "function", Function: kernel.FunctionDef{Name: "read_file", Description: "Read a file", Parameters: map[string]interface{}{"type": "object"}}},
		{Type: "function", Function: kernel.FunctionDef{Name: "write_file", Description: "Write a file", Parameters: map[string]interface{}{"type": "object"}}},
	})
	resp := runServer(t, ex, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)

	tools := resp["result"].(map[string]interface{})["tools"].([]interface{})
	if len(tools) != 2 {
		t.Fatalf("got %d tools, want 2", len(tools))
	}
	first := tools[0].(map[string]interface{})
	if first["name"] != "read_file" {
		t.Errorf("first tool name = %v, want read_file", first["name"])
	}
}

func TestServerCallTool(t *testing.T) {
	ex := newMockExecutor(nil)
	ex.result = &kernel.ToolResult{Content: "file contents"}
	resp := runServer(t, ex, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"/tmp/x.go"}}}`)

	if ex.lastCall.Function.Name != "read_file" {
		t.Errorf("executed tool = %q, want read_file", ex.lastCall.Function.Name)
	}
	if !strings.Contains(ex.lastCall.Function.Arguments, "/tmp/x.go") {
		t.Errorf("arguments = %q, want to contain /tmp/x.go", ex.lastCall.Function.Arguments)
	}

	result := resp["result"].(map[string]interface{})
	if result["isError"] == true {
		t.Error("isError should be false on success")
	}
	content := result["content"].([]interface{})[0].(map[string]interface{})
	if !strings.Contains(content["text"].(string), "file contents") {
		t.Errorf("content text = %v, want 'file contents'", content["text"])
	}
}

func TestServerCallToolErrorResult(t *testing.T) {
	// ToolResult with Error field must be surfaced as isError=true.
	ex := newMockExecutor(nil)
	ex.result = &kernel.ToolResult{Error: "boom"}
	resp := runServer(t, ex, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"read_file"}}`)

	result := resp["result"].(map[string]interface{})
	if result["isError"] != true {
		t.Error("isError should be true when ToolResult.Error is set")
	}
}

func TestServerCallToolExecuteError(t *testing.T) {
	// Executor returning a Go error must also surface as isError.
	ex := newMockExecutor(nil)
	ex.err = context.DeadlineExceeded
	resp := runServer(t, ex, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"read_file"}}`)

	result := resp["result"].(map[string]interface{})
	if result["isError"] != true {
		t.Error("isError should be true when executor errors")
	}
}

func TestServerUnknownMethod(t *testing.T) {
	ex := newMockExecutor(nil)
	resp := runServer(t, ex, `{"jsonrpc":"2.0","id":9,"method":"resources/list"}`)

	errObj := resp["error"].(map[string]interface{})
	if errObj["code"] != float64(-32601) {
		t.Errorf("error code = %v, want -32601", errObj["code"])
	}
}

func TestServerParseError(t *testing.T) {
	ex := newMockExecutor(nil)
	resp := runServer(t, ex, `{not valid json`)

	errObj := resp["error"].(map[string]interface{})
	if errObj["code"] != float64(-32700) {
		t.Errorf("error code = %v, want -32700", errObj["code"])
	}
}

func TestServerNotificationNoResponse(t *testing.T) {
	// notifications/initialized is a notification: no response expected.
	ex := newMockExecutor(nil)
	var in, out bytes.Buffer
	in.WriteString(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	s := NewServerWithIO(ex, &in, &out)
	if err := s.Serve(); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("notification should produce no output, got %q", out.String())
	}
}

func TestServerShutdown(t *testing.T) {
	ex := newMockExecutor(nil)
	resp := runServer(t, ex, `{"jsonrpc":"2.0","id":6,"method":"shutdown"}`)
	if _, ok := resp["result"]; !ok {
		t.Errorf("shutdown should return a result, got %v", resp)
	}
}

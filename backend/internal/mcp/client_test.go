package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// mockTransport implements Transport with scripted responses.
type mockTransport struct {
	responses map[string]json.RawMessage // keyed by method
	lastCall  string
}

func (t *mockTransport) Call(method string, params interface{}) (json.RawMessage, error) {
	t.lastCall = method
	if r, ok := t.responses[method]; ok {
		return r, nil
	}
	return json.RawMessage(`{}`), nil
}
func (t *mockTransport) Notify(method string, params interface{}) error { return nil }
func (t *mockTransport) Close() error                                   { return nil }

func newTestClient(t *testing.T, responses map[string]json.RawMessage) *Client {
	t.Helper()
	tr := &mockTransport{responses: responses}
	c := &Client{transport: tr}
	if _, err := c.call("initialize", defaultInitParams); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := c.discoverTools(); err != nil {
		t.Fatalf("discoverTools: %v", err)
	}
	return c
}

func TestClientDiscoverTools(t *testing.T) {
	c := newTestClient(t, map[string]json.RawMessage{
		"tools/list": json.RawMessage(`{
			"tools": [
				{"name":"git_status","description":"Status","inputSchema":{"type":"object"}},
				{"name":"web_search","description":"Search","inputSchema":{"type":"object"}}
			]
		}`),
	})

	tools := c.GetTools()
	if len(tools) != 2 {
		t.Fatalf("got %d tools, want 2", len(tools))
	}
	if tools[0].Function.Name != "mcp_git_status" {
		t.Errorf("tool name = %q, want mcp_git_status prefix", tools[0].Function.Name)
	}
	if tools[1].Function.Name != "mcp_web_search" {
		t.Errorf("tool name = %q, want mcp_web_search", tools[1].Function.Name)
	}
}

func TestClientCallToolText(t *testing.T) {
	c := newTestClient(t, map[string]json.RawMessage{
		"tools/list": json.RawMessage(`{"tools":[{"name":"run","description":"x","inputSchema":{}}]}`),
		"tools/call": json.RawMessage(`{"content":[{"type":"text","text":"hello from server"}],"isError":false}`),
	})

	result, err := c.CallTool("mcp_run", map[string]interface{}{"arg": 1})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.Content != "hello from server" {
		t.Errorf("content = %v, want 'hello from server'", result.Content)
	}
}

func TestClientCallToolIsError(t *testing.T) {
	c := newTestClient(t, map[string]json.RawMessage{
		"tools/list": json.RawMessage(`{"tools":[{"name":"run","description":"x","inputSchema":{}}]}`),
		"tools/call": json.RawMessage(`{"content":[{"type":"text","text":"tool exploded"}],"isError":true}`),
	})

	result, err := c.CallTool("mcp_run", nil)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.Error != "tool exploded" {
		t.Errorf("error = %q, want 'tool exploded'", result.Error)
	}
}

func TestClientCallToolImageAndResource(t *testing.T) {
	c := newTestClient(t, map[string]json.RawMessage{
		"tools/list": json.RawMessage(`{"tools":[{"name":"shot","description":"x","inputSchema":{}}]}`),
		"tools/call": json.RawMessage(`{
			"content":[
				{"type":"image","data":"aGVsbG8=","mimeType":"image/png"},
				{"type":"resource","uri":"file:///tmp/x.txt"},
				{"type":"text","text":"done"}
			],
			"isError":false
		}`),
	})

	result, err := c.CallTool("mcp_shot", nil)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	content := result.Content.(string)
	for _, want := range []string{"[image: image/png, 8 bytes]", "[resource: file:///tmp/x.txt]", "done"} {
		if !strings.Contains(content, want) {
			t.Errorf("content = %q, want to contain %q", content, want)
		}
	}
}

func TestClientTransportErrorPropagates(t *testing.T) {
	// discoverTools failure surfaces as client construction error.
	tr := &mockTransport{responses: map[string]json.RawMessage{}}
	c := &Client{transport: tr}
	if _, err := c.call("initialize", defaultInitParams); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := c.discoverTools(); err != nil {
		t.Fatalf("discoverTools with empty result should not error: %v", err)
	}
}

func TestClientGetToolsNil(t *testing.T) {
	c := &Client{}
	if c.GetTools() != nil {
		t.Error("GetTools on fresh client should be nil")
	}
	if _, err := c.CallTool("x", nil); err == nil {
		t.Error("CallTool on client with nil transport should error")
	}
}

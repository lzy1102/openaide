package infra

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"openaide/backend/core"
	"openaide/backend/internal/mcp"
	"openaide/backend/internal/tools"
)

// fakeMCPHTTPServer implements just enough of MCP Streamable HTTP
// (POST /message, JSON-RPC) to test the register -> execute chain.
func fakeMCPHTTPServer(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			} `json:"params"`
		}
		json.Unmarshal(body, &req)

		switch req.Method {
		case "initialize":
			io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"fake","version":"1"}}}`)
		case "notifications/initialized":
			// no response
		case "tools/list":
			io.WriteString(w, `{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"remote_ping","description":"Ping from remote MCP","inputSchema":{"type":"object","properties":{"msg":{"type":"string"}}}}]}}`)
		case "tools/call":
			msg := ""
			if m, ok := req.Params.Arguments["msg"].(string); ok {
				msg = m
			}
			resp, _ := json.Marshal(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]interface{}{
					"content": []map[string]interface{}{{"type": "text", "text": "pong:" + msg}},
					"isError": false,
				},
			})
			io.WriteString(w, string(resp))
		default:
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"jsonrpc":"2.0","id":0,"error":{"code":-32601,"message":"unknown"}}`)
		}
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestRegisterMCPToolsIntegration(t *testing.T) {
	ts := fakeMCPHTTPServer(t)

	manager := mcp.NewManager()
	if err := manager.ConnectServer("remote", ts.URL, nil, nil); err != nil {
		t.Fatalf("ConnectServer: %v", err)
	}
	defer manager.Shutdown()

	registry := tools.NewRegistry()
	registerMCPTools(registry, manager, "remote", "test")

	if !registry.HasTool("mcp_remote_ping") {
		t.Fatal("mcp_remote_ping should be registered in the tool registry")
	}

	// Execute through the registry: the handler must forward to the MCP server.
	result, err := registry.Execute(context.Background(), kernel.ToolCall{
		ID:   "1",
		Type: "function",
		Function: kernel.FunctionCall{
			Name:      "mcp_remote_ping",
			Arguments: `{"msg":"hi"}`,
		},
	}, "test")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %v", result.Error)
	}
	if !strings.Contains(result.Content.(string), "pong:hi") {
		t.Errorf("content = %q, want 'pong:hi'", result.Content)
	}
}

func TestRegisterMCPToolsDuplicateSkipped(t *testing.T) {
	ts := fakeMCPHTTPServer(t)
	manager := mcp.NewManager()
	if err := manager.ConnectServer("remote", ts.URL, nil, nil); err != nil {
		t.Fatalf("ConnectServer: %v", err)
	}
	defer manager.Shutdown()

	registry := tools.NewRegistry()
	registerMCPTools(registry, manager, "remote", "test")
	// Registering twice must not panic or error — duplicate is skipped with a warning.
	registerMCPTools(registry, manager, "remote", "test")

	if !registry.HasTool("mcp_remote_ping") {
		t.Fatal("mcp_remote_ping should still be registered")
	}
}

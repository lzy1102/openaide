package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"openaide/backend/internal/kernel"
)

// Transport abstracts the underlying MCP communication channel.
type Transport interface {
	Call(method string, params interface{}) (json.RawMessage, error)
	Notify(method string, params interface{}) error
	Close() error
}

// Client MCP 客户端
type Client struct {
	transport Transport
	mu        sync.Mutex
	idSeq     int
	tools     []kernel.ToolDefinition
}

// ============ stdio Transport ============

type stdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	mu     sync.Mutex
	idSeq  int
	done   chan struct{}
}

func newStdioTransport(command string, args []string, env []string) (*stdioTransport, error) {
	cmd := exec.Command(command, args...)
	if env != nil {
		cmd.Env = env
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return &stdioTransport{
		cmd: cmd, stdin: stdin, stdout: bufio.NewScanner(stdout), done: make(chan struct{}),
	}, nil
}

func (t *stdioTransport) Call(method string, params interface{}) (json.RawMessage, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.idSeq++

	req := jsonrpcRequest{JSONRPC: "2.0", ID: t.idSeq, Method: method, Params: params}
	body, _ := json.Marshal(req)

	if _, err := fmt.Fprintf(t.stdin, "%s\n", body); err != nil {
		return nil, fmt.Errorf("mcp write: %w", err)
	}

	type scanResult struct {
		data []byte
		err  error
	}
	ch := make(chan scanResult, 1)
	go func() {
		if !t.stdout.Scan() {
			ch <- scanResult{err: fmt.Errorf("mcp read: %v", t.stdout.Err())}
			return
		}
		ch <- scanResult{data: t.stdout.Bytes()}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		return t.parseResponse(r.data)
	case <-time.After(30 * time.Second):
		t.cmd.Process.Kill()
		return nil, fmt.Errorf("mcp timeout after 30s")
	}
}

func (t *stdioTransport) Notify(method string, params interface{}) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	// JSON-RPC notification: no id field
	req := jsonrpcRequest{JSONRPC: "2.0", Method: method, Params: params}
	body, _ := json.Marshal(req)
	_, err := fmt.Fprintf(t.stdin, "%s\n", body)
	return err
}

func (t *stdioTransport) Close() error {
	if t.stdin != nil {
		t.stdin.Close()
	}
	if t.cmd != nil && t.cmd.Process != nil {
		t.cmd.Process.Kill()
		t.cmd.Wait()
	}
	return nil
}

func (t *stdioTransport) parseResponse(data []byte) (json.RawMessage, error) {
	var resp jsonrpcResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("mcp parse: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("mcp error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

// ============ HTTP Transport (MCP SSE-compatible POST) ============
//
// This is the MCP "Streamable HTTP" transport — JSON-RPC over HTTP POST.
// It works with servers that accept POST to /message and return JSON in the response body.
// Note: this is NOT the legacy SSE transport (GET /sse for streaming events).
// Most MCP servers in the ecosystem (including @anthropic/mcp-server-sse) now use
// this Streamable HTTP variant, not the older SSE+separate-channel model.

type httpTransport struct {
	baseURL string
	client  *http.Client
	mu      sync.Mutex
	idSeq   int
}

func newHTTPTransport(baseURL string) (*httpTransport, error) {
	return &httpTransport{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 60 * time.Second},
	}, nil
}

func (t *httpTransport) Call(method string, params interface{}) (json.RawMessage, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.idSeq++

	req := jsonrpcRequest{JSONRPC: "2.0", ID: t.idSeq, Method: method, Params: params}
	body, _ := json.Marshal(req)

	httpResp, err := t.client.Post(t.baseURL+"/message", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("mcp http: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("mcp http %d: %s", httpResp.StatusCode, string(respBody))
	}

	data, _ := io.ReadAll(httpResp.Body)
	dataStr := strings.TrimSpace(string(data))

	// Strip SSE "data: " prefix if present (some servers wrap in SSE format)
	if strings.HasPrefix(dataStr, "data: ") {
		dataStr = strings.TrimPrefix(dataStr, "data: ")
	}

	var resp jsonrpcResponse
	if err := json.Unmarshal([]byte(dataStr), &resp); err != nil {
		return nil, fmt.Errorf("mcp parse: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("mcp error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

func (t *httpTransport) Notify(method string, params interface{}) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	req := jsonrpcRequest{JSONRPC: "2.0", Method: method, Params: params}
	body, _ := json.Marshal(req)
	resp, err := t.client.Post(t.baseURL+"/message", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (t *httpTransport) Close() error { return nil }

// ============ Client ============

// ConnectStdio connects to a local MCP server via stdio.
func ConnectStdio(command string, args []string, env []string) (*Client, error) {
	t, err := newStdioTransport(command, args, env)
	if err != nil {
		return nil, err
	}
	return newClient(t)
}

// ConnectSSE connects to a remote MCP server via HTTP (MCP Streamable transport).
// Despite the name "SSE" (matching ecosystem convention in .mcp.json),
// this uses HTTP POST — the standard MCP Streamable HTTP transport.
func ConnectSSE(serverURL string) (*Client, error) {
	t, err := newHTTPTransport(serverURL)
	if err != nil {
		return nil, err
	}
	return newClient(t)
}

// Connect is the legacy API — stdio only, kept for backward compatibility.
func Connect(command string, args ...string) (*Client, error) {
	return ConnectStdio(command, args, nil)
}

var defaultInitParams = map[string]interface{}{
	"protocolVersion": "2024-11-05",
	"capabilities": map[string]interface{}{
		"tools": map[string]interface{}{},
	},
	"clientInfo": map[string]string{"name": "openaide", "version": "3.0.0"},
}

func newClient(t Transport) (*Client, error) {
	c := &Client{transport: t}
	if _, err := c.call("initialize", defaultInitParams); err != nil {
		t.Close()
		return nil, fmt.Errorf("mcp init failed: %w", err)
	}
	// MCP lifecycle: client must send initialized notification after initialize
	if err := c.notify("notifications/initialized", nil); err != nil {
		t.Close()
		return nil, fmt.Errorf("mcp initialized notification failed: %w", err)
	}
	if err := c.discoverTools(); err != nil {
		t.Close()
		return nil, fmt.Errorf("mcp discover tools: %w", err)
	}
	return c, nil
}

func (c *Client) call(method string, params interface{}) (json.RawMessage, error) {
	return c.transport.Call(method, params)
}

func (c *Client) notify(method string, params interface{}) error {
	return c.transport.Notify(method, params)
}

// discoverTools fetches tool definitions from the MCP server.
func (c *Client) discoverTools() error {
	result, err := c.call("tools/list", nil)
	if err != nil {
		return err
	}

	var listResp struct {
		Tools []struct {
			Name        string                 `json:"name"`
			Description string                 `json:"description"`
			InputSchema map[string]interface{} `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(result, &listResp); err != nil {
		return fmt.Errorf("parse tools/list: %w", err)
	}

	for _, t := range listResp.Tools {
		c.tools = append(c.tools, kernel.ToolDefinition{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "mcp_" + t.Name,
				Description: fmt.Sprintf("[MCP] %s", t.Description),
				Parameters:  t.InputSchema,
			},
		})
	}
	return nil
}

// GetTools returns discovered tool definitions.
func (c *Client) GetTools() []kernel.ToolDefinition { return c.tools }

// CallTool invokes a tool on the MCP server.
func (c *Client) CallTool(name string, args map[string]interface{}) (*kernel.ToolResult, error) {
	result, err := c.call("tools/call", map[string]interface{}{
		"name":      strings.TrimPrefix(name, "mcp_"),
		"arguments": args,
	})
	if err != nil {
		return nil, err
	}

	var callResp struct {
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Data     string `json:"data"`     // base64 for images
			MIMEType string `json:"mimeType"` // for images/resources
			URI      string `json:"uri"`      // for resource links
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(result, &callResp); err != nil {
		return nil, fmt.Errorf("parse call result: %w", err)
	}

	var parts []string
	for _, item := range callResp.Content {
		switch item.Type {
		case "text":
			if item.Text != "" {
				parts = append(parts, item.Text)
			}
		case "image":
			if item.Data != "" {
				parts = append(parts, fmt.Sprintf("[image: %s, %d bytes]", item.MIMEType, len(item.Data)))
			}
		case "resource":
			if item.Text != "" {
				parts = append(parts, item.Text)
			} else if item.URI != "" {
				parts = append(parts, fmt.Sprintf("[resource: %s]", item.URI))
			}
		default:
			if item.Text != "" {
				parts = append(parts, item.Text)
			}
		}
	}
	content := strings.Join(parts, "\n")
	if callResp.IsError {
		return &kernel.ToolResult{Error: content}, nil
	}
	return &kernel.ToolResult{Content: content}, nil
}

// Close sends shutdown and closes the transport.
func (c *Client) Close() error {
	c.notify("shutdown", nil)
	return c.transport.Close()
}

// ============ JSON-RPC Types ============

type jsonrpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

// Shared types used by both Client and Server.

type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type CallResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ============ Manager ============

// Manager manages multiple MCP server connections.
type Manager struct {
	servers map[string]*Client
	mu      sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{servers: make(map[string]*Client)}
}

// ConnectServer connects to an MCP server. Supports both stdio and SSE.
// For stdio: pass command + args + optional env (key=value format, nil inherits parent).
// For SSE: pass "sse" as command, URL as first arg.
func (m *Manager) ConnectServer(id, command string, args []string, env []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.servers[id]; exists {
		return fmt.Errorf("mcp server %q already connected", id)
	}

	var c *Client
	var err error
	if command == "sse" || strings.HasPrefix(command, "http://") || strings.HasPrefix(command, "https://") {
		url := command
		if len(args) > 0 {
			url = args[0]
		}
		if !strings.HasPrefix(url, "http") {
			url = command
		}
		c, err = ConnectSSE(url)
	} else {
		c, err = ConnectStdio(command, args, env)
	}
	if err != nil {
		return fmt.Errorf("mcp connect %q: %w", id, err)
	}
	m.servers[id] = c
	return nil
}

// GetServerTools returns tool definitions from a connected server.
func (m *Manager) GetServerTools(id string) []kernel.ToolDefinition {
	m.mu.RLock()
	c, ok := m.servers[id]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	return c.GetTools()
}

// CallTool invokes a tool on a specific server.
func (m *Manager) CallTool(serverID, toolName string, args map[string]interface{}) (*kernel.ToolResult, error) {
	m.mu.RLock()
	c, ok := m.servers[serverID]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("mcp server %q not found", serverID)
	}
	return c.CallTool(toolName, args)
}

// EnvMap converts a map to key=value env slice for exec.Cmd.
func EnvMap(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	env := make([]string, 0, len(m))
	for k, v := range m {
		env = append(env, k+"="+v)
	}
	return env
}

// Shutdown disconnects all servers.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, c := range m.servers {
		c.Close()
		delete(m.servers, id)
	}
}

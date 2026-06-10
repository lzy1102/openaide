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
}

func newStdioTransport(command string, args ...string) (*stdioTransport, error) {
	cmd := exec.Command(command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil { return nil, err }
	stdout, err := cmd.StdoutPipe()
	if err != nil { return nil, err }
	if err := cmd.Start(); err != nil { return nil, err }

	return &stdioTransport{
		cmd: cmd, stdin: stdin, stdout: bufio.NewScanner(stdout),
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

	if !t.stdout.Scan() {
		return nil, fmt.Errorf("mcp read: %v", t.stdout.Err())
	}

	return t.parseResponse(t.stdout.Bytes())
}

func (t *stdioTransport) Close() error {
	if t.stdin != nil { t.stdin.Close() }
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

// ============ SSE / HTTP Transport ============

type sseTransport struct {
	baseURL   string
	client    *http.Client
	mu        sync.Mutex
	idSeq     int
	closeCh   chan struct{}
}

func newSSETransport(baseURL string) (*sseTransport, error) {
	t := &sseTransport{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 60 * time.Second},
		closeCh: make(chan struct{}),
	}
	// Validate connectivity with a ping (mcp sends initialized notification)
	return t, nil
}

func (t *sseTransport) Call(method string, params interface{}) (json.RawMessage, error) {
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

	// SSE response: "data: {...}\n\n" or plain JSON
	data, _ := io.ReadAll(httpResp.Body)
	dataStr := strings.TrimSpace(string(data))

	// Strip SSE "data: " prefix if present
	if strings.HasPrefix(dataStr, "data: ") {
		dataStr = strings.TrimPrefix(dataStr, "data: ")
	}

	var resp jsonrpcResponse
	if err := json.Unmarshal([]byte(dataStr), &resp); err != nil {
		return nil, fmt.Errorf("mcp sse parse: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("mcp error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

func (t *sseTransport) Close() error {
	close(t.closeCh)
	return nil
}

// ============ Client ============

// ConnectStdio connects to a local MCP server via stdio.
func ConnectStdio(command string, args ...string) (*Client, error) {
	t, err := newStdioTransport(command, args...)
	if err != nil { return nil, err }
	return newClient(t)
}

// ConnectSSE connects to a remote MCP server via HTTP/SSE.
func ConnectSSE(serverURL string) (*Client, error) {
	t, err := newSSETransport(serverURL)
	if err != nil { return nil, err }
	return newClient(t)
}

// Connect is the legacy API — stdio only, kept for backward compatibility.
func Connect(command string, args ...string) (*Client, error) {
	return ConnectStdio(command, args...)
}

var defaultInitParams = map[string]interface{}{
	"protocolVersion": "2024-11-05",
	"capabilities":    map[string]interface{}{},
	"clientInfo":      map[string]string{"name": "openaide", "version": "3.0.0"},
}

func newClient(t Transport) (*Client, error) {
	c := &Client{transport: t}
	if _, err := c.call("initialize", defaultInitParams); err != nil {
		t.Close()
		return nil, fmt.Errorf("mcp init failed: %w", err)
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

// discoverTools fetches tool definitions from the MCP server.
func (c *Client) discoverTools() error {
	result, err := c.call("tools/list", nil)
	if err != nil { return err }

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
	if err != nil { return nil, err }

	var callResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(result, &callResp); err != nil {
		return nil, fmt.Errorf("parse call result: %w", err)
	}

	var texts []string
	for _, item := range callResp.Content {
		if item.Text != "" {
			texts = append(texts, item.Text)
		}
	}
	content := strings.Join(texts, "\n")
	if callResp.IsError {
		return &kernel.ToolResult{Error: content}, nil
	}
	return &kernel.ToolResult{Content: content}, nil
}

// Close shuts down the transport.
func (c *Client) Close() error { return c.transport.Close() }

// ============ JSON-RPC Types ============

type jsonrpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
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
// For stdio: pass command + args. For SSE: pass "sse" as command, URL as first arg.
func (m *Manager) ConnectServer(id, command string, args ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.servers[id]; exists {
		return fmt.Errorf("mcp server %q already connected", id)
	}

	var c *Client
	var err error
	if command == "sse" || strings.HasPrefix(command, "http://") || strings.HasPrefix(command, "https://") {
		url := command
		if len(args) > 0 { url = args[0] }
		if !strings.HasPrefix(url, "http") { url = command }
		c, err = ConnectSSE(url)
	} else {
		c, err = ConnectStdio(command, args...)
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
	if !ok { return nil }
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

// Shutdown disconnects all servers.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, c := range m.servers {
		c.Close()
		delete(m.servers, id)
	}
}

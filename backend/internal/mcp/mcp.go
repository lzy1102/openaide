package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// MCP协议版本: 2024-11-05
// 支持 stdio 传输的 MCP Server 管理

// ServerInfo MCP Server 信息
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Tool MCP 工具定义
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// CallResult 工具调用结果
type CallResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ContentItem 内容项
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// jsonrpc 消息
type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Client MCP 客户端 — 管理一个MCP Server进程
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	mu     sync.Mutex
	idSeq  int
	tools  []Tool
}

// Connect 连接到MCP Server进程
func Connect(command string, args ...string) (*Client, error) {
	cmd := exec.Command(command, args...)

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

	c := &Client{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewScanner(stdout),
	}

	// 初始化握手
	if _, err := c.call("initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]string{"name": "openaide", "version": "3.0.0"},
	}); err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("mcp init failed: %w", err)
	}

	// 发现工具
	if err := c.discoverTools(); err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("mcp discover failed: %w", err)
	}

	return c, nil
}

// discoverTools 发现MCP Server提供的工具
func (c *Client) discoverTools() error {
	result, err := c.call("tools/list", nil)
	if err != nil {
		return err
	}

	var wrapper struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(result, &wrapper); err != nil {
		return err
	}

	c.tools = wrapper.Tools
	return nil
}

// ListTools 列出工具
func (c *Client) ListTools() []Tool {
	return c.tools
}

// CallTool 调用MCP工具
func (c *Client) CallTool(name string, args map[string]interface{}) (*CallResult, error) {
	result, err := c.call("tools/call", map[string]interface{}{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return nil, err
	}

	var cr CallResult
	if err := json.Unmarshal(result, &cr); err != nil {
		return nil, err
	}
	return &cr, nil
}

// Close 关闭连接
func (c *Client) Close() error {
	c.call("shutdown", nil)
	return c.cmd.Process.Kill()
}

func (c *Client) call(method string, params interface{}) (json.RawMessage, error) {
	c.mu.Lock()

	c.idSeq++
	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      c.idSeq,
		Method:  method,
	}
	if params != nil {
		p, _ := json.Marshal(params)
		req.Params = p
	}

	data, _ := json.Marshal(req)
	data = append(data, '\n')

	if _, err := c.stdin.Write(data); err != nil {
		c.mu.Unlock()
		return nil, err
	}

	// 在 goroutine 中读取响应，加超时保护
	type scanResult struct {
		data []byte
		err  error
	}
	ch := make(chan scanResult, 1)
	go func() {
		if !c.stdout.Scan() {
			ch <- scanResult{err: fmt.Errorf("no response from MCP server")}
			return
		}
		ch <- scanResult{data: c.stdout.Bytes()}
	}()

	var respData []byte
	select {
	case result := <-ch:
		if result.err != nil {
			c.mu.Unlock()
			return nil, result.err
		}
		respData = result.data
	case <-time.After(30 * time.Second):
		c.mu.Unlock()
		c.cmd.Process.Kill() // 超时杀进程，释放阻塞的 Scan() goroutine
		return nil, fmt.Errorf("MCP server timeout")
	}

	var resp jsonrpcResponse
	if err := json.Unmarshal(respData, &resp); err != nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}

	c.mu.Unlock()

	if resp.Error != nil {
		return nil, fmt.Errorf("MCP error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Result, nil
}

// ============ Manager ============

// Manager MCP Server 管理器
type Manager struct {
	clients map[string]*Client
	mu      sync.RWMutex
}

// NewManager 创建管理器
func NewManager() *Manager {
	return &Manager{clients: make(map[string]*Client)}
}

// ConnectServer 连接MCP Server
func (m *Manager) ConnectServer(id, command string, args ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.clients[id]; exists {
		return fmt.Errorf("server already connected: %s", id)
	}

	client, err := Connect(command, args...)
	if err != nil {
		return err
	}

	m.clients[id] = client
	return nil
}

// GetServerTools 获取指定MCP Server的工具列表
func (m *Manager) GetServerTools(id string) []Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.clients[id]
	if !ok {
		return nil
	}
	return c.ListTools()
}

// GetAllTools 获取所有MCP Server的工具
func (m *Manager) GetAllTools() []Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var tools []Tool
	for _, c := range m.clients {
		tools = append(tools, c.ListTools()...)
	}
	return tools
}

// CallTool 调用指定Server的工具
func (m *Manager) CallTool(serverID, toolName string, args map[string]interface{}) (*CallResult, error) {
	m.mu.RLock()
	c, ok := m.clients[serverID]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("mcp server not found: %s", serverID)
	}

	return c.CallTool(toolName, args)
}

// Shutdown 关闭所有连接
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, c := range m.clients {
		c.Close()
		delete(m.clients, id)
	}
}

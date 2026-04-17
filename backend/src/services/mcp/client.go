package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// MCPClient MCP 客户端，负责与单个 MCP Server 通信
type MCPClient struct {
	ID        string
	Name      string
	Transport string // "stdio" | "sse"
	Command   string
	Args      []string
	URL       string
	Env       map[string]string
	SessionID string
	Tools     []MCPTool
	mu        sync.RWMutex
	connected bool

	// stdio 传输
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	// SSE 传输
	httpClient *http.Client
	sseURL     string

	// 通用
	nextID int
	logger  Logger
}

// Logger 日志接口
type Logger interface {
	Info(ctx context.Context, format string, args ...interface{})
	Error(ctx context.Context, format string, args ...interface{})
	Warn(ctx context.Context, format string, args ...interface{})
}

// MCPTool MCP 工具定义
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema,omitempty"`
}

// MCPToolResult MCP 工具调用结果
type MCPToolResult struct {
	Content []MCPContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// MCPContent MCP 内容块
type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// jsonRPCRequest JSON-RPC 请求
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonRPCResponse JSON-RPC 响应
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

// jsonRPCError JSON-RPC 错误
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// NewMCPClient 创建 MCP 客户端
func NewMCPClient(id, name, transport, command, url string, args []string, env map[string]string, logger Logger) *MCPClient {
	return &MCPClient{
		ID:         id,
		Name:       name,
		Transport:  transport,
		Command:    command,
		Args:       args,
		URL:        url,
		Env:        env,
		Tools:      make([]MCPTool, 0),
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     logger,
	}
}

// Connect 连接到 MCP Server
func (c *MCPClient) Connect(ctx context.Context) error {
	switch c.Transport {
	case "stdio":
		return c.connectStdio(ctx)
	case "sse":
		return c.connectSSE(ctx)
	default:
		return fmt.Errorf("unsupported transport: %s", c.Transport)
	}
}

// Disconnect 断开连接
func (c *MCPClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	var err error

	if c.Transport == "stdio" && c.cmd != nil && c.cmd.Process != nil {
		// 发送关闭通知
		c.sendStdio(&jsonRPCRequest{
			JSONRPC: "2.0",
			ID:      c.nextRequestID(),
			Method:  "notifications/cancelled",
		})
		if c.stdin != nil {
			c.stdin.Close()
		}
		if c.stdout != nil {
			c.stdout.Close()
		}
		if c.cmd.Process != nil {
			err = c.cmd.Process.Kill()
		}
	}

	c.connected = false
	c.cmd = nil
	c.stdin = nil
	c.stdout = nil
	c.stderr = nil
	c.SessionID = ""
	c.Tools = make([]MCPTool, 0)

	return err
}

// Initialize 初始化 MCP 连接（能力协商）
func (c *MCPClient) Initialize(ctx context.Context) error {
	result, err := c.sendRequest(ctx, "initialize", map[string]interface{}{
		"protocolVersion": "2025-03-26",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"clientInfo": map[string]interface{}{
			"name":    "openaide",
			"version": "1.0.0",
		},
	})
	if err != nil {
		return fmt.Errorf("initialize failed: %w", err)
	}

	var initResult struct {
		ProtocolVersion string                 `json:"protocolVersion"`
		Capabilities    map[string]interface{} `json:"capabilities"`
		ServerInfo      map[string]interface{} `json:"serverInfo"`
	}
	if err := json.Unmarshal(result, &initResult); err != nil {
		return fmt.Errorf("failed to parse initialize result: %w", err)
	}

	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()

	c.logger.Info(ctx, "[MCP] %s initialized: %s (protocol %s)",
		c.Name, initResult.ServerInfo["name"], initResult.ProtocolVersion)

	// 发送 initialized 通知
	c.sendNotification("notifications/initialized", nil)

	return nil
}

// ListTools 发现 MCP Server 提供的工具
func (c *MCPClient) ListTools(ctx context.Context) ([]MCPTool, error) {
	result, err := c.sendRequest(ctx, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("tools/list failed: %w", err)
	}

	var listResult struct {
		Tools []MCPTool `json:"tools"`
	}
	if err := json.Unmarshal(result, &listResult); err != nil {
		return nil, fmt.Errorf("failed to parse tools/list result: %w", err)
	}

	c.mu.Lock()
	c.Tools = listResult.Tools
	c.mu.Unlock()

	c.logger.Info(ctx, "[MCP] %s: discovered %d tools", c.Name, len(listResult.Tools))

	return listResult.Tools, nil
}

// CallTool 调用 MCP 工具
func (c *MCPClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (*MCPToolResult, error) {
	params := map[string]interface{}{
		"name":      name,
		"arguments": args,
	}

	result, err := c.sendRequest(ctx, "tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("tools/call failed for %s: %w", name, err)
	}

	var toolResult MCPToolResult
	if err := json.Unmarshal(result, &toolResult); err != nil {
		return nil, fmt.Errorf("failed to parse tools/call result: %w", err)
	}

	return &toolResult, nil
}

// ==================== stdio 传输 ====================

func (c *MCPClient) connectStdio(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, c.Command, c.Args...)
	if len(c.Env) > 0 {
		for k, v := range c.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start MCP server %s: %w", c.Command, err)
	}

	c.cmd = cmd
	c.stdin = stdinPipe
	c.stdout = stdoutPipe
	c.stderr = stderrPipe

	// 读取 stderr 防止阻塞
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := c.stderr.Read(buf)
			if n > 0 {
				c.logger.Warn(ctx, "[MCP] %s stderr: %s", c.Name, string(buf[:n]))
			}
			if err != nil {
				return
			}
		}
	}()

	c.logger.Info(ctx, "[MCP] %s started (pid=%d)", c.Name, cmd.Process.Pid)
	return nil
}

func (c *MCPClient) sendStdio(req *jsonRPCRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = c.stdin.Write(data)
	return err
}

func (c *MCPClient) readStdioResponse(expectedID int) (json.RawMessage, error) {
	decoder := json.NewDecoder(c.stdout)

	// TODO: 考虑添加读取超时

	for {
		var msg jsonRPCResponse
		if err := decoder.Decode(&msg); err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		// 忽略通知
		if msg.ID == 0 {
			continue
		}

		// 匹配请求 ID
		if expectedID == 0 || msg.ID == expectedID {
			if msg.Error != nil {
				return nil, fmt.Errorf("MCP error %d: %s", msg.Error.Code, msg.Error.Message)
			}
			return msg.Result, nil
		}
	}
}

// ==================== SSE 传输 ====================

func (c *MCPClient) connectSSE(ctx context.Context) error {
	if c.URL == "" {
		return fmt.Errorf("SSE transport requires URL")
	}

	c.sseURL = c.URL

	// 发送 initialize 请求
	initParams := map[string]interface{}{
		"protocolVersion": "2025-03-26",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"clientInfo": map[string]interface{}{
			"name":    "openaide",
			"version": "1.0.0",
		},
	}
	result, err := c.sendSSERequest(ctx, "initialize", initParams)
	if err != nil {
		return fmt.Errorf("SSE initialize failed: %w", err)
	}

	// 提取 session ID（如果返回了的话）
	var initResult map[string]interface{}
	json.Unmarshal(result, &initResult)

	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()

	c.logger.Info(ctx, "[MCP] %s connected via SSE to %s", c.Name, c.URL)

	// 发送 initialized 通知
	c.sendSSENotification("notifications/initialized", nil)

	return nil
}

func (c *MCPClient) sendSSERequest(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	req := &jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextRequestID(),
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.sseURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.SessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", c.SessionID)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("SSE request failed: %w", err)
	}
	defer resp.Body.Close()

	// 检查是否为 SSE 响应
	contentType := resp.Header.Get("Content-Type")
	if c.SessionID == "" {
		if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
			c.SessionID = sid
		}
	}

	if strings.Contains(contentType, "text/event-stream") {
		return c.readSSEResponse(resp.Body)
	}

	// 普通 JSON 响应
	var rpcResp jsonRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("failed to decode SSE response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("MCP error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

func (c *MCPClient) readSSEResponse(body io.Reader) (json.RawMessage, error) {
	decoder := json.NewDecoder(body)

	for {
		var event struct {
			Data string `json:"data"`
		}
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				return nil, io.EOF
			}
			continue
		}

		if event.Data == "" {
			continue
		}

		var msg jsonRPCResponse
		if err := json.Unmarshal([]byte(event.Data), &msg); err != nil {
			continue // 跳过非 JSON-RPC 消息
		}

		if msg.Error != nil {
			return nil, fmt.Errorf("MCP error %d: %s", msg.Error.Code, msg.Error.Message)
		}

		if msg.Result != nil {
			return msg.Result, nil
		}
	}
}

func (c *MCPClient) sendSSENotification(method string, params interface{}) {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return
	}

	httpReq, err := http.NewRequest("POST", c.sseURL, bytes.NewReader(data))
	if err != nil {
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.SessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", c.SessionID)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// ==================== 通用方法 ====================

// sendRequest 发送 JSON-RPC 请求并等待响应
func (c *MCPClient) sendRequest(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	c.mu.Lock()
	id := c.nextRequestID()
	c.mu.Unlock()

	req := &jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	switch c.Transport {
	case "stdio":
		if err := c.sendStdio(req); err != nil {
			return nil, err
		}
		return c.readStdioResponse(id)

	case "sse":
		return c.sendSSERequest(ctx, method, params)

	default:
		return nil, fmt.Errorf("unsupported transport: %s", c.Transport)
	}
}

// sendNotification 发送通知（不等待响应）
func (c *MCPClient) sendNotification(method string, params interface{}) {
	switch c.Transport {
	case "stdio":
		c.sendStdio(&jsonRPCRequest{
			JSONRPC: "2.0",
			ID:      0, // 通知 ID 为 0
			Method:  method,
			Params:  params,
		})
	case "sse":
		c.sendSSENotification(method, params)
	}
}

// nextRequestID 递增请求 ID
func (c *MCPClient) nextRequestID() int {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()
	return id
}

// GetTools 获取工具列表（线程安全）
func (c *MCPClient) GetTools() []MCPTool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Tools
}

// IsConnected 检查是否已连接
func (c *MCPClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

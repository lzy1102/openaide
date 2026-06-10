package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"openaide/backend/internal/kernel"
)

// Server MCP Server — 通过 stdio 暴露内置工具
// 实现 MCP 协议（2024-11-05），使外部 MCP 客户端可发现并调用所有内置工具
type Server struct {
	executor kernel.ToolExecutor
	reader   io.Reader
	writer   io.Writer
	mu       sync.Mutex
	idSeq    int
}

// NewServer 创建 MCP Server
func NewServer(executor kernel.ToolExecutor) *Server {
	return &Server{
		executor: executor,
		reader:   os.Stdin,
		writer:   os.Stdout,
	}
}

// NewServerWithIO 创建 MCP Server（指定 IO，便于测试）
func NewServerWithIO(executor kernel.ToolExecutor, reader io.Reader, writer io.Writer) *Server {
	return &Server{
		executor: executor,
		reader:   reader,
		writer:   writer,
	}
}

// Serve 启动 MCP Server，阻塞直到 stdin 关闭
func (s *Server) Serve() error {
	scanner := bufio.NewScanner(s.reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req jsonrpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(0, -32700, "Parse error", err.Error())
			continue
		}

		s.handleRequest(req)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("mcp server read error: %w", err)
	}
	return nil
}

func (s *Server) handleRequest(req jsonrpcRequest) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "ping":
		s.handlePing(req)
	case "tools/list":
		s.handleListTools(req)
	case "tools/call":
		s.handleCallTool(req)
	case "notifications/initialized":
		// no response needed
	case "shutdown":
		s.handleShutdown(req)
	default:
		s.sendError(req.ID, -32601, "Method not found",
			fmt.Sprintf("unknown method: %s", req.Method))
	}
}

// handlePing responds to heartbeat checks.
func (s *Server) handlePing(req jsonrpcRequest) {
	s.sendResult(req.ID, map[string]interface{}{})
}

// handleInitialize 处理 MCP 初始化握手
func (s *Server) handleInitialize(req jsonrpcRequest) {
	s.sendResult(req.ID, map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"serverInfo": map[string]string{
			"name":    "openaide",
			"version": "3.0.0",
		},
	})
}

// handleListTools 返回工具列表（转换为 MCP 格式）
func (s *Server) handleListTools(req jsonrpcRequest) {
	defs := s.executor.GetDefinitions()
	tools := make([]Tool, 0, len(defs))
	for _, d := range defs {
		tools = append(tools, Tool{
			Name:        d.Function.Name,
			Description: d.Function.Description,
			InputSchema: d.Function.Parameters,
		})
	}
	s.sendResult(req.ID, map[string]interface{}{
		"tools": tools,
	})
}

// handleCallTool 执行工具调用
func (s *Server) handleCallTool(req jsonrpcRequest) {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments,omitempty"`
	}
	paramsBytes, _ := json.Marshal(req.Params)
	if err := json.Unmarshal(paramsBytes, &params); err != nil {
		s.sendError(req.ID, -32602, "Invalid params", err.Error())
		return
	}

	// 将 map[string]interface{} 转为 JSON 字符串参数
	argsJSON, _ := json.Marshal(params.Arguments)

	result, err := s.executor.Execute(context.Background(), kernel.ToolCall{
		ID:   fmt.Sprintf("mcp_%d", req.ID),
		Type: "function",
		Function: kernel.FunctionCall{
			Name:      params.Name,
			Arguments: string(argsJSON),
		},
	}, "mcp")

	contentStr := ""
	isError := false
	if err != nil {
		contentStr = fmt.Sprintf("Error: %s", err.Error())
		isError = true
	} else if result != nil {
		contentStr = fmt.Sprintf("%v", result.Content)
		if result.Error != "" {
			contentStr = fmt.Sprintf("Error: %s", result.Error)
			isError = true
		}
	}

	s.sendResult(req.ID, CallResult{
		Content: []ContentItem{
			{Type: "text", Text: contentStr},
		},
		IsError: isError,
	})
}

func (s *Server) handleShutdown(req jsonrpcRequest) {
	s.sendResult(req.ID, map[string]interface{}{})
}

// sendResult 发送成功响应
func (s *Server) sendResult(id int, result interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, _ := json.Marshal(jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  mustMarshalJSON(result),
	})
	data = append(data, '\n')
	s.writer.Write(data)
}

// sendError 发送错误响应
func (s *Server) sendError(id int, code int, message string, data string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	jsonData, _ := json.Marshal(jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &jsonrpcError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	})
	jsonData = append(jsonData, '\n')
	s.writer.Write(jsonData)
}

func mustMarshalJSON(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

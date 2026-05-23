package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"openaide/backend/internal/auth"
	"openaide/backend/internal/channel"
	"openaide/backend/internal/kernel"
	"openaide/backend/internal/orchestration"
)

// Server API 服务器
type Server struct {
	orchestrator    *orchestration.Orchestrator
	authService     *auth.Service
	addr            string
	server          *http.Server
	mux             *http.ServeMux
	channelRegistry *channel.Registry
}

// NewServer 创建 API 服务器
func NewServer(orch *orchestration.Orchestrator, addr string, authSvc *auth.Service) *Server {
	if addr == "" {
		addr = ":8080"
	}

	s := &Server{
		orchestrator: orch,
		authService:  authSvc,
		addr:         addr,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/chat", s.handleChat)
	mux.HandleFunc("/api/v1/chat/stream", s.handleChatStream)
	mux.HandleFunc("/api/v1/sessions", s.handleSessions)
	mux.HandleFunc("/api/v1/sessions/", s.handleSessionDetail)
	mux.HandleFunc("/api/v1/memory/search", s.handleMemorySearch)
	mux.HandleFunc("/api/v1/tools", s.handleTools)
	mux.HandleFunc("/api/v1/stats", s.handleStats)
	mux.HandleFunc("/api/v1/metrics", s.handleMetrics)
	mux.HandleFunc("/api/v1/channels", s.handleChannels)
	mux.HandleFunc("/api/v1/auth/", authSvc.AuthHandler)
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/health", s.handleHealth)
	s.mux = mux

	// 应用中间件链: CORS → Auth
	var handler http.Handler = mux
	handler = NewRateLimiter(20, 200).Middleware(handler)
	handler = s.withCORS(handler)
	if authSvc != nil {
		handler = authSvc.Middleware(handler)
	}

	s.server = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
	}

	return s
}

// Start 启动服务器
func (s *Server) Start() error {
	slog.Info("API server starting", "addr", s.addr)
	return s.server.ListenAndServe()
}

// Stop 停止服务器
func (s *Server) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// ============ HTTP Handlers ============

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	ctx := r.Context()
	resp, err := s.orchestrator.ProcessQuery(ctx, req.UserID, req.ProjectID, req.Message, kernel.QueryOptions{
		ModelID:      req.Model,
		Temperature:  req.Temperature,
		MaxTokens:    req.MaxTokens,
		ToolFilter:   req.Tools,
		EnableStream: false,
	})
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.writeJSON(w, http.StatusOK, ChatResponse{
		Content:    resp.Content,
		ToolCalls:  resp.ToolCalls,
		TokensUsed: resp.TokensUsed,
		Duration:   resp.Duration.Milliseconds(),
		Model:      resp.Model,
	})
	RecordMetrics(resp.TokensUsed, resp.ToolCalls, false)
}

// sendSSE writes an SSE event, handling marshal errors gracefully.
func sendSSE(w http.ResponseWriter, flusher http.Flusher, event StreamEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		slog.Error("SSE marshal failed", "error", err)
		fmt.Fprintf(w, "data: {\"error\":\"internal marshal error\"}\n\n")
	} else {
		fmt.Fprintf(w, "data: %s\n\n", data)
	}
	flusher.Flush()
}

func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	ctx := r.Context()
	stream, err := s.orchestrator.ProcessQueryStream(ctx, req.UserID, req.ProjectID, req.Message, kernel.QueryOptions{
		ModelID:      req.Model,
		Temperature:  req.Temperature,
		MaxTokens:    req.MaxTokens,
		ToolFilter:   req.Tools,
		EnableStream: true,
	})
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	// SSE 流式输出
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	for chunk := range stream {
		event := StreamEvent{
			Type:        string(chunk.Type),
			Content:     chunk.Content,
			ToolCallID:  chunk.ToolCallID,
			ToolName:    chunk.ToolName,
			Round:       chunk.Round,
			TotalRounds: chunk.TotalRounds,
		}

		// 推理内容通过 thinking 类型传递
		if chunk.Type == kernel.ChunkTypeThinking && chunk.ReasoningContent != "" {
			event.Content = chunk.ReasoningContent
		}

		switch chunk.Type {
		case kernel.ChunkTypeError:
			if chunk.Error != nil {
				event.Error = chunk.Error.Error()
			}
			sendSSE(w, flusher, event)
			return

		case kernel.ChunkTypeDone:
			if chunk.Usage != nil {
				event.TokensUsed = chunk.Usage.TotalTokens
			}
			sendSSE(w, flusher, event)
			return

		case kernel.ChunkTypeToolDone:
			if chunk.ToolResult != nil {
				event.ToolResult = chunk.ToolResult.Content
			}
		}

		sendSSE(w, flusher, event)
	}
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// 列出会话
		userID := sanitizeParam(r.URL.Query().Get("user_id"))
		projectID := sanitizeParam(r.URL.Query().Get("project_id"))
		limit := 10
		offset := 0
		if l := r.URL.Query().Get("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
				limit = v
			}
		}
		if o := r.URL.Query().Get("offset"); o != "" {
			if v, err := strconv.Atoi(o); err == nil && v >= 0 {
				offset = v
			}
		}

		sessions, err := s.orchestrator.ListSessions(r.Context(), projectID, userID, limit, offset)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err)
			return
		}

		var result []SessionInfo
		for _, sess := range sessions {
			result = append(result, sessionToInfo(sess))
		}
		s.writeJSON(w, http.StatusOK, result)

	case http.MethodPost:
		// 创建会话
		var req CreateSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, err)
			return
		}
		session, err := s.orchestrator.CreateSession(r.Context(), req.ProjectID, req.UserID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err)
			return
		}
		s.writeJSON(w, http.StatusCreated, sessionToInfo(session))

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Path[len("/api/v1/sessions/"):]
	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	// 权限检查：验证请求者是否为会话所有者
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		userID = r.Header.Get("X-User-ID")
	}

	switch r.Method {
	case http.MethodGet:
		session, err := s.orchestrator.GetSession(r.Context(), sessionID)
		if err != nil {
			s.writeError(w, http.StatusNotFound, err)
			return
		}
		if userID != "" && session.UserID != userID {
			s.writeError(w, http.StatusForbidden, fmt.Errorf("access denied"))
			return
		}

		var messages []MessageInfo
		for _, msg := range session.Messages {
			messages = append(messages, MessageInfo{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"session":  sessionToInfo(session),
			"messages": messages,
		})

	case http.MethodDelete:
		// 删除前验证所有权
		session, err := s.orchestrator.GetSession(r.Context(), sessionID)
		if err == nil && userID != "" && session.UserID != userID {
			s.writeError(w, http.StatusForbidden, fmt.Errorf("access denied"))
			return
		}
		if err := s.orchestrator.DeleteSession(r.Context(), sessionID); err != nil {
			s.writeError(w, http.StatusInternalServerError, err)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMemorySearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Query parameter 'q' required", http.StatusBadRequest)
		return
	}

	messages, score, err := s.orchestrator.SearchMemory(r.Context(), query, 10)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	var result []MessageInfo
	for _, msg := range messages {
		result = append(result, MessageInfo{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	s.writeJSON(w, http.StatusOK, MemorySearchResponse{
		Query:   query,
		Results: result,
		Score:   score,
	})
}

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defs := s.orchestrator.GetToolDefinitions()
	s.writeJSON(w, http.StatusOK, defs)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := s.orchestrator.GetStats()
	s.writeJSON(w, http.StatusOK, stats)
}

// 全局指标计数器
var (
	metricsRequests  atomic.Int64
	metricsTokens    atomic.Int64
	metricsToolCalls atomic.Int64
	metricsErrors    atomic.Int64
)

// RecordMetrics records a completed request for the /metrics endpoint.
func RecordMetrics(tokens, toolCalls int, isError bool) {
	metricsRequests.Add(1)
	metricsTokens.Add(int64(tokens))
	metricsToolCalls.Add(int64(toolCalls))
	if isError {
		metricsErrors.Add(1)
	}
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// 收集运行时指标
	stats := s.orchestrator.GetStats()
	stats["requests_total"] = metricsRequests.Load()
	stats["tokens_total"] = metricsTokens.Load()
	stats["tool_calls_total"] = metricsToolCalls.Load()
	stats["errors_total"] = metricsErrors.Load()
	s.writeJSON(w, http.StatusOK, stats)
}

// sanitizeParam 清理参数，防止路径遍历和注入
func sanitizeParam(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "..", "")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	if len(s) > 100 {
		s = s[:100]
	}
	return s
}

// SetChannelRegistry 设置渠道注册表
func (s *Server) SetChannelRegistry(registry *channel.Registry) {
	s.channelRegistry = registry
}

// RegisterHandler 返回HTTP处理器注册函数
// Channel在Start时通过此回调注册Webhook端点
func (s *Server) RegisterHandler() channel.HTTPHandler {
	return func(pattern string, handler http.HandlerFunc) {
		s.mux.HandleFunc(pattern, handler)
	}
}

func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var result []map[string]string
		if s.channelRegistry != nil {
			for _, ch := range s.channelRegistry.List() {
				result = append(result, map[string]string{
					"id":   ch.ID(),
					"name": ch.Name(),
					"type": string(ch.Type()),
				})
			}
		}
		if result == nil {
			result = []map[string]string{}
		}
		s.writeJSON(w, http.StatusOK, result)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// ============ Middleware ============

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ============ Helpers ============

func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (s *Server) writeError(w http.ResponseWriter, status int, err error) {
	s.writeJSON(w, status, ErrorResponse{Error: err.Error()})
}

// ============ Request/Response Types ============

// ChatRequest 聊天请求
type ChatRequest struct {
	Message     string   `json:"message"`
	UserID      string   `json:"user_id,omitempty"`
	ProjectID   string   `json:"project_id,omitempty"`
	Model       string   `json:"model,omitempty"`
	Temperature float64  `json:"temperature,omitempty"`
	MaxTokens   int      `json:"max_tokens,omitempty"`
	Tools       []string `json:"tools,omitempty"`
}

// ChatResponse 聊天响应
type ChatResponse struct {
	Content    string `json:"content"`
	ToolCalls  int    `json:"tool_calls"`
	TokensUsed int    `json:"tokens_used"`
	Duration   int64  `json:"duration_ms"`
	Model      string `json:"model"`
}

// StreamEvent 流式事件
type StreamEvent struct {
	Type        string      `json:"type"`
	Content     string      `json:"content,omitempty"`
	TokensUsed  int         `json:"tokens_used,omitempty"`
	Error       string      `json:"error,omitempty"`
	ToolCallID  string      `json:"tool_call_id,omitempty"`
	ToolName    string      `json:"tool_name,omitempty"`
	ToolResult  interface{} `json:"tool_result,omitempty"`
	Round       int         `json:"round,omitempty"`
	TotalRounds int         `json:"total_rounds,omitempty"`
}

// SessionInfo 会话信息
type SessionInfo struct {
	ID           string `json:"id"`
	ProjectID    string `json:"project_id"`
	UserID       string `json:"user_id"`
	MessageCount int    `json:"message_count"`
	CreatedAt    string `json:"created_at,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

func sessionToInfo(s *kernel.Session) SessionInfo {
	info := SessionInfo{
		ID:           s.ID,
		ProjectID:    s.ProjectID,
		UserID:       s.UserID,
		MessageCount: len(s.Messages),
	}
	if !s.CreatedAt.IsZero() {
		info.CreatedAt = s.CreatedAt.Format(time.RFC3339)
	}
	if !s.UpdatedAt.IsZero() {
		info.UpdatedAt = s.UpdatedAt.Format(time.RFC3339)
	}
	return info
}

// CreateSessionRequest 创建会话请求
type CreateSessionRequest struct {
	ProjectID string `json:"project_id"`
	UserID    string `json:"user_id"`
}

// MessageInfo 消息信息
type MessageInfo struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// MemorySearchResponse 记忆搜索响应
type MemorySearchResponse struct {
	Query   string        `json:"query"`
	Results []MessageInfo `json:"results"`
	Score   float64       `json:"score"`
}

// ErrorResponse 错误响应
type ErrorResponse struct {
	Error string `json:"error"`
}

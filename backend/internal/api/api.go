package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"openaide/backend/internal/auth"
	"openaide/backend/internal/kernel"
	"openaide/backend/internal/orchestration"
)

// Server API 服务器
type Server struct {
	orchestrator *orchestration.Orchestrator
	authService  *auth.Service
	addr         string
	server       *http.Server
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
	mux.HandleFunc("/api/v1/auth/", authSvc.AuthHandler)
	mux.HandleFunc("/health", s.handleHealth)

	// 应用中间件链: CORS → Auth
	var handler http.Handler = mux
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
		if chunk.Error != nil {
			data, _ := json.Marshal(StreamEvent{Type: "error", Error: chunk.Error.Error()})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			return
		}

		event := StreamEvent{
			Type:    "chunk",
			Content: chunk.Content,
		}
		if chunk.Done {
			event.Type = "done"
			if chunk.Usage != nil {
				event.TokensUsed = chunk.Usage.TotalTokens
			}
		}

		data, _ := json.Marshal(event)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// 列出会话
		userID := r.URL.Query().Get("user_id")
		projectID := r.URL.Query().Get("project_id")

		sessions, err := s.orchestrator.ListSessions(r.Context(), projectID, userID, 10)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err)
			return
		}

		var result []SessionInfo
		for _, sess := range sessions {
			result = append(result, SessionInfo{
				ID:        sess.ID,
				ProjectID: sess.ProjectID,
				UserID:    sess.UserID,
			})
		}
		s.writeJSON(w, http.StatusOK, result)

	case http.MethodPost:
		// 创建会话
		var req CreateSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, err)
			return
		}
		s.writeJSON(w, http.StatusCreated, SessionInfo{
			ID:        "new-session-id",
			ProjectID: req.ProjectID,
			UserID:    req.UserID,
		})

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

	switch r.Method {
	case http.MethodGet:
		// 获取会话历史
		messages, err := s.orchestrator.GetSessionHistory(r.Context(), sessionID, 50)
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
		s.writeJSON(w, http.StatusOK, result)

	case http.MethodDelete:
		// 删除会话
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

	stats := s.orchestrator.GetStats()
	s.writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := s.orchestrator.GetStats()
	s.writeJSON(w, http.StatusOK, stats)
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
	Type       string `json:"type"`
	Content    string `json:"content,omitempty"`
	TokensUsed int    `json:"tokens_used,omitempty"`
	Error      string `json:"error,omitempty"`
}

// SessionInfo 会话信息
type SessionInfo struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	UserID    string `json:"user_id"`
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

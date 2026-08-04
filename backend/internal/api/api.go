package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
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
	kernel          kernel.Kernel
	authService     *auth.Service
	addr            string
	server          *http.Server
	mux             *http.ServeMux
	channelRegistry *channel.Registry
	frontendHandler http.Handler // embedded frontend (optional)
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
	mux.HandleFunc("/metrics", s.handlePrometheus) // Prometheus standard endpoint
	mux.HandleFunc("/api/v1/channels", s.handleChannels)
	if authSvc != nil {
		mux.HandleFunc("/api/v1/auth/", authSvc.AuthHandler)
	}
	mux.HandleFunc("/api/v1/tasks", s.handleTasks)
	mux.HandleFunc("/api/v1/config", s.handleConfig)
	mux.HandleFunc("/api/v1/projects", s.handleProjects)
	mux.HandleFunc("/api/v1/projects/", s.handleProjectDetail)
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/health", s.handleHealth)
	// Frontend catch-all (registered later via SetFrontendHandler)
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

// SetFrontendHandler registers the embedded frontend as the catch-all handler.
// All non-API routes serve the SPA (index.html, JS, CSS, assets).
func (s *Server) SetFrontendHandler(h http.Handler) {
	s.frontendHandler = h
	// Register as catch-all — the default mux pattern "/" matches all unmatched paths
	// We use a wrapper to let API routes take priority
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Let registered API routes handle these prefixes
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws") || strings.HasPrefix(r.URL.Path, "/health") || strings.HasPrefix(r.URL.Path, "/metrics") {
			http.NotFound(w, r)
			return
		}
		s.frontendHandler.ServeHTTP(w, r)
	})
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

type ctxKey string

// withTrace 从请求头提取或生成 trace ID，注入 context
func withTrace(r *http.Request) context.Context {
	ctx := r.Context()
	tid := r.Header.Get("X-Trace-ID")
	if tid == "" {
		b := make([]byte, 6)
		rand.Read(b)
		tid = hex.EncodeToString(b)
	}
	slog.Debug("api request start", "trace_id", tid, "method", r.Method, "path", r.URL.Path)
	return context.WithValue(ctx, ctxKey("trace_id"), tid)
}

// traceID 从 context 提取 trace ID
func traceID(ctx context.Context) string {
	if id, ok := ctx.Value(ctxKey("trace_id")).(string); ok {
		return id
	}
	return "-"
}

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

	ctx := withTrace(r)
	defer slog.Debug("api request end", "trace_id", traceID(ctx))
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

	ctx := withTrace(r)
	defer slog.Debug("api stream end", "trace_id", traceID(ctx))
	stream, err := s.orchestrator.ProcessQueryStream(ctx, "", req.UserID, req.ProjectID, req.Message, kernel.QueryOptions{
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

	done := ctx.Done()
	for {
		select {
		case <-done:
			return // 客户端断开，停止消耗流
		case chunk, ok := <-stream:
			if !ok {
				return
			}
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
	stats := s.orchestrator.GetStats()
	stats["requests_total"] = metricsRequests.Load()
	stats["tokens_total"] = metricsTokens.Load()
	stats["tool_calls_total"] = metricsToolCalls.Load()
	stats["errors_total"] = metricsErrors.Load()
	s.writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.kernel == nil {
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"total_tasks": 0, "recent_count": 0, "tasks": []interface{}{}})
		return
	}
	if r.URL.Query().Get("summary") == "true" {
		s.writeJSON(w, http.StatusOK, s.kernel.TaskMetricsSummary())
		return
	}
	n := 50
	if v := r.URL.Query().Get("n"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 500 {
			n = parsed
		}
	}
	s.writeJSON(w, http.StatusOK, s.kernel.RecentTasks(n))
}

// handlePrometheus returns metrics in Prometheus text format.
// Standard /metrics endpoint for Prometheus/Grafana scraping.
func (s *Server) handlePrometheus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	orchStats := s.orchestrator.GetStats()

	fmt.Fprintf(w, "# HELP openaide_requests_total Total number of API requests.\n")
	fmt.Fprintf(w, "# TYPE openaide_requests_total counter\n")
	fmt.Fprintf(w, "openaide_requests_total %d\n", metricsRequests.Load())

	fmt.Fprintf(w, "# HELP openaide_tokens_total Total tokens consumed (prompt + completion).\n")
	fmt.Fprintf(w, "# TYPE openaide_tokens_total counter\n")
	fmt.Fprintf(w, "openaide_tokens_total %d\n", metricsTokens.Load())

	fmt.Fprintf(w, "# HELP openaide_tool_calls_total Total tool calls executed.\n")
	fmt.Fprintf(w, "# TYPE openaide_tool_calls_total counter\n")
	fmt.Fprintf(w, "openaide_tool_calls_total %d\n", metricsToolCalls.Load())

	fmt.Fprintf(w, "# HELP openaide_errors_total Total request errors.\n")
	fmt.Fprintf(w, "# TYPE openaide_errors_total counter\n")
	fmt.Fprintf(w, "openaide_errors_total %d\n", metricsErrors.Load())

	if v, ok := orchStats["sessions_total"]; ok {
		fmt.Fprintf(w, "# HELP openaide_sessions_total Total number of sessions.\n")
		fmt.Fprintf(w, "# TYPE openaide_sessions_total gauge\n")
		fmt.Fprintf(w, "openaide_sessions_total %v\n", v)
	}
	if v, ok := orchStats["active_channels"]; ok {
		fmt.Fprintf(w, "# HELP openaide_active_channels Number of active channel connections.\n")
		fmt.Fprintf(w, "# TYPE openaide_active_channels gauge\n")
		fmt.Fprintf(w, "openaide_active_channels %v\n", v)
	}
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

// SetKernel sets the kernel reference for task metrics access.
func (s *Server) SetKernel(k kernel.Kernel) {
	s.kernel = k
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
	WorkingDir  string   `json:"working_dir,omitempty"` // 项目工作目录（Server 模式）
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

// handleConfig 读/写配置文件
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	configPath := os.Getenv("HOME") + "/.openaide/config.yaml"
	if r.Method == http.MethodGet {
		data, err := os.ReadFile(configPath)
		if err != nil {
			http.Error(w, `{"error":"config not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/yaml")
		w.Write(data)
		return
	}
	if r.Method == http.MethodPut || r.Method == http.MethodPost {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, `{"error":"read body failed"}`, http.StatusBadRequest)
			return
		}
		if err := os.WriteFile(configPath, data, 0644); err != nil {
			http.Error(w, `{"error":"write config failed"}`, http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return
	}
	http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
}

// handleProjects 项目管理（基于目录路径）
func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	projectsFile := os.Getenv("HOME") + "/.openaide/projects.json"

	type Project struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Path string `json:"path"`
	}

	loadProjects := func() []Project {
		data, _ := os.ReadFile(projectsFile)
		var projects []Project
		json.Unmarshal(data, &projects)
		return projects
	}

	saveProjects := func(projects []Project) {
		data, _ := json.MarshalIndent(projects, "", "  ")
		os.WriteFile(projectsFile, data, 0644)
	}

	if r.Method == http.MethodGet {
		projects := loadProjects()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(projects)
		return
	}

	if r.Method == http.MethodPost {
		var proj Project
		if err := json.NewDecoder(r.Body).Decode(&proj); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		// 验证路径存在
		if info, err := os.Stat(proj.Path); err != nil || !info.IsDir() {
			http.Error(w, `{"error":"directory not found"}`, http.StatusBadRequest)
			return
		}
		proj.ID = fmt.Sprintf("proj-%d", time.Now().UnixMilli())
		if proj.Name == "" {
			proj.Name = filepath.Base(proj.Path)
		}

		projects := loadProjects()
		projects = append(projects, proj)
		saveProjects(projects)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(proj)
		return
	}

	http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
}

// handleProjectDetail 单个项目操作 (GET/PUT/DELETE)
// 路由: /api/v1/projects/{id}
func (s *Server) handleProjectDetail(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Path[len("/api/v1/projects/"):]
	if projectID == "" {
		http.Error(w, `{"error":"project ID required"}`, http.StatusBadRequest)
		return
	}

	projectsFile := os.Getenv("HOME") + "/.openaide/projects.json"

	type Project struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Path string `json:"path"`
	}

	loadProjects := func() []Project {
		data, _ := os.ReadFile(projectsFile)
		var projects []Project
		json.Unmarshal(data, &projects)
		return projects
	}

	saveProjects := func(projects []Project) {
		data, _ := json.MarshalIndent(projects, "", "  ")
		os.WriteFile(projectsFile, data, 0644)
	}

	switch r.Method {
	case http.MethodGet:
		for _, p := range loadProjects() {
			if p.ID == projectID {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(p)
				return
			}
		}
		http.Error(w, `{"error":"project not found"}`, http.StatusNotFound)

	case http.MethodPut:
		var updated Project
		if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		projects := loadProjects()
		found := false
		for i, p := range projects {
			if p.ID == projectID {
				updated.ID = projectID
				projects[i] = updated
				found = true
				break
			}
		}
		if !found {
			http.Error(w, `{"error":"project not found"}`, http.StatusNotFound)
			return
		}
		saveProjects(projects)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(updated)

	case http.MethodDelete:
		projects := loadProjects()
		filtered := make([]Project, 0, len(projects))
		deleted := false
		for _, p := range projects {
			if p.ID == projectID {
				deleted = true
			} else {
				filtered = append(filtered, p)
			}
		}
		if !deleted {
			http.Error(w, `{"error":"project not found"}`, http.StatusNotFound)
			return
		}
		saveProjects(filtered)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// ErrorResponse 错误响应
type ErrorResponse struct {
	Error string `json:"error"`
}

package orchestration

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"openaide/backend/internal/kernel"
)

// Orchestrator 编排器 - 协调内核与外部系统
type Orchestrator struct {
	kernel      kernel.Kernel
	llmGateway  kernel.LLMProvider
	toolExec    kernel.ToolExecutor
	memory      kernel.Memory
	sessions    kernel.SessionStore
	compressor  kernel.ContextCompressor
	permission  kernel.PermissionChecker
}

// NewOrchestrator 创建编排器
func NewOrchestrator(
	k kernel.Kernel,
	llm kernel.LLMProvider,
	tools kernel.ToolExecutor,
	mem kernel.Memory,
	sessions kernel.SessionStore,
) *Orchestrator {
	return &Orchestrator{
		kernel:     k,
		llmGateway: llm,
		toolExec:   tools,
		memory:     mem,
		sessions:   sessions,
	}
}

// SetContextCompressor 设置上下文压缩器
func (o *Orchestrator) SetContextCompressor(c kernel.ContextCompressor) {
	o.compressor = c
}

// SetPermissionChecker 设置权限检查器
func (o *Orchestrator) SetPermissionChecker(p kernel.PermissionChecker) {
	o.permission = p
}

// ProcessQuery 处理用户查询（完整编排流程）
func (o *Orchestrator) ProcessQuery(ctx context.Context, userID, projectID, content string, opts kernel.QueryOptions) (*kernel.Response, error) {
	start := time.Now()

	// 1. 获取或创建会话
	session, err := o.getOrCreateSession(ctx, projectID, userID)
	if err != nil {
		return nil, fmt.Errorf("session error: %w", err)
	}

	// 2. 构建查询
	query := &kernel.Query{
		SessionID: session.ID,
		Content:   content,
		UserID:    userID,
		ProjectID: projectID,
		Options:   opts,
	}

	// 3. 权限检查（前置）
	if o.permission != nil {
		allowed, reason := o.permission.Check(ctx, "query.process", "kernel", map[string]interface{}{
			"user_id":    userID,
			"project_id": projectID,
			"content":    content,
		})
		if !allowed {
			return &kernel.Response{
				Content:  fmt.Sprintf("请求被拒绝: %s", reason),
				Duration: time.Since(start),
			}, nil
		}
	}

	// 4. 调用内核处理
	resp, err := o.kernel.Process(ctx, query)
	if err != nil {
		slog.Error("Kernel process failed", "error", err, "session", session.ID)
		return nil, err
	}

	// 5. 保存到记忆
	if o.memory != nil {
		go o.saveInteraction(ctx, session.ID, content, resp.Content)
	}

	slog.Info("Query processed", "session", session.ID, "duration", time.Since(start), "tokens", resp.TokensUsed)
	return resp, nil
}

// ProcessQueryStream 流式处理用户查询
func (o *Orchestrator) ProcessQueryStream(ctx context.Context, userID, projectID, content string, opts kernel.QueryOptions) (<-chan kernel.StreamChunk, error) {
	// 获取或创建会话
	session, err := o.getOrCreateSession(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}

	query := &kernel.Query{
		SessionID: session.ID,
		Content:   content,
		UserID:    userID,
		ProjectID: projectID,
		Options:   opts,
	}

	return o.kernel.ProcessStream(ctx, query)
}

// GetSessionHistory 获取会话历史
func (o *Orchestrator) GetSessionHistory(ctx context.Context, sessionID string, limit int) ([]kernel.Message, error) {
	if o.memory == nil {
		return nil, fmt.Errorf("memory not configured")
	}
	return o.memory.Load(ctx, sessionID, limit)
}

// ListSessions 列出会话
func (o *Orchestrator) ListSessions(ctx context.Context, projectID, userID string, limit int) ([]*kernel.Session, error) {
	if o.sessions == nil {
		return nil, fmt.Errorf("session store not configured")
	}
	return o.sessions.List(ctx, projectID, userID, limit)
}

// SearchMemory 搜索记忆
func (o *Orchestrator) SearchMemory(ctx context.Context, query string, limit int) ([]kernel.Message, float64, error) {
	if o.memory == nil {
		return nil, 0, fmt.Errorf("memory not configured")
	}
	return o.memory.Search(ctx, query, limit)
}

// CompressSession 压缩会话上下文
func (o *Orchestrator) CompressSession(ctx context.Context, sessionID string) error {
	if o.compressor == nil {
		return fmt.Errorf("compressor not configured")
	}

	messages, err := o.memory.Load(ctx, sessionID, 100)
	if err != nil {
		return err
	}

	compressed, saved, err := o.compressor.Compress(messages, 4000)
	if err != nil {
		return err
	}

	slog.Info("Session compressed", "session", sessionID, "saved_tokens", saved, "before", len(messages), "after", len(compressed))
	return nil
}

// GetStats 获取系统统计
func (o *Orchestrator) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"kernel_state": o.kernel.GetState().String(),
	}

	if o.toolExec != nil {
		// 工具数量
		if registry, ok := o.toolExec.(interface{ Count() int }); ok {
			stats["tool_count"] = registry.Count()
		}
	}

	return stats
}

// ============ 内部方法 ============

func (o *Orchestrator) getOrCreateSession(ctx context.Context, projectID, userID string) (*kernel.Session, error) {
	// 列出已有会话
	if o.sessions != nil {
		sessions, err := o.sessions.List(ctx, projectID, userID, 1)
		if err == nil && len(sessions) > 0 {
			return sessions[0], nil
		}
	}

	// 创建新会话
	if o.sessions != nil {
		return o.sessions.Create(ctx, projectID, userID)
	}

	return nil, fmt.Errorf("session store not configured")
}

func (o *Orchestrator) saveInteraction(ctx context.Context, sessionID, userContent, assistantContent string) {
	messages := []kernel.Message{
		{Role: "user", Content: userContent},
		{Role: "assistant", Content: assistantContent},
	}
	if err := o.memory.Save(ctx, sessionID, messages); err != nil {
		slog.Warn("Failed to save interaction", "error", err)
	}
}

// ============ 增强能力集成 ============

// EnhancedOrchestrator 增强编排器（带反思、学习、模式检测）
type EnhancedOrchestrator struct {
	*Orchestrator
	reflection      kernel.Reflection
	learner         kernel.Learner
	patternDetector kernel.PatternDetector
}

// NewEnhancedOrchestrator 创建增强编排器
func NewEnhancedOrchestrator(base *Orchestrator) *EnhancedOrchestrator {
	return &EnhancedOrchestrator{Orchestrator: base}
}

// SetReflection 设置反思能力
func (e *EnhancedOrchestrator) SetReflection(r kernel.Reflection) {
	e.reflection = r
}

// SetLearner 设置学习能力
func (e *EnhancedOrchestrator) SetLearner(l kernel.Learner) {
	e.learner = l
}

// SetPatternDetector 设置模式检测器
func (e *EnhancedOrchestrator) SetPatternDetector(pd kernel.PatternDetector) {
	e.patternDetector = pd
}

// ProcessQuery 处理查询（带增强能力）
func (e *EnhancedOrchestrator) ProcessQuery(ctx context.Context, userID, projectID, content string, opts kernel.QueryOptions) (*kernel.Response, error) {
	// 调用基础编排
	resp, err := e.Orchestrator.ProcessQuery(ctx, userID, projectID, content, opts)
	if err != nil {
		return nil, err
	}

	// 异步执行增强能力
	go e.enhance(ctx, userID, projectID, content, resp)

	return resp, nil
}

func (e *EnhancedOrchestrator) enhance(ctx context.Context, userID, projectID, query string, resp *kernel.Response) {
	// 获取会话历史用于模式检测
	if e.patternDetector != nil && e.memory != nil {
		sessions, _ := e.sessions.List(ctx, projectID, userID, 1)
		if len(sessions) > 0 {
			messages, _ := e.memory.Load(ctx, sessions[0].ID, 50)
			if len(messages) > 0 {
				patterns, _ := e.patternDetector.Detect(ctx, sessions[0].ID, messages)
				if len(patterns) > 0 {
					slog.Debug("Patterns detected", "count", len(patterns))
				}
			}
		}
	}

	// 学习
	if e.learner != nil {
		record := kernel.ExecutionRecord{
			Query:     query,
			Response:  resp.Content,
			Success:   resp.Error == "",
			Duration:  int64(resp.Duration),
		}
		if err := e.learner.Learn(ctx, record); err != nil {
			slog.Warn("Learning failed", "error", err)
		}
	}
}

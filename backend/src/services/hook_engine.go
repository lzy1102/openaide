package services

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"openaide/backend/src/models"
)

type HookEvent string

const (
	HookPreToolCall   HookEvent = "pre_tool_call"
	HookPostToolCall  HookEvent = "post_tool_call"
	HookPreCompact    HookEvent = "pre_compact"
	HookPostCompact   HookEvent = "post_compact"
	HookPreSend       HookEvent = "pre_send"
	HookPostResponse  HookEvent = "post_response"
	HookSessionStart  HookEvent = "session_start"
	HookSessionEnd    HookEvent = "session_end"
	HookOnError       HookEvent = "on_error"
)

type HookOutcome string

const (
	HookContinue HookOutcome = "continue"
	HookBlock    HookOutcome = "block"
	HookModify   HookOutcome = "modify"
	HookRetry    HookOutcome = "retry"
)

type HookContext struct {
	Event       HookEvent              `json:"event"`
	SessionID   string                 `json:"session_id"`
	DialogueID  string                 `json:"dialogue_id"`
	ToolName    string                 `json:"tool_name,omitempty"`
	ToolParams  map[string]interface{} `json:"tool_params,omitempty"`
	Command     string                 `json:"command,omitempty"`
	FilePath    string                 `json:"file_path,omitempty"`
	UserID      string                 `json:"user_id,omitempty"`
	ModelID     string                 `json:"model_id,omitempty"`
	Input       string                 `json:"input,omitempty"`
	Output      string                 `json:"output,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Duration    time.Duration          `json:"duration,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type HookResult struct {
	Outcome     HookOutcome            `json:"outcome"`
	ModifiedCtx *HookContext           `json:"modified_ctx,omitempty"`
	Message     string                 `json:"message,omitempty"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

type HookHandler func(ctx context.Context, hctx *HookContext) (*HookResult, error)

type HookRegistration struct {
	ID       string
	Event    HookEvent
	Name     string
	Priority int
	Handler  HookHandler
	Async    bool
}

type HookEngine struct {
	mu          sync.RWMutex
	hooks       map[HookEvent][]*HookRegistration
	eventBus    *EventBus
	execPolicy  *ExecPolicyService
}

func NewHookEngine(eventBus *EventBus, execPolicy *ExecPolicyService) *HookEngine {
	eng := &HookEngine{
		hooks:      make(map[HookEvent][]*HookRegistration),
		eventBus:   eventBus,
		execPolicy: execPolicy,
	}
	eng.initBuiltinHooks()
	return eng
}

func (e *HookEngine) initBuiltinHooks() {
	e.Register(HookPreToolCall, "exec_policy_check", 100, false, e.execPolicyHook)
	e.Register(HookPostToolCall, "usage_tracker", 10, true, e.usageTrackerHook)
	e.Register(HookPostResponse, "cost_tracker", 10, true, e.costTrackerHook)
	e.Register(HookPreCompact, "pre_compact_log", 10, true, e.preCompactLogHook)
	e.Register(HookPostCompact, "post_compact_log", 10, true, e.postCompactLogHook)
	e.Register(HookOnError, "error_logger", 10, true, e.errorLoggerHook)
}

func (e *HookEngine) Register(event HookEvent, name string, priority int, async bool, handler HookHandler) string {
	e.mu.Lock()
	defer e.mu.Unlock()

	id := fmt.Sprintf("hook-%s-%s-%d", event, name, time.Now().UnixNano())
	reg := &HookRegistration{
		ID:       id,
		Event:    event,
		Name:     name,
		Priority: priority,
		Handler:  handler,
		Async:    async,
	}
	e.hooks[event] = append(e.hooks[event], reg)
	e.sortHooks(event)
	slog.Info("Hook registered", "component", "HookEngine", "event", string(event), "name", name)
	return id
}

func (e *HookEngine) Unregister(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for event, hooks := range e.hooks {
		for i, h := range hooks {
			if h.ID == id {
				e.hooks[event] = append(hooks[:i], hooks[i+1:]...)
				return true
			}
		}
	}
	return false
}

func (e *HookEngine) Fire(ctx context.Context, event HookEvent, hctx *HookContext) (*HookResult, error) {
	e.mu.RLock()
	hooks := make([]*HookRegistration, len(e.hooks[event]))
	copy(hooks, e.hooks[event])
	e.mu.RUnlock()

	if hctx == nil {
		hctx = &HookContext{Event: event}
	}
	hctx.Event = event

	for _, hook := range hooks {
		if hook.Async {
			go func(h *HookRegistration) {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("Panic in async hook", "component", "HookEngine", "hook", h.Name, "recovered", r)
					}
				}()
				hookCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if _, err := h.Handler(hookCtx, hctx); err != nil {
					slog.Error("Async hook error", "component", "HookEngine", "hook", h.Name, "error", err)
				}
			}(hook)
			continue
		}

		result, err := hook.Handler(ctx, hctx)
		if err != nil {
			slog.Error("Hook error", "component", "HookEngine", "hook", hook.Name, "error", err)
			continue
		}
		if result != nil {
			switch result.Outcome {
			case HookBlock:
				return result, fmt.Errorf("blocked by hook %s: %s", hook.Name, result.Message)
			case HookModify:
				if result.ModifiedCtx != nil {
					*hctx = *result.ModifiedCtx
				}
			case HookRetry:
				return result, fmt.Errorf("retry requested by hook %s: %s", hook.Name, result.Message)
			}
		}
	}

	return &HookResult{Outcome: HookContinue}, nil
}

func (e *HookEngine) ListHooks() map[HookEvent][]*HookRegistration {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make(map[HookEvent][]*HookRegistration)
	for event, hooks := range e.hooks {
		copied := make([]*HookRegistration, len(hooks))
		copy(copied, hooks)
		result[event] = copied
	}
	return result
}

func (e *HookEngine) sortHooks(event HookEvent) {
	hooks := e.hooks[event]
	for i := 0; i < len(hooks); i++ {
		for j := i + 1; j < len(hooks); j++ {
			if hooks[i].Priority > hooks[j].Priority {
				hooks[i], hooks[j] = hooks[j], hooks[i]
			}
		}
	}
}

func (e *HookEngine) execPolicyHook(ctx context.Context, hctx *HookContext) (*HookResult, error) {
	if e.execPolicy == nil {
		return &HookResult{Outcome: HookContinue}, nil
	}

	if hctx.ToolName == "execute_command" || hctx.ToolName == "run_code" {
		cmd := ""
		if hctx.ToolParams != nil {
			if c, ok := hctx.ToolParams["command"].(string); ok {
				cmd = c
			}
		}
		if cmd == "" {
			return &HookResult{Outcome: HookContinue}, nil
		}

		eval := e.execPolicy.Evaluate(ctx, cmd)
		switch eval.Decision {
		case ExecPolicyDeny:
			return &HookResult{
				Outcome: HookBlock,
				Message: fmt.Sprintf("Command blocked by policy: %s (%s)", cmd, eval.Reason),
				Data: map[string]interface{}{
					"risk_level":  eval.RiskLevel,
					"rule_matched": eval.RuleMatched,
				},
			}, nil
		case ExecPolicyAsk:
			return &HookResult{
				Outcome: HookModify,
				Message: fmt.Sprintf("Command requires approval: %s (%s)", cmd, eval.Reason),
				Data: map[string]interface{}{
					"requires_approval": true,
					"risk_level":        eval.RiskLevel,
					"rule_matched":      eval.RuleMatched,
					"sandboxed":         eval.Sandboxed,
				},
			}, nil
		}
	}

	if hctx.ToolName == "write_file" || hctx.ToolName == "edit_file" {
		path := ""
		if hctx.ToolParams != nil {
			if p, ok := hctx.ToolParams["path"].(string); ok {
				path = p
			} else if p, ok := hctx.ToolParams["file_path"].(string); ok {
				path = p
			}
		}
		if path != "" {
			eval := e.execPolicy.EvaluateFileAccess(ctx, path, true)
			if eval.Decision == ExecPolicyDeny {
				return &HookResult{
					Outcome: HookBlock,
					Message: fmt.Sprintf("File access blocked: %s (%s)", path, eval.Reason),
				}, nil
			}
		}
	}

	return &HookResult{Outcome: HookContinue}, nil
}

func (e *HookEngine) usageTrackerHook(ctx context.Context, hctx *HookContext) (*HookResult, error) {
	if e.eventBus != nil {
		e.eventBus.Publish(ctx, models.EventTopicTool, "tool_usage", "hook_engine", map[string]interface{}{
			"tool_name":   hctx.ToolName,
			"dialogue_id": hctx.DialogueID,
			"session_id":  hctx.SessionID,
			"duration_ms": hctx.Duration.Milliseconds(),
		})
	}
	return &HookResult{Outcome: HookContinue}, nil
}

func (e *HookEngine) costTrackerHook(ctx context.Context, hctx *HookContext) (*HookResult, error) {
	if e.eventBus != nil {
		e.eventBus.Publish(ctx, models.EventTopicMessage, "cost_update", "hook_engine", map[string]interface{}{
			"dialogue_id": hctx.DialogueID,
			"model_id":    hctx.ModelID,
			"input":       hctx.Input,
			"output":      hctx.Output,
		})
	}
	return &HookResult{Outcome: HookContinue}, nil
}

func (e *HookEngine) preCompactLogHook(ctx context.Context, hctx *HookContext) (*HookResult, error) {
	slog.Info("Pre-compact hook fired", "component", "HookEngine", "dialogue_id", hctx.DialogueID)
	return &HookResult{Outcome: HookContinue}, nil
}

func (e *HookEngine) postCompactLogHook(ctx context.Context, hctx *HookContext) (*HookResult, error) {
	slog.Info("Post-compact hook fired", "component", "HookEngine", "dialogue_id", hctx.DialogueID)
	if e.eventBus != nil {
		e.eventBus.Publish(ctx, models.EventTopicTool, "compaction_done", "hook_engine", map[string]interface{}{
			"dialogue_id": hctx.DialogueID,
			"metadata":    hctx.Metadata,
		})
	}
	return &HookResult{Outcome: HookContinue}, nil
}

func (e *HookEngine) errorLoggerHook(ctx context.Context, hctx *HookContext) (*HookResult, error) {
	slog.Error("Error hook fired", "component", "HookEngine", "dialogue_id", hctx.DialogueID, "error", hctx.Error)
	return &HookResult{Outcome: HookContinue}, nil
}

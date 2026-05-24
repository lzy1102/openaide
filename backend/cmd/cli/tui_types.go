package main

import (
	"time"

	"openaide/backend/internal/kernel"
	"openaide/backend/internal/orchestration"
)

// ── Enums ──────────────────────────────────────────────────

// StreamPhase 流式处理阶段
type StreamPhase int

const (
	PhaseIdle StreamPhase = iota
	PhasePlanPreview
	PhaseDeepPlan
	PhaseStreaming
	PhaseExecuting
	PhaseError
)

// OverlayType 弹窗类型
type OverlayType int

const (
	OverlayNone OverlayType = iota
	OverlaySessions
	OverlayModels
	OverlayHelp
	OverlayPlanConfirm
	OverlayProposalSelect
	OverlayLang
	OverlayLog
)

// WorkKind 后台工作类型
type WorkKind int

const (
	WorkNone WorkKind = iota
	WorkStreaming
	WorkPlanPreview
	WorkDeepPlan
	WorkPlanExecution
	WorkSingleRole
	WorkTeamChain
)

// ── Chat Messages ──────────────────────────────────────────

// Message 聊天消息（支持思考内容）
type Message struct {
	Role     string // "user", "assistant", "system", "tool_call", "tool", "error"
	Content  string
	Thinking string // 推理内容，完成后可展开
}

// ── Stream Messages ────────────────────────────────────────

// StreamContentMsg 流式内容块
type StreamContentMsg struct {
	Content  string
	Thinking string
}

// StreamToolMsg 工具调用或结果通知
type StreamToolMsg struct {
	Name    string
	IsCall  bool   // true=工具调用开始, false=结果摘要
	Summary string // 结果摘要（仅 IsCall=false 时有效）
}

// StreamDoneMsg 流完成
type StreamDoneMsg struct {
	Tokens int
	Tools  int
	Err    error
}

// ── Plan Messages ──────────────────────────────────────────

// PlanProposalMsg 任务规划结果
type PlanProposalMsg struct {
	Plan  *orchestration.Plan
	Query string
	Err   error
}

// DeepPlanProgressMsg 深度规划阶段进度
type DeepPlanProgressMsg struct {
	Phase    string // "research", "propose", "plan"
	Progress string
	Err      error
}

// DeepPlanResultMsg 深度规划完整结果
type DeepPlanResultMsg struct {
	Result *orchestration.DeepPlanResult
	Query  string
	Err    error
}

// ── Execution Messages ─────────────────────────────────────

// ExecutionProgressMsg 计划执行进度
type ExecutionProgressMsg struct {
	Phase   string // "execute", "verify", "review"
	Substep string // "步骤 2/5: 实现功能 [coder]"
	Elapsed time.Duration
	Done    bool
	Err     error
}

// ExecutionResultMsg 计划执行结果
type ExecutionResultMsg struct {
	Content string
	Tokens  int
	Tools   int
	Err     error
}

// ── Session Messages ───────────────────────────────────────

// SessionListMsg 会话列表
type SessionListMsg struct {
	Sessions []*kernel.Session
	Session  *kernel.Session
	Err      error
}

// SessionCreatedMsg 新会话创建
type SessionCreatedMsg struct {
	Session *kernel.Session
	Err     error
}

// SessionDeletedMsg 会话删除
type SessionDeletedMsg struct {
	ID  string
	Err error
}

// ── Navigation Messages ────────────────────────────────────

// SwitchOverlayMsg 切换弹窗
type SwitchOverlayMsg struct {
	Overlay OverlayType
}

// OverlayDismissedMsg 弹窗关闭
type OverlayDismissedMsg struct{}

// SubmitMsg 用户提交查询
type SubmitMsg struct {
	Query string
}

// ── Internal Messages ──────────────────────────────────────

type spinnerTick struct{}

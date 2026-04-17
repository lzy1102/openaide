package services

import (
	"fmt"
)

// WorkflowStateMachine 工作流状态机
type WorkflowStateMachine struct {
	transitions map[WorkflowStatus][]WorkflowStatus
}

// NewWorkflowStateMachine 创建工作流状态机
func NewWorkflowStateMachine() *WorkflowStateMachine {
	sm := &WorkflowStateMachine{
		transitions: make(map[WorkflowStatus][]WorkflowStatus),
	}

	// 定义有效的状态转换
	sm.transitions[StatusPending] = []WorkflowStatus{StatusRunning, StatusCancelled}
	sm.transitions[StatusRunning] = []WorkflowStatus{StatusCompleted, StatusFailed, StatusPaused, StatusCancelled}
	sm.transitions[StatusPaused] = []WorkflowStatus{StatusRunning, StatusCancelled}
	sm.transitions[StatusCompleted] = []WorkflowStatus{} // 终态
	sm.transitions[StatusFailed] = []WorkflowStatus{StatusPending} // 可以重新执行
	sm.transitions[StatusCancelled] = []WorkflowStatus{} // 终态

	return sm
}

// CanTransition 检查是否可以转换状态
func (sm *WorkflowStateMachine) CanTransition(from, to WorkflowStatus) bool {
	allowed, exists := sm.transitions[from]
	if !exists {
		return false
	}

	for _, status := range allowed {
		if status == to {
			return true
		}
	}

	return false
}

// Transition 执行状态转换
func (sm *WorkflowStateMachine) Transition(from, to WorkflowStatus) error {
	if !sm.CanTransition(from, to) {
		return fmt.Errorf("invalid state transition from %s to %s", from, to)
	}
	return nil
}

// GetNextStates 获取可转换的下一状态
func (sm *WorkflowStateMachine) GetNextStates(current WorkflowStatus) []WorkflowStatus {
	return sm.transitions[current]
}

// IsTerminalState 检查是否是终态
func (sm *WorkflowStateMachine) IsTerminalState(status WorkflowStatus) bool {
	return status == StatusCompleted || status == StatusCancelled
}

// StepStateMachine 步骤状态机
type StepStateMachine struct {
	transitions map[StepStatus][]StepStatus
}

// NewStepStateMachine 创建步骤状态机
func NewStepStateMachine() *StepStateMachine {
	sm := &StepStateMachine{
		transitions: make(map[StepStatus][]StepStatus),
	}

	// 定义有效的状态转换
	sm.transitions[StepStatusPending] = []StepStatus{StepStatusRunning, StepStatusSkipped}
	sm.transitions[StepStatusRunning] = []StepStatus{StepStatusCompleted, StepStatusFailed, StepStatusRetrying}
	sm.transitions[StepStatusRetrying] = []StepStatus{StepStatusRunning, StepStatusFailed}
	sm.transitions[StepStatusCompleted] = []StepStatus{} // 终态
	sm.transitions[StepStatusFailed] = []StepStatus{StepStatusRetrying, StepStatusPending}
	sm.transitions[StepStatusSkipped] = []StepStatus{} // 终态

	return sm
}

// CanTransition 检查是否可以转换状态
func (sm *StepStateMachine) CanTransition(from, to StepStatus) bool {
	allowed, exists := sm.transitions[from]
	if !exists {
		return false
	}

	for _, status := range allowed {
		if status == to {
			return true
		}
	}

	return false
}

// Transition 执行状态转换
func (sm *StepStateMachine) Transition(from, to StepStatus) error {
	if !sm.CanTransition(from, to) {
		return fmt.Errorf("invalid step state transition from %s to %s", from, to)
	}
	return nil
}

// IsTerminalState 检查是否是终态
func (sm *StepStateMachine) IsTerminalState(status StepStatus) bool {
	return status == StepStatusCompleted || status == StepStatusSkipped
}

// WorkflowStateEvent 工作流状态事件
type WorkflowStateEvent struct {
	InstanceID string
	FromStatus WorkflowStatus
	ToStatus   WorkflowStatus
	Timestamp  int64
	Metadata   map[string]interface{}
}

// StepStateEvent 步骤状态事件
type StepStateEvent struct {
	InstanceID string
	StepID     string
	FromStatus StepStatus
	ToStatus   StepStatus
	Timestamp  int64
	Metadata   map[string]interface{}
}

// StateEventDispatcher 状态事件分发器
type StateEventDispatcher struct {
	workflowListeners []func(WorkflowStateEvent)
	stepListeners     []func(StepStateEvent)
}

// NewStateEventDispatcher 创建状态事件分发器
func NewStateEventDispatcher() *StateEventDispatcher {
	return &StateEventDispatcher{
		workflowListeners: make([]func(WorkflowStateEvent), 0),
		stepListeners:     make([]func(StepStateEvent), 0),
	}
}

// OnWorkflowStateChange 注册工作流状态变化监听器
func (d *StateEventDispatcher) OnWorkflowStateChange(listener func(WorkflowStateEvent)) {
	d.workflowListeners = append(d.workflowListeners, listener)
}

// OnStepStateChange 注册步骤状态变化监听器
func (d *StateEventDispatcher) OnStepStateChange(listener func(StepStateEvent)) {
	d.stepListeners = append(d.stepListeners, listener)
}

// DispatchWorkflowEvent 分发工作流状态事件
func (d *StateEventDispatcher) DispatchWorkflowEvent(event WorkflowStateEvent) {
	for _, listener := range d.workflowListeners {
		listener(event)
	}
}

// DispatchStepEvent 分发步骤状态事件
func (d *StateEventDispatcher) DispatchStepEvent(event StepStateEvent) {
	for _, listener := range d.stepListeners {
		listener(event)
	}
}

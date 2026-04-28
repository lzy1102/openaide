package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"openaide/backend/src/services/llm"
)

// ReActState ReAct 循环的显式状态
type ReActState string

const (
	StateIdle       ReActState = "idle"
	StateThinking   ReActState = "thinking"
	StateToolCall   ReActState = "tool_call"
	StateObserving  ReActState = "observing"
	StateCompleted  ReActState = "completed"
	StateError      ReActState = "error"
	StateMaxRounds  ReActState = "max_rounds"
)

// ReActStep ReAct 循环的单个步骤记录
type ReActStep struct {
	Round       int                    `json:"round"`
	State       ReActState             `json:"state"`
	Timestamp   time.Time              `json:"timestamp"`
	Duration    time.Duration          `json:"duration_ms"`
	LLMMessage  *llm.Message           `json:"llm_message,omitempty"`
	ToolCalls   []ToolCallRecord       `json:"tool_calls,omitempty"`
	Observation string                 `json:"observation,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ToolCallRecord 工具调用记录
type ToolCallRecord struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
	Result    string                 `json:"result"`
	Duration  time.Duration          `json:"duration_ms"`
	Error     string                 `json:"error,omitempty"`
}

// ReActSession 完整的 ReAct 会话记录
type ReActSession struct {
	SessionID   string                 `json:"session_id"`
	DialogueID  string                 `json:"dialogue_id"`
	UserID      string                 `json:"user_id"`
	ModelID     string                 `json:"model_id"`
	StartTime   time.Time              `json:"start_time"`
	EndTime     *time.Time             `json:"end_time,omitempty"`
	Steps       []ReActStep            `json:"steps"`
	FinalState  ReActState             `json:"final_state"`
	TotalRounds int                    `json:"total_rounds"`
	TotalTokens int                    `json:"total_tokens"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	mu          sync.RWMutex
}

// ReActStateMachine ReAct 状态机
type ReActStateMachine struct {
	session    *ReActSession
	currentRound int
	mu         sync.RWMutex
}

// NewReActStateMachine 创建新的 ReAct 状态机
func NewReActStateMachine(sessionID, dialogueID, userID, modelID string) *ReActStateMachine {
	return &ReActStateMachine{
		session: &ReActSession{
			SessionID:  sessionID,
			DialogueID: dialogueID,
			UserID:     userID,
			ModelID:    modelID,
			StartTime:  time.Now(),
			Steps:      []ReActStep{},
			FinalState: StateIdle,
			Metadata:   make(map[string]interface{}),
		},
		currentRound: 0,
	}
}

// StartThinking 开始思考阶段
func (sm *ReActStateMachine) StartThinking() *ReActStep {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.currentRound++
	step := ReActStep{
		Round:     sm.currentRound,
		State:     StateThinking,
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}
	sm.session.Steps = append(sm.session.Steps, step)
	return &sm.session.Steps[len(sm.session.Steps)-1]
}

// EndThinking 结束思考阶段
func (sm *ReActStateMachine) EndThinking(step *ReActStep, msg *llm.Message) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	step.Duration = time.Since(step.Timestamp)
	step.LLMMessage = msg
}

// StartToolCall 开始工具调用阶段
func (sm *ReActStateMachine) StartToolCall(toolCalls []llm.ToolCall) *ReActStep {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	step := ReActStep{
		Round:     sm.currentRound,
		State:     StateToolCall,
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}

	for _, tc := range toolCalls {
		var args map[string]interface{}
		json.Unmarshal([]byte(tc.Function.Arguments), &args)
		step.ToolCalls = append(step.ToolCalls, ToolCallRecord{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	sm.session.Steps = append(sm.session.Steps, step)
	return &sm.session.Steps[len(sm.session.Steps)-1]
}

// EndToolCall 结束工具调用阶段
func (sm *ReActStateMachine) EndToolCall(step *ReActStep, results []ToolCallRecord) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	step.Duration = time.Since(step.Timestamp)
	step.ToolCalls = results
}

// StartObservation 开始观察阶段
func (sm *ReActStateMachine) StartObservation(observation string) *ReActStep {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	step := ReActStep{
		Round:       sm.currentRound,
		State:       StateObserving,
		Timestamp:   time.Now(),
		Observation: observation,
		Metadata:    make(map[string]interface{}),
	}
	sm.session.Steps = append(sm.session.Steps, step)
	return &sm.session.Steps[len(sm.session.Steps)-1]
}

// EndObservation 结束观察阶段
func (sm *ReActStateMachine) EndObservation(step *ReActStep) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	step.Duration = time.Since(step.Timestamp)
}

// Complete 标记会话完成
func (sm *ReActStateMachine) Complete(state ReActState, totalTokens int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	sm.session.EndTime = &now
	sm.session.FinalState = state
	sm.session.TotalRounds = sm.currentRound
	sm.session.TotalTokens = totalTokens
}

// GetSession 获取会话记录（只读副本）
func (sm *ReActStateMachine) GetSession() *ReActSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// 返回深拷贝
	sessionCopy := *sm.session
	stepsCopy := make([]ReActStep, len(sm.session.Steps))
	copy(stepsCopy, sm.session.Steps)
	sessionCopy.Steps = stepsCopy
	return &sessionCopy
}

// GetCurrentState 获取当前状态
func (sm *ReActStateMachine) GetCurrentState() ReActState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if len(sm.session.Steps) == 0 {
		return StateIdle
	}
	return sm.session.Steps[len(sm.session.Steps)-1].State
}

// SessionRecorder 会话记录器
type SessionRecorder struct {
	sessions map[string]*ReActStateMachine
	mu       sync.RWMutex
	dataDir  string
}

// NewSessionRecorder 创建会话记录器
func NewSessionRecorder(dataDir string) *SessionRecorder {
	if dataDir == "" {
		dataDir = "./sessions"
	}
	os.MkdirAll(dataDir, 0755)
	return &SessionRecorder{
		sessions: make(map[string]*ReActStateMachine),
		dataDir:  dataDir,
	}
}

// StartSession 开始记录新会话
func (r *SessionRecorder) StartSession(sessionID, dialogueID, userID, modelID string) *ReActStateMachine {
	sm := NewReActStateMachine(sessionID, dialogueID, userID, modelID)

	r.mu.Lock()
	r.sessions[sessionID] = sm
	r.mu.Unlock()

	return sm
}

// GetSession 获取会话状态机
func (r *SessionRecorder) GetSession(sessionID string) *ReActStateMachine {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sessions[sessionID]
}

// ExportSession 导出会话为 JSON
func (r *SessionRecorder) ExportSession(sessionID string) ([]byte, error) {
	sm := r.GetSession(sessionID)
	if sm == nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	session := sm.GetSession()
	return json.MarshalIndent(session, "", "  ")
}

// SaveSessionToFile 保存会话到文件
func (r *SessionRecorder) SaveSessionToFile(sessionID string) (string, error) {
	data, err := r.ExportSession(sessionID)
	if err != nil {
		return "", err
	}

	filename := filepath.Join(r.dataDir, fmt.Sprintf("session_%s_%s.json",
		sessionID, time.Now().Format("20060102_150405")))
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return "", err
	}

	return filename, nil
}

// ListSessions 列出所有会话
func (r *SessionRecorder) ListSessions() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.sessions))
	for id := range r.sessions {
		ids = append(ids, id)
	}
	return ids
}

// CleanupOldSessions 清理旧会话（保留最近 N 个）
func (r *SessionRecorder) CleanupOldSessions(keep int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.sessions) <= keep {
		return
	}

	// 简单的清理策略：保留最近开始的 keep 个会话
	// 实际生产环境可以按时间排序
	count := 0
	for id := range r.sessions {
		count++
		if count > keep {
			delete(r.sessions, id)
		}
	}
}

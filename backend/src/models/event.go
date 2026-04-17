package models

import "time"

// Event 事件模型
type Event struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	Topic     string    `json:"topic" gorm:"index"`     // 事件主题
	Type      string    `json:"type"`                    // 事件类型
	Source    string    `json:"source"`                  // 事件来源
	Data      JSONMap   `json:"data" gorm:"type:json"`   // 事件数据
	Metadata  JSONMap   `json:"metadata" gorm:"type:json"` // 事件元数据
	Status    string    `json:"status" gorm:"default:pending"` // pending, processed, failed
	CreatedAt time.Time `json:"created_at" gorm:"index"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
}

// 预定义事件主题
const (
	EventTopicMessage  = "message"
	EventTopicTool     = "tool"
	EventTopicPlan     = "plan"
	EventTopicFeedback = "feedback"
	EventTopicKnowledge = "knowledge"
	EventTopicMemory   = "memory"
	EventTopicModel    = "model"
	EventTopicSkill    = "skill"
	EventTopicSystem   = "system"
)

// 预定义事件类型
const (
	EventTypeMessageReceived    = "message.received"
	EventTypeMessageSent        = "message.sent"
	EventTypeToolCalled         = "tool.called"
	EventTypeToolCompleted      = "tool.completed"
	EventTypeToolFailed         = "tool.failed"
	EventTypePlanCreated        = "plan.created"
	EventTypePlanStepCompleted  = "plan.step_completed"
	EventTypePlanCompleted      = "plan.completed"
	EventTypePlanFailed         = "plan.failed"
	EventTypeFeedbackReceived   = "feedback.received"
	EventTypeOptimizationApplied = "optimization.applied"
	EventTypeKnowledgeExtracted = "knowledge.extracted"
	EventTypeMemoryUpdated      = "memory.updated"
	EventTypeModelRouted        = "model.routed"
	EventTypeSkillMatched       = "skill.matched"
	EventTypeSkillExecuted      = "skill.executed"
	EventTypeSystemError        = "system.error"
	EventTypeSystemStarted      = "system.started"
)

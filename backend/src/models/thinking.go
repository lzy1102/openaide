package models

import "time"

// Thought 思考模型
type Thought struct {
	ID          string    `json:"id"`
	Content     string    `json:"content"`
	Type        string    `json:"type"` // analysis, planning, problem-solving, etc.
	UserID      string    `json:"user_id"`
	Status      string    `json:"status"` // draft, published, archived
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// 纠错相关字段
	QualityScore    *float64  `json:"quality_score,omitempty"`    // 质量分数
	LastCorrectionID *string  `json:"last_correction_id,omitempty"` // 最后一次修正ID
	CorrectionCount int       `json:"correction_count"`           // 修正次数
}

// Correction 修正模型
type Correction struct {
	ID          string    `json:"id"`
	ThoughtID   string    `json:"thought_id"`
	Content     string    `json:"content"`
	UserID      string    `json:"user_id"`
	Status      string    `json:"status"` // pending, resolved, rejected
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// 纠错详情字段
	OriginalContent  string     `json:"original_content,omitempty"`  // 原始内容
	ChangesApplied   string     `json:"changes_applied,omitempty"`   // 应用的修改（JSON）
	IterationNumber  int        `json:"iteration_number"`            // 迭代编号
	QualityBefore    *float64   `json:"quality_before,omitempty"`    // 修正前质量分数
	QualityAfter     *float64   `json:"quality_after,omitempty"`     // 修正后质量分数
	ResolvedIssues   string     `json:"resolved_issues,omitempty"`   // 解决的问题列表（JSON）
	Metadata         string     `json:"metadata,omitempty"`          // 元数据（JSON）
}

// CorrectionHistory 纠错历史模型（用于追踪完整的纠错过程）
type CorrectionHistory struct {
	ID              string    `json:"id"`
	ThoughtID       string    `json:"thought_id"`
	CorrectionID    string    `json:"correction_id"`
	UserID          string    `json:"user_id"`
	IterationNumber int       `json:"iteration_number"`
	InputContent    string    `json:"input_content"`
	OutputContent   string    `json:"output_content"`
	QualityScore    float64   `json:"quality_score"`
	IssuesDetected  string    `json:"issues_detected"` // JSON
	ChangesApplied  string    `json:"changes_applied"` // JSON
	TokenUsage      string    `json:"token_usage,omitempty"` // JSON
	DurationMs      int64     `json:"duration_ms"`
	CreatedAt       time.Time `json:"created_at"`
}

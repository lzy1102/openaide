package models

import "time"

type SelfReflection struct {
	ID           string    `json:"id" gorm:"primaryKey"`
	DialogueID   string    `json:"dialogue_id" gorm:"index"`
	UserID       string    `json:"user_id" gorm:"index"`
	Query        string    `json:"query" gorm:"type:text"`
	Response     string    `json:"response" gorm:"type:text"`
	QualityScore float64   `json:"quality_score"`
	Issues       string    `json:"issues" gorm:"type:text"`
	Improvements string    `json:"improvements" gorm:"type:text"`
	Confidence   float64   `json:"confidence"`
	Status       string    `json:"status" gorm:"default:'pending'"` // pending, reviewed, applied
	CreatedAt    time.Time `json:"created_at"`
}

type RepetitivePattern struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	UserID      string    `json:"user_id" gorm:"index"`
	PatternType string    `json:"pattern_type" gorm:"index"` // query_pattern, action_sequence, topic_repetition
	Pattern     string    `json:"pattern" gorm:"type:text"`
	Frequency   int       `json:"frequency"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
	SampleQuery string    `json:"sample_query" gorm:"type:text"`
	Suggestion  string    `json:"suggestion" gorm:"type:text"`
	Status      string    `json:"status" gorm:"default:'detected'"` // detected, skill_created, ignored
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SkillEvolution struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	SkillID     string    `json:"skill_id" gorm:"index"`
	VersionFrom string    `json:"version_from"`
	VersionTo   string    `json:"version_to"`
	ChangeType  string    `json:"change_type"` // trigger_optimization, prompt_improvement, parameter_adjustment, auto_generated
	ChangeDesc  string    `json:"change_desc" gorm:"type:text"`
	BeforeState string    `json:"before_state" gorm:"type:text"`
	AfterState  string    `json:"after_state" gorm:"type:text"`
	Trigger     string    `json:"trigger"` // reflection, pattern, feedback, auto
	Confidence  float64   `json:"confidence"`
	Status      string    `json:"status" gorm:"default:'pending'"` // pending, applied, rolled_back
	AppliedAt   *time.Time `json:"applied_at"`
	CreatedAt   time.Time `json:"created_at"`
}

type CapabilityGap struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	UserID      string    `json:"user_id" gorm:"index"`
	GapType     string    `json:"gap_type" gorm:"index"` // missing_skill, weak_response, no_tool, knowledge_lack
	Description string    `json:"description" gorm:"type:text"`
	Evidence    string    `json:"evidence" gorm:"type:text"`
	Frequency   int       `json:"frequency"`
	Severity    string    `json:"severity"` // low, medium, high
	Suggestion  string    `json:"suggestion" gorm:"type:text"`
	Status      string    `json:"status" gorm:"default:'detected'"` // detected, skill_created, ignored
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type EvolutionMetrics struct {
	ID                     string    `json:"id" gorm:"primaryKey"`
	Period                 string    `json:"period" gorm:"index"` // daily, weekly, monthly
	StartDate              time.Time `json:"start_date"`
	EndDate                time.Time `json:"end_date"`
	ReflectionsCount       int       `json:"reflections_count"`
	PatternsDetected       int       `json:"patterns_detected"`
	SkillsEvolved          int       `json:"skills_evolved"`
	GapsIdentified         int       `json:"gaps_identified"`
	OptimizationsApplied   int       `json:"optimizations_applied"`
	AvgQualityScore        float64   `json:"avg_quality_score"`
	QualityImprovement     float64   `json:"quality_imvement"`
	AutoSkillCreations     int       `json:"auto_skill_creations"`
	CreatedAt              time.Time `json:"created_at"`
}

package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UsageRecord 使用量记录
type UsageRecord struct {
	ID           string                 `json:"id" gorm:"primaryKey"`
	UserID       string                 `json:"user_id" gorm:"index;not null"`
	DialogueID   string                 `json:"dialogue_id" gorm:"index"`
	MessageID    string                 `json:"message_id" gorm:"index"`

	// 模型信息
	Provider     string                 `json:"provider" gorm:"size:50;index"`    // openai, anthropic, etc.
	ModelID      string                 `json:"model_id" gorm:"size:100;index"`
	ModelName    string                 `json:"model_name" gorm:"size:100"`

	// Token 使用量
	PromptTokens     int64              `json:"prompt_tokens"`
	CompletionTokens int64              `json:"completion_tokens"`
	TotalTokens      int64              `json:"total_tokens"`

	// 成本计算
	InputCost        float64            `json:"input_cost"`        // 输入成本 (USD)
	OutputCost       float64            `json:"output_cost"`       // 输出成本 (USD)
	TotalCost        float64            `json:"total_cost"`        // 总成本 (USD)

	// 请求详情
	RequestType      string             `json:"request_type" gorm:"size:20"` // chat, embedding, rag
	IsStreaming      bool               `json:"is_streaming"`
	Duration         int64              `json:"duration"`         // 请求耗时 (毫秒)
	Success          bool               `json:"success"`
	ErrorMessage     string             `json:"error_message" gorm:"type:text"`

	// 元数据
	Metadata         map[string]interface{} `json:"metadata" gorm:"type:json"`

	CreatedAt        time.Time           `json:"created_at" gorm:"index"`
}

// DailyUsage 每日使用量汇总
type DailyUsage struct {
	ID           string    `json:"id" gorm:"primaryKey"`
	UserID       string    `json:"user_id" gorm:"index;not null"`
	Date         time.Time `json:"date" gorm:"index;not null"` // 日期 (只保留日期部分)

	// Token 统计
	TotalRequests      int64   `json:"total_requests"`
	TotalPromptTokens  int64   `json:"total_prompt_tokens"`
	TotalCompletionTokens int64 `json:"total_completion_tokens"`
	TotalTokens        int64   `json:"total_tokens"`

	// 成本统计
	TotalCost          float64 `json:"total_cost"`

	// 按模型统计
	ModelUsage         map[string]int64 `json:"model_usage" gorm:"type:json"` // model_id -> request_count
	ProviderUsage      map[string]int64 `json:"provider_usage" gorm:"type:json"` // provider -> request_count

	UpdatedAt          time.Time `json:"updated_at"`
}

// MonthlyUsage 每月使用量汇总
type MonthlyUsage struct {
	ID           string    `json:"id" gorm:"primaryKey"`
	UserID       string    `json:"user_id" gorm:"index;not null"`
	Year         int       `json:"year"`
	Month        int       `json:"month"`

	// Token 统计
	TotalRequests      int64   `json:"total_requests"`
	TotalPromptTokens  int64   `json:"total_prompt_tokens"`
	TotalCompletionTokens int64 `json:"total_completion_tokens"`
	TotalTokens        int64   `json:"total_tokens"`

	// 成本统计
	TotalCost          float64 `json:"total_cost"`
	BudgetLimit        float64 `json:"budget_limit"` // 预算限制
	BudgetUsedPercent  float64 `json:"budget_used_percent"`

	UpdatedAt          time.Time `json:"updated_at"`
}

// ModelPricing 模型定价配置
type ModelPricing struct {
	ID              string    `json:"id" gorm:"primaryKey"`
	Provider        string    `json:"provider" gorm:"size:50;not null"`
	ModelName       string    `json:"model_name" gorm:"size:100;not null"`
	InputPricePer1K float64   `json:"input_price_per_1k"`  // 每1K输入token价格 (USD)
	OutputPricePer1K float64  `json:"output_price_per_1k"` // 每1K输出token价格 (USD)

	// 缓存定价 (可选)
	CacheReadPricePer1K  float64 `json:"cache_read_price_per_1k"`
	CacheWritePricePer1K float64 `json:"cache_write_price_per_1k"`

	IsActive        bool      `json:"is_active" gorm:"default:true"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// UserBudget 用户预算配置
type UserBudget struct {
	ID              string    `json:"id" gorm:"primaryKey"`
	UserID          string    `json:"user_id" gorm:"uniqueIndex;not null"`
	MonthlyBudget   float64   `json:"monthly_budget"`   // 月度预算 (USD)
	DailyBudget     float64   `json:"daily_budget"`     // 每日预算 (USD)

	// 警报配置
	AlertThresholds []int     `json:"alert_thresholds" gorm:"type:json"` // 警报阈值百分比 [50, 80, 100]
	AlertEmail      string    `json:"alert_email" gorm:"size:100"`
	AlertWebhook    string    `json:"alert_webhook" gorm:"size:255"`

	// 当前使用 (缓存)
	CurrentMonthUsage float64 `json:"current_month_usage"`
	CurrentDayUsage   float64 `json:"current_day_usage"`

	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// BeforeCreate 创建前钩子
func (r *UsageRecord) BeforeCreate(tx *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	return nil
}

func (d *DailyUsage) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	return nil
}

func (m *MonthlyUsage) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	return nil
}

func (p *ModelPricing) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	return nil
}

func (b *UserBudget) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	return nil
}

// TableName 指定表名
func (UsageRecord) TableName() string {
	return "usage_records"
}

func (DailyUsage) TableName() string {
	return "daily_usages"
}

func (MonthlyUsage) TableName() string {
	return "monthly_usages"
}

func (ModelPricing) TableName() string {
	return "model_pricings"
}

func (UserBudget) TableName() string {
	return "user_budgets"
}

package models

import "time"

// Model 模型管理模型
type Model struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Type        string    `json:"type"` // llm, embedding, etc.
	Provider    string    `json:"provider"`
	Version     string    `json:"version"`
	APIKey      string    `json:"api_key,omitempty"`
	BaseURL     string    `json:"base_url"`
	Config      JSONMap   `json:"config" gorm:"type:json"`
	Status      string    `json:"status"` // enabled, disabled
	Tags        JSONSlice `json:"tags" gorm:"type:json"`            // 擅长领域: ["code", "reasoning", "creative", "fast"]
	Capabilities JSONMap `json:"capabilities" gorm:"type:json"`     // 能力标记: {"tool_calling":true, "streaming":true}
	Priority    int       `json:"priority" gorm:"default:0"`       // 同类中优先级，越大越优先
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ModelInstance 模型实例模型
type ModelInstance struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	ModelID   string    `json:"model_id"`
	ModelName string    `json:"model_name"`
	Config    JSONMap   `json:"config" gorm:"type:json"`
	Status    string    `json:"status"` // pending, running, completed, failed
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ModelExecution 模型执行模型
type ModelExecution struct {
	ID         string    `json:"id" gorm:"primaryKey"`
	ModelID    string    `json:"model_id"`
	ModelName  string    `json:"model_name"`
	InstanceID string    `json:"instance_id"`
	Parameters JSONMap   `json:"parameters" gorm:"type:json"`
	Status     string    `json:"status"` // pending, running, completed, failed
	Result     JSONMap   `json:"result" gorm:"type:json"`
	Error      string    `json:"error"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
}

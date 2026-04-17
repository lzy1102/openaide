package models

import "time"

// Plugin 插件模型
type Plugin struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Version     string    `json:"version"`
	Author      string    `json:"author"`
	Category    string    `json:"category"`
	Status      string    `json:"status"` // installed, enabled, disabled
	Path        string    `json:"path"`
	Config      JSONMap   `json:"config" gorm:"type:json"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PluginInstance 插件实例模型
type PluginInstance struct {
	ID         string    `json:"id" gorm:"primaryKey"`
	PluginID   string    `json:"plugin_id"`
	PluginName string    `json:"plugin_name"`
	Config     JSONMap   `json:"config" gorm:"type:json"`
	Status     string    `json:"status"` // pending, running, completed, failed
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// PluginExecution 插件执行模型
type PluginExecution struct {
	ID         string    `json:"id" gorm:"primaryKey"`
	PluginID   string    `json:"plugin_id"`
	PluginName string    `json:"plugin_name"`
	InstanceID string    `json:"instance_id"`
	Parameters JSONMap   `json:"parameters" gorm:"type:json"`
	Status     string    `json:"status"` // pending, running, completed, failed
	Result     *JSONAny  `json:"result" gorm:"type:json"`
	Error      string    `json:"error"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
}

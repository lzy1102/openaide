package models

import "time"

// MCPServer MCP Server 配置
type MCPServer struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"size:100;not null"`
	Transport string    `json:"transport" gorm:"size:20;not null"` // stdio, sse
	Command   string    `json:"command" gorm:"size:500"`
	Args      string    `json:"args" gorm:"size:1000"`
	URL       string    `json:"url" gorm:"size:500"`
	Env       string    `json:"env" gorm:"type:json"`
	Enabled   bool      `json:"enabled" gorm:"default:true"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

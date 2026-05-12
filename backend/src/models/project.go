package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Project struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	UserID      string    `json:"user_id" gorm:"index"`
	Name        string    `json:"name" gorm:"size:200;not null"`
	Description string    `json:"description" gorm:"type:text"`
	Color       string    `json:"color" gorm:"size:20"`
	Icon        string    `json:"icon" gorm:"size:50"`
	SystemPrompt string   `json:"system_prompt" gorm:"type:text"`
	ModelID     string    `json:"model_id" gorm:"size:100"`
	WorkingDir  string    `json:"working_dir" gorm:"size:500"`
	SortOrder   int       `json:"sort_order" gorm:"default:0"`
	IsDefault   bool      `json:"is_default" gorm:"default:false"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Project) TableName() string {
	return "projects"
}

func (p *Project) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	return nil
}

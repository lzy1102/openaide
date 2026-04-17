package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PromptTemplate 提示词模板
type PromptTemplate struct {
	ID          string                 `json:"id" gorm:"primaryKey"`
	Name        string                 `json:"name" gorm:"uniqueIndex;size:100;not null"`
	Description string                 `json:"description" gorm:"size:500"`
	Category    string                 `json:"category" gorm:"size:50;index"` // system, user, assistant, function

	// 模板内容
	Template    string                 `json:"template" gorm:"type:text;not null"`
	Variables   PromptVariableSlice    `json:"variables" gorm:"type:json"` // 可替换变量定义

	// 元数据
	Tags        JSONSlice              `json:"tags" gorm:"type:json"`
	Version     string                 `json:"version" gorm:"size:20"`
	ParentID    string                 `json:"parent_id" gorm:"size:36;index"` // 父版本ID，用于版本管理

	// 使用统计
	UseCount    int64                  `json:"use_count" gorm:"default:0"`
	LastUsedAt  *time.Time             `json:"last_used_at"`

	// 状态
	IsActive    bool                   `json:"is_active" gorm:"default:true"`
	IsDefault   bool                   `json:"is_default" gorm:"default:false"` // 是否为默认模板

	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	CreatedBy   string                 `json:"created_by" gorm:"size:36"`
}

// PromptVariable 提示词变量定义
type PromptVariable struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Type         string `json:"type"`         // string, number, boolean, array, object
	Required     bool   `json:"required"`
	DefaultValue string `json:"default_value"`
	Example      string `json:"example"`
}

// PromptInstance 提示词实例 (填充后的提示词)
type PromptInstance struct {
	ID           string                 `json:"id" gorm:"primaryKey"`
	TemplateID   string                 `json:"template_id" gorm:"index"`
	TemplateName string                 `json:"template_name"`

	// 填充后的内容
	RenderedContent string                 `json:"rendered_content" gorm:"type:text"`
	VariablesUsed   JSONMap                `json:"variables_used" gorm:"type:json"`

	// 关联
	DialogueID  string    `json:"dialogue_id" gorm:"index"`
	MessageID   string    `json:"message_id" gorm:"index"`

	CreatedAt   time.Time `json:"created_at"`
}

// BeforeCreate 创建前钩子
func (t *PromptTemplate) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	return nil
}

// BeforeCreate 创建前钩子
func (i *PromptInstance) BeforeCreate(tx *gorm.DB) error {
	if i.ID == "" {
		i.ID = uuid.New().String()
	}
	return nil
}

// TableName 指定表名
func (PromptTemplate) TableName() string {
	return "prompt_templates"
}

// TableName 指定表名
func (PromptInstance) TableName() string {
	return "prompt_instances"
}

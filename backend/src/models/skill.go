package models

import "time"

// Skill 技能模型
type Skill struct {
	ID                 string    `json:"id" gorm:"primaryKey"`
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	Category           string    `json:"category"`
	Version            string    `json:"version"`
	Author             string    `json:"author"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	Enabled            bool      `json:"enabled"`
	Config             JSONMap   `json:"config" gorm:"type:json"`
	Triggers           JSONSlice `json:"triggers" gorm:"type:json"`                     // 触发关键词: ["翻译", "translate"]
	SystemPromptOverride string `json:"system_prompt_override" gorm:"type:text"`        // 覆盖系统提示
	Tools              JSONSlice `json:"tools" gorm:"type:json"`                        // 所需工具: ["web_search", "code_execute"]
	ModelPreference    string    `json:"model_preference"`                              // 偏好模型标签: "code", "creative"
	Builtin            bool      `json:"builtin" gorm:"default:false"`                  // 是否内置技能
	
	// SKILL.md 标准字段扩展
	AllowedTools       JSONSlice `json:"allowed_tools" gorm:"type:json"`                // 允许的工具列表
	InstructionBody    string    `json:"instruction_body" gorm:"type:text"`             // Markdown 指令正文
	SourceFormat       string    `json:"source_format"`                                 // 来源格式: "skill_md", "json"
	SourcePath         string    `json:"source_path"`                                   // 来源文件路径
	Tags               JSONSlice `json:"tags" gorm:"type:json"`                         // 标签列表
}

// SkillParameter 技能参数模型
type SkillParameter struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	SkillID     string    `json:"skill_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Type        string    `json:"type"`
	Required    bool      `json:"required"`
	Default     *JSONAny  `json:"default" gorm:"type:json"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SkillExecution 技能执行模型
type SkillExecution struct {
	ID         string    `json:"id" gorm:"primaryKey"`
	SkillID    string    `json:"skill_id"`
	SkillName  string    `json:"skill_name"`
	Parameters JSONMap   `json:"parameters" gorm:"type:json"`
	Status     string    `json:"status"` // pending, running, completed, failed
	Result     *JSONAny  `json:"result" gorm:"type:json"`
	Error      string    `json:"error"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
}

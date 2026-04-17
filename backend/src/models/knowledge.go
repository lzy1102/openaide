package models

import "time"

// Knowledge 知识条目
type Knowledge struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	Title       string    `json:"title"`                       // 标题
	Content     string    `json:"content"`                     // 内容
	Summary     string    `json:"summary"`                     // 摘要
	CategoryID  string    `json:"category_id"`                 // 分类ID
	Source      string    `json:"source"`                      // 来源（dialogue/document/web）
	SourceID    string    `json:"source_id"`                   // 来源ID
	Embedding   []float64 `json:"embedding" gorm:"type:json"`  // 向量（1536维）
	Confidence  float64   `json:"confidence"`                  // 置信度
	AccessCount int       `json:"access_count"`                // 访问次数
	UserID      string    `json:"user_id"`                     // 用户ID
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// KnowledgeCategory 知识分类
type KnowledgeCategory struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	ParentID    string    `json:"parent_id"`
	UserID      string    `json:"user_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// KnowledgeTag 知识标签
type KnowledgeTag struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

// KnowledgeTagRelation 知识-标签关联
type KnowledgeTagRelation struct {
	ID          string `json:"id" gorm:"primaryKey"`
	KnowledgeID string `json:"knowledge_id"`
	TagID       string `json:"tag_id"`
}

// Document 导入的文档
type Document struct {
	ID         string    `json:"id" gorm:"primaryKey"`
	Title      string    `json:"title"`
	FileType   string    `json:"file_type"`   // txt, md, pdf, docx
	FilePath   string    `json:"file_path"`
	Content    string    `json:"content" gorm:"type:text"`
	ChunkCount int       `json:"chunk_count"` // 分块数量
	Status     string    `json:"status"`      // pending, processing, completed, failed
	UserID     string    `json:"user_id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

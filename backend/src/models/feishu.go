package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// FeishuUser 飞书用户
type FeishuUser struct {
	ID           string    `json:"id" gorm:"primaryKey;size:64"`
	OpenID       string    `json:"open_id" gorm:"size:64;uniqueIndex;not null"`
	UnionID      string    `json:"union_id" gorm:"size:64"`
	Name         string    `json:"name" gorm:"size:128"`
	Avatar       string    `json:"avatar" gorm:"size:512"`
	ChatType     string    `json:"chat_type" gorm:"size:32;default:p2p"` // p2p 或 group
	LastActiveAt time.Time `json:"last_active_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (FeishuUser) TableName() string {
	return "feishu_users"
}

func (u *FeishuUser) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now
	return nil
}

// FeishuSession 飞书会话
type FeishuSession struct {
	ID           string    `json:"id" gorm:"primaryKey;size:64"`
	SessionKey   string    `json:"session_key" gorm:"size:128;uniqueIndex;not null"` // p2p:{open_id} 或 group:{chat_id}
	DialogueID   string    `json:"dialogue_id" gorm:"size:64;not null"`
	ModelID      string    `json:"model_id" gorm:"size:64"`
	MessageCount int       `json:"message_count" gorm:"default:0"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (FeishuSession) TableName() string {
	return "feishu_sessions"
}

func (s *FeishuSession) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	now := time.Now()
	s.CreatedAt = now
	s.UpdatedAt = now
	return nil
}

// FeishuMessageLog 飞书消息日志
type FeishuMessageLog struct {
	ID        string    `json:"id" gorm:"primaryKey;size:64"`
	MessageID string    `json:"message_id" gorm:"size:64;index"`
	ChatID    string    `json:"chat_id" gorm:"size:64;index"`
	Content   string    `json:"content" gorm:"type:text"`
	Direction string    `json:"direction" gorm:"size:16"` // inbound 或 outbound
	Status    string    `json:"status" gorm:"size:32;default:success"` // success, error
	Duration  int64     `json:"duration"` // 处理耗时(ms)
	CreatedAt time.Time `json:"created_at"`
}

func (FeishuMessageLog) TableName() string {
	return "feishu_message_logs"
}

func (l *FeishuMessageLog) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	l.CreatedAt = time.Now()
	return nil
}

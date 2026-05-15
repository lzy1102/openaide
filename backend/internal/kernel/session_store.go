package kernel

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SessionStoreAdapter 内存会话存储（用于新内核）
type SessionStoreAdapter struct {
	sessions map[string]*Session
}

// NewSessionStoreAdapter 创建内存会话存储
func NewSessionStoreAdapter() *SessionStoreAdapter {
	return &SessionStoreAdapter{
		sessions: make(map[string]*Session),
	}
}

// Create 创建会话
func (s *SessionStoreAdapter) Create(ctx context.Context, projectID, userID string) (*Session, error) {
	session := &Session{
		ID:        uuid.New().String(),
		ProjectID: projectID,
		UserID:    userID,
		Messages:  make([]Message, 0),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	s.sessions[session.ID] = session
	return session, nil
}

// Get 获取会话
func (s *SessionStoreAdapter) Get(ctx context.Context, sessionID string) (*Session, error) {
	if session, ok := s.sessions[sessionID]; ok {
		return session, nil
	}
	return nil, fmt.Errorf("session not found: %s", sessionID)
}

// Update 更新会话
func (s *SessionStoreAdapter) Update(ctx context.Context, session *Session) error {
	s.sessions[session.ID] = session
	return nil
}

// List 列出会话
func (s *SessionStoreAdapter) List(ctx context.Context, projectID, userID string, limit int) ([]*Session, error) {
	var result []*Session
	for _, session := range s.sessions {
		if (projectID == "" || session.ProjectID == projectID) &&
			(userID == "" || session.UserID == userID) {
			result = append(result, session)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

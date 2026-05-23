package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// FileSessionStore 文件持久化会话存储 — 死机/重启后可恢复
type FileSessionStore struct {
	mu       sync.RWMutex
	dataDir  string
	sessions map[string]*Session // 内存索引
}

// NewFileSessionStore 创建文件会话存储
func NewFileSessionStore(dataDir string) (*FileSessionStore, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create session dir failed: %w", err)
	}

	s := &FileSessionStore{
		dataDir:  dataDir,
		sessions: make(map[string]*Session),
	}

	// 恢复已有会话
	s.recover()
	return s, nil
}

// SessionStoreAdapter 内存会话存储（兼容旧代码，重启丢失）
type SessionStoreAdapter struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewSessionStoreAdapter 创建内存会话存储
func NewSessionStoreAdapter() *SessionStoreAdapter {
	return &SessionStoreAdapter{
		sessions: make(map[string]*Session),
	}
}

// ============ FileSessionStore 实现（持久化） ============

func (s *FileSessionStore) Create(ctx context.Context, projectID, userID string) (*Session, error) {
	session := &Session{
		ID:        uuid.New().String(),
		ProjectID: projectID,
		UserID:    userID,
		Messages:  make([]Message, 0),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  make(map[string]interface{}),
	}

	s.mu.Lock()
	s.sessions[session.ID] = session
	s.mu.Unlock()

	if err := s.save(session); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *FileSessionStore) Get(ctx context.Context, sessionID string) (*Session, error) {
	s.mu.RLock()
	session, ok := s.sessions[sessionID]
	s.mu.RUnlock()

	if ok {
		return session, nil
	}

	// 尝试从磁盘加载（可能在其他实例创建或索引未加载）
	return s.load(sessionID)
}

func (s *FileSessionStore) Update(ctx context.Context, session *Session) error {
	session.UpdatedAt = time.Now()

	s.mu.Lock()
	s.sessions[session.ID] = session
	s.mu.Unlock()

	return s.save(session)
}

func (s *FileSessionStore) List(ctx context.Context, projectID, userID string, limit, offset int) ([]*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var matched []*Session
	for _, session := range s.sessions {
		if (projectID == "" || session.ProjectID == projectID) &&
			(userID == "" || session.UserID == userID) {
			matched = append(matched, session)
		}
	}

	if offset >= len(matched) {
		return nil, nil
	}
	matched = matched[offset:]

	if limit > 0 && len(matched) > limit {
		matched = matched[:limit]
	}
	return matched, nil
}

// Count 返回会话总数
func (s *FileSessionStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

// Delete 删除会话
func (s *FileSessionStore) Delete(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()

	path := s.sessionPath(sessionID)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove session file failed: %w", err)
	}
	return nil
}

// ============ 持久化 ============

func (s *FileSessionStore) sessionPath(id string) string {
	return filepath.Join(s.dataDir, id+".json")
}

// CleanupOldSessions 删除超过 ttl 的旧会话
func (s *FileSessionStore) CleanupOldSessions(ctx context.Context, ttl time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-ttl)
	var deleted int
	for id, session := range s.sessions {
		if session.UpdatedAt.Before(cutoff) {
			os.Remove(s.sessionPath(id))
			delete(s.sessions, id)
			deleted++
		}
	}
	return deleted, nil
}

func (s *FileSessionStore) save(session *Session) error {
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.sessionPath(session.ID), data, 0644)
}

func (s *FileSessionStore) load(sessionID string) (*Session, error) {
	path := s.sessionPath(sessionID)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.sessions[session.ID] = &session
	s.mu.Unlock()

	return &session, nil
}

// recover 从磁盘恢复所有会话（启动时调用）
func (s *FileSessionStore) recover() {
	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := entry.Name()[:len(entry.Name())-5] // remove .json
		session, err := s.load(id)
		if err == nil {
			s.sessions[id] = session
		}
	}
}

// ============ SessionStoreAdapter 实现（内存，兼容旧代码） ============

func (s *SessionStoreAdapter) Create(ctx context.Context, projectID, userID string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
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

func (s *SessionStoreAdapter) Get(ctx context.Context, sessionID string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if session, ok := s.sessions[sessionID]; ok {
		return session, nil
	}
	return nil, fmt.Errorf("session not found: %s", sessionID)
}

func (s *SessionStoreAdapter) Update(ctx context.Context, session *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
	return nil
}

func (s *SessionStoreAdapter) Delete(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
	return nil
}

func (s *SessionStoreAdapter) List(ctx context.Context, projectID, userID string, limit, offset int) ([]*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var matched []*Session
	for _, session := range s.sessions {
		if (projectID == "" || session.ProjectID == projectID) &&
			(userID == "" || session.UserID == userID) {
			matched = append(matched, session)
		}
	}

	if offset >= len(matched) {
		return nil, nil
	}
	matched = matched[offset:]

	if limit > 0 && len(matched) > limit {
		matched = matched[:limit]
	}
	return matched, nil
}

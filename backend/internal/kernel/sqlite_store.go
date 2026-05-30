package kernel

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteSessionStore persists sessions in a SQLite database.
// Thread-safe; all exported methods use the mutex.
type SQLiteSessionStore struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewSQLiteSessionStore opens (or creates) a SQLite session database.
// The path is the file path to the SQLite database file.
func NewSQLiteSessionStore(path string) (*SQLiteSessionStore, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite serializes writes

	store := &SQLiteSessionStore{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	slog.Info("SQLite session store opened", "path", path)
	return store, nil
}

func (s *SQLiteSessionStore) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			title TEXT DEFAULT '',
			messages_json TEXT NOT NULL DEFAULT '[]',
			metadata_json TEXT DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_sessions_project_user ON sessions(project_id, user_id);
		CREATE INDEX IF NOT EXISTS idx_sessions_updated ON sessions(updated_at);
	`)
	return err
}

// Create inserts a new session.
func (s *SQLiteSessionStore) Create(ctx context.Context, projectID, userID string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session := &Session{
		ID:        NewSessionID(),
		ProjectID: projectID,
		UserID:    userID,
		Messages:  []Message{},
		Metadata:  make(map[string]interface{}),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	msgJSON, _ := json.Marshal(session.Messages)
	metaJSON, _ := json.Marshal(session.Metadata)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, project_id, user_id, title, messages_json, metadata_json, created_at, updated_at)
		 VALUES (?, ?, ?, '', ?, ?, ?, ?)`,
		session.ID, projectID, userID, string(msgJSON), string(metaJSON),
		session.CreatedAt.Format(time.RFC3339), session.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return session, nil
}

// Get retrieves a session by ID.
func (s *SQLiteSessionStore) Get(ctx context.Context, sessionID string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var (
		id, projectID, userID, title, msgJSON, metaJSON string
		createdStr, updatedStr                           string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, user_id, title, messages_json, metadata_json, created_at, updated_at
		 FROM sessions WHERE id = ?`, sessionID,
	).Scan(&id, &projectID, &userID, &title, &msgJSON, &metaJSON, &createdStr, &updatedStr)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	return s.scanSession(id, projectID, userID, title, msgJSON, metaJSON, createdStr, updatedStr)
}

// Update saves session changes to the database.
func (s *SQLiteSessionStore) Update(ctx context.Context, session *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session.UpdatedAt = time.Now()
	msgJSON, _ := json.Marshal(session.Messages)
	metaJSON, _ := json.Marshal(session.Metadata)
	title := ""
	if t, ok := session.Metadata["title"]; ok {
		title, _ = t.(string)
	}

	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET title = ?, messages_json = ?, metadata_json = ?, updated_at = ?
		 WHERE id = ?`,
		title, string(msgJSON), string(metaJSON), session.UpdatedAt.Format(time.RFC3339), session.ID,
	)
	return err
}

// List returns sessions for a project/user, ordered by most recently updated.
func (s *SQLiteSessionStore) List(ctx context.Context, projectID, userID string, limit, offset int) ([]*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, user_id, title, messages_json, metadata_json, created_at, updated_at
		 FROM sessions WHERE project_id = ? AND user_id = ?
		 ORDER BY updated_at DESC LIMIT ? OFFSET ?`,
		projectID, userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		var id, pid, uid, title, msgJSON, metaJSON, createdStr, updatedStr string
		if err := rows.Scan(&id, &pid, &uid, &title, &msgJSON, &metaJSON, &createdStr, &updatedStr); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sess, err := s.scanSession(id, pid, uid, title, msgJSON, metaJSON, createdStr, updatedStr)
		if err != nil {
			slog.Warn("skipping corrupt session", "id", id, "error", err)
			continue
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

// Delete removes a session.
func (s *SQLiteSessionStore) Delete(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, sessionID)
	return err
}

// Close closes the database connection.
func (s *SQLiteSessionStore) Close() error {
	return s.db.Close()
}

// CleanupOldSessions removes sessions older than the given duration.
func (s *SQLiteSessionStore) CleanupOldSessions(ctx context.Context, maxAge time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-maxAge).Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE updated_at < ?`, cutoff,
	)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	if n > 0 {
		slog.Info("SQLite session cleanup", "removed", n, "older_than", cutoff)
	}
	return n, nil
}

// Search finds sessions whose title or messages contain the query string.
func (s *SQLiteSessionStore) Search(ctx context.Context, projectID, query string, limit int) ([]*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, user_id, title, messages_json, metadata_json, created_at, updated_at
		 FROM sessions WHERE project_id = ? AND (title LIKE ? OR messages_json LIKE ?)
		 ORDER BY updated_at DESC LIMIT ?`,
		projectID, "%"+query+"%", "%"+query+"%", limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		var id, pid, uid, title, msgJSON, metaJSON, createdStr, updatedStr string
		if err := rows.Scan(&id, &pid, &uid, &title, &msgJSON, &metaJSON, &createdStr, &updatedStr); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sess, err := s.scanSession(id, pid, uid, title, msgJSON, metaJSON, createdStr, updatedStr)
		if err != nil {
			continue
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

// Count returns the total number of sessions.
func (s *SQLiteSessionStore) Count(ctx context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&n)
	return n, err
}

// ── Helpers ──────────────────────────────────────────────

func (s *SQLiteSessionStore) scanSession(id, projectID, userID, title, msgJSON, metaJSON, createdStr, updatedStr string) (*Session, error) {
	var messages []Message
	if err := json.Unmarshal([]byte(msgJSON), &messages); err != nil {
		messages = []Message{}
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(metaJSON), &metadata); err != nil {
		metadata = make(map[string]interface{})
	}
	if title != "" {
		metadata["title"] = title
	}

	createdAt, _ := time.Parse(time.RFC3339, createdStr)
	updatedAt, _ := time.Parse(time.RFC3339, updatedStr)

	return &Session{
		ID:        id,
		ProjectID: projectID,
		UserID:    userID,
		Messages:  messages,
		Metadata:  metadata,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

// NewSessionID generates a short unique session ID.
func NewSessionID() string {
	var sb strings.Builder
	sb.Grow(8)
	// Simple hex-based ID from nanosecond time
	n := time.Now().UnixNano()
	for i := 0; i < 8; i++ {
		sb.WriteByte("0123456789abcdef"[n&0xf])
		n >>= 4
	}
	return sb.String()
}

// Ensure SQLiteSessionStore implements SessionStore.
var _ SessionStore = (*SQLiteSessionStore)(nil)

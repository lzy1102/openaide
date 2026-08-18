package kernel

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	_ "modernc.org/sqlite"

	"openaide/backend/core/actor"
)

// SessionActor is a CSP-style session store backed by SQLite.
// All session data lives in a single goroutine — zero locks.
// External callers communicate via channels through the Actor.
type SessionActor struct {
	super *actor.Actor
	db    *sql.DB
}

// NewSessionActor creates and starts a session actor.
func NewSessionActor(path string) (*SessionActor, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	a := &SessionActor{
		super: actor.NewActor(256),
		db:    db,
	}

	// Run migration synchronously before any commands
	a.super.Send(func() {
		a.migrate()
	})

	slog.Info("Session actor started", "path", path)
	return a, nil
}

// ── SessionStore interface ──────────────────────────────────

func (a *SessionActor) Create(ctx context.Context, projectID, userID string) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}
	var session *Session
	var createErr error
	a.super.Send(func() {
		session = &Session{
			ID:        NewSessionID(),
			ProjectID: projectID,
			UserID:    userID,
			Messages:  []Message{},
			Metadata:  make(map[string]interface{}),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		msgJSON, err := json.Marshal(session.Messages)
		if err != nil {
			createErr = fmt.Errorf("marshal messages: %w", err)
			return
		}
		metaJSON, err := json.Marshal(session.Metadata)
		if err != nil {
			createErr = fmt.Errorf("marshal metadata: %w", err)
			return
		}
		_, err = a.db.ExecContext(ctx,
			`INSERT INTO sessions (id, project_id, user_id, title, messages_json, metadata_json, created_at, updated_at)
			 VALUES (?, ?, ?, '', ?, ?, ?, ?)`,
			session.ID, projectID, userID, string(msgJSON), string(metaJSON),
			session.CreatedAt.Format(time.RFC3339), session.UpdatedAt.Format(time.RFC3339))
		if err != nil {
			createErr = fmt.Errorf("session actor create: %w", err)
		}
	})
	if createErr != nil {
		return nil, createErr
	}
	return session, nil
}

func (a *SessionActor) Get(ctx context.Context, sessionID string) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}
	var session *Session
	a.super.Send(func() {
		session = a.scanOne(ctx, sessionID)
	})
	if session == nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	return session, nil
}

func (a *SessionActor) Update(ctx context.Context, session *Session) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}
	var updateErr error
	a.super.Send(func() {
		session.UpdatedAt = time.Now()
		msgJSON, err := json.Marshal(session.Messages)
		if err != nil {
			updateErr = fmt.Errorf("marshal messages: %w", err)
			return
		}
		metaJSON, err := json.Marshal(session.Metadata)
		if err != nil {
			updateErr = fmt.Errorf("marshal metadata: %w", err)
			return
		}
		title := ""
		if t, ok := session.Metadata["title"]; ok {
			title, _ = t.(string)
		}
		_, err = a.db.ExecContext(ctx,
			`UPDATE sessions SET title=?, messages_json=?, metadata_json=?, updated_at=? WHERE id=?`,
			title, string(msgJSON), string(metaJSON), session.UpdatedAt.Format(time.RFC3339), session.ID)
		if err != nil {
			updateErr = fmt.Errorf("session actor update: %w", err)
		}
	})
	return updateErr
}

func (a *SessionActor) List(ctx context.Context, projectID, userID string, limit, offset int) ([]*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}
	if limit <= 0 {
		limit = 50
	}
	var result []*Session
	a.super.Send(func() {
		rows, err := a.db.QueryContext(ctx,
			`SELECT id, project_id, user_id, title, messages_json, metadata_json, created_at, updated_at
			 FROM sessions WHERE project_id=? AND user_id=?
			 ORDER BY updated_at DESC LIMIT ? OFFSET ?`,
			projectID, userID, limit, offset)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			s := scanSessionRow(rows)
			if s != nil {
				result = append(result, s)
			}
		}
	})
	return result, nil
}

func (a *SessionActor) Delete(ctx context.Context, sessionID string) error {
	a.super.Send(func() {
		a.db.ExecContext(ctx, `DELETE FROM sessions WHERE id=?`, sessionID)
	})
	return nil
}

// ── Extra methods ──────────────────────────────────────────

func (a *SessionActor) CleanupOldSessions(ctx context.Context, maxAge time.Duration) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("context cancelled: %w", err)
	}
	var removed int
	a.super.Send(func() {
		cutoff := time.Now().Add(-maxAge).Format(time.RFC3339)
		r, err := a.db.ExecContext(ctx, `DELETE FROM sessions WHERE updated_at < ?`, cutoff)
		if err == nil {
			n, _ := r.RowsAffected()
			removed = int(n)
		}
	})
	if removed > 0 {
		slog.Info("Session actor cleanup", "removed", removed)
	}
	return removed, nil
}

// Search finds sessions by content query.
func (a *SessionActor) Search(ctx context.Context, projectID, query string, limit int) ([]*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}
	if limit <= 0 {
		limit = 20
	}
	var result []*Session
	a.super.Send(func() {
		rows, err := a.db.QueryContext(ctx,
			`SELECT id, project_id, user_id, title, messages_json, metadata_json, created_at, updated_at
			 FROM sessions WHERE project_id=? AND (title LIKE ? OR messages_json LIKE ?)
			 ORDER BY updated_at DESC LIMIT ?`,
			projectID, "%"+query+"%", "%"+query+"%", limit)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			s := scanSessionRow(rows)
			if s != nil {
				result = append(result, s)
			}
		}
	})
	return result, nil
}

// Count returns total sessions.
func (a *SessionActor) Count(ctx context.Context) int {
	var n int
	a.super.Send(func() {
		a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&n)
	})
	return n
}

// Stop gracefully shuts down the actor and closes the database.
func (a *SessionActor) Stop() error {
	a.super.Stop()
	return a.db.Close()
}

// ── Internal (called inside actor goroutine) ──────────────

func (a *SessionActor) migrate() {
	a.db.Exec(`
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
		CREATE INDEX IF NOT EXISTS idx_actor_sessions_project_user ON sessions(project_id, user_id);
		CREATE INDEX IF NOT EXISTS idx_actor_sessions_updated ON sessions(updated_at);
	`)
}

func (a *SessionActor) scanOne(ctx context.Context, id string) *Session {
	row := a.db.QueryRowContext(ctx,
		`SELECT id, project_id, user_id, title, messages_json, metadata_json, created_at, updated_at
		 FROM sessions WHERE id=?`, id)
	s := scanSessionRow(row)
	if s != nil {
		return s.SafeCopy()
	}
	return nil
}

func scanSessionRow(scanner interface {
	Scan(dest ...interface{}) error
}) *Session {
	var id, projectID, userID, title, msgJSON, metaJSON, createdStr, updatedStr string
	if err := scanner.Scan(&id, &projectID, &userID, &title, &msgJSON, &metaJSON, &createdStr, &updatedStr); err != nil {
		return nil
	}
	var messages []Message
	json.Unmarshal([]byte(msgJSON), &messages)
	if messages == nil {
		messages = []Message{}
	}
	var metadata map[string]interface{}
	json.Unmarshal([]byte(metaJSON), &metadata)
	if metadata == nil {
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
	}
}

// ── Interface check ────────────────────────────────────────

// NewSessionID generates a short hex session ID.
func NewSessionID() string {
	n := time.Now().UnixNano()
	const hex = "0123456789abcdef"
	var b [8]byte
	for i := range b {
		b[i] = hex[n&0xf]
		n >>= 4
	}
	return string(b[:])
}

var _ SessionStore = (*SessionActor)(nil)

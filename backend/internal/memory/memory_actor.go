package memory

import (
	"context"
	"database/sql"
	"encoding/json"

	"openaide/backend/internal/kernel"

	_ "modernc.org/sqlite"
)

// MemoryActor is a CSP-style memory store backed by SQLite.
// All data lives in a single goroutine — zero locks.
type MemoryActor struct {
	super    *kernel.Actor
	embedder kernel.Embedder
	db       *sql.DB
}

// NewMemoryActor creates and starts a memory actor.
func NewMemoryActor(path string) (*MemoryActor, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	a := &MemoryActor{
		super: kernel.NewActor(64),
		db:    db,
	}
	a.super.Send(func() { a.migrate() })
	return a, nil
}

func (a *MemoryActor) SetEmbedder(e kernel.Embedder) {
	a.super.Send(func() { a.embedder = e })
}

func (a *MemoryActor) migrate() {
	a.db.Exec(`CREATE TABLE IF NOT EXISTS memory_items (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		content TEXT NOT NULL DEFAULT '',
		level INTEGER DEFAULT 0,
		embedding TEXT DEFAULT '[]',
		created_at TEXT NOT NULL DEFAULT ''
	)`)
	a.db.Exec(`CREATE INDEX IF NOT EXISTS idx_memory_session ON memory_items(session_id)`)
}

// Save stores messages. Implements kernel.Memory.
func (a *MemoryActor) Save(ctx context.Context, sessionID string, messages []kernel.Message) error {
	for _, msg := range messages {
		item := &MemoryItem{
			ID:        kernel.NewSessionID(),
			SessionID: sessionID,
			Content:   msg.Content,
			Level:     LevelWorking,
		}
		if a.embedder != nil && a.embedder.Dimension() > 0 {
			if vec, err := a.embedder.Embed(ctx, msg.Content); err == nil && len(vec) > 0 {
				item.Embedding = vec
			}
		}
		a.super.Send(func() {
			embJSON, _ := json.Marshal(item.Embedding)
			a.db.ExecContext(ctx,
				`INSERT INTO memory_items (id, session_id, content, level, embedding, created_at) VALUES (?, ?, ?, ?, ?, datetime('now'))`,
				item.ID, item.SessionID, item.Content, int(item.Level), string(embJSON))
		})
	}
	return nil
}

// Load loads messages for a session. Implements kernel.Memory.
func (a *MemoryActor) Load(ctx context.Context, sessionID string, limit int) ([]kernel.Message, error) {
	var messages []kernel.Message
	a.super.Send(func() {
		q := `SELECT content FROM memory_items WHERE session_id=? ORDER BY created_at DESC`
		if limit > 0 {
			q += ` LIMIT ?`
			rows, _ := a.db.QueryContext(ctx, q, sessionID, limit)
			if rows != nil {
				defer rows.Close()
				for rows.Next() {
					var content string
					rows.Scan(&content)
					messages = append(messages, kernel.Message{Role: "assistant", Content: content})
				}
			}
		} else {
			rows, _ := a.db.QueryContext(ctx, q, sessionID)
			if rows != nil {
				defer rows.Close()
				for rows.Next() {
					var content string
					rows.Scan(&content)
					messages = append(messages, kernel.Message{Role: "assistant", Content: content})
				}
			}
		}
	})
	return messages, nil
}

// Search finds matching memories. Implements kernel.Memory.
func (a *MemoryActor) Search(ctx context.Context, query string, limit int) ([]kernel.Message, float64, error) {
	var queryVec []float32
	if a.embedder != nil {
		queryVec, _ = a.embedder.Embed(ctx, query)
	}

	var messages []kernel.Message
	var bestScore float64
	a.super.Send(func() {
		rows, _ := a.db.QueryContext(ctx, `SELECT content, embedding FROM memory_items`)
		if rows == nil {
			return
		}
		defer rows.Close()
		type entry struct {
			msg   kernel.Message
			score float64
		}
		var entries []entry
		for rows.Next() {
			var content, embJSON string
			rows.Scan(&content, &embJSON)
			var emb []float32
			json.Unmarshal([]byte(embJSON), &emb)
			score := float64(0)
			if len(queryVec) > 0 && len(emb) == len(queryVec) {
				score = kernel.CosineSimilarity(queryVec, emb)
				if score < 0.5 {
					continue
				}
			}
			entries = append(entries, entry{kernel.Message{Role: "assistant", Content: content}, score})
		}
		for i := 0; i < len(entries); i++ {
			for j := i + 1; j < len(entries); j++ {
				if entries[j].score > entries[i].score {
					entries[i], entries[j] = entries[j], entries[i]
				}
			}
		}
		if limit <= 0 {
			limit = 10
		}
		for i := 0; i < len(entries) && len(messages) < limit; i++ {
			messages = append(messages, entries[i].msg)
			if entries[i].score > bestScore {
				bestScore = entries[i].score
			}
		}
	})
	return messages, bestScore, nil
}

// Compress compacts old memory items.
func (a *MemoryActor) Compress(ctx context.Context, sessionID string) error {
	a.super.Send(func() {
		a.db.ExecContext(ctx,
			`DELETE FROM memory_items WHERE id IN (SELECT id FROM memory_items WHERE session_id=? ORDER BY created_at ASC LIMIT max(0, (SELECT COUNT(*)-20 FROM memory_items WHERE session_id=?)))`,
			sessionID, sessionID)
	})
	return nil
}

// Stop shuts down the actor.
func (a *MemoryActor) Stop() {
	a.super.Stop()
	a.db.Close()
}

var _ kernel.Memory = (*MemoryActor)(nil)

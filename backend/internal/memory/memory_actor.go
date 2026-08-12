package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"openaide/backend/internal/kernel"
	"openaide/backend/internal/kernel/actor"
	"openaide/backend/internal/rag"

	_ "modernc.org/sqlite"
)

// MemoryActor is a CSP-style memory store backed by SQLite.
// All data lives in a single goroutine — zero locks.
// Level 记忆层级
type Level int

const (
	LevelWorking Level = 0
	LevelShort   Level = 1
	LevelLong    Level = 2
)

// 外部向量库集合名
const (
	CollectionMemory  = "memory"
	CollectionArchive = "archive"
	CollectionCore    = "core"
)

// MemoryItem 记忆条目
type MemoryItem struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Content   string    `json:"content"`
	Role      string    `json:"role"` // user/assistant/system/tool — 历史恢复时保持原角色
	Level     Level     `json:"level"`
	CreatedAt time.Time `json:"created_at"`
}

// MemoryActor is a CSP-style memory store backed by SQLite.
// Semantic retrieval is delegated to an external vector store (rag.Retriever).
type MemoryActor struct {
	super     *actor.Actor
	retriever rag.Retriever
	db        *sql.DB
}

// NewMemoryActor creates and starts a memory actor.
func NewMemoryActor(path string) (*MemoryActor, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	a := &MemoryActor{
		super:     actor.NewActor(64),
		db:        db,
		retriever: rag.NoopRetriever{},
	}
	a.super.Send(func() {
		a.migrate()
	})
	return a, nil
}

// SetRetriever injects the external retrieval backend.
func (a *MemoryActor) SetRetriever(r rag.Retriever) {
	if r == nil {
		r = rag.NoopRetriever{}
	}
	a.super.Send(func() { a.retriever = r })
}

func (a *MemoryActor) migrate() {
	a.db.Exec(`CREATE TABLE IF NOT EXISTS memory_items (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		content TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL DEFAULT 'assistant',
		level INTEGER DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT ''
	)`)
	// 旧库升级:补 role 列(已有数据无角色信息,默认 assistant 与旧行为一致)
	a.db.Exec(`ALTER TABLE memory_items ADD COLUMN role TEXT NOT NULL DEFAULT 'assistant'`)
	a.db.Exec(`CREATE INDEX IF NOT EXISTS idx_memory_session ON memory_items(session_id)`)

	// MemGPT-style archival storage
	a.db.Exec(`CREATE TABLE IF NOT EXISTS memory_archive (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		summary TEXT NOT NULL DEFAULT '',
		messages_json TEXT NOT NULL DEFAULT '[]',
		importance REAL DEFAULT 0.5,
		archived_at TEXT NOT NULL DEFAULT ''
	)`)
	a.db.Exec(`CREATE INDEX IF NOT EXISTS idx_archive_session ON memory_archive(session_id)`)

	// Core facts — persistent knowledge that survives across all sessions
	a.db.Exec(`CREATE TABLE IF NOT EXISTS core_facts (
		id TEXT PRIMARY KEY,
		content TEXT NOT NULL DEFAULT '',
		importance REAL DEFAULT 0.5,
		created_at TEXT NOT NULL DEFAULT '',
		accessed_at TEXT NOT NULL DEFAULT ''
	)`)
}

// Save stores messages locally and indexes them into the external store.
func (a *MemoryActor) Save(ctx context.Context, sessionID string, messages []kernel.Message) error {
	if len(messages) == 0 {
		return nil
	}

	items := make([]*MemoryItem, len(messages))
	docs := make([]rag.Document, 0, len(messages))
	for i, msg := range messages {
		id := kernel.NewSessionID()
		items[i] = &MemoryItem{
			ID:        id,
			SessionID: sessionID,
			Content:   msg.Content,
			Role:      msg.Role,
			Level:     LevelWorking,
		}
		docs = append(docs, rag.Document{
			ID:      id,
			Content: msg.Content,
			Metadata: map[string]string{
				"session_id": sessionID,
			},
		})
	}

	// 外部索引(Noop 时静默返回 nil)
	_ = a.retriever.Index(ctx, CollectionMemory, docs)

	// Single actor dispatch for all inserts
	a.super.Send(func() {
		for _, item := range items {
			a.db.ExecContext(ctx,
				`INSERT INTO memory_items (id, session_id, content, role, level, created_at) VALUES (?, ?, ?, ?, ?, datetime('now'))`,
				item.ID, item.SessionID, item.Content, item.Role, int(item.Level))
		}
	})
	return nil
}

// Load loads messages for a session. Implements kernel.Memory.
// 恢复消息时保留原始 role(旧库无 role 列时回退 assistant)。
func (a *MemoryActor) Load(ctx context.Context, sessionID string, limit int) ([]kernel.Message, error) {
	var messages []kernel.Message
	a.super.Send(func() {
		q := `SELECT content, role FROM memory_items WHERE session_id=? ORDER BY created_at DESC`
		if limit > 0 {
			q += ` LIMIT ?`
			rows, _ := a.db.QueryContext(ctx, q, sessionID, limit)
			if rows != nil {
				defer rows.Close()
				for rows.Next() {
					var content, role string
					rows.Scan(&content, &role)
					if role == "" {
						role = "assistant"
					}
					messages = append(messages, kernel.Message{Role: role, Content: content})
				}
			}
		} else {
			rows, _ := a.db.QueryContext(ctx, q, sessionID)
			if rows != nil {
				defer rows.Close()
				for rows.Next() {
					var content, role string
					rows.Scan(&content, &role)
					if role == "" {
						role = "assistant"
					}
					messages = append(messages, kernel.Message{Role: role, Content: content})
				}
			}
		}
	})
	return messages, nil
}

// Search finds matching memories via the external vector store.
// Returns empty results (not an error) when the store is unavailable.
func (a *MemoryActor) Search(ctx context.Context, query string, limit int) ([]kernel.Message, float64, error) {
	if limit <= 0 {
		limit = 10
	}
	results, err := a.retriever.Search(ctx, CollectionMemory, query, limit)
	if err != nil || len(results) == 0 {
		return nil, 0, nil
	}

	messages := make([]kernel.Message, 0, len(results))
	bestScore := float64(0)
	for _, r := range results {
		messages = append(messages, kernel.Message{Role: "assistant", Content: r.Content})
		if r.Score > bestScore {
			bestScore = r.Score
		}
	}
	return messages, bestScore, nil
}

// Compress compacts old memory items (local + external).
func (a *MemoryActor) Compress(ctx context.Context, sessionID string) error {
	var deletedIDs []string
	a.super.Send(func() {
		rows, err := a.db.QueryContext(ctx,
			`SELECT id FROM memory_items WHERE session_id=? ORDER BY created_at ASC LIMIT max(0, (SELECT COUNT(*)-20 FROM memory_items WHERE session_id=?))`,
			sessionID, sessionID)
		if err == nil {
			for rows.Next() {
				var id string
				if rows.Scan(&id) == nil {
					deletedIDs = append(deletedIDs, id)
				}
			}
			rows.Close()
		}
		for _, id := range deletedIDs {
			a.db.ExecContext(ctx, `DELETE FROM memory_items WHERE id=?`, id)
		}
	})
	if len(deletedIDs) > 0 {
		_ = a.retriever.Delete(ctx, CollectionMemory, deletedIDs)
	}
	return nil
}

// Delete removes a memory item (local + external).
func (a *MemoryActor) Delete(ctx context.Context, id string) {
	a.super.Send(func() {
		a.db.ExecContext(ctx, `DELETE FROM memory_items WHERE id=?`, id)
	})
	_ = a.retriever.Delete(ctx, CollectionMemory, []string{id})
}

// Stop shuts down the actor.
func (a *MemoryActor) Stop() {
	a.super.Stop()
	a.db.Close()
}

// ── MemGPT-style Archival Memory ────────────────────────────

// ArchiveConversation stores a compressed conversation summary locally
// and indexes the summary into the external archive collection.
func (a *MemoryActor) ArchiveConversation(ctx context.Context, sessionID, summary string, messages []kernel.Message, importance float64) error {
	msgJSON, _ := json.Marshal(messages)
	now := time.Now().Format(time.RFC3339)
	archiveID := sessionID + "-archive"

	_ = a.retriever.Index(ctx, CollectionArchive, []rag.Document{{
		ID:      archiveID,
		Content: summary,
		Metadata: map[string]string{
			"session_id": sessionID,
		},
	}})

	a.super.Send(func() {
		a.db.ExecContext(ctx,
			`INSERT OR REPLACE INTO memory_archive (id, session_id, summary, messages_json, importance, archived_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			archiveID, sessionID, summary, string(msgJSON), importance, now)
	})
	return nil
}

// RetrieveArchive searches archived conversations via the external store,
// then fetches the full message list from the local archive table.
func (a *MemoryActor) RetrieveArchive(ctx context.Context, query string, limit int) ([]kernel.Message, float64, error) {
	if limit <= 0 {
		limit = 5
	}
	results, err := a.retriever.Search(ctx, CollectionArchive, query, limit)
	if err != nil || len(results) == 0 {
		return nil, 0, nil
	}

	var allMsgs []kernel.Message
	bestScore := float64(0)
	a.super.Send(func() {
		for _, r := range results {
			var msgJSON string
			err := a.db.QueryRowContext(ctx,
				`SELECT messages_json FROM memory_archive WHERE id=?`, r.ID).Scan(&msgJSON)
			if err != nil {
				continue
			}
			var messages []kernel.Message
			if json.Unmarshal([]byte(msgJSON), &messages) == nil {
				allMsgs = append(allMsgs, messages...)
			}
			if r.Score > bestScore {
				bestScore = r.Score
			}
		}
	})
	return allMsgs, bestScore, nil
}

// ── Core Facts (persistent knowledge) ────────────────────────

// StoreCoreFact persists a key fact locally and indexes it externally.
func (a *MemoryActor) StoreCoreFact(ctx context.Context, content string, importance float64) {
	now := time.Now().Format(time.RFC3339)
	id := fmt.Sprintf("fact_%d", time.Now().UnixNano())

	_ = a.retriever.Index(ctx, CollectionCore, []rag.Document{{
		ID:      id,
		Content: content,
	}})

	a.super.Send(func() {
		a.db.ExecContext(ctx,
			`INSERT OR REPLACE INTO core_facts (id, content, importance, created_at, accessed_at)
			 VALUES (?, ?, ?, ?, ?)`,
			id, content, importance, now, now)
	})
}

// GetCoreFacts retrieves the most relevant core facts via the external store.
func (a *MemoryActor) GetCoreFacts(ctx context.Context, query string, limit int) []string {
	if limit <= 0 {
		limit = 5
	}
	results, err := a.retriever.Search(ctx, CollectionCore, query, limit)
	if err != nil || len(results) == 0 {
		return nil
	}
	facts := make([]string, 0, len(results))
	for _, r := range results {
		facts = append(facts, r.Content)
	}
	return facts
}

var _ kernel.Memory = (*MemoryActor)(nil)

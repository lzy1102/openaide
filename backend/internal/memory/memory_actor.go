package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"openaide/backend/internal/kernel"

	_ "modernc.org/sqlite"
)

// MemoryActor is a CSP-style memory store backed by SQLite.
// All data lives in a single goroutine — zero locks.
// Level 记忆层级
type Level int

const (
	LevelWorking  Level = 0
	LevelShort    Level = 1
	LevelLong     Level = 2
)

// MemoryItem 记忆条目
type MemoryItem struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Content   string    `json:"content"`
	Level     Level     `json:"level"`
	Embedding []float32 `json:"embedding,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// memVec holds an in-memory embedding for fast search.
type memVec struct {
	id  string
	vec []float32
}

// MemoryActor is a CSP-style memory store backed by SQLite with
// an in-memory vector cache. Search is pure in-memory.
const maxMemVectors = 5000

type MemoryActor struct {
	super    *kernel.Actor
	embedder kernel.Embedder
	db       *sql.DB
	cache    []memVec
	embCache map[string][]float32
	embKeys  []string
}

// NewMemoryActor creates and starts a memory actor.
func NewMemoryActor(path string) (*MemoryActor, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	a := &MemoryActor{
		super:    kernel.NewActor(64),
		db:       db,
		embCache: make(map[string][]float32),
	}
	a.super.Send(func() {
		a.migrate()
		a.loadCache()
	})
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

// Save stores messages using batch embedding (one API call for all).
func (a *MemoryActor) Save(ctx context.Context, sessionID string, messages []kernel.Message) error {
	if len(messages) == 0 {
		return nil
	}

	// Batch embed with cache
	var embeddings [][]float32
	if a.embedder != nil && a.embedder.Dimension() > 0 {
		texts := make([]string, len(messages))
		for i, msg := range messages {
			texts[i] = msg.Content
		}
		// Check cache first, only embed uncached texts
		var uncached []int
		for i, t := range texts {
			if v, ok := a.embCache[hashMemKey(t)]; ok {
				if embeddings == nil {
					embeddings = make([][]float32, len(messages))
				}
				embeddings[i] = v
			} else {
				uncached = append(uncached, i)
			}
		}
		if len(uncached) > 0 {
			uncachedTexts := make([]string, len(uncached))
			for i, idx := range uncached {
				uncachedTexts[i] = texts[idx]
			}
			newEmbs, _ := a.embedder.EmbedBatch(ctx, uncachedTexts)
			if embeddings == nil {
				embeddings = make([][]float32, len(messages))
			}
			for i, idx := range uncached {
				if i < len(newEmbs) {
					embeddings[idx] = newEmbs[i]
					// Store in cache
					a.embCache[hashMemKey(texts[idx])] = newEmbs[i]
					a.embKeys = append(a.embKeys, hashMemKey(texts[idx]))
				}
			}
		}
		// LRU evict embeddings
		for len(a.embKeys) > 200 {
			delete(a.embCache, a.embKeys[0])
			a.embKeys = a.embKeys[1:]
		}
	}

	// Build items
	items := make([]*MemoryItem, len(messages))
	for i, msg := range messages {
		vec := []float32(nil)
		if i < len(embeddings) {
			vec = embeddings[i]
		}
		items[i] = &MemoryItem{
			ID:        kernel.NewSessionID(),
			SessionID: sessionID,
			Content:   msg.Content,
			Level:     LevelWorking,
			Embedding: vec,
		}
	}

	// Single actor dispatch for all inserts
	a.super.Send(func() {
		for _, item := range items {
			embJSON, _ := json.Marshal(item.Embedding)
			a.db.ExecContext(ctx,
				`INSERT INTO memory_items (id, session_id, content, level, embedding, created_at) VALUES (?, ?, ?, ?, ?, datetime('now'))`,
				item.ID, item.SessionID, item.Content, int(item.Level), string(embJSON))
			a.cache = append(a.cache, memVec{id: item.ID, vec: item.Embedding})
		}
		// LRU evict
		for len(a.cache) > maxMemVectors {
			oldest := a.cache[0]
			a.db.ExecContext(ctx, `DELETE FROM memory_items WHERE id=?`, oldest.id)
			a.cache = a.cache[1:]
		}
	})
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

// Search finds matching memories using in-memory vector cache.
func (a *MemoryActor) Search(ctx context.Context, query string, limit int) ([]kernel.Message, float64, error) {
	var queryVec []float32
	if a.embedder != nil {
		queryVec, _ = a.embedder.Embed(ctx, query)
	}
	if limit <= 0 {
		limit = 10
	}

	var messages []kernel.Message
	var bestScore float64
	a.super.Send(func() {
		type entry struct {
			id    string
			score float64
		}
		var entries []entry
		for _, dv := range a.cache {
			score := float64(0)
			if len(queryVec) > 0 && len(dv.vec) == len(queryVec) {
				score = kernel.CosineSimilarity(queryVec, dv.vec)
				if score < 0.5 {
					continue
				}
			}
			entries = append(entries, entry{id: dv.id, score: score})
		}
		for i := 0; i < len(entries); i++ {
			for j := i + 1; j < len(entries); j++ {
				if entries[j].score > entries[i].score {
					entries[i], entries[j] = entries[j], entries[i]
				}
			}
		}
		// Fetch content for top results
		for i := 0; i < len(entries) && len(messages) < limit; i++ {
			var content string
			a.db.QueryRowContext(ctx, `SELECT content FROM memory_items WHERE id=?`, entries[i].id).Scan(&content)
			messages = append(messages, kernel.Message{Role: "assistant", Content: content})
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
		// Rebuild cache
		a.loadCacheLocked()
	})
	return nil
}

// Delete removes a memory item.
func (a *MemoryActor) Delete(ctx context.Context, id string) {
	a.super.Send(func() {
		a.db.ExecContext(ctx, `DELETE FROM memory_items WHERE id=?`, id)
		for i, dv := range a.cache {
			if dv.id == id {
				a.cache = append(a.cache[:i], a.cache[i+1:]...)
				break
			}
		}
	})
}

// Stop shuts down the actor.
func (a *MemoryActor) Stop() {
	a.super.Stop()
	a.db.Close()
}

func (a *MemoryActor) loadCache() {
	a.loadCacheLocked()
}

func (a *MemoryActor) loadCacheLocked() {
	rows, err := a.db.Query(`SELECT id, embedding FROM memory_items`)
	if err != nil {
		return
	}
	defer rows.Close()
	a.cache = nil
	for rows.Next() {
		var id, embJSON string
		if err := rows.Scan(&id, &embJSON); err != nil {
			continue
		}
		var emb []float32
		if json.Unmarshal([]byte(embJSON), &emb) == nil {
			a.cache = append(a.cache, memVec{id: id, vec: emb})
		}
	}
}

func hashMemKey(s string) string {
	h := 0
	for _, r := range s {
		h = h*31 + int(r)
	}
	return fmt.Sprintf("%x", h)
}

var _ kernel.Memory = (*MemoryActor)(nil)

package memory

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"

	"openaide/backend/internal/kernel"
)

// MemoryActor is a CSP-style memory store implementing kernel.Memory.
// All data lives in a single goroutine — zero locks.
// Embedding calls run outside the actor.
type MemoryActor struct {
	super    *kernel.Actor
	embedder kernel.Embedder
	dir      string
	items    map[string]*MemoryItem
}

// NewMemoryActor creates and starts a memory actor.
func NewMemoryActor(dir string) (*MemoryActor, error) {
	os.MkdirAll(dir, 0755)
	a := &MemoryActor{
		super: kernel.NewActor(64),
		dir:   dir,
		items: make(map[string]*MemoryItem),
	}
	a.loadFromDisk()
	return a, nil
}

func (a *MemoryActor) SetEmbedder(e kernel.Embedder) {
	a.super.Send(func() { a.embedder = e })
}

// Save stores messages as memory items. Implements kernel.Memory.
func (a *MemoryActor) Save(ctx context.Context, sessionID string, messages []kernel.Message) error {
	for _, msg := range messages {
		item := &MemoryItem{
			ID:        kernel.NewSessionID(),
			SessionID: sessionID,
			Content:   msg.Content,
			Level:     LevelWorking,
		}
		// Embed outside actor
		if a.embedder != nil && a.embedder.Dimension() > 0 {
			if vec, err := a.embedder.Embed(ctx, msg.Content); err == nil && len(vec) > 0 {
				item.Embedding = vec
			}
		}
		a.super.Send(func() {
			a.items[item.ID] = item
			a.saveToDisk(item)
		})
	}
	return nil
}

// Load loads messages for a session. Implements kernel.Memory.
func (a *MemoryActor) Load(ctx context.Context, sessionID string, limit int) ([]kernel.Message, error) {
	var messages []kernel.Message
	a.super.Send(func() {
		var found []*MemoryItem
		for _, item := range a.items {
			if item.SessionID == sessionID {
				found = append(found, item)
			}
		}
		if limit > 0 && len(found) > limit {
			found = found[len(found)-limit:]
		}
		for _, item := range found {
			messages = append(messages, kernel.Message{Role: "assistant", Content: item.Content})
		}
	})
	return messages, nil
}

// Search finds matching memories via embedding. Implements kernel.Memory.
func (a *MemoryActor) Search(ctx context.Context, query string, limit int) ([]kernel.Message, float64, error) {
	var queryVec []float32
	if a.embedder != nil {
		queryVec, _ = a.embedder.Embed(ctx, query)
	}

	var messages []kernel.Message
	var bestScore float64
	a.super.Send(func() {
		type entry struct {
			msg   kernel.Message
			score float64
		}
		var entries []entry
		for _, item := range a.items {
			score := float64(0)
			if len(queryVec) > 0 && len(item.Embedding) == len(queryVec) {
				score = kernel.CosineSimilarity(queryVec, item.Embedding)
				if score < 0.5 { continue }
			}
			entries = append(entries, entry{
				msg:   kernel.Message{Role: "assistant", Content: item.Content},
				score: score,
			})
		}
		// Sort by score descending
		for i := 0; i < len(entries); i++ {
			for j := i + 1; j < len(entries); j++ {
				if entries[j].score > entries[i].score {
					entries[i], entries[j] = entries[j], entries[i]
				}
			}
		}
		if limit <= 0 { limit = 10 }
		for i := 0; i < len(entries) && len(messages) < limit; i++ {
			messages = append(messages, entries[i].msg)
			if entries[i].score > bestScore { bestScore = entries[i].score }
		}
	})
	return messages, bestScore, nil
}

// Compress compacts old memory items.
func (a *MemoryActor) Compress(ctx context.Context, sessionID string) error {
	a.super.Send(func() {
		var items []*MemoryItem
		for _, item := range a.items {
			if item.SessionID == sessionID { items = append(items, item) }
		}
		if len(items) > 20 {
			for i := 0; i < len(items)-20; i++ {
				id := items[i].ID
				delete(a.items, id)
				os.Remove(filepath.Join(a.dir, id+".json"))
			}
		}
	})
	return nil
}

// Stop shuts down the actor.
func (a *MemoryActor) Stop() { a.super.Stop() }

func (a *MemoryActor) saveToDisk(item *MemoryItem) {
	data, _ := json.MarshalIndent(item, "", "  ")
	os.WriteFile(filepath.Join(a.dir, item.ID+".json"), data, 0644)
}

func (a *MemoryActor) loadFromDisk() {
	entries, err := os.ReadDir(a.dir)
	if err != nil { return }
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" { continue }
		data, err := os.ReadFile(filepath.Join(a.dir, e.Name()))
		if err != nil { continue }
		var item MemoryItem
		if json.Unmarshal(data, &item) == nil && item.ID != "" {
			a.items[item.ID] = &item
		}
	}
	slog.Info("Memory actor loaded", "count", len(a.items), "dir", a.dir)
}

var _ kernel.Memory = (*MemoryActor)(nil)

package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"

	"openaide/backend/internal/kernel"

	_ "modernc.org/sqlite"
)

// docVector holds an in-memory embedding for fast search.
type docVector struct {
	id  string
	vec []float32
}

// Actor is a CSP-style knowledge base backed by SQLite with an
// in-memory vector cache. Search is pure in-memory cosine similarity.
type Actor struct {
	super    *kernel.Actor
	embedder kernel.Embedder
	db       *sql.DB
	cache    []docVector // in-memory vector index
}

// NewActor creates and starts a knowledge actor.
func NewActor(path string) (*Actor, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	a := &Actor{
		super: kernel.NewActor(64),
		db:    db,
	}
	a.super.Send(func() {
		a.migrate()
		a.loadCache()
	})
	return a, nil
}

func (a *Actor) SetEmbedder(e kernel.Embedder) {
	a.super.Send(func() { a.embedder = e })
}

// Add stores a document. Embedding runs OUTSIDE the actor.
func (a *Actor) Add(ctx context.Context, title, content, source string, tags []string) (*Document, error) {
	doc := &Document{
		ID:      kernel.NewSessionID(),
		Title:   title,
		Content: content,
		Source:  source,
		Tags:    tags,
	}

	if a.embedder != nil && a.embedder.Dimension() > 0 {
		embedText := title
		if content != "" {
			embedText = title + "\n" + content
		}
		vec, err := a.embedder.Embed(ctx, embedText)
		if err == nil && len(vec) > 0 {
			doc.Embedding = vec
		}
	}

	a.super.Send(func() {
		embJSON, _ := json.Marshal(doc.Embedding)
		tagsJSON, _ := json.Marshal(tags)
		a.db.ExecContext(ctx,
			`INSERT OR REPLACE INTO documents (id, title, content, source, tags, embedding) VALUES (?, ?, ?, ?, ?, ?)`,
			doc.ID, doc.Title, doc.Content, doc.Source, string(tagsJSON), string(embJSON))
		// Update in-memory cache
		a.cache = append(a.cache, docVector{id: doc.ID, vec: doc.Embedding})
	})
	return doc, nil
}

// Search finds matching documents using in-memory vector cache.
// Embedding runs OUTSIDE the actor.
func (a *Actor) Search(ctx context.Context, query string, limit int) ([]*Document, error) {
	queryVec, hasVec := a.embedQuery(ctx, query)
	if limit <= 0 {
		limit = 10
	}

	var results []*Document
	a.super.Send(func() {
		type scored struct {
			id    string
			score float64
		}
		var candidates []scored
		for _, dv := range a.cache {
			score := float64(0.5)
			if hasVec && len(dv.vec) == len(queryVec) {
				score = kernel.CosineSimilarity(queryVec, dv.vec)
			}
			candidates = append(candidates, scored{id: dv.id, score: score})
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })

		for i := 0; i < len(candidates) && len(results) < limit; i++ {
			row := a.db.QueryRowContext(ctx,
				`SELECT id, title, content, source, tags, embedding FROM documents WHERE id=?`,
				candidates[i].id)
			var id, title, content, source, tagsJSON, embJSON string
			if err := row.Scan(&id, &title, &content, &source, &tagsJSON, &embJSON); err != nil {
				continue
			}
			var tags []string
			json.Unmarshal([]byte(tagsJSON), &tags)
			var emb []float32
			json.Unmarshal([]byte(embJSON), &emb)
			results = append(results, &Document{
				ID: id, Title: title, Content: content,
				Source: source, Tags: tags, Embedding: emb,
			})
		}
	})
	return results, nil
}

func (a *Actor) Get(ctx context.Context, id string) *Document {
	var doc *Document
	a.super.Send(func() {
		row := a.db.QueryRowContext(ctx, `SELECT id, title, content, source, tags, embedding FROM documents WHERE id=?`, id)
		var id, title, content, source, tagsJSON, embJSON string
		if err := row.Scan(&id, &title, &content, &source, &tagsJSON, &embJSON); err != nil {
			return
		}
		var emb []float32
		json.Unmarshal([]byte(embJSON), &emb)
		var tags []string
		json.Unmarshal([]byte(tagsJSON), &tags)
		doc = &Document{ID: id, Title: title, Content: content, Source: source, Tags: tags, Embedding: emb}
	})
	return doc
}

func (a *Actor) Delete(ctx context.Context, id string) {
	a.super.Send(func() {
		a.db.ExecContext(ctx, `DELETE FROM documents WHERE id=?`, id)
		// Remove from cache
		for i, dv := range a.cache {
			if dv.id == id {
				a.cache = append(a.cache[:i], a.cache[i+1:]...)
				break
			}
		}
	})
}

func (a *Actor) InjectContext(ctx context.Context, query string, maxTokens int) (string, []string, error) {
	docs, err := a.Search(ctx, query, 3)
	if err != nil {
		return "", nil, err
	}
	var ids []string
	var texts []string
	tokens := 0
	for _, doc := range docs {
		if tokens > maxTokens {
			break
		}
		texts = append(texts, doc.Content)
		ids = append(ids, doc.ID)
		tokens += len([]rune(doc.Content)) / 2
	}
	var sb string
	for _, t := range texts {
		if len([]rune(sb)) > maxTokens*2 {
			break
		}
		sb += t + "\n"
	}
	return sb, ids, nil
}

func (a *Actor) AddKnowledge(ctx context.Context, title, content, source string, tags []string) (string, error) {
	doc, err := a.Add(ctx, title, content, source, tags)
	if err != nil {
		return "", err
	}
	return doc.ID, nil
}

func (a *Actor) SearchKnowledge(ctx context.Context, query string, limit int) ([]kernel.KnowledgeItem, error) {
	docs, err := a.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	items := make([]kernel.KnowledgeItem, len(docs))
	for i, doc := range docs {
		items[i] = kernel.KnowledgeItem{
			Title: doc.Title, Content: doc.Content, Tags: doc.Tags,
		}
	}
	return items, nil
}

func (a *Actor) RecordKnowledgeUsage(ctx context.Context, docIDs []string, qualityScore float64) {}

func (a *Actor) Stop() {
	a.super.Stop()
	a.db.Close()
}

func (a *Actor) migrate() {
	a.db.Exec(`CREATE TABLE IF NOT EXISTS documents (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL DEFAULT '',
		content TEXT NOT NULL DEFAULT '',
		source TEXT DEFAULT '',
		tags TEXT DEFAULT '[]',
		embedding TEXT DEFAULT '[]'
	)`)
	a.db.Exec(`CREATE INDEX IF NOT EXISTS idx_knowledge_title ON documents(title)`)
}

// loadCache loads all embeddings from SQLite into memory for fast search.
func (a *Actor) loadCache() {
	rows, err := a.db.Query(`SELECT id, embedding FROM documents`)
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
			a.cache = append(a.cache, docVector{id: id, vec: emb})
		}
	}
}

func (a *Actor) embedQuery(ctx context.Context, query string) ([]float32, bool) {
	if a.embedder == nil || a.embedder.Dimension() == 0 {
		return nil, false
	}
	vec, err := a.embedder.Embed(ctx, query)
	return vec, err == nil && len(vec) > 0
}

var _ kernel.KnowledgeCollector = (*Actor)(nil)

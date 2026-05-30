package knowledge

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"openaide/backend/internal/kernel"
)

// Actor is a CSP-style knowledge base. All documents live in a single
// goroutine — zero locks. Embedding calls run outside the actor.
type Actor struct {
	super    *kernel.Actor
	embedder kernel.Embedder
	dir      string
	docs     map[string]*Document
}

// NewActor creates and starts a knowledge actor.
func NewActor(dir string) (*Actor, error) {
	os.MkdirAll(dir, 0755)
	a := &Actor{
		super: kernel.NewActor(64),
		dir:   dir,
		docs:  make(map[string]*Document),
	}
	a.loadFromDisk()
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

	// Embed outside actor
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
		a.docs[doc.ID] = doc
		a.saveOne(doc)
	})
	return doc, nil
}

// Search finds matching documents. Embedding runs OUTSIDE the actor.
func (a *Actor) Search(ctx context.Context, query string, limit int) ([]*Document, error) {
	// Embed outside actor
	queryVec, hasVec := a.embedQuery(ctx, query)

	var results []*Document
	a.super.Send(func() {
		if limit <= 0 { limit = 10 }
		type scored struct {
			doc   *Document
			score float64
		}
		var candidates []scored
		for _, doc := range a.docs {
			score := float64(0)
			if hasVec && len(doc.Embedding) == len(queryVec) {
				score = kernel.CosineSimilarity(queryVec, doc.Embedding)
			} else {
				score = 0.5
			}
			candidates = append(candidates, scored{doc, score})
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
		for i := 0; i < len(candidates) && len(results) < limit; i++ {
			results = append(results, candidates[i].doc)
		}
	})
	return results, nil
}

func (a *Actor) Get(ctx context.Context, id string) *Document {
	var doc *Document
	a.super.Send(func() { doc = a.docs[id] })
	return doc
}

func (a *Actor) Delete(ctx context.Context, id string) {
	a.super.Send(func() {
		delete(a.docs, id)
		os.Remove(filepath.Join(a.dir, id+".json"))
	})
}

func (a *Actor) InjectContext(ctx context.Context, query string, maxTokens int) (string, []string, error) {
	docs, err := a.Search(ctx, query, 3)
	if err != nil { return "", nil, err }
	var ids []string
	var texts []string
	tokens := 0
	for _, doc := range docs {
		if tokens > maxTokens { break }
		texts = append(texts, doc.Content)
		ids = append(ids, doc.ID)
		tokens += len([]rune(doc.Content)) / 2
	}
	// Join context texts up to token limit.
	var sb string
	for _, t := range texts {
		if len([]rune(sb)) > maxTokens*2 { break }
		sb += t + "\n"
	}
	return sb, ids, nil
}

// AddKnowledge adds a knowledge entry. Implements kernel.KnowledgeCollector.
func (a *Actor) AddKnowledge(ctx context.Context, title, content, source string, tags []string) (string, error) {
	doc, err := a.Add(ctx, title, content, source, tags)
	if err != nil { return "", err }
	return doc.ID, nil
}

// SearchKnowledge wraps Search for kernel.KnowledgeCollector compatibility.
func (a *Actor) SearchKnowledge(ctx context.Context, query string, limit int) ([]kernel.KnowledgeItem, error) {
	docs, err := a.Search(ctx, query, limit)
	if err != nil { return nil, err }
	items := make([]kernel.KnowledgeItem, len(docs))
	for i, doc := range docs {
		items[i] = kernel.KnowledgeItem{
			Title: doc.Title, Content: doc.Content, Tags: doc.Tags,
		}
	}
	return items, nil
}

// RecordKnowledgeUsage records quality feedback. (no-op for actor)
func (a *Actor) RecordKnowledgeUsage(ctx context.Context, docIDs []string, qualityScore float64) {}

// Stop shuts down the actor.
func (a *Actor) Stop() { a.super.Stop() }

func (a *Actor) embedQuery(ctx context.Context, query string) ([]float32, bool) {
	if a.embedder == nil || a.embedder.Dimension() == 0 {
		return nil, false
	}
	vec, err := a.embedder.Embed(ctx, query)
	return vec, err == nil && len(vec) > 0
}

func (a *Actor) saveOne(doc *Document) {
	data, _ := json.MarshalIndent(doc, "", "  ")
	os.WriteFile(filepath.Join(a.dir, doc.ID+".json"), data, 0644)
}

func (a *Actor) loadFromDisk() {
	entries, err := os.ReadDir(a.dir)
	if err != nil { return }
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" { continue }
		data, err := os.ReadFile(filepath.Join(a.dir, e.Name()))
		if err != nil { continue }
		var doc Document
		if json.Unmarshal(data, &doc) == nil && doc.ID != "" {
			a.docs[doc.ID] = &doc
		}
	}
	slog.Info("Knowledge actor loaded", "count", len(a.docs), "dir", a.dir)
}

// Ensure interface compliance.
var _ kernel.KnowledgeCollector = (*Actor)(nil)

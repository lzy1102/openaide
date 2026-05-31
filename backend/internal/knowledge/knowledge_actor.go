package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"openaide/backend/internal/kernel"

	_ "modernc.org/sqlite"
)

const numProjections = 8 // 2^8 = 256 buckets

// docVector holds an in-memory embedding for fast search.
type docVector struct {
	id  string
	vec []float32
}

// Actor is a CSP-style knowledge base with approximate nearest neighbor search.
// Uses random projection bucketing: O(n/256) search instead of O(n).
const maxCachedVectors = 5000
const maxCachedEmbeddings = 200

type Actor struct {
	super      *kernel.Actor
	embedder   kernel.Embedder
	llm        kernel.LLMProvider // optional: refines knowledge
	db         *sql.DB
	cache      []docVector
	embCache   map[string][]float32 // query hash → embedding, LRU
	embKeys    []string              // LRU order for embCache
	proj       [][]float32           // random projection vectors
	buckets    [][]int               // bucketID → cache indices
	dim        int
}

// SetLLM injects an LLM for knowledge refinement.
func (a *Actor) SetLLM(llm kernel.LLMProvider) { a.super.Send(func() { a.llm = llm }) }

// NewActor creates and starts a knowledge actor.
func NewActor(path string) (*Actor, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	a := &Actor{
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
		a.indexOne(doc.ID, doc.Embedding)
	})
	return doc, nil
}

// Search finds matching documents using bucketed approximate search.
// Falls back to exact search if no results in target bucket.
func (a *Actor) Search(ctx context.Context, query string, limit int) ([]*Document, error) {
	queryVec, _ := a.embedQuery(ctx, query)
	if limit <= 0 {
		limit = 10
	}

	var results []*Document
	a.super.Send(func() {
		candidates := a.searchCandidates(queryVec, limit*4) // oversample
		// Sort and take top
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
		for i := 0; i < len(candidates) && len(results) < limit; i++ {
			row := a.db.QueryRowContext(ctx,
				`SELECT id, title, content, source, tags, embedding FROM documents WHERE id=?`, candidates[i].id)
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

type scoredDoc struct {
	id    string
	score float64
}

// searchCandidates returns scored candidates by searching the target bucket
// and neighboring buckets. Falls back to full scan if bucket is empty.
func (a *Actor) searchCandidates(queryVec []float32, want int) []scoredDoc {
	if len(a.cache) == 0 {
		return nil
	}

	// Determine target bucket
	bucket := a.bucketFor(queryVec)

	// Collect candidates from target bucket + neighbors
	var candidates []scoredDoc
	bucketsToSearch := []int{bucket}
	// Add neighboring buckets (1-bit flips)
	for bit := 0; bit < numProjections && len(bucketsToSearch) < 8; bit++ {
		neighbor := bucket ^ (1 << bit)
		bucketsToSearch = append(bucketsToSearch, neighbor)
	}

	for _, b := range bucketsToSearch {
		if b < 0 || b >= len(a.buckets) {
			continue
		}
		for _, idx := range a.buckets[b] {
			if idx >= len(a.cache) {
				continue
			}
			dv := a.cache[idx]
			score := float64(0.5)
			if len(queryVec) > 0 && len(dv.vec) == len(queryVec) {
				score = kernel.CosineSimilarity(queryVec, dv.vec)
			}
			candidates = append(candidates, scoredDoc{id: dv.id, score: score})
		}
	}

	// Fallback: if bucket search returned too few, do full scan
	if len(candidates) < want && len(a.cache) > len(candidates) {
		candidates = nil
		for _, dv := range a.cache {
			score := float64(0.5)
			if len(queryVec) > 0 && len(dv.vec) == len(queryVec) {
				score = kernel.CosineSimilarity(queryVec, dv.vec)
			}
			candidates = append(candidates, scoredDoc{id: dv.id, score: score})
		}
	}
	return candidates
}

// indexOne adds a document to the in-memory index.
func (a *Actor) indexOne(id string, vec []float32) {
	if len(vec) > 0 {
		// Initialize projections on first embedding
		if a.dim == 0 {
			a.dim = len(vec)
			a.initProjections()
		}
		if len(vec) == a.dim {
			bucket := a.bucketFor(vec)
			idx := len(a.cache)
			a.cache = append(a.cache, docVector{id: id, vec: vec})
			a.buckets[bucket] = append(a.buckets[bucket], idx)
			return
		}
	}
	// No embedding or dimension mismatch — add to cache without bucketing
	a.cache = append(a.cache, docVector{id: id, vec: vec})
	// LRU evict oldest
	for len(a.cache) > maxCachedVectors {
		a.removeFromCache(a.cache[0].id)
	}
}

// bucketFor returns the bucket index for a vector.
func (a *Actor) bucketFor(vec []float32) int {
	if len(a.proj) == 0 {
		return 0
	}
	bucket := 0
	for i, p := range a.proj {
		if dotProduct(vec, p) > 0 {
			bucket |= 1 << i
		}
	}
	return bucket
}

// initProjections creates random projection vectors.
func (a *Actor) initProjections() {
	a.proj = make([][]float32, numProjections)
	for i := range a.proj {
		a.proj[i] = make([]float32, a.dim)
		for j := range a.proj[i] {
			a.proj[i][j] = float32(rand.NormFloat64())
		}
	}
	numBuckets := 1 << numProjections
	a.buckets = make([][]int, numBuckets)
	// Re-index existing cache entries
	for idx, dv := range a.cache {
		if len(dv.vec) == a.dim {
			b := a.bucketFor(dv.vec)
			a.buckets[b] = append(a.buckets[b], idx)
		}
	}
}

// Refine deduplicates, summarizes, and stores knowledge from an agent response.
// Returns the stored document ID, or empty string if filtered out.
func (a *Actor) Refine(ctx context.Context, query, response string, sessionID string) string {
	// Step 1: check for near-duplicate
	if a.embedder != nil && a.embedder.Dimension() > 0 {
		vec, err := a.embedder.Embed(ctx, query)
		if err == nil && len(vec) > 0 {
			var bestID string
			var bestScore float64
			a.super.Send(func() {
				for _, dv := range a.cache {
					if len(dv.vec) == len(vec) {
						s := kernel.CosineSimilarity(vec, dv.vec)
						if s > bestScore {
							bestScore = s
							bestID = dv.id
						}
					}
				}
			})
			if bestScore > 0.85 && bestID != "" {
				// Merge: update existing entry with new response as additional content
				a.super.Send(func() {
					a.db.ExecContext(ctx, `UPDATE documents SET content = content || '\n\n---\n' || ? WHERE id = ?`, response, bestID)
				})
				return bestID
			}
		}
	}

	// Step 2: refine with LLM (if available)
	title := query
	content := response
	shouldStore := true

	if a.llm != nil {
		var result struct {
			Title       string   `json:"title"`
			Facts       []string `json:"facts"`
			Files       []string `json:"files"`
			Errors      []string `json:"errors"`
			Decisions   []string `json:"decisions"`
			ShouldStore bool     `json:"should_store"`
		}
		prompt := fmt.Sprintf(`Extract key knowledge from this AI agent response. Keep only technically valuable information.

Query: %s

Response: %s

Reply with JSON only:
{"title": "5-10 word summary", "facts": ["key fact"], "files": ["file.go"], "errors": [], "decisions": [], "should_store": true}

Set should_store=false if the response is just pleasantries or has no lasting value.`, truncateStr(query, 200), truncateStr(response, 2000))

		resp, err := a.llm.Chat(ctx, []kernel.Message{
			{Role: "user", Content: prompt},
		}, nil, map[string]interface{}{"max_tokens": 500, "temperature": 0.2, "route": "execution", "no_thinking": true})
		if err == nil && resp.Content != "" {
			if json.Unmarshal([]byte(resp.Content), &result) == nil {
				title = result.Title
				if len(result.Facts) > 0 {
					content = ""
					for _, f := range result.Facts {
						content += "• " + f + "\n"
					}
				}
				if len(result.Files) > 0 {
					content += "\nFiles: " + stringsJoin(result.Files, ", ")
				}
				shouldStore = result.ShouldStore
			}
		}
	}

	if !shouldStore {
		return ""
	}

	// Step 3: store refined knowledge
	if len(title) > 120 {
		title = title[:120]
	}
	tags := []string{"refined", "session:" + sessionID}
	if _, err := a.Add(ctx, title, content, "auto-refine", tags); err != nil {
		return ""
	}
	return title
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
		// Rebuild index from remaining cache entries
		a.removeFromCache(id)
	})
}

func (a *Actor) removeFromCache(id string) {
	var remaining []docVector
	for _, dv := range a.cache {
		if dv.id != id {
			remaining = append(remaining, dv)
		}
	}
	a.cache = remaining
	// Rebuild buckets
	if len(a.proj) > 0 {
		for i := range a.buckets {
			a.buckets[i] = nil
		}
		for idx, dv := range a.cache {
			if len(dv.vec) == a.dim {
				b := a.bucketFor(dv.vec)
				a.buckets[b] = append(a.buckets[b], idx)
			}
		}
	}
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
	// Initialize projections with first embedding that has correct dim
	for _, dv := range a.cache {
		if len(dv.vec) > 0 && a.dim == 0 {
			a.dim = len(dv.vec)
			a.initProjections()
			return // initProjections rebuilds buckets for all cached entries
		}
	}
}

func (a *Actor) embedQuery(ctx context.Context, query string) ([]float32, bool) {
	if a.embedder == nil || a.embedder.Dimension() == 0 {
		return nil, false
	}
	key := hashStr(query)
	// Check cache
	a.super.Send(func() {
		if _, ok := a.embCache[key]; ok {
			// Move to front (LRU)
			for i, k := range a.embKeys {
				if k == key {
					a.embKeys = append(a.embKeys[:i], a.embKeys[i+1:]...)
					break
				}
			}
			a.embKeys = append(a.embKeys, key)
		}
	})
	// Fast path: cached
	if v, ok := a.embCache[key]; ok {
		return v, true
	}
	// Slow path: API call
	vec, err := a.embedder.Embed(ctx, query)
	if err != nil || len(vec) == 0 {
		return nil, false
	}
	// Store in cache
	a.super.Send(func() {
		if a.embCache == nil {
			a.embCache = make(map[string][]float32)
		}
		a.embCache[key] = vec
		a.embKeys = append(a.embKeys, key)
		// LRU evict
		for len(a.embKeys) > maxCachedEmbeddings {
			delete(a.embCache, a.embKeys[0])
			a.embKeys = a.embKeys[1:]
		}
	})
	return vec, true
}

// hashStr returns a short hex hash for cache keys.
func hashStr(s string) string {
	h := 0
	for _, r := range s {
		h = h*31 + int(r)
	}
	return fmt.Sprintf("%x", h)
}

// dotProduct computes the dot product of two float32 vectors.
func dotProduct(a, b []float32) float64 {
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

func truncateStr(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}

func stringsJoin(ss []string, sep string) string { return strings.Join(ss, sep) }

var _ kernel.KnowledgeCollector = (*Actor)(nil)

package rag

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// PGVectorConfig configures the pgvector-backed Retriever.
type PGVectorConfig struct {
	DSN            string // PostgreSQL connection string
	EmbeddingURL   string // external embedding API base URL (OpenAI-compatible /embeddings)
	EmbeddingKey   string // API key for the embedding endpoint
	EmbeddingModel string // model name sent to the embedding endpoint
	Collection     string // table name (default "openaide_docs")
}

// PGVector is a Retriever backed by PostgreSQL + pgvector.
//
// Embedding generation and vector search happen here, not in the OpenAIDE
// core: Index sends text to the configured external embedding API, stores
// the returned vector in a pgvector column, and Search performs a cosine
// query with an embedding of the user query.
type PGVector struct {
	db         *sql.DB
	emb        *embedder
	collection string
}

// NewPGVector connects to PostgreSQL and ensures the vector extension and
// table exist. Returns an error when the store is unreachable.
func NewPGVector(cfg PGVectorConfig) (*PGVector, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("pgvector: empty DSN")
	}
	if cfg.EmbeddingURL == "" {
		return nil, fmt.Errorf("pgvector: empty embedding URL")
	}
	if cfg.Collection == "" {
		cfg.Collection = "openaide_docs"
	}

	db, err := sql.Open("postgres", cfg.DSN)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("pgvector: ping: %w", err)
	}

	pv := &PGVector{
		db:         db,
		emb:        newEmbedder(cfg.EmbeddingURL, cfg.EmbeddingKey, cfg.EmbeddingModel),
		collection: cfg.Collection,
	}
	if err := pv.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return pv, nil
}

func (pv *PGVector) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE EXTENSION IF NOT EXISTS vector`,
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			embedding vector(1536) NOT NULL,
			metadata TEXT NOT NULL DEFAULT '{}'
		)`, pv.collection),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_vec_idx ON %s USING ivfflat (embedding vector_cosine_ops)`,
			pv.collection, pv.collection),
	}
	for _, s := range stmts {
		if _, err := pv.db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("pgvector: migrate: %w", err)
		}
	}
	return nil
}

// Index embeds each document via the external API and upserts it.
func (pv *PGVector) Index(ctx context.Context, collection string, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}
	texts := make([]string, len(docs))
	for i, d := range docs {
		texts[i] = d.Content
	}
	vecs, err := pv.emb.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("pgvector: embed: %w", err)
	}

	tx, err := pv.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt := fmt.Sprintf(
		`INSERT INTO %s (id, content, embedding, metadata) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (id) DO UPDATE SET content = EXCLUDED.content, embedding = EXCLUDED.embedding, metadata = EXCLUDED.metadata`,
		collection)
	for i, d := range docs {
		meta, _ := json.Marshal(d.Metadata)
		vec := vectorString(vecs[i])
		if _, err := tx.ExecContext(ctx, stmt, d.ID, d.Content, vec, string(meta)); err != nil {
			return fmt.Errorf("pgvector: upsert %s: %w", d.ID, err)
		}
	}
	return tx.Commit()
}

// Search embeds the query and returns the top-k nearest documents.
func (pv *PGVector) Search(ctx context.Context, collection, query string, limit int) ([]Result, error) {
	vecs, err := pv.emb.Embed(ctx, []string{query})
	if err != nil || len(vecs) == 0 {
		return nil, nil // degrade to empty results when embedding fails
	}
	if limit <= 0 {
		limit = 5
	}
	q := fmt.Sprintf(
		`SELECT id, content, metadata, 1 - (embedding <=> $1) AS score
		 FROM %s ORDER BY embedding <=> $1 LIMIT $2`, collection)
	rows, err := pv.db.QueryContext(ctx, q, vectorString(vecs[0]), limit)
	if err != nil {
		return nil, fmt.Errorf("pgvector: search: %w", err)
	}
	defer rows.Close()

	var out []Result
	for rows.Next() {
		var r Result
		var meta string
		if err := rows.Scan(&r.ID, &r.Content, &meta, &r.Score); err != nil {
			continue
		}
		json.Unmarshal([]byte(meta), &r.Metadata)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Delete removes documents by ID from a collection.
func (pv *PGVector) Delete(ctx context.Context, collection string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	_, err := pv.db.ExecContext(ctx,
		fmt.Sprintf(`DELETE FROM %s WHERE id IN (%s)`, collection, strings.Join(placeholders, ",")),
		args...)
	return err
}

// Ping reports whether the backing store is reachable.
func (pv *PGVector) Ping(ctx context.Context) error {
	return pv.db.PingContext(ctx)
}

// vectorString renders a []float32 as a pgvector literal, e.g. "[0.1,0.2]".
func vectorString(v []float32) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%g", f)
	}
	sb.WriteByte(']')
	return sb.String()
}

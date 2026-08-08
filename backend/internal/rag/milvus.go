package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

// MilvusConfig configures the Milvus-backed Retriever.
type MilvusConfig struct {
	EmbeddingURL   string // external OpenAI-compatible /embeddings endpoint
	EmbeddingKey   string // API key for the embedding endpoint
	EmbeddingModel string // model name sent to the embedding endpoint
	Collection     string // collection name (default "openaide_docs")
	Address        string // gRPC address, e.g. "localhost:19530"
	Username       string // username for auth
	Password       string // password for auth
}

// Milvus is a Retriever backed by Milvus.
//
// Documents are embedded via the external embedding API and stored in a
// collection with a fixed schema (id/content/metadata/embedding). Search
// performs a cosine ANN query and returns the top-k documents.
type Milvus struct {
	client     *milvusclient.Client
	emb        *embedder
	collection string
}

// NewMilvus connects to Milvus.
func NewMilvus(cfg MilvusConfig) (*Milvus, error) {
	if cfg.Address == "" {
		cfg.Address = "localhost:19530"
	}
	if cfg.Collection == "" {
		cfg.Collection = "openaide_docs"
	}
	if cfg.EmbeddingURL == "" {
		return nil, fmt.Errorf("milvus: empty embedding URL")
	}

	clientCfg := &milvusclient.ClientConfig{Address: cfg.Address}
	if cfg.Username != "" {
		clientCfg.Username = cfg.Username
		clientCfg.Password = cfg.Password
	}
	// The milvus client dials with grpc.WithBlock (see DefaultGrpcOpts), so
	// without a deadline the connection attempt hangs forever when the store
	// is unreachable. Bound it: construction fails fast, and reachability is
	// verified afterwards by Ping in the factory.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cli, err := milvusclient.New(ctx, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("milvus: new client: %w", err)
	}
	return &Milvus{
		client:     cli,
		emb:        newEmbedder(cfg.EmbeddingURL, cfg.EmbeddingKey, cfg.EmbeddingModel),
		collection: sanitizeCollection(cfg.Collection),
	}, nil
}

// Index embeds each document and inserts it into the collection.
func (m *Milvus) Index(ctx context.Context, collection string, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}
	col := sanitizeCollection(collection)

	texts := make([]string, len(docs))
	for i, d := range docs {
		texts[i] = d.Content
	}
	vecs, err := m.emb.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("milvus: embed: %w", err)
	}
	if len(vecs) == 0 {
		return fmt.Errorf("milvus: embed returned no vectors")
	}

	has, err := m.client.HasCollection(ctx, milvusclient.NewHasCollectionOption(col))
	if err != nil {
		return fmt.Errorf("milvus: has collection: %w", err)
	}
	if !has {
		if err := m.createCollection(ctx, col, len(vecs[0])); err != nil {
			return err
		}
	}

	ids := make([]string, len(docs))
	contents := make([]string, len(docs))
	metas := make([]string, len(docs))
	for i, d := range docs {
		meta, _ := json.Marshal(d.Metadata)
		ids[i] = d.ID
		contents[i] = d.Content
		metas[i] = string(meta)
	}
	_, err = m.client.Insert(ctx, milvusclient.NewColumnBasedInsertOption(col,
		column.NewColumnVarChar("id", ids),
		column.NewColumnVarChar("content", contents),
		column.NewColumnVarChar("metadata", metas),
		column.NewColumnFloatVector("embedding", len(vecs[0]), vecs),
	))
	if err != nil {
		return fmt.Errorf("milvus: insert: %w", err)
	}
	return nil
}

// Search embeds the query and returns the top-k nearest documents.
func (m *Milvus) Search(ctx context.Context, collection, query string, limit int) ([]Result, error) {
	vecs, err := m.emb.Embed(ctx, []string{query})
	if err != nil || len(vecs) == 0 {
		return nil, nil // degrade to empty results when embedding fails
	}
	if limit <= 0 {
		limit = 5
	}
	col := sanitizeCollection(collection)

	res, err := m.client.Search(ctx, milvusclient.NewSearchOption(col, limit, []entity.Vector{entity.FloatVector(vecs[0])}).
		WithANNSField("embedding").
		WithOutputFields("id", "content", "metadata"))
	if err != nil {
		return nil, fmt.Errorf("milvus: search: %w", err)
	}
	if len(res) == 0 {
		return nil, nil
	}
	rs := res[0]

	out := make([]Result, 0, rs.ResultCount)
	for i := 0; i < rs.ResultCount; i++ {
		var meta map[string]string
		metaCol := rs.GetColumn("metadata")
		if metaCol != nil {
			if s, err := metaCol.GetAsString(i); err == nil {
				json.Unmarshal([]byte(s), &meta)
			}
		}
		content, _ := rs.GetColumn("content").GetAsString(i)
		out = append(out, Result{
			ID:       resultIDString(rs, i),
			Content:  content,
			Score:    float64(rs.Scores[i]),
			Metadata: meta,
		})
	}
	return out, nil
}

// Delete removes documents by ID from a collection.
func (m *Milvus) Delete(ctx context.Context, collection string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = strconv.Quote(id)
	}
	expr := "id in [" + strings.Join(quoted, ",") + "]"
	_, err := m.client.Delete(ctx, milvusclient.NewDeleteOption(sanitizeCollection(collection)).WithExpr(expr))
	if err != nil {
		return fmt.Errorf("milvus: delete: %w", err)
	}
	return nil
}

// Ping reports whether Milvus is reachable.
func (m *Milvus) Ping(ctx context.Context) error {
	_, err := m.client.GetServerVersion(ctx, milvusclient.NewGetServerVersionOption())
	if err != nil {
		return fmt.Errorf("milvus: ping: %w", err)
	}
	return nil
}

func (m *Milvus) createCollection(ctx context.Context, name string, dim int) error {
	schema := entity.NewSchema().
		WithField(entity.NewField().WithName("id").WithDataType(entity.FieldTypeVarChar).WithIsPrimaryKey(true).WithMaxLength(512)).
		WithField(entity.NewField().WithName("content").WithDataType(entity.FieldTypeVarChar).WithMaxLength(65535)).
		WithField(entity.NewField().WithName("metadata").WithDataType(entity.FieldTypeVarChar).WithMaxLength(65535)).
		WithField(entity.NewField().WithName("embedding").WithDataType(entity.FieldTypeFloatVector).WithDim(int64(dim)))
	if err := m.client.CreateCollection(ctx, milvusclient.NewCreateCollectionOption(name, schema)); err != nil {
		return fmt.Errorf("milvus: create collection: %w", err)
	}
	if _, err := m.client.CreateIndex(ctx, milvusclient.NewCreateIndexOption(name, "embedding", index.NewAutoIndex(entity.COSINE))); err != nil {
		return fmt.Errorf("milvus: create index: %w", err)
	}
	if _, err := m.client.LoadCollection(ctx, milvusclient.NewLoadCollectionOption(name)); err != nil {
		return fmt.Errorf("milvus: load collection: %w", err)
	}
	return nil
}

// resultIDString extracts the id column value for row i.
func resultIDString(rs milvusclient.ResultSet, i int) string {
	idCol := rs.GetColumn("id")
	if idCol == nil {
		return ""
	}
	s, err := idCol.GetAsString(i)
	if err != nil {
		return ""
	}
	return s
}

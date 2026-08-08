package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/amikos-tech/chroma-go/pkg/embeddings"
)

// ChromaConfig configures the Chroma-backed Retriever.
type ChromaConfig struct {
	EmbeddingURL   string // external OpenAI-compatible /embeddings endpoint
	EmbeddingKey   string // API key for the embedding endpoint
	EmbeddingModel string // model name sent to the embedding endpoint
	Collection     string // collection name (default "openaide_docs")
	URL            string // chroma server address, default "http://localhost:8000"
	Token          string // X-Chroma-Token auth token
}

// ChromaRetriever is a Retriever backed by a Chroma vector store.
type ChromaRetriever struct {
	client     v2.Client
	collection v2.Collection
	emb        *embedder
	name       string
}

// noopEmbeddingFunction satisfies the embeddings.EmbeddingFunction interface
// without doing any work. It is passed to GetOrCreateCollection to stop the
// client from auto-wiring the default ONNX embedding function, which downloads
// the ONNX runtime on first use. Embeddings are always provided explicitly.
type noopEmbeddingFunction struct{}

func (noopEmbeddingFunction) EmbedDocuments(context.Context, []string) ([]embeddings.Embedding, error) {
	return nil, nil
}

func (noopEmbeddingFunction) EmbedQuery(context.Context, string) (embeddings.Embedding, error) {
	return nil, nil
}

func (noopEmbeddingFunction) Name() string { return "noop" }

func (noopEmbeddingFunction) GetConfig() embeddings.EmbeddingFunctionConfig {
	return embeddings.EmbeddingFunctionConfig{}
}

func (noopEmbeddingFunction) DefaultSpace() embeddings.DistanceMetric { return embeddings.COSINE }

func (noopEmbeddingFunction) SupportedSpaces() []embeddings.DistanceMetric {
	return []embeddings.DistanceMetric{embeddings.COSINE}
}

// NewChroma connects to the Chroma server and (re)creates the collection.
func NewChroma(cfg ChromaConfig) (*ChromaRetriever, error) {
	if cfg.URL == "" {
		cfg.URL = "http://localhost:8000"
	}
	if cfg.Collection == "" {
		cfg.Collection = "openaide_docs"
	}
	if cfg.EmbeddingURL == "" {
		return nil, fmt.Errorf("chroma: empty embedding URL")
	}

	opts := []v2.ClientOption{v2.WithBaseURL(cfg.URL), v2.WithDebug()}
	if cfg.Token != "" {
		opts = append(opts, v2.WithAuth(
			v2.NewTokenAuthCredentialsProvider(cfg.Token, v2.XChromaTokenHeader),
		))
	}
	client, err := v2.NewHTTPClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("chroma: new client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	colName := sanitizeCollection(cfg.Collection)
	col, err := client.GetOrCreateCollection(ctx, colName,
		v2.WithHNSWSpaceCreate(embeddings.COSINE),
		// An explicit embedding function is required: with a nil EF the client
		// auto-wires the default ONNX function, which downloads the ONNX runtime
		// and hangs offline. We always precompute embeddings and pass them with
		// WithEmbeddings/WithQueryEmbeddings, so the EF is never invoked.
		v2.WithEmbeddingFunctionCreate(noopEmbeddingFunction{}),
		// Do not persist the noop EF config server-side; the server stores its
		// own defaults and dimension is inferred from the first upsert batch.
		v2.WithDisableEFConfigStorage(),
	)
	if err != nil {
		return nil, fmt.Errorf("chroma: get-or-create collection: %w", err)
	}

	return &ChromaRetriever{
		client:     client,
		collection: col,
		emb:        newEmbedder(cfg.EmbeddingURL, cfg.EmbeddingKey, cfg.EmbeddingModel),
		name:       colName,
	}, nil
}

// Index embeds documents and upserts them into the collection.
func (r *ChromaRetriever) Index(ctx context.Context, collection string, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}
	col, err := r.collectionFor(ctx, collection)
	if err != nil {
		return err
	}

	texts := make([]string, len(docs))
	for i, d := range docs {
		texts[i] = d.Content
	}
	vecs, err := r.emb.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("chroma: embed: %w", err)
	}
	if len(vecs) == 0 {
		return fmt.Errorf("chroma: embed returned no vectors")
	}

	ids := make([]v2.DocumentID, len(docs))
	metas := make([]v2.DocumentMetadata, len(docs))
	embs := make([]embeddings.Embedding, len(docs))
	for i, d := range docs {
		ids[i] = v2.DocumentID(d.ID)
		meta := make(map[string]interface{}, len(d.Metadata))
		for k, v := range d.Metadata {
			meta[k] = v
		}
		dm, err := v2.NewDocumentMetadataFromMap(meta)
		if err != nil {
			return fmt.Errorf("chroma: metadata %s: %w", d.ID, err)
		}
		metas[i] = dm
		embs[i] = embeddings.NewEmbeddingFromFloat32(vecs[i])
	}

	if err := col.Upsert(ctx,
		v2.WithIDs(ids...),
		v2.WithTexts(texts...),
		v2.WithEmbeddings(embs...),
		v2.WithMetadatas(metas...),
	); err != nil {
		return fmt.Errorf("chroma: upsert: %w", err)
	}
	return nil
}

// Search embeds the query and runs a nearest-neighbor query.
func (r *ChromaRetriever) Search(ctx context.Context, collection, query string, limit int) ([]Result, error) {
	vecs, err := r.emb.Embed(ctx, []string{query})
	if err != nil || len(vecs) == 0 {
		return nil, nil // degrade to empty results when embedding fails
	}
	if limit <= 0 {
		limit = 5
	}
	col, err := r.collectionFor(ctx, collection)
	if err != nil {
		return nil, err
	}

	res, err := col.Query(ctx,
		v2.WithQueryEmbeddings(embeddings.NewEmbeddingFromFloat32(vecs[0])),
		v2.WithNResults(limit),
		v2.WithInclude(v2.IncludeDocuments, v2.IncludeMetadatas, v2.IncludeDistances),
	)
	if err != nil {
		return nil, fmt.Errorf("chroma: query: %w", err)
	}

	return parseChromaQuery(res), nil
}

// Delete removes documents by ID from the collection.
func (r *ChromaRetriever) Delete(ctx context.Context, collection string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	col, err := r.collectionFor(ctx, collection)
	if err != nil {
		return err
	}
	docIDs := make([]v2.DocumentID, len(ids))
	for i, id := range ids {
		docIDs[i] = v2.DocumentID(id)
	}
	if err := col.Delete(ctx, v2.WithIDs(docIDs...)); err != nil {
		return fmt.Errorf("chroma: delete: %w", err)
	}
	return nil
}

// Ping verifies connectivity by listing collections.
func (r *ChromaRetriever) Ping(ctx context.Context) error {
	if _, err := r.client.ListCollections(ctx); err != nil {
		return fmt.Errorf("chroma: ping: %w", err)
	}
	return nil
}

// collectionFor returns the collection for the given name, getting it from the
// server when the name differs from the default used at construction time.
func (r *ChromaRetriever) collectionFor(ctx context.Context, collection string) (v2.Collection, error) {
	col := sanitizeCollection(collection)
	if col == r.name {
		return r.collection, nil
	}
	c, err := r.client.GetCollection(ctx, col)
	if err != nil {
		return nil, fmt.Errorf("chroma: get collection %s: %w", col, err)
	}
	return c, nil
}

// parseChromaQuery converts a QueryResult into []Result.
func parseChromaQuery(res v2.QueryResult) []Result {
	if res == nil || res.CountGroups() == 0 {
		return nil
	}
	idGroups := res.GetIDGroups()
	if len(idGroups) == 0 {
		return nil
	}
	ids := idGroups[0]
	docs := docsGroup(res.GetDocumentsGroups())
	metas := metaGroup(res.GetMetadatasGroups())
	dists := distGroup(res.GetDistancesGroups())

	n := len(ids)
	if n > len(docs) {
		n = len(docs)
	}
	out := make([]Result, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, Result{
			ID:       string(ids[i]),
			Content:  docs[i],
			Score:    float64(dists[i]),
			Metadata: metadataMap(metas, i),
		})
	}
	return out
}

func docsGroup(groups []v2.Documents) []string {
	if len(groups) == 0 {
		return nil
	}
	out := make([]string, len(groups[0]))
	for i, d := range groups[0] {
		out[i] = d.ContentString()
	}
	return out
}

func metaGroup(groups []v2.DocumentMetadatas) []v2.DocumentMetadata {
	if len(groups) == 0 {
		return nil
	}
	return groups[0]
}

func distGroup(groups []embeddings.Distances) []float32 {
	if len(groups) == 0 {
		return nil
	}
	out := make([]float32, len(groups[0]))
	for i, d := range groups[0] {
		out[i] = float32(d)
	}
	return out
}

// metadataMap converts a Chroma DocumentMetadata into map[string]string.
func metadataMap(metas []v2.DocumentMetadata, i int) map[string]string {
	if i >= len(metas) || metas[i] == nil {
		return nil
	}
	// The simplest lossless round-trip is JSON: DocumentMetadataImpl marshals
	// its internal map, and our Metadata type is map[string]string.
	b, err := json.Marshal(metas[i])
	if err != nil {
		return nil
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		switch t := v.(type) {
		case string:
			out[k] = t
		case float64:
			out[k] = fmt.Sprintf("%v", t)
		case bool:
			out[k] = fmt.Sprintf("%v", t)
		}
	}
	return out
}

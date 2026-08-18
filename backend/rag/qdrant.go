package rag

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/qdrant/go-client/qdrant"
)

// QdrantConfig configures the Qdrant-backed Retriever.
type QdrantConfig struct {
	EmbeddingURL   string // external OpenAI-compatible /embeddings endpoint
	EmbeddingKey   string // API key for the embedding endpoint
	EmbeddingModel string // model name sent to the embedding endpoint
	Collection     string // collection name (default "openaide_docs")
	Host           string // default "localhost"
	Port           int    // gRPC port, default 6334
	APIKey         string // API key for Qdrant
	UseTLS         bool   // enable TLS
}

// Qdrant is a Retriever backed by Qdrant.
//
// Documents are embedded via the external embedding API and stored as points
// with a payload of content + metadata. Search performs a nearest-neighbor
// cosine query.
type Qdrant struct {
	client     *qdrant.Client
	emb        *embedder
	collection string
}

// NewQdrant connects to Qdrant and ensures the collection exists.
func NewQdrant(cfg QdrantConfig) (*Qdrant, error) {
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if cfg.Port == 0 {
		cfg.Port = 6334
	}
	if cfg.Collection == "" {
		cfg.Collection = "openaide_docs"
	}
	if cfg.EmbeddingURL == "" {
		return nil, fmt.Errorf("qdrant: empty embedding URL")
	}

	client, err := qdrant.NewClient(&qdrant.Config{
		Host:   cfg.Host,
		Port:   cfg.Port,
		APIKey: cfg.APIKey,
		UseTLS: cfg.UseTLS,
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant: new client: %w", err)
	}
	return &Qdrant{
		client:     client,
		emb:        newEmbedder(cfg.EmbeddingURL, cfg.EmbeddingKey, cfg.EmbeddingModel),
		collection: sanitizeCollection(cfg.Collection),
	}, nil
}

// Index embeds each document via the external API and upserts it as a point.
func (q *Qdrant) Index(ctx context.Context, collection string, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}
	col := sanitizeCollection(collection)

	texts := make([]string, len(docs))
	for i, d := range docs {
		texts[i] = d.Content
	}
	vecs, err := q.emb.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("qdrant: embed: %w", err)
	}
	if len(vecs) == 0 {
		return fmt.Errorf("qdrant: embed returned no vectors")
	}

	exists, err := q.client.CollectionExists(ctx, col)
	if err != nil {
		return fmt.Errorf("qdrant: collection exists: %w", err)
	}
	if !exists {
		if err := q.client.CreateCollection(ctx, &qdrant.CreateCollection{
			CollectionName: col,
			VectorsConfig: &qdrant.VectorsConfig{
				Config: &qdrant.VectorsConfig_Params{
					Params: &qdrant.VectorParams{
						Size:     uint64(len(vecs[0])),
						Distance: qdrant.Distance_Cosine,
					},
				},
			},
		}); err != nil {
			return fmt.Errorf("qdrant: create collection: %w", err)
		}
	}

	points := make([]*qdrant.PointStruct, 0, len(docs))
	for i, d := range docs {
		meta, _ := json.Marshal(d.Metadata)
		points = append(points, &qdrant.PointStruct{
			Id: &qdrant.PointId{
				PointIdOptions: &qdrant.PointId_Uuid{Uuid: docUUID(d.ID)},
			},
			Payload: map[string]*qdrant.Value{
				"content":  qdrant.NewValueString(d.Content),
				"metadata": qdrant.NewValueString(string(meta)),
			},
			Vectors: &qdrant.Vectors{
				VectorsOptions: &qdrant.Vectors_Vector{Vector: &qdrant.Vector{Data: vecs[i]}},
			},
		})
	}
	if _, err := q.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: col,
		Points:         points,
	}); err != nil {
		return fmt.Errorf("qdrant: upsert: %w", err)
	}
	return nil
}

// Search embeds the query and returns the top-k nearest documents.
func (q *Qdrant) Search(ctx context.Context, collection, query string, limit int) ([]Result, error) {
	vecs, err := q.emb.Embed(ctx, []string{query})
	if err != nil || len(vecs) == 0 {
		return nil, nil // degrade to empty results when embedding fails
	}
	if limit <= 0 {
		limit = 5
	}
	col := sanitizeCollection(collection)

	lim := uint64(limit)
	resp, err := q.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: col,
		Query: &qdrant.Query{
			Variant: &qdrant.Query_Nearest{
				Nearest: qdrant.NewVectorInputDense(vecs[0]),
			},
		},
		Limit: &lim,
		WithPayload: &qdrant.WithPayloadSelector{
			SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true},
		},
		WithVectors: &qdrant.WithVectorsSelector{
			SelectorOptions: &qdrant.WithVectorsSelector_Enable{Enable: false},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant: query: %w", err)
	}

	out := make([]Result, 0, len(resp))
	for _, p := range resp {
		var meta map[string]string
		if v, ok := p.GetPayload()["metadata"]; ok {
			json.Unmarshal([]byte(v.GetStringValue()), &meta)
		}
		out = append(out, Result{
			ID:       pointIDString(p.GetId()),
			Content:  p.GetPayload()["content"].GetStringValue(),
			Score:    float64(p.GetScore()),
			Metadata: meta,
		})
	}
	return out, nil
}

// Delete removes documents by ID from a collection.
func (q *Qdrant) Delete(ctx context.Context, collection string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	pts := make([]*qdrant.PointId, 0, len(ids))
	for _, id := range ids {
		pts = append(pts, &qdrant.PointId{
			PointIdOptions: &qdrant.PointId_Uuid{Uuid: docUUID(id)},
		})
	}
	_, err := q.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: sanitizeCollection(collection),
		Points: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Points{
				Points: &qdrant.PointsIdsList{Ids: pts},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("qdrant: delete: %w", err)
	}
	return nil
}

// Ping reports whether Qdrant is reachable.
func (q *Qdrant) Ping(ctx context.Context) error {
	_, err := q.client.GetCollectionInfo(ctx, q.collection)
	if err != nil {
		return fmt.Errorf("qdrant: ping: %w", err)
	}
	return nil
}

// docUUID deterministically maps an arbitrary document ID to a UUID v4 string
// accepted by Qdrant's PointId (which requires a valid UUID format).
func docUUID(id string) string {
	sum := md5.Sum([]byte(id))
	sum[6] = (sum[6] & 0x0f) | 0x40 // version 4
	sum[8] = (sum[8] & 0x3f) | 0x80 // variant 10
	h := hex.EncodeToString(sum[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// pointIDString extracts the UUID string from a PointId.
func pointIDString(id *qdrant.PointId) string {
	if id == nil {
		return ""
	}
	return id.GetUuid()
}

// sanitizeCollection makes a collection name safe for the backing store
// (Qdrant allows [a-zA-Z0-9_-.] and must not start with - or .).
func sanitizeCollection(name string) string {
	if name == "" {
		name = "openaide_docs"
	}
	var sb strings.Builder
	for i, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.'
		if !ok || (i == 0 && (r == '-' || r == '.')) {
			sb.WriteByte('_')
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

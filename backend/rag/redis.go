package rag

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

// RedisConfig configures the Redis + RediSearch-backed Retriever.
type RedisConfig struct {
	EmbeddingURL   string // external OpenAI-compatible /embeddings endpoint
	EmbeddingKey   string // API key for the embedding endpoint
	EmbeddingModel string // model name sent to the embedding endpoint
	Collection     string // collection name (default "openaide_docs")
	Addr           string // redis address, default "localhost:6379"
	Password       string // redis password
	DB             int    // redis DB number
}

// RedisRetriever is a Retriever backed by Redis + RediSearch.
//
// Each collection maps to a RediSearch index (`openaide:{collection}:idx`)
// over hash keys prefixed `openaide:{collection}:doc:`. Embeddings are stored
// as raw little-endian float32 bytes and searched with the KNN vector query.
type RedisRetriever struct {
	rdb        *redis.Client
	emb        *embedder
	dim        int
	collection string
}

// NewRedisRetriever connects to Redis.
func NewRedisRetriever(cfg RedisConfig) (*RedisRetriever, error) {
	if cfg.Addr == "" {
		cfg.Addr = "localhost:6379"
	}
	if cfg.Collection == "" {
		cfg.Collection = "openaide_docs"
	}
	if cfg.EmbeddingURL == "" {
		return nil, fmt.Errorf("redis: empty embedding URL")
	}
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	return &RedisRetriever{
		rdb:        rdb,
		emb:        newEmbedder(cfg.EmbeddingURL, cfg.EmbeddingKey, cfg.EmbeddingModel),
		collection: sanitizeCollection(cfg.Collection),
	}, nil
}

// Index embeds each document and stores it as a hash with a vector field.
func (r *RedisRetriever) Index(ctx context.Context, collection string, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}
	col := sanitizeCollection(collection)

	texts := make([]string, len(docs))
	for i, d := range docs {
		texts[i] = d.Content
	}
	vecs, err := r.emb.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("redis: embed: %w", err)
	}
	if len(vecs) == 0 {
		return fmt.Errorf("redis: embed returned no vectors")
	}
	if r.dim == 0 {
		r.dim = len(vecs[0])
		if err := r.createIndex(ctx, col); err != nil {
			return err
		}
	}

	for i, d := range docs {
		meta, _ := json.Marshal(d.Metadata)
		key := r.docKey(col, d.ID)
		if err := r.rdb.HSet(ctx, key,
			"content", d.Content,
			"metadata", string(meta),
			"embedding", float32Bytes(vecs[i]),
		).Err(); err != nil {
			return fmt.Errorf("redis: hset %s: %w", key, err)
		}
	}
	return nil
}

// Search embeds the query and runs a KNN vector search.
func (r *RedisRetriever) Search(ctx context.Context, collection, query string, limit int) ([]Result, error) {
	vecs, err := r.emb.Embed(ctx, []string{query})
	if err != nil || len(vecs) == 0 {
		return nil, nil // degrade to empty results when embedding fails
	}
	if limit <= 0 {
		limit = 5
	}
	col := sanitizeCollection(collection)
	idx := r.idxName(col)

	knnQuery := fmt.Sprintf("(*)=>[KNN %d @embedding $BLOB AS score]", limit)
	res, err := r.rdb.Do(ctx,
		"FT.SEARCH", idx, knnQuery,
		"PARAMS", "2", "BLOB", float32Bytes(vecs[0]),
		"SORTBY", "score", "ASC",
		"DIALECT", "2",
		"LIMIT", "0", strconv.Itoa(limit),
	).Result()
	if err != nil {
		if strings.Contains(err.Error(), "no such index") {
			return nil, nil // collection not indexed yet: empty results
		}
		return nil, fmt.Errorf("redis: ft.search: %w", err)
	}

	return parseSearchResults(col, res)
}

// Delete removes documents by ID from a collection.
func (r *RedisRetriever) Delete(ctx context.Context, collection string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = r.docKey(sanitizeCollection(collection), id)
	}
	if err := r.rdb.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("redis: del: %w", err)
	}
	return nil
}

// Ping reports whether Redis is reachable.
func (r *RedisRetriever) Ping(ctx context.Context) error {
	if err := r.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis: ping: %w", err)
	}
	return nil
}

func (r *RedisRetriever) createIndex(ctx context.Context, col string) error {
	idx := r.idxName(col)
	res, err := r.rdb.Do(ctx,
		"FT.CREATE", idx,
		"ON", "HASH",
		"PREFIX", "1", r.docPrefix(col),
		"SCHEMA",
		"content", "TEXT",
		"metadata", "TEXT",
		"embedding", "VECTOR", "HNSW", "6",
		"TYPE", "FLOAT32",
		"DIM", strconv.Itoa(r.dim),
		"DISTANCE_METRIC", "COSINE",
	).Result()
	if err != nil {
		if strings.Contains(fmt.Sprint(res), "already exists") || strings.Contains(err.Error(), "already exists") {
			return nil
		}
		return fmt.Errorf("redis: ft.create: %w", err)
	}
	return nil
}

func (r *RedisRetriever) idxName(col string) string {
	return "openaide:" + col + ":idx"
}

func (r *RedisRetriever) docPrefix(col string) string {
	return "openaide:" + col + ":doc:"
}

func (r *RedisRetriever) docKey(col, id string) string {
	return r.docPrefix(col) + id
}

// float32Bytes packs []float32 into little-endian bytes for RediSearch.
func float32Bytes(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// parseSearchResults parses the FT.SEARCH flat result list:
// [total, key1, fields1, key2, fields2, ...] where fields are
// [name1, value1, name2, value2, ...].
func parseSearchResults(col string, raw interface{}) ([]Result, error) {
	arr, ok := raw.([]interface{})
	if !ok || len(arr) < 1 {
		return nil, nil
	}
	total, _ := arr[0].(int64)
	if total == 0 {
		return nil, nil
	}
	prefix := "openaide:" + sanitizeCollection(col) + ":doc:"
	out := make([]Result, 0, total)
	for i := 1; i+1 < len(arr); i += 2 {
		key, _ := arr[i].(string)
		fields, ok := arr[i+1].([]interface{})
		if !ok {
			continue
		}
		res := Result{ID: strings.TrimPrefix(key, prefix)}
		for j := 0; j+1 < len(fields); j += 2 {
			name, _ := fields[j].(string)
			switch name {
			case "content":
				res.Content, _ = fields[j+1].(string)
			case "metadata":
				if s, ok := fields[j+1].(string); ok {
					json.Unmarshal([]byte(s), &res.Metadata)
				}
			case "score":
				if s, ok := fields[j+1].(string); ok {
					if f, err := strconv.ParseFloat(s, 64); err == nil {
						res.Score = 1 - f // RediSearch 返回 cosine 距离;归一化为相似度(越大越好,与其余后端一致)
					}
				}
			}
		}
		out = append(out, res)
	}
	return out, nil
}

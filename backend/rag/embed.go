package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// embedder 调用外部 OpenAI 兼容 /embeddings 端点生成向量。
// 所有基于外部 embedding API 的后端(pgvector/qdrant/milvus)复用同一实现。
type embedder struct {
	url    string
	key    string
	model  string
	client *http.Client
}

func newEmbedder(url, key, model string) *embedder {
	if model == "" {
		model = "text-embedding-3-small"
	}
	return &embedder{
		url:    strings.TrimSuffix(url, "/"),
		key:    key,
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Embed 调用外部 embedding API,返回每个文本的向量。
func (e *embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	body, _ := json.Marshal(map[string]interface{}{
		"model": e.model,
		"input": texts,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.key != "" {
		req.Header.Set("Authorization", "Bearer "+e.key)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("embedding API %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var payload struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	out := make([][]float32, 0, len(payload.Data))
	for _, d := range payload.Data {
		out = append(out, d.Embedding)
	}
	return out, nil
}

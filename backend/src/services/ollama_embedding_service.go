package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// OllamaEmbeddingService Ollama Embedding 服务实现
type OllamaEmbeddingService struct {
	baseURL    string
	model      string
	httpClient *http.Client
	cache      *CacheService
}

// NewOllamaEmbeddingService 创建 Ollama Embedding 服务
func NewOllamaEmbeddingService(baseURL, model string, cache *CacheService) *OllamaEmbeddingService {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "nomic-embed-text"
	}

	return &OllamaEmbeddingService{
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		cache: cache,
	}
}

// OllamaEmbeddingRequest Ollama 嵌入请求
type OllamaEmbeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// OllamaEmbeddingResponse Ollama 嵌入响应
type OllamaEmbeddingResponse struct {
	Embedding []float64 `json:"embedding"`
}

// GenerateEmbedding 生成单个文本的 Embedding
func (s *OllamaEmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float64, error) {
	cacheKey := s.getCacheKey(text)
	if s.cache != nil {
		if cached, found := s.cache.Get(cacheKey); found {
			if embedding, ok := cached.([]float64); ok {
				return embedding, nil
			}
		}
	}

	// 构建请求
	reqBody := OllamaEmbeddingRequest{
		Model:  s.model,
		Prompt: text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 发送请求
	reqURL, _ := url.JoinPath(s.baseURL, "/api/embeddings")
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 处理响应
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var embeddingResp OllamaEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// 缓存结果
	if s.cache != nil {
		s.cache.Set(cacheKey, embeddingResp.Embedding, 24*time.Hour)
	}

	return embeddingResp.Embedding, nil
}

// GenerateEmbeddings 批量生成 Embedding
func (s *OllamaEmbeddingService) GenerateEmbeddings(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("texts cannot be empty")
	}

	// Ollama 不支持批量，逐个处理
	embeddings := make([][]float64, len(texts))
	for i, text := range texts {
		embedding, err := s.GenerateEmbedding(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("failed to generate embedding for text %d: %w", i, err)
		}
		embeddings[i] = embedding
	}

	return embeddings, nil
}

// getCacheKey 生成缓存键
func (s *OllamaEmbeddingService) getCacheKey(text string) string {
	hash := sha256.Sum256([]byte(text))
	return "ollama_embedding:" + hex.EncodeToString(hash[:])
}

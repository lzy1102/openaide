package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"time"
)

// EmbeddingService Embedding 生成服务接口
type EmbeddingService interface {
	// GenerateEmbedding 生成单个文本的 Embedding
	GenerateEmbedding(ctx context.Context, text string) ([]float64, error)

	// GenerateEmbeddings 批量生成 Embedding
	GenerateEmbeddings(ctx context.Context, texts []string) ([][]float64, error)
}

// OpenAIEmbeddingService OpenAI Embedding 服务实现
type OpenAIEmbeddingService struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
	cache      *CacheService
}

// NewOpenAIEmbeddingService 创建 OpenAI Embedding 服务
func NewOpenAIEmbeddingService(apiKey, baseURL, model string, cache *CacheService) *OpenAIEmbeddingService {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "text-embedding-ada-002"
	}

	return &OpenAIEmbeddingService{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		cache: cache,
	}
}

// EmbeddingRequest OpenAI Embedding API 请求
type EmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// EmbeddingResponse OpenAI Embedding API 响应
type EmbeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// GenerateEmbedding 生成单个文本的 Embedding
func (s *OpenAIEmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float64, error) {
	cacheKey := s.getCacheKey(text)
	if s.cache != nil {
		if cached, found := s.cache.Get(cacheKey); found {
			if embedding, ok := cached.([]float64); ok {
				return embedding, nil
			}
		}
	}

	// 调用 API
	embeddings, err := s.GenerateEmbeddings(ctx, []string{text})
	if err != nil {
		return nil, err
	}

	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	// 缓存结果
	embedding := embeddings[0]
	if s.cache != nil {
		s.cache.Set(cacheKey, embedding, 24*time.Hour)
	}

	return embedding, nil
}

// GenerateEmbeddings 批量生成 Embedding
func (s *OpenAIEmbeddingService) GenerateEmbeddings(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("texts cannot be empty")
	}

	// 检查缓存
	cachedEmbeddings := make([][]float64, len(texts))
	uncachedIndices := make([]int, 0, len(texts))
	uncachedTexts := make([]string, 0, len(texts))

	for i, text := range texts {
		cacheKey := s.getCacheKey(text)
		if s.cache != nil {
			if cached, found := s.cache.Get(cacheKey); found {
				if embedding, ok := cached.([]float64); ok {
					cachedEmbeddings[i] = embedding
					continue
				}
			}
		}
		uncachedIndices = append(uncachedIndices, i)
		uncachedTexts = append(uncachedTexts, text)
	}

	// 如果全部命中缓存
	if len(uncachedTexts) == 0 {
		return cachedEmbeddings, nil
	}

	// 构建请求
	reqBody := EmbeddingRequest{
		Model: s.model,
		Input: uncachedTexts,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 发送请求
	reqURL, _ := url.JoinPath(s.baseURL, "/embeddings")
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

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

	var embeddingResp EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// 合并缓存和新生成的结果
	for i, data := range embeddingResp.Data {
		originalIndex := uncachedIndices[i]
		cachedEmbeddings[originalIndex] = data.Embedding

		// 缓存新生成的 embedding
		cacheKey := s.getCacheKey(uncachedTexts[i])
		if s.cache != nil {
			s.cache.Set(cacheKey, data.Embedding, 24*time.Hour)
		}
	}

	return cachedEmbeddings, nil
}

// getCacheKey 生成缓存键
func (s *OpenAIEmbeddingService) getCacheKey(text string) string {
	hash := sha256.Sum256([]byte(text))
	return "embedding:" + hex.EncodeToString(hash[:])
}

// CosineSimilarity 计算余弦相似度
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// EuclideanDistance 计算欧几里得距离
func EuclideanDistance(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}

	var sum float64
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff
	}

	return math.Sqrt(sum)
}

// DotProduct 计算点积
func DotProduct(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}

	var result float64
	for i := range a {
		result += a[i] * b[i]
	}

	return result
}

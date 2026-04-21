package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"openaide/backend/src/services/llm"
)

// SmartCacheService 智能缓存服务
// 缓存常见查询的响应，减少重复token消耗
type SmartCacheService struct {
	cache           *CacheService
	tokenEstimator  *TokenEstimator
	hitCount        map[string]int
	missCount       int
	totalSavedTokens int64
}

// CacheEntry 缓存条目
type CacheEntry struct {
	Key          string                 `json:"key"`
	Query        string                 `json:"query"`
	Response     string                 `json:"response"`
	ModelID      string                 `json:"model_id"`
	CreatedAt    time.Time              `json:"created_at"`
	ExpiresAt    time.Time              `json:"expires_at"`
	HitCount     int                    `json:"hit_count"`
	SavedTokens  int64                  `json:"saved_tokens"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// NewSmartCacheService 创建智能缓存服务
func NewSmartCacheService(cache *CacheService) *SmartCacheService {
	return &SmartCacheService{
		cache:            cache,
		tokenEstimator:   NewTokenEstimator(),
		hitCount:         make(map[string]int),
		totalSavedTokens: 0,
	}
}

// GenerateCacheKey 生成缓存键
// 基于查询内容、模型和选项生成唯一键
func (s *SmartCacheService) GenerateCacheKey(query string, modelID string, options map[string]interface{}) string {
	// 规范化查询（去除多余空格、转为小写）
	normalizedQuery := s.normalizeQuery(query)

	// 构建缓存键数据
	keyData := map[string]interface{}{
		"query":    normalizedQuery,
		"model":    modelID,
		"options":  s.filterCacheOptions(options),
	}

	// 序列化并哈希
	data, _ := json.Marshal(keyData)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:16]) // 取前16字节
}

// Get 获取缓存的响应
func (s *SmartCacheService) Get(query string, modelID string, options map[string]interface{}) (*CacheEntry, bool) {
	key := s.GenerateCacheKey(query, modelID, options)

	if val, found := s.cache.Get("smart_cache:" + key); found {
		if entry, ok := val.(*CacheEntry); ok {
			// 检查是否过期
			if time.Now().Before(entry.ExpiresAt) {
				entry.HitCount++
				s.hitCount[key]++

				// 估算节省的token
				savedTokens := s.tokenEstimator.EstimateTokens(query, modelID)
				savedTokens += s.tokenEstimator.EstimateTokens(entry.Response, modelID)
				s.totalSavedTokens += int64(savedTokens)
				entry.SavedTokens += int64(savedTokens)

				return entry, true
			}
		}
	}

	s.missCount++
	return nil, false
}

// Set 设置缓存
func (s *SmartCacheService) Set(query string, modelID string, options map[string]interface{}, response string, ttl time.Duration) {
	key := s.GenerateCacheKey(query, modelID, options)

	entry := &CacheEntry{
		Key:       key,
		Query:     query,
		Response:  response,
		ModelID:   modelID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(ttl),
		HitCount:  0,
		Metadata: map[string]interface{}{
			"options_hash": s.hashOptions(options),
		},
	}

	s.cache.Set("smart_cache:"+key, entry, ttl)
}

// ShouldCache 判断是否应该缓存此查询
func (s *SmartCacheService) ShouldCache(query string, options map[string]interface{}) bool {
	// 不缓存空查询
	if strings.TrimSpace(query) == "" {
		return false
	}

	// 不缓存过短的查询（可能是测试或随意输入）
	if len(strings.TrimSpace(query)) < 10 {
		return false
	}

	// 不缓存包含敏感信息的查询
	if s.containsSensitiveInfo(query) {
		return false
	}

	// 不缓存工具调用相关的查询
	if options != nil {
		if _, hasTools := options["tools"]; hasTools {
			return false
		}
		if _, hasToolFilter := options["tool_filter"]; hasToolFilter {
			return false
		}
	}

	// 检查是否是知识类查询（适合缓存）
	return s.isCacheableQueryType(query)
}

// GetStats 获取缓存统计
func (s *SmartCacheService) GetStats() map[string]interface{} {
	totalHits := 0
	for _, count := range s.hitCount {
		totalHits += count
	}

	totalRequests := totalHits + s.missCount
	hitRate := 0.0
	if totalRequests > 0 {
		hitRate = float64(totalHits) / float64(totalRequests) * 100
	}

	return map[string]interface{}{
		"total_hits":        totalHits,
		"total_misses":      s.missCount,
		"hit_rate":          fmt.Sprintf("%.2f%%", hitRate),
		"total_saved_tokens": s.totalSavedTokens,
		"unique_queries":    len(s.hitCount),
	}
}

// Clear 清空缓存
func (s *SmartCacheService) Clear() {
	s.cache.Flush()
	s.hitCount = make(map[string]int)
	s.missCount = 0
	s.totalSavedTokens = 0
}

// normalizeQuery 规范化查询
func (s *SmartCacheService) normalizeQuery(query string) string {
	// 转为小写
	query = strings.ToLower(query)

	// 去除多余空格
	query = strings.Join(strings.Fields(query), " ")

	// 去除标点符号（保留中文标点）
	var result strings.Builder
	for _, r := range query {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
			(r >= '\u4e00' && r <= '\u9fff') || // 中文字符
			(r == ' ') {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// filterCacheOptions 过滤缓存选项
// 只保留影响响应的关键选项
func (s *SmartCacheService) filterCacheOptions(options map[string]interface{}) map[string]interface{} {
	if options == nil {
		return nil
	}

	// 只保留temperature等影响输出的关键参数
	filtered := make(map[string]interface{})
	for _, key := range []string{"temperature", "top_p", "max_tokens"} {
		if val, ok := options[key]; ok {
			filtered[key] = val
		}
	}
	return filtered
}

// hashOptions 哈希选项
func (s *SmartCacheService) hashOptions(options map[string]interface{}) string {
	filtered := s.filterCacheOptions(options)
	if filtered == nil {
		return ""
	}
	data, _ := json.Marshal(filtered)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:8])
}

// containsSensitiveInfo 检查是否包含敏感信息
func (s *SmartCacheService) containsSensitiveInfo(query string) bool {
	sensitivePatterns := []string{
		"密码", "password", "passwd", "pwd",
		"密钥", "secret", "key", "token",
		"身份证", "id card", "身份证号",
		"银行卡", "credit card", "cvv",
		"手机号", "phone", "手机号码",
		"地址", "address", "住址",
	}

	lowerQuery := strings.ToLower(query)
	for _, pattern := range sensitivePatterns {
		if strings.Contains(lowerQuery, pattern) {
			return true
		}
	}
	return false
}

// isCacheableQueryType 判断是否是适合缓存的查询类型
func (s *SmartCacheService) isCacheableQueryType(query string) bool {
	// 知识类查询关键词
	knowledgePatterns := []string{
		"什么是", "什么是", "解释", "介绍", "定义",
		"how to", "what is", "explain", "define", "describe",
		"区别", "比较", "对比", "difference", "compare",
		"原理", "机制", "原理", "principle", "mechanism",
		"教程", "指南", "tutorial", "guide", "how do",
		"为什么", "原因", "why", "reason", "cause",
		"列表", "列举", "list", "enumerate",
	}

	lowerQuery := strings.ToLower(query)
	for _, pattern := range knowledgePatterns {
		if strings.Contains(lowerQuery, pattern) {
			return true
		}
	}

	// 代码示例类查询
	codePatterns := []string{
		"示例", "例子", "example", "sample",
		"代码", "code", "snippet",
	}
	for _, pattern := range codePatterns {
		if strings.Contains(lowerQuery, pattern) {
			return true
		}
	}

	return false
}

// CacheMiddleware 缓存中间件
// 用于在DialogueService中集成缓存
func (s *SmartCacheService) CacheMiddleware(
	query string,
	modelID string,
	options map[string]interface{},
	executeFunc func() (*llm.ChatResponse, error),
) (*llm.ChatResponse, bool, error) {
	// 检查是否应该缓存
	if !s.ShouldCache(query, options) {
		resp, err := executeFunc()
		return resp, false, err
	}

	// 尝试获取缓存
	if entry, found := s.Get(query, modelID, options); found {
		// 返回缓存的响应
		cachedResp := &llm.ChatResponse{
			Choices: []llm.Choice{
				{
					Message: llm.Message{
						Role:    "assistant",
						Content: entry.Response,
					},
					FinishReason: "stop",
				},
			},
			Usage: &llm.Usage{
				PromptTokens:     0, // 缓存命中不消耗token
				CompletionTokens: 0,
				TotalTokens:      0,
			},
			Cached: true,
		}
		return cachedResp, true, nil
	}

	// 执行原始请求
	resp, err := executeFunc()
	if err != nil {
		return nil, false, err
	}

	// 缓存响应
	if resp != nil && len(resp.Choices) > 0 {
		response := resp.Choices[0].Message.Content
		// 缓存1小时
		s.Set(query, modelID, options, response, 1*time.Hour)
	}

	return resp, false, nil
}

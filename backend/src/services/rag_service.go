package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"openaide/backend/src/services/llm"
)

// RAGService RAG 检索增强生成服务接口
type RAGService interface {
	// RetrieveAndGenerate 检索相关内容并生成回答
	RetrieveAndGenerate(ctx context.Context, query string, options RAGOptions) (*RAGResponse, error)

	// Retrieve 检索相关上下文
	Retrieve(ctx context.Context, query string, topK int) ([]KnowledgeSearchResult, error)

	// BuildContext 构建上下文
	BuildContext(results []KnowledgeSearchResult, maxTokens int) string

	// GenerateWithRAG 使用 RAG 生成回答
	GenerateWithRAG(ctx context.Context, query string, context string) (string, error)

	// RetrieveAndGenerateStream 流式检索并生成回答
	RetrieveAndGenerateStream(ctx context.Context, query string, options RAGOptions) (<-chan llm.ChatStreamChunk, error)
}

// RAGOptions RAG 选项
type RAGOptions struct {
	TopK            int     // 检索数量
	MaxContextTokens int    // 最大上下文 Token 数
	MinScore        float64 // 最小相似度阈值
	Temperature     float64 // 生成温度
	IncludeSources  bool    // 是否包含来源
	Model           string  // 使用的模型
	MaxTokens       int     // 最大生成 Token 数
	SystemPrompt    string  // 系统提示词
}

// RAGResponse RAG 响应
type RAGResponse struct {
	// Answer 生成的回答
	Answer string `json:"answer"`

	// Sources 引用来源
	Sources []RAGSource `json:"sources"`

	// Query 原始查询
	Query string `json:"query"`

	// Context 使用的上下文
	Context string `json:"context,omitempty"`

	// RetrievedCount 检索到的文档数量
	RetrievedCount int `json:"retrieved_count"`

	// ProcessedTime 处理时间
	ProcessedTime time.Duration `json:"processed_time"`

	// TokenUsage Token 使用情况
	TokenUsage *llm.Usage `json:"token_usage,omitempty"`
}

// RAGSource RAG 来源
type RAGSource struct {
	// ID 来源 ID
	ID string `json:"id"`

	// Title 标题
	Title string `json:"title"`

	// Content 内容片段
	Content string `json:"content"`

	// Summary 摘要
	Summary string `json:"summary,omitempty"`

	// Score 相似度分数
	Score float64 `json:"score"`

	// Source 来源类型
	Source string `json:"source"`

	// SourceID 来源 ID
	SourceID string `json:"source_id,omitempty"`

	// CategoryID 分类 ID
	CategoryID string `json:"category_id,omitempty"`
}

// ragService RAG 服务实现
type ragService struct {
	knowledge KnowledgeService
	llm       llm.LLMClient
	cache     *CacheService
}

// NewRAGService 创建 RAG 服务
func NewRAGService(knowledge KnowledgeService, llmClient llm.LLMClient, cache *CacheService) RAGService {
	return &ragService{
		knowledge: knowledge,
		llm:       llmClient,
		cache:     cache,
	}
}

// RetrieveAndGenerate 检索相关内容并生成回答
func (s *ragService) RetrieveAndGenerate(ctx context.Context, query string, options RAGOptions) (*RAGResponse, error) {
	startTime := time.Now()

	// 设置默认选项
	if options.TopK <= 0 {
		options.TopK = 5
	}
	if options.MaxContextTokens <= 0 {
		options.MaxContextTokens = 4000
	}
	if options.MinScore <= 0 {
		options.MinScore = 0.3
	}
	if options.Temperature < 0 {
		options.Temperature = 0.7
	}
	if options.MaxTokens <= 0 {
		options.MaxTokens = 2000
	}

	// 检查缓存
	cacheKey := s.getCacheKey(query, options)
	if cached, found := s.cache.Get(cacheKey); found {
		if response, ok := cached.(*RAGResponse); ok {
			return response, nil
		}
	}

	// 检索相关内容
	results, err := s.Retrieve(ctx, query, options.TopK)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve: %w", err)
	}

	// 过滤低分结果
	filteredResults := s.filterByScore(results, options.MinScore)

	// 构建上下文
	context := s.BuildContext(filteredResults, options.MaxContextTokens)

	// 生成回答
	answer, err := s.GenerateWithRAG(ctx, query, context)
	if err != nil {
		return nil, fmt.Errorf("failed to generate: %w", err)
	}

	// 构建响应
	response := &RAGResponse{
		Answer:         answer,
		Query:          query,
		Context:        context,
		RetrievedCount: len(filteredResults),
		ProcessedTime:  time.Since(startTime),
	}

	// 添加来源
	if options.IncludeSources {
		response.Sources = s.buildSources(filteredResults)
	}

	// 缓存响应
	s.cache.Set(cacheKey, response, 10*time.Minute)

	return response, nil
}

// Retrieve 检索相关上下文
func (s *ragService) Retrieve(ctx context.Context, query string, topK int) ([]KnowledgeSearchResult, error) {
	if topK <= 0 {
		topK = 5
	}

	// 使用混合搜索获取相关内容
	results, err := s.knowledge.HybridSearchKnowledge(ctx, query, topK)
	if err != nil {
		return nil, fmt.Errorf("hybrid search failed: %w", err)
	}

	// 增加访问计数
	for _, result := range results {
		_ = s.knowledge.IncrementAccessCount(result.ID)
	}

	return results, nil
}

// BuildContext 构建上下文
func (s *ragService) BuildContext(results []KnowledgeSearchResult, maxTokens int) string {
	if len(results) == 0 {
		return ""
	}

	var context strings.Builder

	// 添加上下文引导语
	context.WriteString("参考信息：\n\n")

	// 添加每个检索结果
	for i, result := range results {
		context.WriteString(fmt.Sprintf("[%d] %s\n", i+1, result.Title))

		// 优先使用摘要
		if result.Summary != "" {
			context.WriteString(result.Summary)
			context.WriteString("\n")
		} else if result.Content != "" {
			// 限制内容长度
			content := result.Content
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			context.WriteString(content)
			context.WriteString("\n")
		}

		// 添加来源信息
		if result.Source != "" {
			context.WriteString(fmt.Sprintf("来源: %s", result.Source))
			if result.SourceID != "" {
				context.WriteString(fmt.Sprintf(" (ID: %s)", result.SourceID))
			}
			context.WriteString("\n")
		}

		context.WriteString(fmt.Sprintf("相关度: %.2f\n\n", result.Score))

		// 估算 token 数，如果超过限制则停止
		estimatedTokens := context.Len() / 3 // 粗略估算：1 token ≈ 3 字符
		if estimatedTokens > maxTokens {
			// 移除最后一个结果
			contextStr := context.String()
			lastNewline := strings.LastIndex(contextStr[:len(contextStr)-100], "\n\n")
			if lastNewline > 0 {
				return contextStr[:lastNewline]
			}
			break
		}
	}

	return context.String()
}

// GenerateWithRAG 使用 RAG 生成回答
func (s *ragService) GenerateWithRAG(ctx context.Context, query string, context string) (string, error) {
	// 构建系统提示词
	systemPrompt := s.buildSystemPrompt()

	// 构建消息
	messages := []llm.Message{
		{
			Role:    llm.RoleSystem,
			Content: systemPrompt,
		},
	}

	// 添加上下文
	if context != "" {
		messages = append(messages, llm.Message{
			Role:    llm.RoleSystem,
			Content: context,
		})
	}

	// 添加用户问题
	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: query,
	})

	// 构建请求
	req := &llm.ChatRequest{
		Messages:    messages,
		Model:       "", // 使用默认模型
		Temperature: 0.7,
		MaxTokens:   2000,
	}

	// 调用 LLM
	resp, err := s.llm.Chat(ctx, req)
	if err != nil {
		return "", fmt.Errorf("LLM chat failed: %w", err)
	}

	// 提取回答
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response choices")
	}

	return resp.Choices[0].Message.Content, nil
}

// RetrieveAndGenerateStream 流式检索并生成回答
func (s *ragService) RetrieveAndGenerateStream(ctx context.Context, query string, options RAGOptions) (<-chan llm.ChatStreamChunk, error) {
	// 设置默认选项
	if options.TopK <= 0 {
		options.TopK = 5
	}
	if options.MaxContextTokens <= 0 {
		options.MaxContextTokens = 4000
	}
	if options.MinScore <= 0 {
		options.MinScore = 0.3
	}
	if options.Temperature < 0 {
		options.Temperature = 0.7
	}

	// 检索相关内容
	results, err := s.Retrieve(ctx, query, options.TopK)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve: %w", err)
	}

	// 过滤低分结果
	filteredResults := s.filterByScore(results, options.MinScore)

	// 构建上下文
	context := s.BuildContext(filteredResults, options.MaxContextTokens)

	// 构建系统提示词
	systemPrompt := options.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = s.buildSystemPrompt()
	}

	// 构建消息
	messages := []llm.Message{
		{
			Role:    llm.RoleSystem,
			Content: systemPrompt,
		},
	}

	// 添加上下文
	if context != "" {
		messages = append(messages, llm.Message{
			Role:    llm.RoleSystem,
			Content: context,
		})
	}

	// 添加用户问题
	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: query,
	})

	// 构建请求
	req := &llm.ChatRequest{
		Messages:    messages,
		Model:       options.Model,
		Temperature: options.Temperature,
		MaxTokens:   options.MaxTokens,
	}

	// 调用流式 LLM
	return s.llm.ChatStream(ctx, req)
}

// filterByScore 根据分数过滤结果
func (s *ragService) filterByScore(results []KnowledgeSearchResult, minScore float64) []KnowledgeSearchResult {
	if minScore <= 0 {
		return results
	}

	filtered := make([]KnowledgeSearchResult, 0, len(results))
	for _, result := range results {
		if result.Score >= minScore {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

// buildSources 构建来源列表
func (s *ragService) buildSources(results []KnowledgeSearchResult) []RAGSource {
	sources := make([]RAGSource, 0, len(results))
	for _, result := range results {
		content := result.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}

		sources = append(sources, RAGSource{
			ID:         result.ID,
			Title:      result.Title,
			Content:    content,
			Summary:    result.Summary,
			Score:      result.Score,
			Source:     result.Source,
			SourceID:   result.SourceID,
			CategoryID: result.CategoryID,
		})
	}
	return sources
}

// buildSystemPrompt 构建系统提示词
func (s *ragService) buildSystemPrompt() string {
	return `你是一个智能助手，擅长回答用户的问题。

基于提供的参考信息回答用户问题。请遵循以下规则：

1. 如果参考信息中包含答案，请基于参考信息作答，并标注信息来源
2. 如果参考信息不足以回答问题，请说明并提供你能给出的最佳回答
3. 回答时要条理清晰，重点突出
4. 可以适当引用参考信息中的具体内容，使用 [1], [2] 等标记来源
5. 如果参考信息之间有冲突，请指出并说明

请用自然、友好的语气回答问题。`
}

// getCacheKey 生成缓存键
func (s *ragService) getCacheKey(query string, options RAGOptions) string {
	return fmt.Sprintf("rag:%s:%d:%.2f", query, options.TopK, options.MinScore)
}

// 默认的角色常量
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

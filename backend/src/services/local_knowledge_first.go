package services

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"openaide/backend/src/services/llm"
)

const (
	LocalKnowledgeHighThreshold   = 0.85
	LocalKnowledgeMediumThreshold = 0.50
)

type LocalKnowledgeResult struct {
	Answer     string
	Sources    []RAGSource
	Score      float64
	FromLocal  bool
	SavedTokens int
}

type LocalKnowledgeStats struct {
	TotalQueries   int64
	LocalHits      int64
	PartialHits    int64
	Misses         int64
	SavedTokens    int64
}

type LocalKnowledgeFirst struct {
	knowledgeSvc KnowledgeService
	ragSvc       RAGService
	tokenEst     *TokenEstimator
	stats        LocalKnowledgeStats
}

func NewLocalKnowledgeFirst(knowledgeSvc KnowledgeService, ragSvc RAGService, tokenEst *TokenEstimator) *LocalKnowledgeFirst {
	return &LocalKnowledgeFirst{
		knowledgeSvc: knowledgeSvc,
		ragSvc:       ragSvc,
		tokenEst:     tokenEst,
	}
}

func (s *LocalKnowledgeFirst) Query(ctx context.Context, query string, topK int) (*LocalKnowledgeResult, error) {
	atomic.AddInt64(&s.stats.TotalQueries, 1)

	searchCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	results, err := s.ragSvc.Retrieve(searchCtx, query, topK)
	if err != nil {
		atomic.AddInt64(&s.stats.Misses, 1)
		return nil, err
	}

	if len(results) == 0 {
		atomic.AddInt64(&s.stats.Misses, 1)
		return &LocalKnowledgeResult{FromLocal: false}, nil
	}

	best := results[0]

	if best.Score >= LocalKnowledgeHighThreshold {
		atomic.AddInt64(&s.stats.LocalHits, 1)
		answer := s.buildDirectAnswer(results)
		savedTokens := s.tokenEst.EstimateTokens(query, "default") +
			s.tokenEst.EstimateTokens(answer, "default")
		atomic.AddInt64(&s.stats.SavedTokens, int64(savedTokens))

		return &LocalKnowledgeResult{
			Answer:      answer,
			Sources:     s.toRAGSources(results),
			Score:       best.Score,
			FromLocal:   true,
			SavedTokens: savedTokens,
		}, nil
	}

	if best.Score >= LocalKnowledgeMediumThreshold {
		atomic.AddInt64(&s.stats.PartialHits, 1)
		return &LocalKnowledgeResult{
			Answer:    s.buildReferenceContext(results),
			Sources:   s.toRAGSources(results),
			Score:     best.Score,
			FromLocal: false,
		}, nil
	}

	atomic.AddInt64(&s.stats.Misses, 1)
	return &LocalKnowledgeResult{FromLocal: false}, nil
}

func (s *LocalKnowledgeFirst) buildDirectAnswer(results []KnowledgeSearchResult) string {
	var sb strings.Builder
	best := results[0]

	sb.WriteString(best.Content)

	if best.Summary != "" && best.Summary != best.Content {
		sb.WriteString("\n\n")
		sb.WriteString(best.Summary)
	}

	if len(results) > 1 {
		sb.WriteString("\n\n---\n相关补充：\n")
		for i := 1; i < len(results) && i < 3; i++ {
			sb.WriteString(fmt.Sprintf("- %s", results[i].Summary))
			if results[i].Summary == "" {
				content := results[i].Content
				if len(content) > 200 {
					content = content[:200] + "..."
				}
				sb.WriteString(content)
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func (s *LocalKnowledgeFirst) buildReferenceContext(results []KnowledgeSearchResult) string {
	return s.ragSvc.BuildContext(results, 1500)
}

func (s *LocalKnowledgeFirst) toRAGSources(results []KnowledgeSearchResult) []RAGSource {
	sources := make([]RAGSource, 0, len(results))
	for _, r := range results {
		sources = append(sources, RAGSource{
			ID:       r.ID,
			Title:    r.Title,
			Content:  r.Content,
			Summary:  r.Summary,
			Score:    r.Score,
			Source:   r.Source,
			SourceID: r.SourceID,
		})
	}
	return sources
}

func (s *LocalKnowledgeFirst) GetStats() LocalKnowledgeStats {
	return LocalKnowledgeStats{
		TotalQueries: atomic.LoadInt64(&s.stats.TotalQueries),
		LocalHits:    atomic.LoadInt64(&s.stats.LocalHits),
		PartialHits:  atomic.LoadInt64(&s.stats.PartialHits),
		Misses:       atomic.LoadInt64(&s.stats.Misses),
		SavedTokens:  atomic.LoadInt64(&s.stats.SavedTokens),
	}
}

func (s *LocalKnowledgeFirst) ShouldTryLocal(query string) bool {
	if len(strings.TrimSpace(query)) < 4 {
		return false
	}

	skipPatterns := []string{"帮我写", "生成", "创建", "翻译这段", "改写", "续写"}
	lower := strings.ToLower(query)
	for _, p := range skipPatterns {
		if strings.HasPrefix(lower, p) {
			return false
		}
	}

	return true
}

func (s *LocalKnowledgeFirst) ToStreamChunks(answer string) <-chan llm.ChatStreamChunk {
	ch := make(chan llm.ChatStreamChunk, 1)
	go func() {
		defer close(ch)
		ch <- llm.ChatStreamChunk{
			Choices: []llm.StreamChoice{
				{
					Delta: llm.StreamDelta{
						Content: answer,
						Role:    "assistant",
					},
				},
			},
		}
	}()
	return ch
}

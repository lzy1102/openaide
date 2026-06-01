package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SimpleLearner 简单学习实现，支持语义匹配
type SimpleLearner struct {
	dataDir  string
	insights []Insight
	mu       sync.RWMutex
	llm      LLMProvider // 可选：LLM 语义分类
	embedder Embedder    // 可选：语义匹配
}

// SetLLM 注入 LLM 用于智能偏好检测
func (l *SimpleLearner) SetLLM(llm LLMProvider) { l.llm = llm }

// SetEmbedder 注入向量化器用于语义匹配
func (l *SimpleLearner) SetEmbedder(e Embedder) { l.embedder = e }

// Insight 学习洞察
type Insight struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"` // pattern, preference, correction, strategy
	Content    string    `json:"content"`
	Embedding  []float32 `json:"embedding,omitempty"`
	Frequency  int       `json:"frequency"`
	Confidence float64   `json:"confidence"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// NewSimpleLearner 创建简单学习器
func NewSimpleLearner(dataDir string) (*SimpleLearner, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	l := &SimpleLearner{
		dataDir:  dataDir,
		insights: make([]Insight, 0),
	}

	l.load()
	return l, nil
}

// Learn 从交互中学习
func (l *SimpleLearner) Learn(ctx context.Context, record ExecutionRecord) error {
	var newInsights []Insight

	// Extract patterns outside lock (embedding may call LLM API)
	if record.Success && len(record.ToolCalls) > 0 {
		toolNames := make([]string, len(record.ToolCalls))
		for i, tc := range record.ToolCalls {
			toolNames[i] = tc.Function.Name
		}
		pattern := strings.Join(toolNames, " -> ")
		newInsights = append(newInsights, Insight{
			Type: "pattern", Content: fmt.Sprintf("成功模式: %s", pattern),
			Frequency: 1, Confidence: 0.7,
		})
	}
	if !record.Success {
		newInsights = append(newInsights, Insight{
			Type: "correction", Content: fmt.Sprintf("失败类型: %s", record.Error),
			Frequency: 1, Confidence: 0.5,
		})
	}
	if l.llm != nil {
		if pref := l.detectPreferenceWithLLM(record.Query); pref != "" {
			newInsights = append(newInsights, Insight{
				Type: "preference", Content: pref, Frequency: 1, Confidence: 0.7,
			})
		}
	} else if strings.Contains(record.Query, "代码") || strings.Contains(record.Query, "code") {
		newInsights = append(newInsights, Insight{
			Type: "preference", Content: "用户偏好代码相关回答", Frequency: 1, Confidence: 0.6,
		})
	}

	// Embed outside lock
	for i := range newInsights {
		if l.embedder != nil && l.embedder.Dimension() > 0 {
			if vec, err := l.embedder.Embed(context.Background(), newInsights[i].Content); err == nil && len(vec) > 0 {
				newInsights[i].Embedding = vec
			}
		}
	}

	// Merge into store (lock only for map/slice update)
	l.mu.Lock()
	for _, insight := range newInsights {
		l.addOrUpdateInsight(insight)
	}
	err := l.save()
	l.mu.Unlock()
	return err
}

func (l *SimpleLearner) detectPreferenceWithLLM(query string) string {
	resp, err := l.llm.Chat(context.Background(), []Message{
		{Role: "user", Content: fmt.Sprintf("Classify this user query into ONE category. Reply with only the category name.\n\nQuery: %s\n\nCategories: coding, writing, research, devops, learning, design, business, general", query)},
	}, nil, map[string]interface{}{"max_tokens": 15, "temperature": 0})
	if err != nil || resp.Content == "" {
		return ""
	}
	cat := strings.TrimSpace(strings.ToLower(resp.Content))
	categoryNames := map[string]string{
		"coding": "用户偏好编程与技术相关回答", "writing": "用户偏好写作与创作相关回答",
		"research": "用户偏好研究与分析相关回答", "devops": "用户偏好运维与部署相关回答",
		"learning": "用户偏好学习与教学相关回答", "design": "用户偏好设计与架构相关回答",
		"business": "用户偏好商业与管理相关回答", "general": "用户偏好通用型回答",
	}
	if name, ok := categoryNames[cat]; ok {
		return name
	}
	return ""
}

// GetInsights finds the most relevant learned insights for a query.
// Uses semantic matching if an embedder is configured, otherwise falls back to text.
func (l *SimpleLearner) GetInsights(ctx context.Context, query string) ([]string, error) {
	// Semantic match via embedding
	if l.embedder != nil && l.embedder.Dimension() > 0 {
		queryVec, err := l.embedder.Embed(ctx, query)
		if err == nil && len(queryVec) > 0 {
			type scored struct {
				content string
				score   float64
			}
			l.mu.RLock()
			var candidates []scored
			for _, insight := range l.insights {
				if insight.Confidence < 0.5 || insight.Frequency < 2 {
					continue // skip low-confidence, rarely-seen insights
				}
				if len(insight.Embedding) == len(queryVec) {
					score := CosineSimilarity(queryVec, insight.Embedding)
					if score > 0.5 {
						candidates = append(candidates, scored{insight.Content, score})
					}
				}
			}
			l.mu.RUnlock()

			// Sort by score descending
			for i := 0; i < len(candidates); i++ {
				for j := i + 1; j < len(candidates); j++ {
					if candidates[j].score > candidates[i].score {
						candidates[i], candidates[j] = candidates[j], candidates[i]
					}
				}
			}
			var results []string
			for i := 0; i < len(candidates) && i < 5; i++ {
				results = append(results, candidates[i].content)
			}
			return results, nil
		}
		// Embedding failed — fall through to text match
	}

	// Text fallback: substring match
	l.mu.RLock()
	defer l.mu.RUnlock()
	var results []string
	queryLower := strings.ToLower(query)
	for _, insight := range l.insights {
		if insight.Confidence < 0.4 {
			continue
		}
		if strings.Contains(strings.ToLower(insight.Content), queryLower) {
			results = append(results, insight.Content)
		}
	}
	return results, nil
}

// GetAllInsights 获取所有洞察
func (l *SimpleLearner) GetAllInsights() []Insight {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]Insight, len(l.insights))
	copy(result, l.insights)
	return result
}

func (l *SimpleLearner) addOrUpdateInsight(newInsight Insight) {
	newInsight.ID = fmt.Sprintf("insight_%d", time.Now().UnixNano())
	newInsight.CreatedAt = time.Now()
	newInsight.UpdatedAt = time.Now()

	// Generate embedding for semantic matching (async, best-effort)
	if l.embedder != nil && l.embedder.Dimension() > 0 {
		if vec, err := l.embedder.Embed(context.Background(), newInsight.Content); err == nil && len(vec) > 0 {
			newInsight.Embedding = vec
		}
	}

	// 查找相似洞察并合并
	for i, existing := range l.insights {
		if existing.Type == newInsight.Type && similarContent(existing.Content, newInsight.Content) {
			l.insights[i].Frequency++
			l.insights[i].Confidence = min(0.95, existing.Confidence+0.05)
			l.insights[i].UpdatedAt = time.Now()
			// Update embedding if new one has better embedding
			if len(newInsight.Embedding) > 0 && len(l.insights[i].Embedding) == 0 {
				l.insights[i].Embedding = newInsight.Embedding
			}
			return
		}
	}

	l.insights = append(l.insights, newInsight)
	if len(l.insights) > 200 {
		l.insights = l.insights[len(l.insights)-200:] // keep last 200
	}
}

func (l *SimpleLearner) save() error {
	path := filepath.Join(l.dataDir, "insights.json")
	data, err := json.MarshalIndent(l.insights, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (l *SimpleLearner) load() error {
	path := filepath.Join(l.dataDir, "insights.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil // 文件不存在不报错
	}
	return json.Unmarshal(data, &l.insights)
}

func similarContent(a, b string) bool {
	// 简单相似度检查：共享至少 50% 的词
	wordsA := strings.Fields(a)
	wordsB := strings.Fields(b)

	if len(wordsA) == 0 || len(wordsB) == 0 {
		return false
	}

	setA := make(map[string]bool)
	for _, w := range wordsA {
		setA[strings.ToLower(w)] = true
	}

	common := 0
	for _, w := range wordsB {
		if setA[strings.ToLower(w)] {
			common++
		}
	}

	return float64(common) >= float64(len(wordsA))*0.5
}

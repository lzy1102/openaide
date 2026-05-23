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

// SimpleLearner 简单学习实现
type SimpleLearner struct {
	dataDir  string
	insights []Insight
	mu       sync.RWMutex
	llm      LLMProvider // 可选：LLM 语义分类
}

// SetLLM 注入 LLM 用于智能偏好检测
func (l *SimpleLearner) SetLLM(llm LLMProvider) { l.llm = llm }

// Insight 学习洞察
type Insight struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`      // pattern, preference, correction, strategy
	Content   string    `json:"content"`
	Frequency int       `json:"frequency"`
	Confidence float64  `json:"confidence"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
	l.mu.Lock()
	defer l.mu.Unlock()

	// 提取模式
	if record.Success && len(record.ToolCalls) > 0 {
		// 学习成功的工具使用模式
		toolNames := make([]string, len(record.ToolCalls))
		for i, tc := range record.ToolCalls {
			toolNames[i] = tc.Function.Name
		}
		pattern := strings.Join(toolNames, " -> ")

		l.addOrUpdateInsight(Insight{
			Type:      "pattern",
			Content:   fmt.Sprintf("成功模式: %s", pattern),
			Frequency: 1,
			Confidence: 0.7,
		})
	}

	// 学习失败模式
	if !record.Success {
		l.addOrUpdateInsight(Insight{
			Type:      "correction",
			Content:   fmt.Sprintf("失败类型: %s", record.Error),
			Frequency: 1,
			Confidence: 0.5,
		})
	}

	// 学习用户偏好（LLM 分类 > 关键词兜底）
	if l.llm != nil {
		if pref := l.detectPreferenceWithLLM(record.Query); pref != "" {
			l.addOrUpdateInsight(Insight{
				Type:       "preference",
				Content:    pref,
				Frequency:  1,
				Confidence: 0.7,
			})
		}
	} else if strings.Contains(record.Query, "代码") || strings.Contains(record.Query, "code") {
		l.addOrUpdateInsight(Insight{
			Type:       "preference",
			Content:    "用户偏好代码相关回答",
			Frequency:  1,
			Confidence: 0.6,
		})
	}

	return l.save()
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

// GetInsights 获取学习洞察
func (l *SimpleLearner) GetInsights(ctx context.Context, query string) ([]string, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var results []string
	queryLower := strings.ToLower(query)

	for _, insight := range l.insights {
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

	// 查找相似洞察并合并
	for i, existing := range l.insights {
		if existing.Type == newInsight.Type && similarContent(existing.Content, newInsight.Content) {
			l.insights[i].Frequency++
			l.insights[i].Confidence = min(0.95, existing.Confidence+0.05)
			l.insights[i].UpdatedAt = time.Now()
			return
		}
	}

	l.insights = append(l.insights, newInsight)
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

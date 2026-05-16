package memory

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// Vector 稀疏向量
type Vector map[string]float64

// VectorIndex 向量索引 — TF-IDF 语义搜索
type VectorIndex struct {
	documents  []vectorDoc
	vocabulary map[string]int // 词 → 文档频率
	docCount   int
}

type vectorDoc struct {
	id      string
	content string
	vector  Vector
}

// NewVectorIndex 创建向量索引
func NewVectorIndex() *VectorIndex {
	return &VectorIndex{
		vocabulary: make(map[string]int),
	}
}

// Add 添加文档到索引
func (vi *VectorIndex) Add(id, content string) {
	vi.docCount++
	tokens := tokenize(content)
	tf := termFrequency(tokens)

	// 更新词汇文档频率
	seen := make(map[string]bool)
	for token := range tf {
		if !seen[token] {
			vi.vocabulary[token]++
			seen[token] = true
		}
	}

	vi.documents = append(vi.documents, vectorDoc{
		id:      id,
		content: content,
		vector:  tf,
	})
}

// Search 搜索最相似的文档，返回 ID 列表和相似度分数
func (vi *VectorIndex) Search(query string, limit int) ([]string, []float64) {
	if len(vi.documents) == 0 {
		return nil, nil
	}

	queryVec := termFrequency(tokenize(query))
	queryVec = vi.tfidf(queryVec)

	type scored struct {
		id    string
		score float64
	}

	var results []scored
	for _, doc := range vi.documents {
		docVec := vi.tfidf(doc.vector)
		sim := cosineSimilarity(queryVec, docVec)
		if sim > 0 {
			results = append(results, scored{doc.id, sim})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	ids := make([]string, len(results))
	scores := make([]float64, len(results))
	for i, r := range results {
		ids[i] = r.id
		scores[i] = r.score
	}

	return ids, scores
}

// tfidf 计算 TF-IDF 权重
func (vi *VectorIndex) tfidf(tf Vector) Vector {
	result := make(Vector)
	n := float64(vi.docCount)

	for term, freq := range tf {
		df := float64(vi.vocabulary[term])
		if df == 0 {
			df = 1
		}
		idf := math.Log(n/df) + 1
		result[term] = freq * idf
	}

	return result
}

// cosineSimilarity 余弦相似度
func cosineSimilarity(a, b Vector) float64 {
	var dot, normA, normB float64

	for term, valA := range a {
		normA += valA * valA
		if valB, ok := b[term]; ok {
			dot += valA * valB
		}
	}

	for _, valB := range b {
		normB += valB * valB
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// termFrequency 计算词频
func termFrequency(tokens []string) Vector {
	tf := make(Vector)
	for _, t := range tokens {
		tf[t]++
	}
	// 归一化
	total := float64(len(tokens))
	if total > 0 {
		for t := range tf {
			tf[t] /= total
		}
	}
	return tf
}

// tokenize 分词（中英文混合）
func tokenize(text string) []string {
	var tokens []string
	var current []rune

	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current = append(current, r)
		} else if unicode.Is(unicode.Han, r) {
			// 中文字符，每个字作为独立 token
			if len(current) > 0 {
				tokens = append(tokens, string(current))
				current = nil
			}
			tokens = append(tokens, string(r))
		} else {
			if len(current) > 0 {
				tokens = append(tokens, string(current))
				current = nil
			}
		}
	}

	if len(current) > 0 {
		tokens = append(tokens, string(current))
	}

	// 过滤太短的 token
	filtered := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if len([]rune(t)) >= 1 {
			filtered = append(filtered, t)
		}
	}

	return filtered
}

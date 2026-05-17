package memory

import (
	"testing"
)

func TestTokenize_English(t *testing.T) {
	tokens := tokenize("hello world")
	if len(tokens) != 2 {
		t.Errorf("expected 2 tokens, got %d: %v", len(tokens), tokens)
	}
}

func TestTokenize_Chinese(t *testing.T) {
	tokens := tokenize("你好世界")
	if len(tokens) != 4 {
		t.Errorf("4 Chinese chars should be 4 tokens, got %d: %v", len(tokens), tokens)
	}
}

func TestTokenize_Mixed(t *testing.T) {
	tokens := tokenize("数据库database配置")
	// 数=CN, 据=CN, 库=CN, database=EN, 配=CN, 置=CN = 6 tokens
	if len(tokens) != 6 {
		t.Errorf("expected 6 tokens (3CN+database+2CN), got %d: %v", len(tokens), tokens)
	}
}

func TestVectorIndex_AddSearch(t *testing.T) {
	vi := NewVectorIndex()
	vi.Add("doc1", "Go is a programming language for building web services")
	vi.Add("doc2", "Python is great for data science and machine learning")
	vi.Add("doc3", "Java enterprise applications with Spring framework")

	ids, scores := vi.Search("web programming Go", 2)
	if len(ids) == 0 {
		t.Error("no results")
	}
	if ids[0] != "doc1" {
		t.Errorf("expected doc1 first, got %s", ids[0])
	}
	if len(scores) != len(ids) {
		t.Error("scores count mismatch")
	}
}

func TestVectorIndex_EmptySearch(t *testing.T) {
	vi := NewVectorIndex()
	ids, _ := vi.Search("nothing", 5)
	if len(ids) != 0 {
		t.Error("expected no results for empty index")
	}
}

func TestVectorIndex_ChineseSearch(t *testing.T) {
	vi := NewVectorIndex()
	vi.Add("d1", "用户认证使用JWT token进行身份验证")
	vi.Add("d2", "数据库连接池配置优化参数")
	vi.Add("d3", "前端React组件渲染性能优化")

	ids, _ := vi.Search("认证 身份 JWT token", 2)
	if len(ids) == 0 {
		t.Error("no results for Chinese search")
	}
	if ids[0] != "d1" {
		t.Errorf("expected d1 first, got %s", ids[0])
	}
}

func TestCosineSimilarity(t *testing.T) {
	a := Vector{"a": 1.0, "b": 0.5}
	b := Vector{"a": 1.0, "b": 0.5}
	sim := cosineSimilarity(a, b)
	if sim < 0.99 {
		t.Errorf("identical vectors should have sim ~1.0, got %.4f", sim)
	}

	c := Vector{"x": 1.0, "y": 1.0}
	sim2 := cosineSimilarity(a, c)
	if sim2 > 0.01 {
		t.Errorf("orthogonal vectors should have sim ~0, got %.4f", sim2)
	}
}

func TestTermFrequency(t *testing.T) {
	tf := termFrequency([]string{"a", "b", "a", "c", "a"})
	if tf["a"] <= tf["b"] {
		t.Error("'a' should have higher frequency")
	}
}

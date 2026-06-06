package kernel

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

// SemanticPatternDetector clusters user queries by embedding similarity and emits
// distillable patterns when clusters mature. Uses embedding-based clustering +
// LLM distillation to extract reusable knowledge.
type SemanticPatternDetector struct {
	embedder  Embedder
	mu        sync.Mutex
	clusters  []queryCluster
	minSize   int
	threshold float64
}

type queryCluster struct {
	examples   []clusterExample
	embeddings [][]float32
	distilled  bool
}

type clusterExample struct {
	query    string
	response string
}

// NewSemanticPatternDetector creates a detector.
func NewSemanticPatternDetector(embedder Embedder, minSize int, threshold float64) *SemanticPatternDetector {
	if minSize < 3 { minSize = 3 }
	if threshold <= 0 || threshold > 1 { threshold = 0.80 }
	return &SemanticPatternDetector{
		embedder: embedder, minSize: minSize, threshold: threshold,
		clusters: make([]queryCluster, 0, 32),
	}
}

// Detect collects query+response pairs, clusters them, and returns mature patterns.
func (d *SemanticPatternDetector) Detect(ctx context.Context, sessionID string, messages []Message) ([]Pattern, error) {
	if d.embedder == nil || d.embedder.Dimension() == 0 {
		return nil, nil
	}

	pairs := extractPairs(messages)
	if len(pairs) == 0 { return nil, nil }

	texts := make([]string, len(pairs))
	for i, p := range pairs { texts[i] = p.query }
	embs, err := d.embedder.EmbedBatch(ctx, texts)
	if err != nil || len(embs) == 0 { return nil, err }

	d.mu.Lock()
	defer d.mu.Unlock()

	for i, p := range pairs {
		if i >= len(embs) { break }
		d.addToCluster(p, embs[i])
	}
	return d.collectPatterns(), nil
}

func (d *SemanticPatternDetector) addToCluster(ex clusterExample, emb []float32) {
	if len(emb) == 0 { return }

	bestIdx, bestSim := -1, d.threshold
	for i, c := range d.clusters {
		if c.distilled { continue }
		for _, e := range c.embeddings {
			if sim := CosineSimilarity(emb, e); sim > bestSim {
				bestSim, bestIdx = sim, i
			}
		}
	}

	if bestIdx >= 0 {
		c := &d.clusters[bestIdx]
		c.examples = append(c.examples, ex)
		if len(c.embeddings) < 20 { c.embeddings = append(c.embeddings, emb) }
	} else {
		d.clusters = append(d.clusters, queryCluster{
			examples: []clusterExample{ex}, embeddings: [][]float32{emb},
		})
	}
	if len(d.clusters) > 100 { d.prune() }
}

func (d *SemanticPatternDetector) collectPatterns() []Pattern {
	var patterns []Pattern
	for i := range d.clusters {
		c := &d.clusters[i]
		if c.distilled || len(c.examples) < d.minSize { continue }
		c.distilled = true
		theme := extractClusterTheme(c.examples)
		if theme == "" { continue }
		patterns = append(patterns, Pattern{
			Type: "distillable_cluster", Description: theme,
			Confidence: clampConf(float64(len(c.examples)) / float64(d.minSize) * 0.9),
			Frequency:  len(c.examples),
		})
	}
	return patterns
}

// GetDistillableExamples returns raw examples from recently extracted clusters.
func (d *SemanticPatternDetector) GetDistillableExamples() [][]clusterExample {
	d.mu.Lock()
	defer d.mu.Unlock()
	var result [][]clusterExample
	for _, c := range d.clusters {
		if c.distilled && len(c.examples) >= d.minSize {
			result = append(result, c.examples)
		}
	}
	return result
}

func (d *SemanticPatternDetector) prune() {
	sort.Slice(d.clusters, func(i, j int) bool {
		return len(d.clusters[i].examples) > len(d.clusters[j].examples)
	})
	if len(d.clusters) > 50 { d.clusters = d.clusters[:50] }
}

// evaluateClusterQuality uses LLM to judge whether a cluster of similar queries
// represents a reusable pattern worth extracting as a skill.
// Returns 0.0–1.0 quality score. Below 0.5 = skip.
func evaluateClusterQuality(ctx context.Context, llm LLMProvider, p Pattern, idx int, examples [][]clusterExample) float64 {
	if llm == nil || p.Frequency < 3 {
		return 0
	}
	prompt := "You are evaluating whether a cluster of user queries forms a coherent, reusable pattern.\n\n"
	prompt += fmt.Sprintf("Theme: %s\nQuery count: %d\n\n", p.Description, p.Frequency)
	if idx < len(examples) && len(examples[idx]) > 0 {
		prompt += "Sample queries:\n"
		n := len(examples[idx])
		if n > 5 { n = 5 }
		for j := 0; j < n; j++ {
			prompt += fmt.Sprintf("- %s\n", truncStr(examples[idx][j].query, 150))
		}
	}
	prompt += "\nJudge: are these queries truly about the same topic and worth distilling into a reusable skill? "
	prompt += "Reply with ONLY a number 0.0–1.0 (0=random noise, 1=clearly reusable pattern)."

	resp, err := llm.Chat(ctx, []Message{{Role: "user", Content: prompt}}, nil,
		map[string]interface{}{"max_tokens": 10, "temperature": 0, "route": "execution", "no_thinking": true})
	if err != nil || resp.Content == "" {
		return 0.5 // default pass on error (don't block)
	}
	var score float64
	fmt.Sscanf(strings.TrimSpace(resp.Content), "%f", &score)
	if score < 0 { score = 0 }
	if score > 1 { score = 1 }
	return score
}

// ── Distillation ─────────────────────────────────────────────

// DistillCluster sends query+response pairs to an LLM and extracts reusable knowledge.
func DistillCluster(ctx context.Context, llm LLMProvider, examples []clusterExample) string {
	if llm == nil || len(examples) < 2 { return "" }

	var sb strings.Builder
	sb.WriteString("You are a knowledge distillation expert. Given similar tasks and their solutions, extract reusable patterns including tool-use strategies.\n\n")
	sb.WriteString("## Similar Tasks\n\n")
	for i, ex := range examples {
		if i >= 8 { break }
		sb.WriteString(fmt.Sprintf("### Task %d\n**Query:** %s\n**Solution:** %s\n\n",
			i+1, truncStr(ex.query, 300), truncStr(ex.response, 800)))
	}
	sb.WriteString("## Your Task\nDistill these into a concise skill card (under 400 words):\n")
	sb.WriteString("1. **Key files** — which files are always involved\n2. **Tool strategy** — optimal tool sequence\n3. **Gotchas** — specific mistakes to avoid\n4. **Best approach** — step-by-step directive")

	resp, err := llm.Chat(ctx, []Message{{Role: "user", Content: sb.String()}}, nil,
		map[string]interface{}{"max_tokens": 600, "temperature": 0.3, "route": "execution", "no_thinking": true})
	if err != nil || resp.Content == "" { return "" }
	return strings.TrimSpace(resp.Content)
}

// ── Helpers ──────────────────────────────────────────────────

func extractPairs(messages []Message) []clusterExample {
	var pairs []clusterExample
	var lastQ string
	for _, msg := range messages {
		if msg.Role == "user" && len(strings.TrimSpace(msg.Content)) > 5 {
			lastQ = strings.TrimSpace(msg.Content)
		} else if msg.Role == "assistant" && lastQ != "" && msg.Content != "" {
			pairs = append(pairs, clusterExample{query: lastQ, response: strings.TrimSpace(msg.Content)})
			lastQ = ""
		}
	}
	return pairs
}

func extractClusterTheme(examples []clusterExample) string {
	if len(examples) < 2 { return "" }
	wordFreq := make(map[string]int)
	for _, ex := range examples {
		seen := make(map[string]bool)
		for _, w := range tokenize(ex.query) {
			if !seen[w] { wordFreq[w]++; seen[w] = true }
		}
	}
	minCount := int(math.Ceil(float64(len(examples)) * 0.6))
	var words []string
	for w, c := range wordFreq {
		if c >= minCount && len(w) > 2 { words = append(words, w) }
	}
	sort.Slice(words, func(i, j int) bool { return wordFreq[words[i]] > wordFreq[words[j]] })
	if len(words) == 0 { return shortestQuery(examples) }
	if len(words) > 4 { words = words[:4] }
	return strings.Join(words, " ")
}

func shortestQuery(examples []clusterExample) string {
	s := examples[0].query
	for _, ex := range examples[1:] {
		if len(ex.query) < len(s) { s = ex.query }
	}
	if len(s) > 80 { s = s[:80] + "..." }
	return s
}

func truncStr(s string, n int) string {
	if len(s) <= n { return s }
	return s[:n] + "..."
}

func clampConf(c float64) float64 {
	if c > 0.95 { return 0.95 }
	if c < 0.3 { return 0.3 }
	return c
}

func tokenize(s string) []string {
	words := strings.Fields(strings.ToLower(s))
	var result []string
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?\"'()[]{}*#")
		if len(w) > 2 && !isStopWord(w) { result = append(result, w) }
	}
	return result
}

func isStopWord(w string) bool {
	switch w {
	case "the", "and", "for", "that", "this", "with", "from", "are",
		"was", "can", "how", "what", "when", "where", "which", "who",
		"will", "have", "has", "not", "but", "all", "any", "just",
		"does", "did", "been", "its", "then", "than", "also", "very",
		"的", "了", "是", "在", "我", "有", "和", "就", "不", "人",
		"都", "一", "个", "上", "也", "很", "到", "说", "要", "去",
		"你", "会", "着", "看", "好", "自己", "这":
		return true
	}
	return false
}

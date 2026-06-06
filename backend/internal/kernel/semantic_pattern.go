package kernel

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

// SemanticPatternDetector clusters user queries and emits distillable patterns.
// Uses embedding-based clustering when an embedder is available, or falls back to
// LLM-based clustering — no embedding API dependency required.
type SemanticPatternDetector struct {
	embedder   Embedder
	llm        LLMProvider // optional: for LLM-based clustering when embedder unavailable
	mu         sync.Mutex
	clusters   []queryCluster
	buffer     []clusterExample // accumulated pairs for LLM-based clustering
	minSize    int
	threshold  float64
}

type queryCluster struct {
	examples    []clusterExample
	embeddings  [][]float32
	distilled   bool
	lastEmitted int // number of examples when last emitted (0 = never)
}

type clusterExample struct {
	query    string
	response string
}

// NewSemanticPatternDetector creates a detector.
func NewSemanticPatternDetector(embedder Embedder, minSize int, threshold float64) *SemanticPatternDetector {
	if minSize < 2 { minSize = 2 }
	if threshold <= 0 || threshold > 1 { threshold = 0.80 }
	return &SemanticPatternDetector{
		embedder: embedder, minSize: minSize, threshold: threshold,
		clusters: make([]queryCluster, 0, 32),
	}
}

// SetLLM injects an LLM provider for LLM-based clustering (no embedding required).
func (d *SemanticPatternDetector) SetLLM(llm LLMProvider) { d.llm = llm }

// Detect collects query+response pairs, clusters them, and returns mature patterns.
// Uses embedding similarity when embedder is available; falls back to LLM clustering.
func (d *SemanticPatternDetector) Detect(ctx context.Context, sessionID string, messages []Message) ([]Pattern, error) {
	pairs := extractPairs(messages)
	if len(pairs) == 0 { return nil, nil }

	d.mu.Lock()
	defer d.mu.Unlock()

	// Try embedding-based clustering first
	if d.embedder != nil && d.embedder.Dimension() > 0 {
		texts := make([]string, len(pairs))
		for i, p := range pairs { texts[i] = p.query }
		embs, err := d.embedder.EmbedBatch(ctx, texts)
		if err == nil && len(embs) > 0 {
			for i, p := range pairs {
				if i >= len(embs) { break }
				d.addToCluster(p, embs[i])
			}
			return d.collectPatterns(), nil
		}
	}

	// Fall back to LLM-based clustering (no embedding needed)
	if d.llm != nil {
		d.buffer = append(d.buffer, pairs...)
		if len(d.buffer) >= d.minSize {
			patterns := d.clusterWithLLM(ctx)
			d.buffer = nil
			return patterns, nil
		}
	}

	return nil, nil
}

// clusterWithLLM sends accumulated queries to LLM for semantic grouping.
// No embedding API required — the LLM does the clustering directly.
func (d *SemanticPatternDetector) clusterWithLLM(ctx context.Context) []Pattern {
	if d.llm == nil || len(d.buffer) < d.minSize { return nil }

	var sb strings.Builder
	sb.WriteString("Group these user queries by topic. A query may belong to multiple groups.\n\n")
	for i, ex := range d.buffer {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, truncStr(ex.query, 100)))
	}
	sb.WriteString("\nOutput one line per group:\n")
	sb.WriteString("GROUP: <theme> | queries: <comma-separated numbers> | reusable: yes/no\n")
	sb.WriteString("Only output groups with 2+ queries. Mark reusable=yes only if the group represents a clear, repeatable pattern worth extracting.")

	resp, err := d.llm.Chat(ctx, []Message{{Role: "user", Content: sb.String()}}, nil,
		map[string]interface{}{"max_tokens": 300, "temperature": 0, "route": "execution", "no_thinking": true})
	if err != nil || resp.Content == "" { return nil }

	var patterns []Pattern
	for _, line := range strings.Split(resp.Content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "GROUP:") { continue }
		parts := strings.SplitN(line, "|", 3)
		theme := strings.TrimPrefix(strings.TrimSpace(parts[0]), "GROUP: ")
		reusable := len(parts) > 2 && strings.Contains(strings.ToLower(parts[2]), "yes")
		if theme != "" && reusable {
			patterns = append(patterns, Pattern{
				Type: "distillable_cluster", Description: theme,
				Confidence: 0.7, Frequency: len(d.buffer),
			})
		}
	}
	return patterns
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
		n := len(c.examples)
		if c.distilled || n < 2 { continue }
		// Only re-emit if cluster has grown significantly (2x) since last emission.
		// Prevents repeated LLM calls for the same cluster at similar sizes.
		if c.lastEmitted > 0 && n < c.lastEmitted*2 { continue }
		c.lastEmitted = n
		theme := extractClusterTheme(c.examples)
		if theme == "" { continue }
		patterns = append(patterns, Pattern{
			Type: "distillable_cluster", Description: theme,
			Confidence: clampConf(float64(n) / float64(max(d.minSize, n)) * 0.9),
			Frequency:  n,
		})
	}
	return patterns
}

// MarkDistilled marks a cluster as successfully distilled after LLM quality gate passes.
func (d *SemanticPatternDetector) MarkDistilled(theme string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.clusters {
		if extractClusterTheme(d.clusters[i].examples) == theme {
			d.clusters[i].distilled = true
			return
		}
	}
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

// evaluateAndDistill does quality evaluation + distillation in a single LLM call.
// Returns "" if the cluster is not worth distilling, or the distilled skill card.
func evaluateAndDistill(ctx context.Context, llm LLMProvider, p Pattern, idx int, examples [][]clusterExample) string {
	if llm == nil { return "" }
	if idx >= len(examples) || len(examples[idx]) < 2 { return "" }

	var sb strings.Builder
	sb.WriteString("You are evaluating whether a cluster of similar user queries forms a reusable pattern.\n\n")
	sb.WriteString(fmt.Sprintf("Theme: %s | Query count: %d\n\n", p.Description, p.Frequency))
	sb.WriteString("Sample queries:\n")
	n := len(examples[idx])
	if n > 8 { n = 8 }
	for j := 0; j < n; j++ {
		sb.WriteString(fmt.Sprintf("- Query: %s\n  Response: %s\n\n",
			truncStr(examples[idx][j].query, 200), truncStr(examples[idx][j].response, 300)))
	}
	sb.WriteString("## Task\n")
	sb.WriteString("Step 1 — Judge: is there a coherent, reusable pattern worth extracting?\n")
	sb.WriteString("Step 2 — If NOT, reply with ONLY the word SKIP.\n")
	sb.WriteString("  If YES, distill into a skill card (under 300 words):\n")
	sb.WriteString("  - Key files always involved\n")
	sb.WriteString("  - Optimal tool sequence\n")
	sb.WriteString("  - Specific gotchas to avoid\n")
	sb.WriteString("  - Step-by-step best approach\n")

	resp, err := llm.Chat(ctx, []Message{{Role: "user", Content: sb.String()}}, nil,
		map[string]interface{}{"max_tokens": 500, "temperature": 0.2, "route": "execution", "no_thinking": true})
	if err != nil || resp.Content == "" { return "" }

	result := strings.TrimSpace(resp.Content)
	if strings.HasPrefix(strings.ToUpper(result), "SKIP") { return "" }
	return result
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

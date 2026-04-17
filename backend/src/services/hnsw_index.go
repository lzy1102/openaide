package services

import (
	"container/heap"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"sync"
	"time"
)

// VectorDocument 向量文档
type VectorDocument struct {
	ID        string                 `json:"id"`
	Content   string                 `json:"content,omitempty"`
	Embedding []float32              `json:"embedding"`
	Metadata  map[string]interface{} `json:"metadata"`
	Score     float64                `json:"score,omitempty"`
}

// SearchResult 搜索结果
type SearchResult struct {
	Document VectorDocument `json:"document"`
	Score    float64        `json:"score"`
	Distance float64        `json:"distance"`
}

// HNSWIndex 纯 Go 实现的 HNSW 向量索引
type HNSWIndex struct {
	mu sync.RWMutex

	M              int
	MaxLevel       int
	EfConstruction int
	EfSearch       int
	ML             float64

	Nodes           map[string]*HNSWNode
	EntryPoint      string
	CurrentMaxLevel int

	Dimension int
	Count     int

	rand *rand.Rand
}

// HNSWNode HNSW 节点
type HNSWNode struct {
	ID        string                 `json:"id"`
	Vector    []float32              `json:"vector"`
	Metadata  map[string]interface{} `json:"metadata"`
	Level     int                    `json:"level"`
	Neighbors [][]string             `json:"neighbors"` // 每层的邻居
}

// NewHNSWIndex 创建 HNSW 索引
func NewHNSWIndex(dimension int) *HNSWIndex {
	return &HNSWIndex{
		M:               16,
		MaxLevel:        16,
		EfConstruction:  200,
		EfSearch:        64,
		ML:              1.0 / math.Log(16),
		Nodes:           make(map[string]*HNSWNode),
		Dimension:       dimension,
		CurrentMaxLevel: -1,
		rand:            rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// SetEfSearch 设置搜索范围
func (h *HNSWIndex) SetEfSearch(ef int) {
	h.EfSearch = ef
}

// Insert 插入向量
func (h *HNSWIndex) Insert(id string, vector []float32, metadata map[string]interface{}) error {
	if len(vector) != h.Dimension {
		return fmt.Errorf("dimension mismatch: expected %d, got %d", h.Dimension, len(vector))
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.Nodes[id]; exists {
		return fmt.Errorf("node %s already exists", id)
	}

	level := h.randomLevel()
	node := &HNSWNode{
		ID:        id,
		Vector:    make([]float32, len(vector)),
		Metadata:  metadata,
		Level:     level,
		Neighbors: make([][]string, level+1),
	}
	copy(node.Vector, vector)

	for i := 0; i <= level; i++ {
		node.Neighbors[i] = make([]string, 0, h.M)
	}

	if h.EntryPoint == "" {
		h.Nodes[id] = node
		h.EntryPoint = id
		h.CurrentMaxLevel = level
		h.Count++
		return nil
	}

	entryPoint := h.EntryPoint
	currentMaxLevel := h.CurrentMaxLevel

	for l := currentMaxLevel; l > level; l-- {
		results := h.searchLayer(vector, entryPoint, 1, l)
		if len(results) == 0 {
			break
		}
		entryPoint = results[0]
	}

	for l := minInt(level, currentMaxLevel); l >= 0; l-- {
		neighbors := h.searchLayer(vector, entryPoint, h.EfConstruction, l)
		selectedNeighbors := h.selectNeighbors(vector, neighbors, h.M)
		node.Neighbors[l] = selectedNeighbors

		for _, neighborID := range selectedNeighbors {
			neighbor := h.Nodes[neighborID]
			if neighbor == nil {
				continue
			}
			neighbor.Neighbors[l] = append(neighbor.Neighbors[l], id)
			if len(neighbor.Neighbors[l]) > h.M {
				neighbor.Neighbors[l] = h.selectNeighbors(neighbor.Vector, neighbor.Neighbors[l], h.M)
			}
		}

		if len(neighbors) > 0 {
			entryPoint = neighbors[0]
		}
	}

	if level > currentMaxLevel {
		h.EntryPoint = id
		h.CurrentMaxLevel = level
	}

	h.Nodes[id] = node
	h.Count++
	return nil
}

// Search 搜索最近邻
func (h *HNSWIndex) Search(query []float32, k int) ([]SearchResult, error) {
	if len(query) != h.Dimension {
		return nil, fmt.Errorf("dimension mismatch: expected %d, got %d", h.Dimension, len(query))
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.EntryPoint == "" {
		return []SearchResult{}, nil
	}

	entryPoint := h.EntryPoint
	currentMaxLevel := h.CurrentMaxLevel

	for l := currentMaxLevel; l > 0; l-- {
		results := h.searchLayer(query, entryPoint, 1, l)
		if len(results) == 0 {
			break
		}
		entryPoint = results[0]
	}

	candidates := h.searchLayer(query, entryPoint, h.EfSearch, 0)

	results := make([]SearchResult, 0, len(candidates))
	for _, id := range candidates {
		node := h.Nodes[id]
		if node == nil {
			continue
		}
		distance := cosineDistance(query, node.Vector)
		results = append(results, SearchResult{
			Document: VectorDocument{
				ID:        node.ID,
				Embedding: node.Vector,
				Metadata:  node.Metadata,
			},
			Score:    1.0 - distance,
			Distance: distance,
		})
	}

	sortSearchResults(results)

	if len(results) > k {
		results = results[:k]
	}

	return results, nil
}

// Delete 删除节点
func (h *HNSWIndex) Delete(id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	node, exists := h.Nodes[id]
	if !exists {
		return fmt.Errorf("node %s not found", id)
	}

	// 从邻居的连接中移除
	for l, neighbors := range node.Neighbors {
		for _, neighborID := range neighbors {
			neighbor := h.Nodes[neighborID]
			if neighbor == nil {
				continue
			}
			// 从邻居的邻居列表中移除当前节点
			neighbor.Neighbors[l] = removeString(neighbor.Neighbors[l], id)
		}
	}

	// 如果删除的是入口点，需要重新选择
	if h.EntryPoint == id {
		// 简单处理：选择第一个剩余节点作为入口点
		for newEntry := range h.Nodes {
			if newEntry != id {
				h.EntryPoint = newEntry
				h.CurrentMaxLevel = h.Nodes[newEntry].Level
				break
			}
		}
		if h.EntryPoint == id {
			// 没有节点了
			h.EntryPoint = ""
			h.CurrentMaxLevel = -1
		}
	}

	delete(h.Nodes, id)
	h.Count--
	return nil
}

// removeString 从字符串切片中移除指定元素
func removeString(slice []string, s string) []string {
	for i, v := range slice {
		if v == s {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

// Save 保存索引到文件
func (h *HNSWIndex) Save(path string) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Load 从文件加载索引
func LoadHNSWIndex(path string) (*HNSWIndex, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var h HNSWIndex
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, err
	}

	return &h, nil
}

// randomLevel 生成随机层
func (h *HNSWIndex) randomLevel() int {
	level := 0
	for h.rand.Float64() < h.ML && level < h.MaxLevel {
		level++
	}
	return level
}

// searchLayer 在指定层搜索
func (h *HNSWIndex) searchLayer(query []float32, entryPoint string, ef, layer int) []string {
	visited := make(map[string]bool)
	candidates := &nodeDistHeap{}
	heap.Init(candidates)
	results := &nodeDistHeap{}
	heap.Init(results)

	entryNode := h.Nodes[entryPoint]
	if entryNode == nil {
		return []string{}
	}

	dist := cosineDistance(query, entryNode.Vector)
	visited[entryPoint] = true
	heap.Push(candidates, nodeDist{entryPoint, dist})
	heap.Push(results, nodeDist{entryPoint, dist})

	for candidates.Len() > 0 {
		current := heap.Pop(candidates).(nodeDist)
		worstResult := (*results)[0].dist

		if current.dist > worstResult {
			break
		}

		node := h.Nodes[current.id]
		if node == nil || layer >= len(node.Neighbors) {
			continue
		}

		for _, neighborID := range node.Neighbors[layer] {
			if visited[neighborID] {
				continue
			}
			visited[neighborID] = true

			neighbor := h.Nodes[neighborID]
			if neighbor == nil {
				continue
			}

			d := cosineDistance(query, neighbor.Vector)
			if results.Len() < ef || d < worstResult {
				heap.Push(candidates, nodeDist{neighborID, d})
				heap.Push(results, nodeDist{neighborID, d})
				if results.Len() > ef {
					heap.Pop(results)
				}
			}
		}
	}

	result := make([]string, results.Len())
	for i := results.Len() - 1; i >= 0; i-- {
		result[i] = heap.Pop(results).(nodeDist).id
	}
	return result
}

// selectNeighbors 选择最优邻居
func (h *HNSWIndex) selectNeighbors(vector []float32, candidates []string, m int) []string {
	if len(candidates) <= m {
		return candidates
	}

	type neighborDist struct {
		id   string
		dist float64
	}

	dists := make([]neighborDist, len(candidates))
	for i, id := range candidates {
		node := h.Nodes[id]
		if node == nil {
			dists[i] = neighborDist{id, math.MaxFloat64}
			continue
		}
		dists[i] = neighborDist{id, cosineDistance(vector, node.Vector)}
	}

	sort.Slice(dists, func(i, j int) bool {
		return dists[i].dist < dists[j].dist
	})

	result := make([]string, m)
	for i := 0; i < m; i++ {
		result[i] = dists[i].id
	}
	return result
}

// nodeDist 节点距离
type nodeDist struct {
	id   string
	dist float64
}

// nodeDistHeap 节点距离堆
type nodeDistHeap []nodeDist

func (h nodeDistHeap) Len() int           { return len(h) }
func (h nodeDistHeap) Less(i, j int) bool { return h[i].dist > h[j].dist }
func (h nodeDistHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *nodeDistHeap) Push(x interface{}) { *h = append(*h, x.(nodeDist)) }
func (h *nodeDistHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// cosineDistance 余弦距离
func cosineDistance(a, b []float32) float64 {
	if len(a) != len(b) {
		return math.MaxFloat64
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 1.0
	}

	similarity := dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
	return 1.0 - similarity
}

// sortSearchResults 排序搜索结果
func sortSearchResults(results []SearchResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

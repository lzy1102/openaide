// Package codeindex 提供基于向量检索的项目代码索引。
//
// 设计原则:
//   - 不参与 ReAct 工具调用,只在 prompt 阶段为 kernel 注入相关代码上下文
//   - SQLite 存储嵌入向量(复用 modernc.org/sqlite,纯 Go 无 CGO)
//   - 增量索引:启动时异步全量扫描,运行时通过 IndexFile() 处理文件变更
//   - 优雅降级:配 embedder 用语义检索,否则退化为 TF-IDF 关键词检索
//   - CSP actor 串行化所有写操作,SQLite 连接由 actor 独占
package codeindex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"openaide/backend/internal/kernel"
	"openaide/backend/internal/kernel/actor"

	_ "modernc.org/sqlite"
)

// Chunk 是 codeindex 包使用的代码块类型。
// 直接复用 kernel.CodeChunk,使 Indexer 满足 kernel.CodeIndexer 接口。
type Chunk = kernel.CodeChunk

// Indexer 是代码索引主结构。CSP actor 串行化所有写操作。
type Indexer struct {
	store    *Store
	embedder kernel.Embedder // nil 表示用 TF-IDF 降级
	actor    *actor.Actor
	root     string
	cfg      Config

	// 索引状态
	mu          sync.RWMutex
	indexing    bool
	indexedAt   time.Time
	chunkCount  int
}

// Config 是 Indexer 的配置。
type Config struct {
	DBPath        string // SQLite 数据库路径
	MaxChunks     int    // 单文件最大 chunk 数(默认 100)
	ChunkSize     int    // chunk 字符数上限(默认 1500)
	ChunkOverlap  int    // chunk 重叠行数(默认 5)
}

// NewIndexer 创建并启动 Indexer。dbPath 为空时使用内存数据库。
func NewIndexer(cfg Config, embedder kernel.Embedder) (*Indexer, error) {
	if cfg.MaxChunks <= 0 {
		cfg.MaxChunks = 100
	}
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 1500
	}
	if cfg.ChunkOverlap < 0 {
		cfg.ChunkOverlap = 5
	}

	store, err := NewStore(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	ix := &Indexer{
		store:    store,
		embedder: embedder,
		actor:    actor.NewActor(32),
		cfg:      cfg,
	}

	// 启动时统计已有 chunk 数
	if count, err := store.Count(); err == nil {
		ix.chunkCount = count
		slog.Info("CodeIndexer loaded", "chunks", count)
	}

	return ix, nil
}

// Stop 关闭 actor 和 store。
func (ix *Indexer) Stop() {
	ix.actor.Stop()
	ix.store.Close()
}

// IndexProject 全量索引指定项目根目录。后台异步执行,不阻塞调用方。
// 重复调用会自动跳过正在进行的索引任务。
func (ix *Indexer) IndexProject(root string) {
	ix.mu.Lock()
	if ix.indexing {
		ix.mu.Unlock()
		return
	}
	ix.indexing = true
	ix.root = root
	ix.mu.Unlock()

	ix.actor.SendAsync(func() {
		defer func() {
			ix.mu.Lock()
			ix.indexing = false
			ix.indexedAt = time.Now()
			ix.mu.Unlock()
		}()
		ix.doIndexProject(root)
	})
}

// IndexFile 增量索引单个文件。文件 hash 未变时跳过。
func (ix *Indexer) IndexFile(absPath string) error {
	ix.mu.RLock()
	root := ix.root
	ix.mu.RUnlock()
	if root == "" {
		return errors.New("IndexProject not called yet")
	}
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return err
	}
	ix.actor.Send(func() {
		ix.doIndexFile(root, rel, absPath)
	})
	return nil
}

// RemoveFile 从索引中删除文件的所有 chunk。
func (ix *Indexer) RemoveFile(absPath string) {
	ix.mu.RLock()
	root := ix.root
	ix.mu.RUnlock()
	if root == "" {
		return
	}
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return
	}
	ix.actor.Send(func() {
		ix.store.DeleteByPath(rel)
	})
}

// Search 返回与 query 语义最相关的 top-K chunk。
// embedder 未配置时使用 TF-IDF 关键词匹配。
func (ix *Indexer) Search(ctx context.Context, query string, limit int) ([]Chunk, error) {
	if limit <= 0 {
		limit = 5
	}
	// 读操作不需要走 actor,SQLite WAL 支持并发读
	if ix.embedder != nil && ix.embedder.Dimension() > 0 {
		return ix.searchSemantic(ctx, query, limit)
	}
	return ix.searchTFIDF(query, limit)
}

// Stats 返回索引状态。
type Stats struct {
	Indexing   bool      `json:"indexing"`
	IndexedAt  time.Time `json:"indexed_at"`
	ChunkCount int       `json:"chunk_count"`
	HasEmbedder bool     `json:"has_embedder"`
}

func (ix *Indexer) Stats() Stats {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	hasEmb := ix.embedder != nil && ix.embedder.Dimension() > 0
	return Stats{
		Indexing:    ix.indexing,
		IndexedAt:   ix.indexedAt,
		ChunkCount:  ix.chunkCount,
		HasEmbedder: hasEmb,
	}
}

// ── 内部:全量索引 ────────────────────────────────────────────

func (ix *Indexer) doIndexProject(root string) {
	slog.Info("CodeIndexer: starting full index", "root", root)
	start := time.Now()

	chunker := NewChunker(ix.cfg)
	fileCount := 0
	chunkCount := 0

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			if d != nil && d.IsDir() && shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		ext := filepath.Ext(path)
		if isBinaryExt(ext) {
			return nil
		}
		// 仅索引 kernel.SymbolParser 支持的扩展名
		if !hasParserFor(ext) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Size() > 500*1024 {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 || data[0] == 0 {
			return nil
		}

		relPath, _ := filepath.Rel(root, path)
		relPath = filepath.ToSlash(relPath)

		chunks := chunker.Chunk(relPath, data)
		if len(chunks) == 0 {
			return nil
		}

		if err := ix.indexChunks(relPath, data, chunks); err != nil {
			slog.Debug("CodeIndexer: index file failed", "path", relPath, "error", err)
			return nil
		}
		fileCount++
		chunkCount += len(chunks)
		return nil
	})

	if err != nil {
		slog.Warn("CodeIndexer: walk error", "error", err)
	}

	ix.mu.Lock()
	ix.chunkCount = chunkCount
	ix.mu.Unlock()

	slog.Info("CodeIndexer: full index complete",
		"files", fileCount, "chunks", chunkCount, "elapsed", time.Since(start))
}

// doIndexFile 索引单个文件(增量更新)。
func (ix *Indexer) doIndexFile(root, relPath, absPath string) {
	data, err := os.ReadFile(absPath)
	if err != nil || len(data) == 0 {
		return
	}
	if len(data) > 500*1024 {
		return
	}
	// hash 未变则跳过
	if !ix.store.FileChanged(relPath, hashBytes(data)) {
		return
	}
	chunker := NewChunker(ix.cfg)
	chunks := chunker.Chunk(relPath, data)
	if len(chunks) == 0 {
		ix.store.DeleteByPath(relPath)
		return
	}
	if err := ix.indexChunks(relPath, data, chunks); err != nil {
		slog.Debug("CodeIndexer: incremental index failed", "path", relPath, "error", err)
	}
}

// indexChunks 计算嵌入并写入存储。
func (ix *Indexer) indexChunks(relPath string, data []byte, chunks []Chunk) error {
	// 先删除旧 chunk
	ix.store.DeleteByPath(relPath)

	fileHash := hashBytes(data)

	// 批量计算 embedding
	contents := make([]string, len(chunks))
	for i, c := range chunks {
		contents[i] = c.Content
	}

	var embeddings [][]float32
	if ix.embedder != nil && ix.embedder.Dimension() > 0 {
		var err error
		embeddings, err = ix.embedder.EmbedBatch(context.Background(), contents)
		if err != nil {
			slog.Debug("CodeIndexer: embed batch failed, falling back", "error", err)
			embeddings = nil
		}
	}

	for i, c := range chunks {
		c.ID = chunkID(relPath, c.StartLine, c.EndLine)
		if i < len(embeddings) && embeddings[i] != nil {
			ix.store.Upsert(c, embeddings[i], fileHash)
		} else {
			ix.store.Upsert(c, nil, fileHash)
		}
	}
	return nil
}

// ── 内部:搜索 ────────────────────────────────────────────────

func (ix *Indexer) searchSemantic(ctx context.Context, query string, limit int) ([]Chunk, error) {
	qVec, err := ix.embedder.Embed(ctx, query)
	if err != nil || len(qVec) == 0 {
		// embed 失败降级到 TF-IDF
		return ix.searchTFIDF(query, limit)
	}
	return ix.store.SearchByVector(qVec, limit)
}

func (ix *Indexer) searchTFIDF(query string, limit int) ([]Chunk, error) {
	return ix.store.SearchByKeyword(query, limit)
}

// ── 辅助 ─────────────────────────────────────────────────────

func chunkID(path string, start, end int) string {
	return fmt.Sprintf("%s:%d-%d", path, start, end)
}

func hashBytes(data []byte) string {
	var h uint64 = 1469598103934665603
	for _, b := range data {
		h ^= uint64(b)
		h *= 1099511628211
	}
	const hex = "0123456789abcdef"
	buf := make([]byte, 16)
	for i := 15; i >= 0; i-- {
		buf[i] = hex[h&0xf]
		h >>= 4
	}
	return string(buf)
}

func shouldSkipDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch name {
	case "vendor", "node_modules", "bin", "dist", "build",
		"__pycache__", "target", ".git", ".svn", ".hg",
		"out", "Debug", "Release", "obj":
		return true
	}
	return false
}

func isBinaryExt(ext string) bool {
	switch ext {
	case ".exe", ".dll", ".so", ".dylib", ".bin",
		".zip", ".tar", ".gz", ".bz2", ".xz",
		".png", ".jpg", ".jpeg", ".gif", ".ico", ".svg",
		".pdf", ".doc", ".docx", ".xls", ".xlsx",
		".mp3", ".mp4", ".avi", ".mov",
		".o", ".a", ".class", ".pyc", ".wasm",
		".lock", ".sum":
		return true
	}
	return false
}

// hasParserFor 检查 kernel 是否注册了处理该扩展名的 parser。
// 通过尝试调用 GenerateRepoMap 后台扫描的相同入口判断。
// 为避免循环依赖,这里硬编码 kernel 支持的扩展名清单。
func hasParserFor(ext string) bool {
	switch ext {
	case ".go", ".py", ".pyi", ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs",
		".rs", ".java", ".kt", ".kts", ".scala", ".rb",
		".c", ".h", ".cc", ".cpp", ".cxx", ".hpp", ".hxx",
		".php", ".swift", ".cs", ".mod":
		return true
	}
	return false
}

// MarshalEmbedding / UnmarshalEmbedding 用于 SQLite 存储的 JSON 序列化。
func MarshalEmbedding(vec []float32) string {
	if len(vec) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(vec)
	return string(b)
}

func UnmarshalEmbedding(s string) []float32 {
	if s == "" || s == "[]" {
		return nil
	}
	var vec []float32
	if err := json.Unmarshal([]byte(s), &vec); err != nil {
		return nil
	}
	return vec
}

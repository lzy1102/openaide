// Package codeindex 提供基于外部向量库的项目代码索引。
//
// 设计原则:
//   - 不参与 ReAct 工具调用,只在 prompt 阶段为 kernel 注入相关代码上下文
//   - 检索完全外挂:chunk 经 rag.Retriever 写入外部向量库(pgvector),
//     搜索也通过 rag.Retriever 完成;本包不实现任何本地检索
//   - SQLite 仅用于文件 hash 追踪与 chunk 元数据(增量索引判断)
//   - CSP actor 串行化所有写操作
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
	"openaide/backend/internal/rag"

	_ "modernc.org/sqlite"
)

// Chunk 是 codeindex 包使用的代码块类型。
// 直接复用 kernel.CodeChunk,使 Indexer 满足 kernel.CodeIndexer 接口。
type Chunk = kernel.CodeChunk

// Collection 是代码 chunk 在外部向量库中的集合名。
const Collection = "code"

// Indexer 是代码索引主结构。CSP actor 串行化所有写操作。
type Indexer struct {
	store     *Store
	retriever rag.Retriever // 外部向量库;Noop 时检索返回空
	actor     *actor.Actor
	root      string
	cfg       Config

	// 索引状态
	mu         sync.RWMutex
	indexing   bool
	indexedAt  time.Time
	chunkCount int
}

// Config 是 Indexer 的配置。
type Config struct {
	DBPath       string  // SQLite 数据库路径
	MaxChunks    int     // 单文件最大 chunk 数(默认 100)
	ChunkSize    int     // chunk 字符数上限(默认 1500)
	ChunkOverlap int     // chunk 重叠行数(默认 5)
	MinScore     float64 // 检索注入的最低相似度;0 = 默认 0.3
	MinScoreMode string  // 阈值策略: "fixed"(默认) / "relative" / "combined"
	ScoreRatio   float64 // relative/combined 模式的相对阈值比例;0 = 默认 0.6
}

// NewIndexer 创建并启动 Indexer。dbPath 为空时使用内存数据库。
func NewIndexer(cfg Config, retriever rag.Retriever) (*Indexer, error) {
	if cfg.MaxChunks <= 0 {
		cfg.MaxChunks = 100
	}
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 1500
	}
	if cfg.ChunkOverlap < 0 {
		cfg.ChunkOverlap = 5
	}
	if cfg.MinScore <= 0 {
		cfg.MinScore = 0.3
	}
	if cfg.ScoreRatio <= 0 {
		cfg.ScoreRatio = 0.6
	}
	switch cfg.MinScoreMode {
	case "relative", "combined":
	default:
		cfg.MinScoreMode = "fixed" // 未知模式回退固定阈值
	}
	if retriever == nil {
		retriever = rag.NoopRetriever{}
	}

	store, err := NewStore(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	ix := &Indexer{
		store:     store,
		retriever: retriever,
		actor:     actor.NewActor(32),
		cfg:       cfg,
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
		ix.deleteFile(rel)
	})
}

// Search 返回与 query 语义最相关的 top-K chunk。
// 外部向量库未连接时返回空结果。
func (ix *Indexer) Search(ctx context.Context, query string, limit int) ([]Chunk, error) {
	if limit <= 0 {
		limit = 5
	}
	results, err := ix.retriever.Search(ctx, Collection, query, limit)
	if err != nil {
		slog.Debug("CodeIndexer: external search failed", "error", err)
		return nil, nil
	}
	out := make([]Chunk, 0, len(results))
	// 阈值策略:
	//  fixed:    score < MinScore 丢弃
	//  relative: score < top*ScoreRatio 丢弃
	//  combined: 先 relative,且 top < MinScore 时全部丢弃
	topScore := float64(0)
	if ix.cfg.MinScoreMode != "fixed" && len(results) > 0 {
		topScore = results[0].Score
		for _, r := range results {
			if r.Score > topScore {
				topScore = r.Score
			}
		}
		if ix.cfg.MinScoreMode == "combined" && topScore < ix.cfg.MinScore {
			return out, nil // 检索整体不相关,全部丢弃
		}
	}
	for _, r := range results {
		switch ix.cfg.MinScoreMode {
		case "relative", "combined":
			if r.Score < topScore*ix.cfg.ScoreRatio {
				continue
			}
		default:
			if r.Score < ix.cfg.MinScore {
				continue // 低相似度 chunk 不注入,避免无关代码噪声
			}
		}
		chunk := Chunk{
			ID:      r.ID,
			Content: r.Content,
			Score:   r.Score,
			Path:    r.Metadata["path"],
			Symbol:  r.Metadata["symbol"],
		}
		if start, ok := r.Metadata["start_line"]; ok {
			fmt.Sscanf(start, "%d", &chunk.StartLine)
		}
		if end, ok := r.Metadata["end_line"]; ok {
			fmt.Sscanf(end, "%d", &chunk.EndLine)
		}
		out = append(out, chunk)
	}
	return out, nil
}

// Stats 返回索引状态。
type Stats struct {
	Indexing   bool      `json:"indexing"`
	IndexedAt  time.Time `json:"indexed_at"`
	ChunkCount int       `json:"chunk_count"`
	Retrieval  bool      `json:"retrieval"` // 外部检索是否可用
}

func (ix *Indexer) Stats() Stats {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	_, ok := ix.retriever.(rag.NoopRetriever)
	return Stats{
		Indexing:   ix.indexing,
		IndexedAt:  ix.indexedAt,
		ChunkCount: ix.chunkCount,
		Retrieval:  !ok,
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
		ix.deleteFile(relPath)
		return
	}
	if err := ix.indexChunks(relPath, data, chunks); err != nil {
		slog.Debug("CodeIndexer: incremental index failed", "path", relPath, "error", err)
	}
}

// indexChunks 将 chunk 写入外部向量库,并记录元数据。
func (ix *Indexer) indexChunks(relPath string, data []byte, chunks []Chunk) error {
	// 先删除旧 chunk(本地 + 外部)
	ix.deleteFile(relPath)

	fileHash := hashBytes(data)

	docs := make([]rag.Document, len(chunks))
	for i, c := range chunks {
		c.ID = chunkID(relPath, c.StartLine, c.EndLine)
		docs[i] = rag.Document{
			ID:      c.ID,
			Content: c.Content,
			Metadata: map[string]string{
				"path":       c.Path,
				"symbol":     c.Symbol,
				"start_line": fmt.Sprintf("%d", c.StartLine),
				"end_line":   fmt.Sprintf("%d", c.EndLine),
			},
		}
		chunks[i] = c
	}

	if err := ix.retriever.Index(context.Background(), Collection, docs); err != nil {
		slog.Debug("CodeIndexer: external index failed", "path", relPath, "error", err)
		return err
	}

	for _, c := range chunks {
		ix.store.Upsert(c, fileHash)
	}
	return nil
}

// deleteFile 删除某文件的全部 chunk:先取本地 ID,再删本地元数据,
// 最后从外部向量库删除。外部检索不可用(Noop)时静默跳过。
func (ix *Indexer) deleteFile(relPath string) {
	ids, err := ix.store.ListByPath(relPath)
	if err != nil {
		ids = nil
	}
	ix.store.DeleteByPath(relPath)
	if len(ids) == 0 {
		return
	}
	if err := ix.retriever.Delete(context.Background(), Collection, ids); err != nil {
		slog.Debug("CodeIndexer: external delete failed", "path", relPath, "error", err)
	}
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

// MarshalEmbedding / UnmarshalEmbedding 保留兼容(SQLite 旧数据可能含向量列)。
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

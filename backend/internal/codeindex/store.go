package codeindex

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store 是 SQLite 持久化层。所有写操作应在 Indexer 的 actor goroutine 中调用。
// 读操作(Search)可以并发(SQLite WAL 模式)。
type Store struct {
	db *sql.DB
	// TF-IDF 倒排索引(内存,用于降级检索)
	invertedMu sync.RWMutex
	inverted   map[string]map[string]int // term → {chunkID: tf}
	docCount   int
}

// NewStore 打开 SQLite 数据库。dbPath 为空时使用内存数据库。
func NewStore(dbPath string) (*Store, error) {
	dsn := dbPath
	if dsn == "" {
		dsn = ":memory:"
	} else {
		dsn = dsn + "?_journal_mode=WAL&_busy_timeout=5000"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // 写串行化;读用 WAL

	s := &Store{
		db:       db,
		inverted: make(map[string]map[string]int),
	}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.loadInvertedIndex(); err != nil {
		// 不致命,降级检索会重建
		fmt.Println("CodeIndexer: load inverted index failed:", err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS code_chunks (
	id TEXT PRIMARY KEY,
	path TEXT NOT NULL,
	start_line INTEGER NOT NULL,
	end_line INTEGER NOT NULL,
	content TEXT NOT NULL,
	symbol TEXT NOT NULL DEFAULT '',
	embedding TEXT DEFAULT '[]',
	file_hash TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_chunks_path ON code_chunks(path);
CREATE INDEX IF NOT EXISTS idx_chunks_hash ON code_chunks(file_hash);
`)
	return err
}

// loadInvertedIndex 从已有 chunks 加载倒排索引(TF-IDF 降级用)。
func (s *Store) loadInvertedIndex() error {
	rows, err := s.db.Query(`SELECT id, content FROM code_chunks`)
	if err != nil {
		return err
	}
	defer rows.Close()

	s.invertedMu.Lock()
	defer s.invertedMu.Unlock()
	s.inverted = make(map[string]map[string]int)
	s.docCount = 0

	for rows.Next() {
		var id, content string
		if err := rows.Scan(&id, &content); err != nil {
			continue
		}
		tokens := tokenize(content)
		tf := make(map[string]int)
		for _, t := range tokens {
			tf[t]++
		}
		for term, count := range tf {
			if s.inverted[term] == nil {
				s.inverted[term] = make(map[string]int)
			}
			s.inverted[term][id] = count
		}
		s.docCount++
	}
	return rows.Err()
}

// Close 关闭数据库。
func (s *Store) Close() {
	s.db.Close()
}

// Upsert 写入/更新一个 chunk。embedding 为 nil 时存空数组。
// 同时更新倒排索引。
func (s *Store) Upsert(c Chunk, embedding []float32, fileHash string) {
	embJSON := MarshalEmbedding(embedding)
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO code_chunks(id, path, start_line, end_line, content, symbol, embedding, file_hash, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Path, c.StartLine, c.EndLine, c.Content, c.Symbol,
		embJSON, fileHash, now(),
	)
	if err != nil {
		return
	}

	// 更新倒排索引
	s.invertedMu.Lock()
	defer s.invertedMu.Unlock()
	tokens := tokenize(c.Content)
	tf := make(map[string]int)
	for _, t := range tokens {
		tf[t]++
	}
	// 删除旧 term 关联
	for term, postings := range s.inverted {
		delete(postings, c.ID)
		if len(postings) == 0 {
			delete(s.inverted, term)
		}
	}
	// 写入新 term
	for term, count := range tf {
		if s.inverted[term] == nil {
			s.inverted[term] = make(map[string]int)
		}
		s.inverted[term][c.ID] = count
	}
	s.docCount++
}

// DeleteByPath 删除指定文件的所有 chunk。
func (s *Store) DeleteByPath(path string) {
	// 先取要删除的 chunk id,以便同步更新倒排索引
	rows, err := s.db.Query(`SELECT id, content FROM code_chunks WHERE path = ?`, path)
	if err != nil {
		return
	}
	var ids []string
	var contents []string
	for rows.Next() {
		var id, content string
		rows.Scan(&id, &content)
		ids = append(ids, id)
		contents = append(contents, content)
	}
	rows.Close()

	if len(ids) == 0 {
		return
	}

	_, err = s.db.Exec(`DELETE FROM code_chunks WHERE path = ?`, path)
	if err != nil {
		return
	}

	// 更新倒排索引
	s.invertedMu.Lock()
	defer s.invertedMu.Unlock()
	for i, id := range ids {
		tokens := tokenize(contents[i])
		for _, t := range tokens {
			if postings, ok := s.inverted[t]; ok {
				delete(postings, id)
				if len(postings) == 0 {
					delete(s.inverted, t)
				}
			}
		}
		s.docCount--
	}
}

// FileChanged 检查文件 hash 是否与索引中存储的不同。
func (s *Store) FileChanged(path, newHash string) bool {
	// 取该文件任意一个 chunk 的 file_hash 比对
	var stored string
	err := s.db.QueryRow(
		`SELECT file_hash FROM code_chunks WHERE path = ? LIMIT 1`,
		path,
	).Scan(&stored)
	if err != nil {
		return true // 文件不在索引中,视为变更
	}
	return stored != newHash
}

// Count 返回 chunk 总数。
func (s *Store) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM code_chunks`).Scan(&n)
	return n, err
}

// SearchByVector 用嵌入向量做语义检索,返回 top-K chunk。
func (s *Store) SearchByVector(queryVec []float32, limit int) ([]Chunk, error) {
	if len(queryVec) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(`SELECT id, path, start_line, end_line, content, symbol, embedding FROM code_chunks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scored struct {
		chunk Chunk
		score float64
	}
	var results []scored

	for rows.Next() {
		var c Chunk
		var embStr string
		if err := rows.Scan(&c.ID, &c.Path, &c.StartLine, &c.EndLine,
			&c.Content, &c.Symbol, &embStr); err != nil {
			continue
		}
		docVec := UnmarshalEmbedding(embStr)
		if len(docVec) != len(queryVec) {
			continue
		}
		score := cosineSim(queryVec, docVec)
		if score > 0 {
			c.Score = score
			results = append(results, scored{c, score})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})
	if len(results) > limit {
		results = results[:limit]
	}

	out := make([]Chunk, len(results))
	for i, r := range results {
		out[i] = r.chunk
	}
	return out, nil
}

// SearchByKeyword 用 TF-IDF 关键词检索(降级路径)。
func (s *Store) SearchByKeyword(query string, limit int) ([]Chunk, error) {
	s.invertedMu.RLock()
	defer s.invertedMu.RUnlock()

	if s.docCount == 0 || len(s.inverted) == 0 {
		return nil, nil
	}

	qTokens := tokenize(query)
	if len(qTokens) == 0 {
		return nil, nil
	}

	// 计算每个 chunk 的累积 TF-IDF 分数
	scores := make(map[string]float64)
	for _, term := range qTokens {
		postings, ok := s.inverted[term]
		if !ok {
			continue
		}
		df := float64(len(postings))
		if df == 0 {
			continue
		}
		// idf = log(N / df)
		idf := math.Log(float64(s.docCount)/df) + 1
		for chunkID, tf := range postings {
			tfF := float64(tf)
			scores[chunkID] += tfF * idf
		}
	}

	if len(scores) == 0 {
		return nil, nil
	}

	// 按分数排序
	type kv struct {
		id    string
		score float64
	}
	var sorted []kv
	for id, sc := range scores {
		sorted = append(sorted, kv{id, sc})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})
	if len(sorted) > limit {
		sorted = sorted[:limit]
	}

	// 查 chunk 元信息
	if len(sorted) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(sorted))
	args := make([]interface{}, len(sorted))
	for i, kv := range sorted {
		placeholders[i] = "?"
		args[i] = kv.id
	}
	query2 := fmt.Sprintf(
		`SELECT id, path, start_line, end_line, content, symbol FROM code_chunks WHERE id IN (%s)`,
		strings.Join(placeholders, ","),
	)
	rows, err := s.db.Query(query2, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	chunkMap := make(map[string]Chunk)
	for rows.Next() {
		var c Chunk
		rows.Scan(&c.ID, &c.Path, &c.StartLine, &c.EndLine, &c.Content, &c.Symbol)
		chunkMap[c.ID] = c
	}

	out := make([]Chunk, 0, len(sorted))
	for _, kv := range sorted {
		if c, ok := chunkMap[kv.id]; ok {
			c.Score = kv.score
			out = append(out, c)
		}
	}
	return out, nil
}

// ── 工具函数 ─────────────────────────────────────────────────

func cosineSim(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		va := float64(a[i])
		vb := float64(b[i])
		dot += va * vb
		na += va * va
		nb += vb * vb
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func now() string {
	return time.Now().Format(time.RFC3339)
}

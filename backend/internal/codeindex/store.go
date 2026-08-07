package codeindex

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

// Store 是 SQLite 持久化层。所有写操作应在 Indexer 的 actor goroutine 中调用。
// 仅存储 chunk 元数据与文件 hash(增量索引判断),检索完全由外部向量库承担。
type Store struct {
	db *sql.DB
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

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
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
	file_hash TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_chunks_path ON code_chunks(path);
CREATE INDEX IF NOT EXISTS idx_chunks_hash ON code_chunks(file_hash);
`)
	return err
}

// Close 关闭数据库。
func (s *Store) Close() {
	s.db.Close()
}

// Upsert 写入/更新一个 chunk 的元数据(向量检索由外部库负责,不在此存储)。
func (s *Store) Upsert(c Chunk, fileHash string) {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO code_chunks(id, path, start_line, end_line, content, symbol, file_hash, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Path, c.StartLine, c.EndLine, c.Content, c.Symbol,
		fileHash, now(),
	)
	if err != nil {
		return
	}
}

// ListByPath 返回指定文件的所有 chunk id。
func (s *Store) ListByPath(path string) ([]string, error) {
	rows, err := s.db.Query(`SELECT id FROM code_chunks WHERE path = ?`, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeleteByPath 删除指定文件的所有 chunk。
func (s *Store) DeleteByPath(path string) {
	_, _ = s.db.Exec(`DELETE FROM code_chunks WHERE path = ?`, path)
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

func now() string {
	return time.Now().Format(time.RFC3339)
}

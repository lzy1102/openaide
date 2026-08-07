package codeindex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"openaide/backend/internal/rag"
)

// memRetriever 是测试用的内存 rag.Retriever 实现,按子串匹配内容。
type memRetriever struct {
	mu   sync.Mutex
	docs map[string]map[string]rag.Document // collection -> id -> doc
}

func newMemRetriever() *memRetriever {
	return &memRetriever{docs: make(map[string]map[string]rag.Document)}
}

func (m *memRetriever) Index(_ context.Context, collection string, docs []rag.Document) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.docs[collection] == nil {
		m.docs[collection] = make(map[string]rag.Document)
	}
	for _, d := range docs {
		m.docs[collection][d.ID] = d
	}
	return nil
}

func (m *memRetriever) Search(_ context.Context, collection, query string, limit int) ([]rag.Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	qTokens := strings.Fields(strings.ToLower(query))
	var out []rag.Result
	for _, d := range m.docs[collection] {
		content := strings.ToLower(d.Content)
		path := strings.ToLower(d.Metadata["path"])
		all := true
		for _, tok := range qTokens {
			if !strings.Contains(content, tok) && !strings.Contains(path, tok) {
				all = false
				break
			}
		}
		if all {
			out = append(out, rag.Result{ID: d.ID, Content: d.Content, Score: 1, Metadata: d.Metadata})
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *memRetriever) Delete(_ context.Context, collection string, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		delete(m.docs[collection], id)
	}
	return nil
}

func (m *memRetriever) Ping(context.Context) error { return nil }

// 等待索引完成(最多 5 秒)
func waitForIndex(t *testing.T, ix *Indexer, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !ix.Stats().Indexing {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("indexer did not finish within timeout")
}

func TestIndexer_SearchWithRetriever(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "auth.go", `package auth

import "fmt"

// Authenticate verifies user credentials against the database.
func Authenticate(user, pass string) bool {
	if user == "admin" && pass == "secret" {
		return true
	}
	return false
}

type User struct {
	Name string
}`)
	writeFile(t, dir, "util.go", `package util

// Helper prints debug info.
func Helper() {
	println("debug")
}`)

	ix, err := NewIndexer(Config{DBPath: dir + "/test.db"}, newMemRetriever())
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Stop()

	ix.IndexProject(dir)
	waitForIndex(t, ix, 5*time.Second)

	results, err := ix.Search(context.Background(), "Authenticate user", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected non-empty results")
	}

	found := false
	for _, c := range results {
		if strings.Contains(c.Path, "auth.go") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected auth.go in results, got: %+v", results)
	}
}

func TestIndexer_SearchNoopReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "auth.go", `package auth
func Authenticate() bool { return true }`)

	ix, err := NewIndexer(Config{DBPath: dir + "/test.db"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Stop()

	ix.IndexProject(dir)
	waitForIndex(t, ix, 3*time.Second)

	results, err := ix.Search(context.Background(), "Authenticate", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results without external store, got %d", len(results))
	}
	if ix.Stats().Retrieval {
		t.Error("expected Retrieval=false for Noop retriever")
	}
}

func TestIndexer_IncrementalFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", `package a
func A() {}`)

	ix, err := NewIndexer(Config{DBPath: dir + "/test.db"}, newMemRetriever())
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Stop()

	ix.IndexProject(dir)
	waitForIndex(t, ix, 3*time.Second)

	// 新增文件
	newFile := filepath.Join(dir, "b.go")
	if err := os.WriteFile(newFile, []byte("package a\nfunc B() {}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ix.IndexFile(newFile); err != nil {
		t.Fatal(err)
	}
	// 等待 actor 处理
	time.Sleep(300 * time.Millisecond)

	results, err := ix.Search(context.Background(), "B", 10)
	if err != nil {
		t.Fatal(err)
	}
	foundB := false
	for _, c := range results {
		if strings.Contains(c.Path, "b.go") {
			foundB = true
			break
		}
	}
	if !foundB {
		t.Errorf("expected b.go in results after IndexFile, got: %+v", results)
	}
}

func TestIndexer_FileChangedSkipsUnchanged(t *testing.T) {
	dir := t.TempDir()
	content := `package a
func A() {}`
	path := filepath.Join(dir, "a.go")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	ix, err := NewIndexer(Config{DBPath: dir + "/test.db"}, newMemRetriever())
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Stop()

	ix.IndexProject(dir)
	waitForIndex(t, ix, 3*time.Second)

	count1 := ix.Stats().ChunkCount

	// 再次调用 IndexFile,内容未变,应该跳过
	if err := ix.IndexFile(path); err != nil {
		t.Fatal(err)
	}
	// 等待 actor 处理
	time.Sleep(200 * time.Millisecond)
	count2 := ix.Stats().ChunkCount

	if count1 != count2 {
		t.Errorf("unchanged file should not change chunk count: before=%d after=%d", count1, count2)
	}
}

func TestIndexer_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	ix, err := NewIndexer(Config{DBPath: dir + "/test.db"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Stop()

	ix.IndexProject(dir)
	waitForIndex(t, ix, 3*time.Second)

	results, err := ix.Search(context.Background(), "anything", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}

func TestChunker_BasicGoFile(t *testing.T) {
	ch := NewChunker(Config{ChunkSize: 1500, MaxChunks: 100, ChunkOverlap: 5})
	content := []byte(`package main

import "fmt"

func main() {
	fmt.Println("hi")
}

type User struct {
	Name string
}

func (u *User) Greet() string {
	return "hi " + u.Name
}
`)
	chunks := ch.Chunk("main.go", content)
	if len(chunks) < 2 {
		t.Errorf("expected >=2 chunks, got %d", len(chunks))
	}
	// 第一个应该是 file_summary
	if chunks[0].Symbol != "file_summary" {
		t.Errorf("expected first chunk to be file_summary, got %s", chunks[0].Symbol)
	}
	// 至少有一个 chunk 包含 main 函数
	foundMain := false
	for _, c := range chunks {
		if strings.Contains(c.Symbol, "main") {
			foundMain = true
			break
		}
	}
	if !foundMain {
		t.Errorf("expected a chunk with symbol 'main', got: %+v", chunks)
	}
}

func TestChunker_LongFile(t *testing.T) {
	ch := NewChunker(Config{ChunkSize: 200, MaxChunks: 100, ChunkOverlap: 2})
	// 构造超长文件:一个大函数
	var sb strings.Builder
	sb.WriteString("package main\n\nfunc bigFunc() {\n")
	for i := 0; i < 100; i++ {
		sb.WriteString("    x := ")
		sb.WriteString(string(rune('0' + i%10)))
		sb.WriteString(" // line padding for testing chunk splitting\n")
	}
	sb.WriteString("}\n")

	chunks := ch.Chunk("big.go", []byte(sb.String()))
	if len(chunks) < 2 {
		t.Errorf("expected >=2 chunks for long file, got %d", len(chunks))
	}
	// 检查每个 chunk 字符数 <= ChunkSize(允许少量超出因 overlap)
	for _, c := range chunks {
		if len(c.Content) > 300 { // 允许 overlap 余量
			t.Errorf("chunk too large: %d chars", len(c.Content))
		}
	}
}

func TestStore_MetadataAndHash(t *testing.T) {
	s, err := NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.Upsert(Chunk{ID: "a:1-5", Path: "a", StartLine: 1, EndLine: 5, Content: "func auth login"}, "h1")
	s.Upsert(Chunk{ID: "b:1-3", Path: "b", StartLine: 1, EndLine: 3, Content: "func util debug"}, "h2")

	ids, err := s.ListByPath("a")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "a:1-5" {
		t.Errorf("expected [a:1-5] for path a, got %v", ids)
	}

	if !s.FileChanged("a", "h2") {
		t.Error("expected FileChanged=true for different hash")
	}
	if s.FileChanged("a", "h1") {
		t.Error("expected FileChanged=false for same hash")
	}
	if !s.FileChanged("missing", "h1") {
		t.Error("expected FileChanged=true for missing file")
	}

	s.DeleteByPath("a")
	if !s.FileChanged("a", "h1") {
		t.Error("expected FileChanged=true after delete")
	}
}

// ── 辅助 ─────────────────────────────────────────────────────

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

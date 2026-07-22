package codeindex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

func TestIndexer_TFIDFSearch(t *testing.T) {
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

	ix, err := NewIndexer(Config{DBPath: dir + "/test.db"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Stop()

	ix.IndexProject(dir)
	waitForIndex(t, ix, 5*time.Second)

	// 搜索 "Authenticate"
	results, err := ix.Search(context.Background(), "Authenticate user", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected non-empty results")
	}

	// 第一个结果应该是 auth.go(因为包含 Authenticate/user 等关键词)
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

func TestIndexer_IncrementalFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", `package a
func A() {}`)

	ix, err := NewIndexer(Config{DBPath: dir + "/test.db"}, nil)
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

	// 检查能搜到 B
	// 注意:tokenize 会过滤单字符英文 token,所以查询用 "func B" 而非 "B function"
	// ("B" 会被过滤,"function" 与 b.go 中的 "func" 不匹配)
	results, err := ix.Search(context.Background(), "func B", 10)
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

	ix, err := NewIndexer(Config{DBPath: dir + "/test.db"}, nil)
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

func TestStore_TFIDFRelevance(t *testing.T) {
	s, err := NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// 写入若干 chunks
	s.Upsert(Chunk{ID: "a:1-5", Path: "a", StartLine: 1, EndLine: 5, Content: "func auth login password user"}, nil, "h1")
	s.Upsert(Chunk{ID: "b:1-3", Path: "b", StartLine: 1, EndLine: 3, Content: "func util debug helper print"}, nil, "h2")

	// 搜索 "auth login"
	results, err := s.SearchByKeyword("auth login", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	if results[0].Path != "a" {
		t.Errorf("expected top result from path 'a', got %s", results[0].Path)
	}
}

func TestTokenize(t *testing.T) {
	tokens := tokenize("func Hello(name string) { return 'hi' }")
	// 应包含 func, hello, name, string, return, hi
	expect := map[string]bool{"func": false, "hello": false, "name": false, "string": false, "return": false}
	for _, tk := range tokens {
		if _, ok := expect[tk]; ok {
			expect[tk] = true
		}
	}
	for k, v := range expect {
		if !v {
			t.Errorf("expected token %s not found in: %v", k, tokens)
		}
	}
}

func TestTokenize_Chinese(t *testing.T) {
	tokens := tokenize("定义 用户 鉴权 函数")
	expect := map[string]bool{"定": false, "义": false, "用": false, "户": false}
	for _, tk := range tokens {
		if _, ok := expect[tk]; ok {
			expect[tk] = true
		}
	}
	for k, v := range expect {
		if !v {
			t.Errorf("expected Chinese token %s not found in: %v", k, tokens)
		}
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

package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditFiles_AllSuccess(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.go", "package a\n\nfunc A() {}\nfunc B() {}\n")
	writeTestFile(t, dir, "b.go", "package b\n\nvar X = 1\n")

	args := `{"edits":[` +
		`{"path":"` + jsonPath(filepath.Join(dir, "a.go")) + `","search_text":"func A() {}","replace_text":"func A() { return }"},` +
		`{"path":"` + jsonPath(filepath.Join(dir, "a.go")) + `","search_text":"func B() {}","replace_text":"func B() int { return 0 }"},` +
		`{"path":"` + jsonPath(filepath.Join(dir, "b.go")) + `","search_text":"var X = 1","replace_text":"var X = 42"}` +
		`]}`
	res, err := handleEditFiles(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.Error != "" {
		t.Fatalf("expected success, got error: %s", res.Error)
	}
	if !strings.Contains(res.Content.(string), "3 edits total") {
		t.Errorf("expected '3 edits total' in result, got: %v", res.Content)
	}

	// 验证文件内容
	a, _ := os.ReadFile(filepath.Join(dir, "a.go"))
	if !strings.Contains(string(a), "func A() { return }") {
		t.Errorf("a.go not updated correctly: %s", string(a))
	}
	if !strings.Contains(string(a), "func B() int { return 0 }") {
		t.Errorf("a.go second edit not applied: %s", string(a))
	}
	b, _ := os.ReadFile(filepath.Join(dir, "b.go"))
	if !strings.Contains(string(b), "var X = 42") {
		t.Errorf("b.go not updated: %s", string(b))
	}
}

func TestEditFiles_PrecheckFailure_NoChanges(t *testing.T) {
	dir := t.TempDir()
	original := "package a\n\nfunc A() {}\n"
	writeTestFile(t, dir, "a.go", original)
	writeTestFile(t, dir, "b.go", "package b\n\nvar X = 1\n")

	// 第二个 edit 的 search_text 不存在
	args := `{"edits":[` +
		`{"path":"` + jsonPath(filepath.Join(dir, "a.go")) + `","search_text":"func A() {}","replace_text":"func A() { return }"},` +
		`{"path":"` + jsonPath(filepath.Join(dir, "b.go")) + `","search_text":"NOT_EXIST","replace_text":"x"}` +
		`]}`
	res, err := handleEditFiles(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.Error == "" {
		t.Fatal("expected precheck failure, got success")
	}
	if !strings.Contains(res.Error, "Pre-check failed") {
		t.Errorf("expected precheck message, got: %s", res.Error)
	}

	// 验证 a.go 未被修改
	a, _ := os.ReadFile(filepath.Join(dir, "a.go"))
	if string(a) != original {
		t.Errorf("a.go was modified despite precheck failure:\nbefore: %q\nafter:  %q", original, string(a))
	}
}

func TestEditFiles_PrecheckAmbiguous(t *testing.T) {
	dir := t.TempDir()
	// 包含 2 个相同的字符串
	writeTestFile(t, dir, "dup.go", "TODO: fix\nTODO: fix\n")

	args := `{"edits":[` +
		`{"path":"` + jsonPath(filepath.Join(dir, "dup.go")) + `","search_text":"TODO: fix","replace_text":"DONE"}` +
		`]}`
	res, err := handleEditFiles(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.Error == "" {
		t.Fatal("expected ambiguous failure")
	}
	if !strings.Contains(res.Error, "found 2 times") {
		t.Errorf("expected ambiguous message, got: %s", res.Error)
	}
}

func TestEditFiles_RollbackOnWriteFailure(t *testing.T) {
	dir := t.TempDir()
	originalA := "package a\n\nfunc A() {}\n"
	writeTestFile(t, dir, "a.go", originalA)

	// 创建只读文件(模拟写入失败)
	readonlyPath := filepath.Join(dir, "readonly.go")
	if err := os.WriteFile(readonlyPath, []byte("content"), 0444); err != nil {
		t.Fatal(err)
	}

	// 第一个 edit 应该成功(a.go),第二个 edit 写只读文件应失败 → 回滚 a.go
	args := `{"edits":[` +
		`{"path":"` + jsonPath(filepath.Join(dir, "a.go")) + `","search_text":"func A() {}","replace_text":"func A() { return }"},` +
		`{"path":"` + jsonPath(readonlyPath) + `","search_text":"content","replace_text":"new"}` +
		`]}`
	res, err := handleEditFiles(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.Error == "" {
		t.Fatal("expected write failure")
	}
	if !strings.Contains(res.Error, "Rolled back") {
		t.Errorf("expected rollback message, got: %s", res.Error)
	}

	// 验证 a.go 被回滚
	a, _ := os.ReadFile(filepath.Join(dir, "a.go"))
	if string(a) != originalA {
		t.Errorf("a.go should be rolled back to original:\nbefore: %q\nafter:  %q", originalA, string(a))
	}
}

func TestEditFiles_SameFileMultipleEdits_OrderMatters(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.go", "package a\n\nvar X = 1\nvar Y = 2\n")

	// 两个 edit 修改同一个文件不同位置,顺序应用
	args := `{"edits":[` +
		`{"path":"` + jsonPath(filepath.Join(dir, "a.go")) + `","search_text":"var X = 1","replace_text":"var X = 10"},` +
		`{"path":"` + jsonPath(filepath.Join(dir, "a.go")) + `","search_text":"var Y = 2","replace_text":"var Y = 20"}` +
		`]}`
	res, err := handleEditFiles(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.Error != "" {
		t.Fatalf("expected success, got: %s", res.Error)
	}

	content, _ := os.ReadFile(filepath.Join(dir, "a.go"))
	s := string(content)
	if !strings.Contains(s, "var X = 10") || !strings.Contains(s, "var Y = 20") {
		t.Errorf("both edits should be applied, got: %s", s)
	}
}

func TestEditFiles_EmptyEdits(t *testing.T) {
	args := `{"edits":[]}`
	res, err := handleEditFiles(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.Error == "" {
		t.Fatal("expected error for empty edits")
	}
}

func TestEditFiles_InvalidJSON(t *testing.T) {
	res, err := handleEditFiles(context.Background(), "not json")
	if err != nil {
		t.Fatal(err)
	}
	if res.Error == "" {
		t.Fatal("expected error for invalid JSON")
	}
}

// ── 辅助 ─────────────────────────────────────────────────────

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// jsonPath 把文件路径转换为 JSON 字符串安全的形态(正斜杠)。
// Windows 下 filepath.Join 返回 "C:\Users\..." ,直接拼入 JSON 会让
// "\U" 被当成 Unicode 转义导致解析失败。Go 的 os.ReadFile/filepath.Abs
// 在 Windows 上同样接受正斜杠路径。
func jsonPath(p string) string {
	return filepath.ToSlash(p)
}

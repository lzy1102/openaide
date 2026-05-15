package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClient_IsRepo(t *testing.T) {
	// 在临时目录创建 git 仓库
	dir := t.TempDir()
	client := NewClient(dir)

	if client.IsRepo() {
		t.Error("Expected non-repo directory")
	}

	// 初始化 git 仓库
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	// 创建 HEAD 文件
	os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0644)

	if !client.IsRepo() {
		t.Error("Expected repo directory")
	}
}

func TestSuggestCommitMessage(t *testing.T) {
	status := &Status{
		Staged: []FileChange{
			{Path: "main.go", Status: "M"},
			{Path: "utils.go", Status: "M"},
		},
	}

	msg := SuggestCommitMessage(status, nil)
	if msg == "" {
		t.Error("Expected non-empty commit message")
	}

	// 检查是否包含前缀
	if !containsAny(msg, []string{"fix:", "feat:", "chore:"}) {
		t.Errorf("Expected commit prefix, got: %s", msg)
	}
}

func TestSuggestCommitMessage_Added(t *testing.T) {
	status := &Status{
		Staged: []FileChange{
			{Path: "new_feature.go", Status: "A"},
		},
	}

	msg := SuggestCommitMessage(status, nil)
	if !containsAny(msg, []string{"feat:"}) {
		t.Errorf("Expected feat prefix for added file, got: %s", msg)
	}
}

func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if contains(s, sub) {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsHelper(s, sub))
}

func containsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

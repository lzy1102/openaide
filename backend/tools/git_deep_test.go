package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHandlersArgValidation(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name string
		call func() string
		want string
	}{
		{"blame_missing_path", func() string { r, _ := handleGitBlame(ctx, `{}`); return r.Error }, "path is required"},
		{"commit_missing_message", func() string { r, _ := handleGitCommit(ctx, `{}`); return r.Error }, "message is required"},
		{"create_branch_missing_name", func() string { r, _ := handleGitCreateBranch(ctx, `{}`); return r.Error }, "name is required"},
		{"status_bad_json", func() string { r, _ := handleGitStatus(ctx, `{oops`); return r.Error }, ""},
		{"log_bad_json", func() string { r, _ := handleGitLog(ctx, `{oops`); return r.Error }, ""},
	}
	for _, c := range cases {
		if msg := c.call(); !strings.Contains(msg, c.want) {
			t.Errorf("%s: error = %q, want containing %q", c.name, msg, c.want)
		}
	}
}

func TestGitHandlersNotARepo(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir() // fresh empty dir, not a git repository

	cases := []struct {
		name string
		call func() string
	}{
		{"status", func() string { r, _ := handleGitStatus(ctx, `{"path":"`+dir+`"}`); return r.Error }},
		{"diff", func() string { r, _ := handleGitDiff(ctx, `{"path":"`+dir+`"}`); return r.Error }},
		{"log", func() string { r, _ := handleGitLog(ctx, `{"path":"`+dir+`"}`); return r.Error }},
	}
	for _, c := range cases {
		if msg := c.call(); !strings.Contains(msg, "not a git") {
			t.Errorf("%s: error = %q, want 'not a git'", c.name, msg)
		}
	}
}

func TestGitHandlersInRealRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	dir := t.TempDir()
	run := func(name string, args ...string) string {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		out, _ := cmd.CombinedOutput()
		return string(out)
	}
	run("git", "init", "-q")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", "file.txt")
	run("git", "commit", "-q", "-m", "initial commit")

	ctx := context.Background()

	// git_status on a clean repo
	r, _ := handleGitStatus(ctx, `{"path":"`+dir+`"}`)
	if r.Error != "" {
		t.Fatalf("git_status error: %v", r.Error)
	}
	if !strings.Contains(r.Content.(string), "Clean working tree") {
		t.Errorf("git_status content = %q, want 'Clean working tree'", r.Content)
	}

	// git_log returns the initial commit
	r, _ = handleGitLog(ctx, `{"path":"`+dir+`","limit":5}`)
	if r.Error != "" {
		t.Fatalf("git_log error: %v", r.Error)
	}
	if !strings.Contains(r.Content.(string), "initial commit") {
		t.Errorf("git_log content = %q, want 'initial commit'", r.Content)
	}

	// git_blame on the committed file
	r, _ = handleGitBlame(ctx, `{"path":"`+filepath.Join(dir, "file.txt")+`"}`)
	if r.Error != "" {
		t.Fatalf("git_blame error: %v", r.Error)
	}
	if !strings.Contains(r.Content.(string), "Test") {
		t.Errorf("git_blame content = %q, want author 'Test'", r.Content)
	}
}

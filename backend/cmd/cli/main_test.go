package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func findModRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func buildCLI(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	bin := filepath.Join(t.TempDir(), "openaide-test")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/cli")
	cmd.Dir = findModRoot()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

func TestIsExistingFile(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	if !isExistingFile(tmpFile) {
		t.Error("existing file should return true")
	}
	if isExistingFile("/nonexistent/path") {
		t.Error("nonexistent path should return false")
	}
	if isExistingFile(t.TempDir()) {
		t.Error("directory should return false")
	}
}

func TestDetectFiles(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test.go")
	if err := os.WriteFile(tmpFile, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("mixed", func(t *testing.T) {
		files, prompts := detectFiles([]string{tmpFile, "fix", "this"})
		if len(files) != 1 || files[0] != tmpFile {
			t.Errorf("files = %v, want [%s]", files, tmpFile)
		}
		if got := strings.Join(prompts, " "); got != "fix this" {
			t.Errorf("prompts = %q, want 'fix this'", got)
		}
	})

	t.Run("all files", func(t *testing.T) {
		f1 := filepath.Join(t.TempDir(), "a.go")
		f2 := filepath.Join(t.TempDir(), "b.go")
		os.WriteFile(f1, []byte("a"), 0644)
		os.WriteFile(f2, []byte("b"), 0644)
		files, prompts := detectFiles([]string{f1, f2})
		if len(files) != 2 {
			t.Errorf("files = %v, want 2 files", files)
		}
		if len(prompts) != 0 {
			t.Errorf("prompts = %v, want empty", prompts)
		}
	})

	t.Run("all prompts", func(t *testing.T) {
		files, prompts := detectFiles([]string{"fix", "this"})
		if len(files) != 0 {
			t.Errorf("files = %v, want empty", files)
		}
		if len(prompts) != 2 {
			t.Errorf("prompts = %v, want 2", prompts)
		}
	})

	t.Run("empty", func(t *testing.T) {
		files, prompts := detectFiles(nil)
		if len(files) != 0 || len(prompts) != 0 {
			t.Errorf("expected empty, got files=%v prompts=%v", files, prompts)
		}
	})
}

func TestBuildPrompt(t *testing.T) {
	t.Run("no files", func(t *testing.T) {
		if got := buildPrompt(nil, "hello"); got != "hello" {
			t.Errorf("got %q, want 'hello'", got)
		}
	})

	t.Run("empty prompt", func(t *testing.T) {
		if got := buildPrompt(nil, ""); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("with file", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.txt")
		os.WriteFile(tmpFile, []byte("file content"), 0644)
		result := buildPrompt([]string{tmpFile}, "review")
		if !strings.Contains(result, "file content") {
			t.Errorf("result should contain file content, got %q", result)
		}
		if !strings.Contains(result, "review") {
			t.Errorf("result should contain prompt, got %q", result)
		}
	})

	t.Run("file only", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.txt")
		os.WriteFile(tmpFile, []byte("file content"), 0644)
		result := buildPrompt([]string{tmpFile}, "")
		if !strings.Contains(result, "file content") {
			t.Errorf("result should contain file content, got %q", result)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		result := buildPrompt([]string{"/nonexistent/file.txt"}, "hello")
		if result != "hello" {
			t.Errorf("missing file should be skipped, got %q", result)
		}
	})

	t.Run("multiple files", func(t *testing.T) {
		dir := t.TempDir()
		f1 := filepath.Join(dir, "a.txt")
		f2 := filepath.Join(dir, "b.txt")
		os.WriteFile(f1, []byte("content a"), 0644)
		os.WriteFile(f2, []byte("content b"), 0644)
		result := buildPrompt([]string{f1, f2}, "combine")
		if !strings.Contains(result, "content a") || !strings.Contains(result, "content b") {
			t.Errorf("result should contain both files, got %q", result)
		}
	})
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 5, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
		{"abc", 10, "abc"},
		{"你好世界", 2, "你好..."},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := truncate(tt.input, tt.max); got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
			}
		})
	}
}

func TestParseFlags_Defaults(t *testing.T) {
	f := parseFlags(nil)
	if f.outputFormat != "text" {
		t.Errorf("default outputFormat = %q, want 'text'", f.outputFormat)
	}
	if f.continueSess {
		t.Error("default continueSess should be false")
	}
	if f.yes {
		t.Error("default yes should be false")
	}
	if f.verbose {
		t.Error("default verbose should be false")
	}
}

func TestParseFlags_BoolFlags(t *testing.T) {
	f := parseFlags([]string{"-c", "-y", "--verbose"})
	if !f.continueSess {
		t.Error("-c not set")
	}
	if !f.yes {
		t.Error("-y not set")
	}
	if !f.verbose {
		t.Error("--verbose not set")
	}
}

func TestParseFlags_Model(t *testing.T) {
	f := parseFlags([]string{"--model", "gpt-4"})
	if f.model != "gpt-4" {
		t.Errorf("model = %q, want 'gpt-4'", f.model)
	}
}

func TestParseFlags_Output(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		f := parseFlags([]string{"--output", "json"})
		if f.outputFormat != "json" {
			t.Errorf("output = %q, want 'json'", f.outputFormat)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		f := parseFlags([]string{"--output", "xml"})
		if f.outputFormat != "text" {
			t.Errorf("output = %q, want 'text'", f.outputFormat)
		}
	})
}

func TestParseFlags_PositionalPrompt(t *testing.T) {
	f := parseFlags([]string{"fix", "this", "bug"})
	if f.prompt != "fix this bug" {
		t.Errorf("prompt = %q, want 'fix this bug'", f.prompt)
	}
	if len(f.contextFiles) != 0 {
		t.Errorf("contextFiles = %v, want empty", f.contextFiles)
	}
}

func TestParseFlags_Mixed(t *testing.T) {
	f := parseFlags([]string{"-c", "-y", "rewrite", "this"})
	if !f.continueSess {
		t.Error("-c not set")
	}
	if !f.yes {
		t.Error("-y not set")
	}
	if f.prompt != "rewrite this" {
		t.Errorf("prompt = %q, want 'rewrite this'", f.prompt)
	}
}

func TestCLI_HelpSubcommand(t *testing.T) {
	bin := buildCLI(t)
	out, err := exec.Command(bin, "help").CombinedOutput()
	if err != nil {
		t.Fatalf("help failed: %v", err)
	}
	if !strings.Contains(string(out), "Usage:") {
		t.Errorf("help output should contain 'Usage:', got %q", string(out))
	}
}

func TestCLI_VersionSubcommand(t *testing.T) {
	bin := buildCLI(t)
	out, err := exec.Command(bin, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("version failed: %v", err)
	}
	if !strings.Contains(string(out), "CLI dev") {
		t.Errorf("version output should contain 'CLI dev', got %q", string(out))
	}
}

func TestCLI_HELPFlag(t *testing.T) {
	bin := buildCLI(t)
	out, err := exec.Command(bin, "-h").CombinedOutput()
	if err != nil {
		t.Fatalf("-h failed: %v", err)
	}
	if !strings.Contains(string(out), "Usage:") {
		t.Errorf("-h output should contain 'Usage:', got %q", string(out))
	}
}

func TestCLI_VersionFlag(t *testing.T) {
	bin := buildCLI(t)
	out, err := exec.Command(bin, "-v").CombinedOutput()
	if err != nil {
		t.Fatalf("-v failed: %v", err)
	}
	if !strings.Contains(string(out), "CLI dev") {
		t.Errorf("-v output should contain 'CLI dev', got %q", string(out))
	}
}

func TestCLI_ChineseLocale(t *testing.T) {
	bin := buildCLI(t)
	cmd := exec.Command(bin, "help")
	cmd.Env = append(os.Environ(), "LANG=zh_CN.UTF-8")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("help with zh_CN failed: %v", err)
	}
	if !strings.Contains(string(out), "用法") {
		t.Errorf("zh_CN help output should contain '用法', got %q", string(out))
	}
}

func TestCLI_StderrOnMissingConfig(t *testing.T) {
	bin := buildCLI(t)
	cmd := exec.Command(bin, "fix", "this")
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Log("one-shot succeeded (unexpected, but not a failure)")
		return
	}
	if !bytes.Contains(out, []byte("启动失败")) && !bytes.Contains(out, []byte("Failed")) && !bytes.Contains(out, []byte("Error")) {
		t.Errorf("expected error message on missing config, got %q", string(out))
	}
}

package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePath_Basic(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"relative", "foo/bar.go", false},
		{"absolute", "/tmp/test.go", false},
		{"dot", ".", false},
		{"parent", "../foo", false},
		{"empty", "", true},
		{"whitespace", "  ", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := validatePath(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("validatePath(%q) expected error, got %q", tc.input, p)
				}
				return
			}
			if err != nil {
				t.Fatalf("validatePath(%q) unexpected error: %v", tc.input, err)
			}
			if !filepath.IsAbs(p) {
				t.Errorf("expected absolute path, got %q", p)
			}
		})
	}
}

func TestValidatePath_CleansTraversal(t *testing.T) {
	p, err := validatePath("/tmp/project/../../etc/passwd")
	if err != nil {
		t.Fatal(err)
	}
	// Should be cleaned to /etc/passwd (absolute + cleaned)
	if p != "/etc/passwd" {
		t.Errorf("expected /etc/passwd, got %q", p)
	}
}

func TestIsSubPath(t *testing.T) {
	tests := []struct {
		parent string
		child  string
		want   bool
	}{
		{"/project", "/project/src/main.go", true},
		{"/project", "/project", true},
		{"/project", "/other/file.go", false},
		{"/project", "/project../evil", false},
		{"/a/b", "/a/b/c/d", true},
		{"/a/b", "/a/bc/d", false},
	}
	for _, tc := range tests {
		t.Run(tc.parent+"→"+tc.child, func(t *testing.T) {
			got := isSubPath(tc.parent, tc.child)
			if got != tc.want {
				t.Errorf("isSubPath(%q, %q) = %v, want %v", tc.parent, tc.child, got, tc.want)
			}
		})
	}
}

func TestResolveSafePath_NoSymlink(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(f, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	real, err := resolveSafePath(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if real != f {
		t.Errorf("expected %q, got %q", f, real)
	}
}

func TestResolveSafePath_SymlinkWithinDir(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	// Symlink points within same dir — should be fine
	_, err := resolveSafePath(link)
	if err != nil {
		t.Fatalf("symlink within same dir should be safe: %v", err)
	}
}

func TestResolveSafePath_SymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(dir, "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create symlink in subdir that escapes to parent
	link := filepath.Join(subdir, "escape.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	_, err := resolveSafePath(link)
	if err == nil {
		t.Error("expected error for symlink escaping parent directory")
	}
}

func TestResolveSafePath_NonexistentFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "does_not_exist.txt")
	// File doesn't exist but parent dir is valid — should succeed
	p, err := resolveSafePath(f)
	if err != nil {
		t.Fatalf("nonexistent file with valid parent should succeed: %v", err)
	}
	if p != f {
		t.Errorf("expected %q, got %q", f, p)
	}
}

func TestValidateAndResolve_Basic(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(f, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	p, err := validateAndResolve(f)
	if err != nil {
		t.Fatal(err)
	}
	if p != f {
		t.Errorf("expected %q, got %q", f, p)
	}
}

func TestValidateAndResolve_Empty(t *testing.T) {
	_, err := validateAndResolve("")
	if err == nil {
		t.Error("expected error for empty path")
	}
}

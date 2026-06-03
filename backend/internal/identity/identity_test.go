package identity

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewDetector(t *testing.T) {
	if d := NewDetector(); d == nil {
		t.Fatal("expected non-nil detector")
	}
}

func TestDetectProjectType(t *testing.T) {
	d := NewDetector()
	tests := []struct {
		name     string
		setup    func(string)
		expected string
	}{
		{"go.mod", func(dir string) { touch(t, filepath.Join(dir, "go.mod")) }, "go"},
		{"package.json", func(dir string) { touch(t, filepath.Join(dir, "package.json")) }, "node"},
		{"Cargo.toml", func(dir string) { touch(t, filepath.Join(dir, "Cargo.toml")) }, "rust"},
		{"pyproject.toml", func(dir string) { touch(t, filepath.Join(dir, "pyproject.toml")) }, "python"},
		{"setup.py", func(dir string) { touch(t, filepath.Join(dir, "setup.py")) }, "python"},
		{"requirements.txt", func(dir string) { touch(t, filepath.Join(dir, "requirements.txt")) }, "python"},
		{"pom.xml", func(dir string) { touch(t, filepath.Join(dir, "pom.xml")) }, "java"},
		{"build.gradle", func(dir string) { touch(t, filepath.Join(dir, "build.gradle")) }, "java"},
		{"composer.json", func(dir string) { touch(t, filepath.Join(dir, "composer.json")) }, "php"},
		{"Gemfile", func(dir string) { touch(t, filepath.Join(dir, "Gemfile")) }, "ruby"},
		{"pubspec.yaml", func(dir string) { touch(t, filepath.Join(dir, "pubspec.yaml")) }, "flutter"},
		{"go.work", func(dir string) { touch(t, filepath.Join(dir, "go.work")) }, "go-workspace"},
		{"unknown", func(dir string) {}, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(dir)
			if got := d.detectProjectType(dir); got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestDetectProjectType_Priority(t *testing.T) {
	d := NewDetector()
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "go.mod"))
	touch(t, filepath.Join(dir, "package.json"))
	if got := d.detectProjectType(dir); got != "go" {
		t.Errorf("go.mod should take priority, got %q", got)
	}
}

func TestDetect(t *testing.T) {
	d := NewDetector()
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "go.mod"))

	ident, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if ident.WorkDir != dir || ident.ProjectType != "go" || ident.ProjectID == "" {
		t.Errorf("incomplete identity: %+v", ident)
	}
}

func TestGenerateProjectID(t *testing.T) {
	d := NewDetector()
	id1 := d.generateProjectID("/home/test/project")
	id2 := d.generateProjectID("/home/test/project")
	id3 := d.generateProjectID("/home/test/other")
	if id1 != id2 {
		t.Error("same path should produce same ID")
	}
	if id1 == id3 {
		t.Error("different paths should produce different IDs")
	}
}

func TestDetectUserID(t *testing.T) {
	d := NewDetector()
	os.Setenv("USER", "testuser")
	if got := d.detectUserID(); got != "testuser" {
		t.Errorf("expected 'testuser', got %q", got)
	}
	os.Unsetenv("USER")
	os.Setenv("USERNAME", "winuser")
	if got := d.detectUserID(); got != "winuser" {
		t.Errorf("expected 'winuser', got %q", got)
	}
	os.Unsetenv("USERNAME")
	if got := d.detectUserID(); got != "anonymous" {
		t.Errorf("expected 'anonymous', got %q", got)
	}
}

func TestNewSessionManager(t *testing.T) {
	sm := NewSessionManager("/tmp/data")
	if sm == nil || sm.dataDir != "/tmp/data" {
		t.Error("NewSessionManager failed")
	}
}

func TestSessionManager_GetProjectDir(t *testing.T) {
	sm := NewSessionManager("/tmp/data")
	got := sm.GetProjectDir(&Identity{ProjectID: "proj_abc"})
	if got != filepath.Join("/tmp/data", "projects", "proj_abc") {
		t.Errorf("unexpected dir: %q", got)
	}
}

func TestSessionManager_GetSessionDir(t *testing.T) {
	sm := NewSessionManager("/tmp/data")
	got := sm.GetSessionDir(&Identity{ProjectID: "proj_abc"}, "s1")
	if got != filepath.Join("/tmp/data", "projects", "proj_abc", "sessions", "s1") {
		t.Errorf("unexpected dir: %q", got)
	}
}

func TestSessionManager_EnsureDirs(t *testing.T) {
	sm := NewSessionManager(t.TempDir())
	if err := sm.EnsureDirs(&Identity{ProjectID: "proj_abc"}); err != nil {
		t.Fatalf("EnsureDirs failed: %v", err)
	}
}

func TestIsSameProject(t *testing.T) {
	a, b := &Identity{ProjectID: "x"}, &Identity{ProjectID: "x"}
	if !IsSameProject(a, b) || IsSameProject(a, nil) || IsSameProject(nil, b) {
		t.Error("IsSameProject logic error")
	}
	if IsSameProject(a, &Identity{ProjectID: "y"}) {
		t.Error("different projects should not be same")
	}
}

func TestGetProjectKey(t *testing.T) {
	if key := GetProjectKey(&Identity{ProjectID: "abc"}); key != "abc" {
		t.Errorf("expected 'abc', got %q", key)
	}
	if key := GetProjectKey(nil); key != "default" {
		t.Errorf("expected 'default', got %q", key)
	}
}

func touch(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	f.Close()
}

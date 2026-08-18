package prompts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsContainCoderAndArchitect(t *testing.T) {
	ds := Defaults()
	names := map[string]bool{}
	for _, p := range ds {
		names[p.Name] = true
	}
	if !names["coder"] || !names["architect"] {
		t.Fatalf("expected built-in coder and architect personas, got %v", names)
	}
	if ds[0].SystemPrompt == "" {
		t.Error("coder persona must have a system prompt")
	}
}

func TestStoreLookupMergesBuiltins(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if p := s.Lookup("coder"); p == nil {
		t.Fatal("expected built-in coder persona to be findable")
	}
	if p := s.Lookup("nonexistent"); p != nil {
		t.Fatalf("expected nil for unknown persona, got %v", p.Name)
	}
}

func TestStoreActivePersists(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	// Nothing active by default.
	if s.Active() != nil {
		t.Fatal("expected no active persona initially")
	}
	p, err := s.SetActive("architect")
	if err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if p.Name != "architect" {
		t.Fatalf("expected architect, got %s", p.Name)
	}
	// Reload from disk.
	s2 := NewStore(dir)
	if a := s2.Active(); a == nil || a.Name != "architect" {
		t.Fatalf("expected persisted active persona architect, got %v", a)
	}
	if _, err := s.SetActive("missing"); err == nil {
		t.Fatal("expected error for unknown persona")
	}
}

func TestStoreLoadsUserPersona(t *testing.T) {
	dir := t.TempDir()
	pdir := filepath.Join(dir, "researcher")
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pdir, "manifest.json"), []byte(`{"name":"researcher","description":"research","tool_allowlist":["search_files"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pdir, "system.md"), []byte("You are a researcher."), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewStore(dir)
	p := s.Lookup("researcher")
	if p == nil {
		t.Fatal("expected user persona researcher to be loaded")
	}
	if p.SystemPrompt != "You are a researcher." {
		t.Fatalf("unexpected prompt: %q", p.SystemPrompt)
	}
	if len(p.ToolAllowlist) != 1 || p.ToolAllowlist[0] != "search_files" {
		t.Fatalf("unexpected allowlist: %v", p.ToolAllowlist)
	}
}

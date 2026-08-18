// Package prompts provides pluggable persona definitions.
//
// A persona is a complete system prompt (identity + rules + modes) that can be
// swapped to change the kernel's behavior. Built-in personas ship with the
// binary; user personas are loaded from ~/.openaide/data/prompts/personas/<name>/.
//
// The active persona replaces the hardcoded L0 layer in the kernel's prompt
// assembly. Without an active persona the kernel falls back to its built-in
// default, preserving existing behavior.
package prompts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Persona is a pluggable identity/prompt bundle.
type Persona struct {
	// Name is the persona id (e.g. "coder", "architect", "researcher").
	Name string `json:"name"`
	// Description is a human-readable summary shown in listings.
	Description string `json:"description,omitempty"`
	// Version is the persona manifest version.
	Version string `json:"version,omitempty"`
	// Enabled marks whether the persona is active. Only one persona is active
	// at a time; the active one replaces the kernel's default L0 layer.
	Enabled bool `json:"enabled"`
	// SystemPrompt is the full L0 prompt for this persona (identity + rules).
	SystemPrompt string `json:"system_prompt,omitempty"`
	// ToolAllowlist optionally restricts which tools this persona may use.
	// Empty means all tools are allowed.
	ToolAllowlist []string `json:"tool_allowlist,omitempty"`
}

// manifest is the on-disk metadata for a persona directory.
type manifest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Enabled     bool     `json:"enabled"`
	ToolAllowlist []string `json:"tool_allowlist,omitempty"`
}

// Store loads, saves, and enumerates personas from a directory layout of
// <dir>/<name>/manifest.json + <dir>/<name>/system.md.
type Store struct {
	dir string
}

// NewStore creates a persona store rooted at dir. The directory is created if
// missing. Built-in personas are returned by Defaults() and merged over user
// personas on Lookup.
func NewStore(dir string) *Store {
	if dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return &Store{dir: dir}
}

// Dir returns the store root (used for diagnostics).
func (s *Store) Dir() string { return s.dir }

// List returns all personas (user + built-in), sorted by name. A persona's
// Enabled flag reflects the active selection; built-in personas are enabled
// only when explicitly chosen.
func (s *Store) List() []*Persona {
	byName := map[string]*Persona{}
	// User personas first so they override built-ins with the same name.
	for _, p := range s.loadUserPersonas() {
		byName[p.Name] = p
	}
	for _, p := range Defaults() {
		if _, exists := byName[p.Name]; !exists {
			byName[p.Name] = p
		}
	}
	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*Persona, 0, len(names))
	for _, n := range names {
		out = append(out, byName[n])
	}
	return out
}

// Lookup returns a persona by name, merging user overrides over built-ins.
func (s *Store) Lookup(name string) *Persona {
	for _, p := range s.List() {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// Active returns the currently enabled persona, or nil if none is selected.
func (s *Store) Active() *Persona {
	enabled := s.loadEnabled()
	if enabled != "" {
		if p := s.Lookup(enabled); p != nil {
			return p
		}
	}
	return nil
}

// ActiveSystemPrompt implements kernel.PersonaProvider. It returns the active
// persona's system prompt, or "" when no persona is selected (kernel then
// falls back to its built-in default L0).
func (s *Store) ActiveSystemPrompt() string {
	if p := s.Active(); p != nil {
		return p.SystemPrompt
	}
	return ""
}

// SetActive marks name as the active persona and persists the choice.
// It returns the resolved persona (merging user overrides) or an error if the
// name is unknown.
func (s *Store) SetActive(name string) (*Persona, error) {
	p := s.Lookup(name)
	if p == nil {
		return nil, &UnknownPersonaError{Name: name}
	}
	if err := s.saveEnabled(name); err != nil {
		return nil, err
	}
	return p, nil
}

// UnknownPersonaError is returned when SetActive references a missing persona.
type UnknownPersonaError struct{ Name string }

func (e *UnknownPersonaError) Error() string {
	return "unknown persona: " + e.Name
}

// loadEnabled reads the persisted active persona name.
func (s *Store) loadEnabled() string {
	if s.dir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(s.dir, "active"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// saveEnabled persists the active persona name.
func (s *Store) saveEnabled(name string) error {
	if s.dir == "" {
		return nil
	}
	return os.WriteFile(filepath.Join(s.dir, "active"), []byte(name), 0o644)
}

// loadUserPersonas scans user persona directories.
func (s *Store) loadUserPersonas() []*Persona {
	if s.dir == "" {
		return nil
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}
	var out []*Persona
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if p := s.loadPersonaDir(e.Name()); p != nil {
			out = append(out, p)
		}
	}
	return out
}

// loadPersonaDir loads a single persona directory (manifest + system.md).
func (s *Store) loadPersonaDir(name string) *Persona {
	base := filepath.Join(s.dir, name)

	var m manifest
	if data, err := os.ReadFile(filepath.Join(base, "manifest.json")); err == nil {
		_ = json.Unmarshal(data, &m)
	}
	sys, err := os.ReadFile(filepath.Join(base, "system.md"))
	if err != nil || len(sys) == 0 {
		return nil
	}
	p := &Persona{
		Name:          m.Name,
		Description:   m.Description,
		Version:       m.Version,
		Enabled:       m.Enabled,
		SystemPrompt:  string(sys),
		ToolAllowlist: m.ToolAllowlist,
	}
	if p.Name == "" {
		p.Name = name
	}
	return p
}

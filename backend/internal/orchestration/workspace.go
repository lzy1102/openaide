package orchestration

import (
	"fmt"
	"sync"
	"time"
)

// Artifact is a named work product that agents can create and consume
type Artifact struct {
	Name      string            `json:"name"`
	Type      string            `json:"type"` // code, analysis, plan, result, error
	Content   string            `json:"content"`
	CreatedBy string            `json:"created_by"`
	CreatedAt time.Time         `json:"created_at"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// Workspace is a shared workspace for agent collaboration.
// Agents write named artifacts; downstream agents read them by name
// instead of receiving raw concatenated text.
type Workspace struct {
	mu        sync.RWMutex
	artifacts map[string]*Artifact
}

// NewWorkspace creates a shared workspace
func NewWorkspace() *Workspace {
	return &Workspace{
		artifacts: make(map[string]*Artifact),
	}
}

// Put stores an artifact
func (w *Workspace) Put(name, artifactType, content, createdBy string) *Artifact {
	w.mu.Lock()
	defer w.mu.Unlock()

	a := &Artifact{
		Name:      name,
		Type:      artifactType,
		Content:   content,
		CreatedBy: createdBy,
		CreatedAt: time.Now(),
	}
	w.artifacts[name] = a
	return a
}

// Get retrieves an artifact by name
func (w *Workspace) Get(name string) *Artifact {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.artifacts[name]
}

// List returns all artifacts, optionally filtered by type
func (w *Workspace) List(artifactType string) []*Artifact {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var result []*Artifact
	for _, a := range w.artifacts {
		if artifactType == "" || a.Type == artifactType {
			result = append(result, a)
		}
	}
	return result
}

// Summary returns a human-readable summary of all artifacts for prompt injection
func (w *Workspace) Summary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if len(w.artifacts) == 0 {
		return ""
	}

	var s string
	s = "## 工作区中已有的产出物：\n"
	for _, a := range w.artifacts {
		preview := a.Content
		if len(preview) > 120 {
			preview = preview[:120] + "..."
		}
		s += fmt.Sprintf("- **%s** (%s by %s): %s\n",
			a.Name, a.Type, a.CreatedBy, preview)
	}
	s += "\n你可以引用这些产出物，避免重复工作。\n"
	return s
}

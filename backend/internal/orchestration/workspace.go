package orchestration

import (
	"fmt"
	"time"

	"openaide/backend/internal/kernel/actor"
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
type Workspace struct {
	artifacts *actor.SafeMap[string, *Artifact]
}

// NewWorkspace creates a shared workspace
func NewWorkspace() *Workspace {
	return &Workspace{
		artifacts: actor.NewSafeMap[string, *Artifact](8),
	}
}

// Put stores an artifact
func (w *Workspace) Put(name, artifactType, content, createdBy string) *Artifact {
	a := &Artifact{
		Name:      name,
		Type:      artifactType,
		Content:   content,
		CreatedBy: createdBy,
		CreatedAt: time.Now(),
	}
	w.artifacts.Store(name, a)
	return a
}

// Get retrieves an artifact by name
func (w *Workspace) Get(name string) *Artifact {
	a, _ := w.artifacts.Load(name)
	return a
}

// List returns all artifacts, optionally filtered by type
func (w *Workspace) List(artifactType string) []*Artifact {
	var result []*Artifact
	w.artifacts.Range(func(_ string, a *Artifact) bool {
		if artifactType == "" || a.Type == artifactType {
			result = append(result, a)
		}
		return true
	})
	return result
}

// Summary returns a human-readable summary of all artifacts for prompt injection
func (w *Workspace) Summary() string {
	if w.artifacts.Len() == 0 {
		return ""
	}

	s := "## 工作区中已有的产出物：\n"
	w.artifacts.Range(func(_ string, a *Artifact) bool {
		preview := a.Content
		if len(preview) > 120 {
			preview = preview[:120] + "..."
		}
		s += fmt.Sprintf("- **%s** (%s by %s): %s\n",
			a.Name, a.Type, a.CreatedBy, preview)
		return true
	})
	s += "\n你可以引用这些产出物，避免重复工作。\n"
	return s
}

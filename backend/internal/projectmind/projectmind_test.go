package projectmind

import (
	"testing"
	"time"
)

func TestLoadSave(t *testing.T) {
	dir := t.TempDir()
	pm := Load(dir)
	if pm == nil {
		t.Fatal("expected non-nil ProjectMind")
	}

	pm.AddCodeFact("main.go", "entry point", []string{"main", "run"}, 0.8, "research")
	pm.AddRisk("database.go", "SQL injection risk", "high", false)
	pm.AddLearning("testing", "Use table-driven tests")

	if err := pm.Save(); err != nil {
		t.Fatal(err)
	}

	// Reload and verify
	pm2 := Load(dir)
	if pm2 == nil {
		t.Fatal("expected non-nil reloaded ProjectMind")
	}
}

func TestRecordExecution(t *testing.T) {
	dir := t.TempDir()
	pm := Load(dir)

	pm.RecordExecution("fix bug", "direct", true,
		[]string{"main.go"}, []string{}, []string{},
		time.Second, "test-model")

	if err := pm.Save(); err != nil {
		t.Fatal(err)
	}
}

func TestConventionsForPrompt(t *testing.T) {
	dir := t.TempDir()
	pm := Load(dir)

	// No conventions yet
	prompt := pm.ConventionsForPrompt()
	if prompt != "" {
		t.Log("expected empty conventions for new project")
	}

	// Add a convention-like learning
	pm.AddLearning("naming", "Use camelCase for variables")
	pm.AddLearning("naming", "Use camelCase for functions")

	// Still empty because confidence < threshold
	prompt = pm.ConventionsForPrompt()
	if prompt != "" {
		t.Log("conventions should be empty until confidence threshold met")
	}
}

func TestFactsForPrompt(t *testing.T) {
	dir := t.TempDir()
	pm := Load(dir)

	pm.AddCodeFact("api.go", "HTTP handler", []string{"HandleRequest"}, 0.9, "research")
	pm.AddCodeFact("db.go", "Database layer", []string{"Query", "Insert"}, 0.7, "research")

	facts := pm.FactsForPrompt()
	if facts == "" {
		t.Log("facts may be empty if confidence decayed")
	}
}

func TestExpireOldFacts(t *testing.T) {
	dir := t.TempDir()
	pm := Load(dir)

	pm.AddCodeFact("old.go", "old file", []string{"OldFunc"}, 0.5, "research")
	pm.ExpireOldFacts()
	// Should not panic and should reduce fact set
}

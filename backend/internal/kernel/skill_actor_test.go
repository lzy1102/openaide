package kernel

import (
	"context"
	"sync"
	"testing"
	"time"
)

type mockLLM struct{ resp string }

func (m *mockLLM) Chat(ctx context.Context, msgs []Message, tools []ToolDefinition, opts map[string]interface{}) (*LLMResponse, error) {
	return &LLMResponse{Content: m.resp}, nil
}
func (m *mockLLM) ChatStream(ctx context.Context, msgs []Message, tools []ToolDefinition, opts map[string]interface{}) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 1)
	ch <- StreamChunk{Type: ChunkTypeContent, Content: m.resp, Done: true}
	close(ch)
	return ch, nil
}
func (m *mockLLM) GetModelID() string { return "mock" }
func (m *mockLLM) SetModelID(s string) {}

func TestSkillActor_DetectLLM(t *testing.T) {
	llm := &mockLLM{resp: "code-review"}
	a := NewSkillActor(llm)
	a.AddSkill("code-review", "Code Review", "Reviews code", "You are a code reviewer", []string{"review", "code"})

	skill := a.DetectSkill(context.Background(), "please review my changes")
	if skill == nil {
		t.Fatal("expected skill match from LLM")
	}
	if skill.ID != "code-review" {
		t.Errorf("expected 'code-review', got '%s'", skill.ID)
	}
}

func TestSkillActor_NoLLM(t *testing.T) {
	// Without LLM, skill detection returns nil — no fallback to keywords.
	// The LLM is the brain. If it's unavailable, the agent is dead.
	a := NewSkillActor(nil)
	a.AddSkill("deploy", "Deploy", "Deploy stuff", "deploy prompt", []string{"deploy", "ship"})

	skill := a.DetectSkill(context.Background(), "let's deploy to production")
	if skill != nil {
		t.Error("without LLM, DetectSkill should return nil — no keyword fallback")
	}
}

func TestSkillActor_InjectPrompt(t *testing.T) {
	llm := &mockLLM{resp: "code-review"}
	a := NewSkillActor(llm)
	a.AddSkill("code-review", "Code Review", "Reviews code", "You are a code reviewer", nil)

	result := a.InjectPrompt(context.Background(), "review this", "base prompt")
	if result == "base prompt" {
		t.Error("expected injected prompt")
	}
}

func TestSkillActor_GetTools(t *testing.T) {
	llm := &mockLLM{resp: "deploy"}
	a := NewSkillActor(llm)
	a.AddClaudeSkill("deploy", "Deploy", "Deploy stuff", "deploy prompt",
		[]string{"deploy"}, []string{"execute_command", "read_file"})

	tools := a.GetTools(context.Background(), "deploy")
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
}

func TestSkillActor_Feedback(t *testing.T) {
	a := NewSkillActor(nil)
	a.AddSkill("test", "Test", "Testing", "prompt", nil)

	// Trigger detection to set lastUsed
	a.super.Send(func() { a.lastUsed = "test" })

	a.RecordLastUsage(8) // Good quality
	time.Sleep(10 * time.Millisecond) // wait for async

	var conf float64
	a.super.Send(func() {
		if s, ok := a.skills["test"]; ok {
			conf = s.Confidence
		}
	})
	if conf <= 0.5 {
		t.Errorf("expected confidence > 0.5 after good feedback, got %.2f", conf)
	}
}

func TestSkillActor_Concurrent(t *testing.T) {
	a := NewSkillActor(nil)
	a.AddSkill("a", "Skill A", "Alpha", "pA", []string{"alpha"})
	a.AddSkill("b", "Skill B", "Beta", "pB", []string{"beta"})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = a.DetectSkill(context.Background(), "alpha test")
			a.RecordUsage("a", 7)
			_ = a.InjectPrompt(context.Background(), "beta query", "base")
			_ = a.GetTools(context.Background(), "alpha query")
		}()
	}
	wg.Wait()
	// If we reach here without deadlock or panic, the test passes.
}

func TestSkillActor_AutoDetectOff(t *testing.T) {
	a := NewSkillActor(nil)
	a.AddSkill("test", "Test", "Testing", "p", []string{"test"})
	a.SetAutoDetect(false)

	skill := a.DetectSkill(context.Background(), "test query")
	if skill != nil {
		t.Error("expected nil when auto-detect is off")
	}
}

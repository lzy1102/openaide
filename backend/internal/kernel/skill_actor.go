package kernel

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"openaide/backend/internal/kernel/actor"
)

// Skill represents a registered skill/prompt template.
type Skill struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Prompt       string   `json:"prompt"`
	Keywords     []string `json:"keywords"`
	Tools        []string `json:"tools"`
	AllowedTools []string `json:"allowed_tools"`
	Enabled      bool     `json:"enabled"`

	UsageCount   int       `json:"usage_count"`
	SuccessCount int       `json:"success_count"`
	Confidence   float64   `json:"confidence"`
	LastUsed     time.Time `json:"last_used"`
}

type skillEntry struct {
	ID, Description string
}

// SkillActor is a CSP-style skill manager. All skill state lives in a single
// goroutine — zero locks. LLM calls for detection happen outside the actor
// to prevent blocking all skill operations.
type SkillActor struct {
	super      *actor.Actor
	llm        LLMProvider
	skills     map[string]*Skill
	autoDetect bool
	lastUsed   string // ID of most recently activated skill
	onSave     func() // optional persistence callback
}

// NewSkillActor creates and starts a skill actor.
func NewSkillActor(llm LLMProvider) *SkillActor {
	a := &SkillActor{
		super:      actor.NewActor(64),
		llm:        llm,
		skills:     make(map[string]*Skill),
		autoDetect: true,
	}
	return a
}

// ── Skill Management ─────────────────────────────────────

func (a *SkillActor) AddSkill(id, name, description, prompt string, keywords []string) {
	a.super.Send(func() {
		a.skills[id] = &Skill{
			ID: id, Name: name, Description: description,
			Prompt: prompt, Keywords: keywords,
			Enabled: true, Confidence: 0.5,
		}
	})
}

func (a *SkillActor) AddClaudeSkill(id, name, description, prompt string, keywords []string, allowedTools []string) {
	a.super.Send(func() {
		a.skills[id] = &Skill{
			ID: id, Name: name, Description: description,
			Prompt: prompt, Keywords: keywords,
			Enabled: true, Confidence: 0.6,
			AllowedTools: allowedTools,
		}
		a.save()
	})
}

func (a *SkillActor) Enable(id string) {
	a.super.Send(func() {
		if s, ok := a.skills[id]; ok { s.Enabled = true; a.save() }
	})
}

func (a *SkillActor) Disable(id string) {
	a.super.Send(func() {
		if s, ok := a.skills[id]; ok { s.Enabled = false; a.save() }
	})
}

func (a *SkillActor) SetAutoDetect(on bool) {
	a.super.Send(func() { a.autoDetect = on })
}

// ── Detection ────────────────────────────────────────────

// DetectSkill finds the best matching skill for a query.
// The LLM call runs OUTSIDE the actor to avoid blocking.
func (a *SkillActor) DetectSkill(ctx context.Context, query string) *Skill {
	slog.Info("Skill actor detect", "query", query[:min(80, len(query))])

	// Step 1: prepare skill list inside actor
	var autoDetect bool
	var skillList []skillEntry
	a.super.Send(func() {
		autoDetect = a.autoDetect
		if !autoDetect {
			return
		}
		for _, s := range a.skills {
			if s.Enabled {
				skillList = append(skillList, skillEntry{s.ID, s.Description})
			}
		}
	})
	if !autoDetect {
		return nil
	}

	// Step 2: LLM call OUTSIDE the actor
	var matchID string
	if a.llm != nil && len(skillList) > 0 {
		matchID = a.detectWithLLM(ctx, query, skillList)
	}

	// Step 3: look up matched skill inside actor
	if matchID != "" {
		var matched *Skill
		a.super.Send(func() {
			if s, ok := a.skills[matchID]; ok {
				matched = s
				a.lastUsed = matchID
			}
		})
		if matched != nil {
			return matched
		}
	}

	return nil
}

// GetTools returns the allowed tools for a skill.
func (a *SkillActor) GetTools(ctx context.Context, query string) []string {
	skill := a.DetectSkill(ctx, query)
	if skill == nil { return nil }
	if len(skill.AllowedTools) > 0 { return skill.AllowedTools }
	return skill.Tools
}

// InjectPrompt injects the skill prompt into the system prompt.
func (a *SkillActor) InjectPrompt(ctx context.Context, query string, basePrompt string) string {
	skill := a.DetectSkill(ctx, query)
	if skill == nil { return basePrompt }
	return basePrompt + fmt.Sprintf("\n\n## Current Active Skill: %s\n%s", skill.Name, skill.Prompt)
}

// ── Feedback ──────────────────────────────────────────────

func (a *SkillActor) RecordUsage(skillID string, qualityScore int) {
	a.super.SendAsync(func() {
		s, ok := a.skills[skillID]
		if !ok { return }
		s.UsageCount++
		s.LastUsed = time.Now()
		if qualityScore >= 6 {
			s.SuccessCount++
			s.Confidence = min(0.95, s.Confidence+0.05)
		} else {
			s.Confidence = max(0.1, s.Confidence-0.1)
		}
		if s.Confidence < 0.3 {
			s.Enabled = false
			slog.Info("Skill auto-disabled", "id", skillID, "confidence", s.Confidence)
		}
		a.save()
	})
}

func (a *SkillActor) RecordLastUsage(qualityScore int) {
	a.super.SendAsync(func() {
		if a.lastUsed != "" {
			// Re-dispatch to RecordUsage logic
			s, ok := a.skills[a.lastUsed]
			if !ok { return }
			s.UsageCount++
			s.LastUsed = time.Now()
			if qualityScore >= 6 {
				s.SuccessCount++
				s.Confidence = min(0.95, s.Confidence+0.05)
			} else {
				s.Confidence = max(0.1, s.Confidence-0.1)
			}
			a.save()
		}
	})
}

// ── Maintenance ───────────────────────────────────────────

func (a *SkillActor) DecayUnused() {
	a.super.SendAsync(func() {
		cutoff := time.Now().Add(-14 * 24 * time.Hour)
		for id, s := range a.skills {
			if s.Enabled && s.LastUsed.Before(cutoff) {
				s.Confidence = max(0.1, s.Confidence-0.1)
				if s.Confidence < 0.3 {
					s.Enabled = false
					slog.Info("Skill decayed", "id", id)
				}
			}
		}
		a.save()
	})
}

// ── Internal ──────────────────────────────────────────────

func (a *SkillActor) save() {
	if a.onSave != nil { a.onSave() }
}

// SetOnSave sets the persistence callback.
func (a *SkillActor) SetOnSave(fn func()) { a.onSave = fn }

// ExportSkills returns all skills for persistence.
func (a *SkillActor) ExportSkills() map[string]*Skill {
	result := make(map[string]*Skill)
	a.super.Send(func() {
		for id, s := range a.skills {
			result[id] = s
		}
	})
	return result
}

// Stop shuts down the actor.
func (a *SkillActor) Stop() { a.super.Stop() }

// Actor returns the underlying actor for direct command dispatch.
func (a *SkillActor) Actor() *actor.Actor { return a.super }

func (a *SkillActor) detectWithLLM(ctx context.Context, query string, skillList []skillEntry) string {
	var b strings.Builder
	for _, s := range skillList {
		b.WriteString(fmt.Sprintf("- %s: %s\n", s.ID, s.Description))
	}
	resp, err := a.llm.Chat(ctx, []Message{
		{Role: "user", Content: fmt.Sprintf(
			"Which skill matches? Reply with the skill ID or 'none'.\n\nSkills:\n%s\nQuery: %s",
			b.String(), query)},
	}, nil, map[string]interface{}{"max_tokens": 30, "temperature": 0, "route": "execution", "no_thinking": true})
	if err != nil { return "" }
	choice := strings.TrimSpace(resp.Content)
	if choice == "" || choice == "none" { return "" }
	return choice
}

// ── Helpers ───────────────────────────────────────────────

// SkillSkill implements the SkillManager interface for backward compat.
// It wraps the actor methods.
func (a *SkillActor) GetSkillManager() *SkillActor { return a }

// Ensure interface compliance.
var _ interface {
	DetectSkill(context.Context, string) *Skill
	InjectPrompt(context.Context, string, string) string
	GetTools(context.Context, string) []string
} = (*SkillActor)(nil)

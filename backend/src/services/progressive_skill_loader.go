package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

const (
	skillMatchPrompt = `Given the following user query, determine which skills from the list are relevant.

User query: %s

Available skills:
%s

Respond with a JSON array of skill IDs that are relevant, ordered by relevance (most relevant first).
If no skills are relevant, respond with [].
Only include skills that are clearly relevant - do not include skills that might be tangentially related.

Example response: ["skill_id_1", "skill_id_2"]`
)

type SkillLevel int

const (
	SkillLevelNone SkillLevel = iota
	SkillLevel0
	SkillLevel1
	SkillLevel2
)

type LoadedSkill struct {
	SkillID   string     `json:"skill_id"`
	Name      string     `json:"name"`
	Level     SkillLevel `json:"level"`
	Summary   string     `json:"summary,omitempty"`
	Content   string     `json:"content,omitempty"`
	Triggers  []string   `json:"triggers,omitempty"`
	Tools     []string   `json:"tools,omitempty"`
	Relevance float64    `json:"relevance"`
}

type ProgressiveSkillLoader struct {
	skillSvc    *SkillService
	modelCaller ModelCaller
	cache       map[string]*LoadedSkill
	cacheMu     sync.RWMutex
	maxLevel0   int
}

func NewProgressiveSkillLoader(skillSvc *SkillService, modelCaller ModelCaller) *ProgressiveSkillLoader {
	return &ProgressiveSkillLoader{
		skillSvc:    skillSvc,
		modelCaller: modelCaller,
		cache:       make(map[string]*LoadedSkill),
		maxLevel0:   5,
	}
}

func (psl *ProgressiveSkillLoader) MatchSkills(ctx context.Context, query string) ([]*LoadedSkill, error) {
	skills, err := psl.skillSvc.ListEnabledSkills()
	if err != nil {
		return nil, fmt.Errorf("failed to list skills: %w", err)
	}

	if len(skills) == 0 {
		return nil, nil
	}

	keywordMatches := psl.matchByKeywords(query, skills)

	if len(keywordMatches) == 0 {
		return nil, nil
	}

	if len(keywordMatches) <= psl.maxLevel0 {
		return psl.loadLevel0Batch(keywordMatches), nil
	}

	llmMatches, err := psl.matchByLLM(ctx, query, skills)
	if err != nil {
		slog.Error("LLM skill matching failed, using keyword results", "component", "ProgressiveSkillLoader", "error", err)
		sort.Slice(keywordMatches, func(i, j int) bool {
			return keywordMatches[i].Relevance > keywordMatches[j].Relevance
		})
		if len(keywordMatches) > psl.maxLevel0 {
			keywordMatches = keywordMatches[:psl.maxLevel0]
		}
		return psl.loadLevel0Batch(keywordMatches), nil
	}

	return psl.loadLevel0Batch(llmMatches), nil
}

func (psl *ProgressiveSkillLoader) matchByKeywords(query string, skills []models.Skill) []*LoadedSkill {
	lowerQuery := strings.ToLower(query)
	var matches []*LoadedSkill

	for i := range skills {
		skill := &skills[i]
		relevance := 0.0

		if skill.Triggers != nil {
			for _, trigger := range skill.Triggers {
				if strings.Contains(lowerQuery, strings.ToLower(trigger)) {
					relevance += 0.5
				}
			}
		}

		if strings.Contains(lowerQuery, strings.ToLower(skill.Name)) {
			relevance += 0.8
		}

		if strings.Contains(lowerQuery, strings.ToLower(skill.Category)) {
			relevance += 0.3
		}

		if relevance > 0 {
			loaded := &LoadedSkill{
				SkillID:   skill.ID,
				Name:      skill.Name,
				Level:     SkillLevelNone,
				Triggers:  jsonSliceToStrings(skill.Triggers),
				Tools:     jsonSliceToStrings(skill.Tools),
				Relevance: relevance,
			}
			matches = append(matches, loaded)
		}
	}

	return matches
}

func (psl *ProgressiveSkillLoader) matchByLLM(ctx context.Context, query string, skills []models.Skill) ([]*LoadedSkill, error) {
	var sb strings.Builder
	for i, skill := range skills {
		summary := skill.Level0Summary
		if summary == "" {
			summary = skill.Description
		}
		if len(summary) > 200 {
			summary = summary[:200] + "..."
		}
		sb.WriteString(fmt.Sprintf("- %s: %s (category: %s)\n", skill.ID, summary, skill.Category))
		if i >= 20 {
			sb.WriteString("... and more\n")
			break
		}
	}

	prompt := fmt.Sprintf(skillMatchPrompt, query, sb.String())

	modelID := psl.findFastModel()
	resp, err := psl.modelCaller.Chat(modelID, []llm.Message{
		{Role: llm.RoleUser, Content: prompt},
	}, map[string]interface{}{"max_tokens": float64(500), "temperature": float64(0.1)})
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty LLM response")
	}

	content := resp.Choices[0].Message.Content
	content = strings.TrimSpace(content)

	var matchedIDs []string
	start := strings.Index(content, "[")
	end := strings.LastIndex(content, "]")
	if start >= 0 && end > start {
		jsonArr := content[start : end+1]
		if err := parseJSONSlice(jsonArr, &matchedIDs); err != nil {
			slog.Error("Failed to parse LLM skill match response", "component", "ProgressiveSkillLoader", "error", err)
			return nil, err
		}
	}

	skillMap := make(map[string]*models.Skill)
	for i := range skills {
		skillMap[skills[i].ID] = &skills[i]
	}

	var result []*LoadedSkill
	for idx, id := range matchedIDs {
		if skill, ok := skillMap[id]; ok {
			relevance := 1.0 - float64(idx)*0.1
			if relevance < 0.1 {
				relevance = 0.1
			}
			result = append(result, &LoadedSkill{
				SkillID:   skill.ID,
				Name:      skill.Name,
				Level:     SkillLevelNone,
				Triggers:  jsonSliceToStrings(skill.Triggers),
				Tools:     jsonSliceToStrings(skill.Tools),
				Relevance: relevance,
			})
		}
	}

	if len(result) > psl.maxLevel0 {
		result = result[:psl.maxLevel0]
	}

	return result, nil
}

func (psl *ProgressiveSkillLoader) loadLevel0Batch(skills []*LoadedSkill) []*LoadedSkill {
	for _, ls := range skills {
		psl.loadLevel0(ls)
	}
	return skills
}

func (psl *ProgressiveSkillLoader) loadLevel0(ls *LoadedSkill) {
	psl.cacheMu.RLock()
	if cached, ok := psl.cache[ls.SkillID]; ok && cached.Level >= SkillLevel0 {
		psl.cacheMu.RUnlock()
		ls.Level = cached.Level
		ls.Summary = cached.Summary
		return
	}
	psl.cacheMu.RUnlock()

	skill, err := psl.skillSvc.GetSkillLevel0(ls.SkillID)
	if err != nil {
		slog.Error("Failed to load skill Level0", "component", "ProgressiveSkillLoader", "skill_id", ls.SkillID, "error", err)
		return
	}

	ls.Level = SkillLevel0
	ls.Summary = skill.Level0Summary
	if ls.Summary == "" {
		ls.Summary = skill.Description
	}

	psl.cacheMu.Lock()
	psl.cache[ls.SkillID] = ls
	psl.cacheMu.Unlock()
}

func (psl *ProgressiveSkillLoader) LoadFullContent(skillID string) (*LoadedSkill, error) {
	psl.cacheMu.RLock()
	if cached, ok := psl.cache[skillID]; ok && cached.Level >= SkillLevel1 {
		psl.cacheMu.RUnlock()
		return cached, nil
	}
	psl.cacheMu.RUnlock()

	skill, err := psl.skillSvc.GetSkillLevel1(skillID)
	if err != nil {
		return nil, fmt.Errorf("failed to load skill Level1: %w", err)
	}

	ls := &LoadedSkill{
		SkillID:  skill.ID,
		Name:     skill.Name,
		Level:    SkillLevel1,
		Summary:  skill.Level0Summary,
		Content:  skill.Level1Content,
		Triggers: jsonSliceToStrings(skill.Triggers),
		Tools:    jsonSliceToStrings(skill.Tools),
	}

	if ls.Summary == "" {
		ls.Summary = skill.Description
	}
	if ls.Content == "" {
		ls.Content = skill.InstructionBody
	}

	psl.cacheMu.Lock()
	psl.cache[skillID] = ls
	psl.cacheMu.Unlock()

	return ls, nil
}

func (psl *ProgressiveSkillLoader) LoadReferences(skillID string) ([]string, error) {
	return psl.skillSvc.GetSkillLevel2(skillID)
}

func (psl *ProgressiveSkillLoader) BuildSkillContext(skills []*LoadedSkill) string {
	var sb strings.Builder
	sb.WriteString("[Matched Skills]\n")
	for _, ls := range skills {
		sb.WriteString(fmt.Sprintf("- %s", ls.Name))
		if ls.Summary != "" {
			maxLen := 500
			summary := ls.Summary
			if len(summary) > maxLen {
				summary = summary[:maxLen] + "..."
			}
			sb.WriteString(fmt.Sprintf(": %s", summary))
		}
		sb.WriteString("\n")
		if len(ls.Tools) > 0 {
			sb.WriteString(fmt.Sprintf("  Required tools: %s\n", strings.Join(ls.Tools, ", ")))
		}
	}
	return sb.String()
}

func (psl *ProgressiveSkillLoader) ClearCache() {
	psl.cacheMu.Lock()
	psl.cache = make(map[string]*LoadedSkill)
	psl.cacheMu.Unlock()
}

func (psl *ProgressiveSkillLoader) findFastModel() string {
	models, err := psl.modelCaller.ListModels()
	if err != nil || len(models) == 0 {
		return ""
	}

	for _, m := range models {
		for _, tag := range m.Tags {
			if strings.TrimSpace(tag) == "fast" {
				return m.ID
			}
		}
	}

	for _, m := range models {
		if m.Status == "enabled" {
			return m.ID
		}
	}

	return models[0].ID
}

func jsonSliceToStrings(slice models.JSONSlice) []string {
	if slice == nil {
		return nil
	}
	return []string(slice)
}

func parseJSONSlice(data string, result *[]string) error {
	return json.Unmarshal([]byte(data), result)
}

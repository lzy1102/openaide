package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"openaide/backend/src/models"
)

func TestProgressiveSkillLoader_MatchByKeywords(t *testing.T) {
	svc := NewProgressiveSkillLoader(nil, nil)

	skills := []models.Skill{
		{ID: "1", Name: "code_search", Category: "search", Triggers: models.JSONSlice{"search", "find", "grep"}, Tools: models.JSONSlice{"search_code", "grep"}},
		{ID: "2", Name: "file_edit", Category: "edit", Triggers: models.JSONSlice{"edit", "modify", "change"}, Tools: models.JSONSlice{"write_file", "edit_file"}},
		{ID: "3", Name: "web_search", Category: "web", Triggers: models.JSONSlice{"web", "internet", "online"}, Tools: models.JSONSlice{"web_search"}},
	}

	matched := svc.matchByKeywords("search for authentication code", skills)
	assert.Greater(t, len(matched), 0, "should match at least one skill")

	for _, s := range matched {
		assert.Contains(t, []string{"1", "2", "3"}, s.SkillID)
	}
}

func TestProgressiveSkillLoader_MatchByKeywords_SpecificSkill(t *testing.T) {
	svc := NewProgressiveSkillLoader(nil, nil)

	skills := []models.Skill{
		{ID: "1", Name: "code_search", Category: "search", Triggers: models.JSONSlice{"search", "find", "grep"}, Tools: models.JSONSlice{"search_code", "grep"}},
	}

	matched := svc.matchByKeywords("write a new module from scratch", skills)
	assert.Equal(t, 0, len(matched), "should not match code_search for write query")
}

func TestProgressiveSkillLoader_BuildSkillContext(t *testing.T) {
	svc := NewProgressiveSkillLoader(nil, nil)

	skills := []*LoadedSkill{
		{SkillID: "1", Name: "code_search", Level: SkillLevel0, Summary: "Search code in repository", Tools: []string{"search_code", "grep"}},
		{SkillID: "2", Name: "file_edit", Level: SkillLevel1, Summary: "Edit files in project", Content: "Full content here", Tools: []string{"write_file"}},
	}

	context := svc.BuildSkillContext(skills)
	assert.Contains(t, context, "code_search")
	assert.Contains(t, context, "file_edit")
}

func TestLoadedSkill_Levels(t *testing.T) {
	level0 := LoadedSkill{Level: SkillLevel0, Summary: "Brief summary"}
	level1 := LoadedSkill{Level: SkillLevel1, Summary: "Brief summary", Content: "Full content"}
	level2 := LoadedSkill{Level: SkillLevel2, Summary: "Brief summary", Content: "Full content"}

	assert.Equal(t, SkillLevel0, level0.Level)
	assert.Empty(t, level0.Content)

	assert.Equal(t, SkillLevel1, level1.Level)
	assert.NotEmpty(t, level1.Content)

	assert.Equal(t, SkillLevel2, level2.Level)
}

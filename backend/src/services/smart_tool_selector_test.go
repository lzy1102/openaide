package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSmartToolSelector_Profiles(t *testing.T) {
	selector := NewSmartToolSelector()

	assert.NotNil(t, selector)
	assert.Equal(t, 4, len(selector.profiles))

	_, hasBuild := selector.profiles["build"]
	_, hasExplore := selector.profiles["explore"]
	_, hasPlan := selector.profiles["plan"]
	_, hasGeneral := selector.profiles["general"]

	assert.True(t, hasBuild, "build profile should exist")
	assert.True(t, hasExplore, "explore profile should exist")
	assert.True(t, hasPlan, "plan profile should exist")
	assert.True(t, hasGeneral, "general profile should exist")
}

func TestSmartToolSelector_SelectTools(t *testing.T) {
	selector := NewSmartToolSelector()

	tools := []map[string]interface{}{
		{"function": map[string]interface{}{"name": "read_file"}},
		{"function": map[string]interface{}{"name": "write_file"}},
		{"function": map[string]interface{}{"name": "execute_command"}},
		{"function": map[string]interface{}{"name": "search_code"}},
		{"function": map[string]interface{}{"name": "delete_file"}},
		{"function": map[string]interface{}{"name": "list_directory"}},
		{"function": map[string]interface{}{"name": "web_search"}},
	}

	selected := selector.SelectTools("search for authentication code", tools)

	for _, tool := range selected {
		toolName := extractToolName(tool)
		assert.NotEqual(t, "write_file", toolName, "explore mode should not include write_file")
		assert.NotEqual(t, "delete_file", toolName, "explore mode should not include delete_file")
	}
}

func TestSmartToolSelector_SelectTools_UnderThreshold(t *testing.T) {
	selector := NewSmartToolSelector()

	tools := []map[string]interface{}{
		{"function": map[string]interface{}{"name": "read_file"}},
		{"function": map[string]interface{}{"name": "write_file"}},
		{"function": map[string]interface{}{"name": "search_code"}},
	}

	selected := selector.SelectTools("do something", tools)
	assert.Equal(t, 3, len(selected), "build profile allows all tools")
}

func TestSmartToolSelector_SetDefaultProfile(t *testing.T) {
	selector := NewSmartToolSelector()

	err := selector.SetDefaultProfile("explore")
	assert.NoError(t, err)
	assert.Equal(t, "explore", selector.defaultProfile)

	err = selector.SetDefaultProfile("nonexistent")
	assert.Error(t, err)
}

func TestSmartToolSelector_SelectProfile(t *testing.T) {
	selector := NewSmartToolSelector()

	tests := []struct {
		query    string
		expected string
	}{
		{"search for all references to this function", "explore"},
		{"analyze the architecture and create a plan", "plan"},
		{"fix the bug in auth", "general"},
		{"deploy the application", "general"},
	}

	for _, tt := range tests {
		profile := selector.SelectProfile(tt.query)
		assert.Equal(t, tt.expected, profile, "query: %s", tt.query)
	}
}

func TestSmartToolSelector_AnalyzeContext(t *testing.T) {
	selector := NewSmartToolSelector()

	assert.Equal(t, "general", selector.AnalyzeContext(nil))
	assert.Equal(t, "general", selector.AnalyzeContext([]string{}))

	assert.Equal(t, "explore", selector.AnalyzeContext([]string{"search", "read_file", "web_search"}))
	assert.Equal(t, "build", selector.AnalyzeContext([]string{"search", "write_file"}))
	assert.Equal(t, "general", selector.AnalyzeContext([]string{"search"}))
}

func TestSmartToolSelector_GetProfile(t *testing.T) {
	selector := NewSmartToolSelector()

	profile := selector.GetProfile("explore")
	assert.NotNil(t, profile)
	assert.Equal(t, "explore", profile.Name)
	assert.Contains(t, profile.DeniedTools, "write_file")

	profile = selector.GetProfile("nonexistent")
	assert.Nil(t, profile)
}

func TestSmartToolSelector_RegisterProfile(t *testing.T) {
	selector := NewSmartToolSelector()

	custom := &ToolProfile{
		Name:         "custom",
		Description:  "Custom profile",
		Mode:         "primary",
		AllowedTools: []string{"read_file", "search"},
		DeniedTools:  []string{"write_file"},
	}

	selector.RegisterProfile(custom)
	profile := selector.GetProfile("custom")
	assert.NotNil(t, profile)
	assert.Equal(t, "custom", profile.Name)
}

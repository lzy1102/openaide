package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeyInfoExtractor_DeduplicateInfos(t *testing.T) {
	extractor := NewKeyInfoExtractor(nil)

	existing := []KeyInfo{
		{Topic: "auth", Content: "Uses JWT tokens", Importance: 0.7, Tags: []string{"security"}},
		{Topic: "database", Content: "PostgreSQL 15", Importance: 0.8, Tags: []string{"infra"}},
	}

	newInfos := []KeyInfo{
		{Topic: "auth", Content: "Uses JWT tokens for authentication", Importance: 0.9, Tags: []string{"security", "auth"}},
		{Topic: "api", Content: "REST API with versioning", Importance: 0.6, Tags: []string{"architecture"}},
	}

	merged := extractor.DeduplicateInfos(existing, newInfos)
	assert.Equal(t, 3, len(merged))

	authFound := false
	for _, info := range merged {
		if info.Topic == "auth" {
			authFound = true
			assert.Equal(t, 0.9, info.Importance, "should keep higher importance version")
		}
	}
	assert.True(t, authFound, "auth topic should exist")
}

func TestKeyInfoExtractor_DeduplicateInfos_NoDuplicates(t *testing.T) {
	extractor := NewKeyInfoExtractor(nil)

	existing := []KeyInfo{
		{Topic: "auth", Content: "Uses JWT tokens", Importance: 0.7},
	}

	newInfos := []KeyInfo{
		{Topic: "database", Content: "PostgreSQL 15", Importance: 0.8},
		{Topic: "api", Content: "REST API", Importance: 0.6},
	}

	merged := extractor.DeduplicateInfos(existing, newInfos)
	assert.Equal(t, 3, len(merged))
}

func TestKeyInfoExtractor_BuildContextFromInfos(t *testing.T) {
	extractor := NewKeyInfoExtractor(nil)

	infos := []KeyInfo{
		{Topic: "auth", Content: "Uses JWT tokens", Importance: 0.9},
		{Topic: "database", Content: "PostgreSQL 15", Importance: 0.8},
		{Topic: "api", Content: "REST API with versioning", Importance: 0.7},
	}

	context := extractor.BuildContextFromInfos(infos, 500)
	assert.Contains(t, context, "auth")
	assert.Contains(t, context, "JWT")
	assert.Contains(t, context, "database")
}

func TestKeyInfoExtractor_BuildContextFromInfos_Empty(t *testing.T) {
	extractor := NewKeyInfoExtractor(nil)

	context := extractor.BuildContextFromInfos(nil, 500)
	assert.Empty(t, context)
}

func TestSimilarity(t *testing.T) {
	assert.Equal(t, 1.0, similarity("hello", "hello"))
	assert.Greater(t, similarity("hello world", "hello"), 0.5)
	assert.Greater(t, similarity("read file", "read files"), 0.5)
	assert.Less(t, similarity("abc", "xyz"), 0.5)
}

func TestExtractJSONFromResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain JSON",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "JSON in markdown code block",
			input:    "```json\n{\"key\": \"value\"}\n```",
			expected: `{"key": "value"}`,
		},
		{
			name:     "JSON with surrounding text",
			input:    "Here is the result:\n{\"key\": \"value\"}\nDone.",
			expected: `{"key": "value"}`,
		},
		{
			name:     "no JSON",
			input:    "no json here",
			expected: "{}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSONFromResponse(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

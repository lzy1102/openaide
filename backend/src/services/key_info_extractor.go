package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"openaide/backend/src/services/llm"
)

type KeyInfo struct {
	Topic     string   `json:"topic"`
	Content   string   `json:"content"`
	Source    string   `json:"source"`
	Importance float64 `json:"importance"`
	Tags      []string `json:"tags"`
}

type ExtractionResult struct {
	KeyInfos     []KeyInfo `json:"key_infos"`
	Summary      string    `json:"summary"`
	ActionItems  []string  `json:"action_items"`
	Decisions    []string  `json:"decisions"`
	Risks        []string  `json:"risks"`
	Confidence   float64   `json:"confidence"`
}

type KeyInfoExtractor struct {
	modelSvc ModelCaller
}

func NewKeyInfoExtractor(modelSvc ModelCaller) *KeyInfoExtractor {
	return &KeyInfoExtractor{modelSvc: modelSvc}
}

func (e *KeyInfoExtractor) ExtractFromToolOutput(ctx context.Context, toolName, toolArgs, toolResult string, modelID string) (*ExtractionResult, error) {
	if toolResult == "" || len(toolResult) < 50 {
		return &ExtractionResult{Confidence: 0}, nil
	}

	if len(toolResult) > 5000 {
		toolResult = toolResult[:5000] + "\n... (truncated)"
	}

	prompt := fmt.Sprintf(`Analyze the following tool execution result and extract key information.

Tool: %s
Arguments: %s

Result:
%s

Extract and return a JSON object with this exact structure:
{
  "key_infos": [{"topic": "topic name", "content": "key finding", "source": "%s", "importance": 0.8, "tags": ["tag1"]}],
  "summary": "one-line summary of the most important finding",
  "action_items": ["action 1", "action 2"],
  "decisions": ["decision 1"],
  "risks": ["risk 1"],
  "confidence": 0.9
}

Rules:
- Only extract genuinely important information, not trivial details
- importance ranges from 0.0 to 1.0
- Keep summaries concise (under 100 chars)
- action_items: things that need to be done based on this result
- decisions: conclusions or choices implied by the result
- risks: potential problems or issues identified
- confidence: how confident you are in the extraction (0.0-1.0)
- Return valid JSON only, no markdown`, toolName, truncateForPrompt(toolArgs, 200), toolResult, toolName)

	llmMessages := []llm.Message{
		{Role: "system", Content: "You are a precise information extraction engine. Return only valid JSON."},
		{Role: "user", Content: prompt},
	}

	resp, err := e.modelSvc.Chat(modelID, llmMessages, nil)
	if err != nil {
		return nil, fmt.Errorf("key info extraction failed: %w", err)
	}

	content := ""
	if resp != nil && len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
	}

	var result ExtractionResult
	content = extractJSONFromResponse(content)
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return &ExtractionResult{
			Summary:    truncateForPrompt(content, 200),
			Confidence: 0.3,
		}, nil
	}

	return &result, nil
}

func (e *KeyInfoExtractor) ExtractFromConversation(ctx context.Context, messages []map[string]interface{}, modelID string) (*ExtractionResult, error) {
	var conversationParts []string
	for _, msg := range messages {
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)
		if content == "" {
			continue
		}
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		conversationParts = append(conversationParts, fmt.Sprintf("[%s]: %s", role, content))
	}

	if len(conversationParts) == 0 {
		return &ExtractionResult{Confidence: 0}, nil
	}

	conversationText := strings.Join(conversationParts, "\n\n")
	if len(conversationText) > 8000 {
		conversationText = conversationText[:8000] + "\n... (truncated)"
	}

	prompt := fmt.Sprintf(`Analyze the following conversation and extract key information.

%s

Extract and return a JSON object:
{
  "key_infos": [{"topic": "topic", "content": "key point", "source": "conversation", "importance": 0.8, "tags": ["tag"]}],
  "summary": "one-line summary",
  "action_items": ["action 1"],
  "decisions": ["decision 1"],
  "risks": ["risk 1"],
  "confidence": 0.9
}

Focus on:
- User's actual intent and requirements
- Technical decisions made
- Constraints and preferences mentioned
- Problems encountered and solutions proposed
- Open questions or unresolved issues

Return valid JSON only.`, conversationText)

	llmMessages := []llm.Message{
		{Role: "system", Content: "You are a precise information extraction engine. Return only valid JSON."},
		{Role: "user", Content: prompt},
	}

	resp, err := e.modelSvc.Chat(modelID, llmMessages, nil)
	if err != nil {
		return nil, fmt.Errorf("conversation extraction failed: %w", err)
	}

	var result ExtractionResult
	content := ""
	if resp != nil && len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
	}
	content = extractJSONFromResponse(content)
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return &ExtractionResult{
			Summary:    truncateForPrompt(content, 200),
			Confidence: 0.3,
		}, nil
	}

	return &result, nil
}

func (e *KeyInfoExtractor) DeduplicateInfos(existing, newInfos []KeyInfo) []KeyInfo {
	merged := make([]KeyInfo, 0, len(existing)+len(newInfos))
	merged = append(merged, existing...)

	for _, ni := range newInfos {
		isDuplicate := false
		for i, ei := range merged {
			if similarity(ei.Topic, ni.Topic) > 0.8 && similarity(ei.Content, ni.Content) > 0.7 {
				if ni.Importance > ei.Importance {
					merged[i] = ni
				}
				isDuplicate = true
				break
			}
		}
		if !isDuplicate {
			merged = append(merged, ni)
		}
	}

	return merged
}

func (e *KeyInfoExtractor) BuildContextFromInfos(infos []KeyInfo, maxTokens int) string {
	if len(infos) == 0 {
		return ""
	}

	var sections []string
	estimatedTokens := 0

	for _, info := range infos {
		entry := fmt.Sprintf("**%s**: %s", info.Topic, info.Content)
		if len(info.Tags) > 0 {
			entry += fmt.Sprintf(" [%s]", strings.Join(info.Tags, ", "))
		}
		entryTokens := len(entry) / 4
		if estimatedTokens+entryTokens > maxTokens {
			break
		}
		sections = append(sections, entry)
		estimatedTokens += entryTokens
	}

	return strings.Join(sections, "\n")
}

func similarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	a = strings.ToLower(a)
	b = strings.ToLower(b)
	if strings.Contains(a, b) || strings.Contains(b, a) {
		return 0.8
	}

	wordsA := strings.Fields(a)
	wordsB := strings.Fields(b)
	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0
	}

	common := 0
	for _, wa := range wordsA {
		for _, wb := range wordsB {
			if wa == wb {
				common++
				break
			}
		}
	}

	maxLen := len(wordsA)
	if len(wordsB) > maxLen {
		maxLen = len(wordsB)
	}
	return float64(common) / float64(maxLen)
}

func truncateForPrompt(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func extractJSONFromResponse(s string) string {
	s = strings.TrimSpace(s)

	if strings.HasPrefix(s, "```") {
		end := strings.Index(s[3:], "```")
		if end > 0 {
			s = s[3 : 3+end]
			if strings.HasPrefix(s, "json\n") {
				s = s[5:]
			}
		}
	}

	start := strings.Index(s, "{")
	if start < 0 {
		return "{}"
	}
	end := strings.LastIndex(s, "}")
	if end <= start {
		return "{}"
	}
	return s[start : end+1]
}

var _ = time.Duration(0)

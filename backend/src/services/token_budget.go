package services

import (
	"fmt"
	"math"
)

type TokenBudget struct {
	Total           int
	SystemPrompt    int
	Conversation    int
	RAGContext      int
	MemoryContext   int
	ToolDefinitions int
	Completion      int
	SafetyBuffer    int
}

type TokenBudgetConfig struct {
	MaxContextTokens  int
	CompletionReserve int
	SafetyBuffer      int
	SystemRatio       float64
	ConversationRatio float64
	RAGRatio          float64
	MemoryRatio       float64
	ToolRatio         float64
}

var DefaultBudgetConfig = TokenBudgetConfig{
	MaxContextTokens:  32000,
	CompletionReserve: 4096,
	SafetyBuffer:      512,
	SystemRatio:       0.15,
	ConversationRatio: 0.45,
	RAGRatio:          0.15,
	MemoryRatio:       0.10,
	ToolRatio:         0.15,
}

func CalculateTokenBudget(modelContextTokens int, toolCount int) TokenBudget {
	cfg := DefaultBudgetConfig
	if modelContextTokens > 0 {
		cfg.MaxContextTokens = modelContextTokens
	}

	available := cfg.MaxContextTokens - cfg.CompletionReserve - cfg.SafetyBuffer
	if available < 1000 {
		available = 1000
	}

	systemBudget := int(float64(available) * cfg.SystemRatio)
	conversationBudget := int(float64(available) * cfg.ConversationRatio)
	ragBudget := int(float64(available) * cfg.RAGRatio)
	memoryBudget := int(float64(available) * cfg.MemoryRatio)
	toolBudget := int(float64(available) * cfg.ToolRatio)

	allocated := systemBudget + conversationBudget + ragBudget + memoryBudget + toolBudget
	remaining := available - allocated
	if remaining > 0 {
		conversationBudget += remaining
	}

	if toolCount > 0 {
		estimatedToolTokens := toolCount * 150
		if estimatedToolTokens > toolBudget {
			overflow := estimatedToolTokens - toolBudget
			conversationBudget -= overflow
			toolBudget = estimatedToolTokens
		}
	}

	if conversationBudget < 1000 {
		conversationBudget = 1000
	}

	return TokenBudget{
		Total:           cfg.MaxContextTokens,
		SystemPrompt:    systemBudget,
		Conversation:    conversationBudget,
		RAGContext:      ragBudget,
		MemoryContext:   memoryBudget,
		ToolDefinitions: toolBudget,
		Completion:      cfg.CompletionReserve,
		SafetyBuffer:    cfg.SafetyBuffer,
	}
}

func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	cjkCount := 0
	latinCount := 0
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF || r >= 0x3040 && r <= 0x30FF || r >= 0xAC00 && r <= 0xD7AF {
			cjkCount++
		} else if r <= 127 {
			latinCount++
		} else {
			cjkCount++
		}
	}
	return int(math.Ceil(float64(cjkCount)*1.5 + float64(latinCount)*0.25))
}

func TruncateToTokenBudget(text string, maxTokens int) string {
	if text == "" || maxTokens <= 0 {
		return ""
	}
	estimated := EstimateTokens(text)
	if estimated <= maxTokens {
		return text
	}

	ratio := float64(maxTokens) / float64(estimated)
	targetChars := int(float64(len(text)) * ratio * 0.95)
	if targetChars < 0 {
		targetChars = 0
	}
	if targetChars >= len(text) {
		return text
	}
	return text[:targetChars] + "\n...[truncated]"
}

func (b TokenBudget) String() string {
	return fmt.Sprintf("TokenBudget(total=%d, system=%d, conversation=%d, rag=%d, memory=%d, tools=%d, completion=%d)",
		b.Total, b.SystemPrompt, b.Conversation, b.RAGContext, b.MemoryContext, b.ToolDefinitions, b.Completion)
}

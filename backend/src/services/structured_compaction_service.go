package services

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

const SUMMARIZATION_SYSTEM_PROMPT = `You are a conversation summarizer. Your task is to create a structured summary of the conversation that preserves all critical context needed to continue the work.

You MUST use this exact format:

## Goal
[What the user is trying to accomplish]

## Constraints & Preferences
- [Requirements mentioned by user]

## Progress
### Done
- [x] [Completed tasks]

### In Progress
- [ ] [Current work]

### Blocked
- [Issues, if any]

## Key Decisions
- **[Decision]**: [Rationale]

## Next Steps
1. [What should happen next]

## Critical Context
- [Data needed to continue, such as: IP addresses, ports, file paths, error messages, configuration values]

Rules:
1. Preserve ALL technical details: IP addresses, ports, file paths, error messages, config values
2. Preserve the reasoning behind key decisions
3. Track which files were read and which were modified
4. Be specific, not vague - "changed port from 8080 to 19375" not "changed some settings"
5. If there are unresolved issues, list them in Blocked
6. The summary should allow a fresh AI session to continue exactly where this one left off`

type CompactionDetails struct {
	ReadFiles     []string `json:"read_files"`
	ModifiedFiles []string `json:"modified_files"`
}

func (d CompactionDetails) Value() (driver.Value, error) {
	return json.Marshal(d)
}

func (d *CompactionDetails) Scan(value interface{}) error {
	if value == nil {
		*d = CompactionDetails{}
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, d)
}

type CompactionEntry struct {
	ID             string            `gorm:"primaryKey"`
	DialogueID     string            `gorm:"index"`
	Summary        string            `gorm:"type:text"`
	FirstKeptMsgID string
	TokensBefore   int
	TokensAfter    int
	Details        CompactionDetails `gorm:"type:json"`
	CreatedAt      time.Time
}

type CompactionResult struct {
	CompactionID   string
	Summary        string
	TokensBefore   int
	TokensAfter    int
	FirstKeptMsgID string
	CompactedCount int
	Details        CompactionDetails
}

type StructuredCompactionService struct {
	db               *gorm.DB
	modelCaller      ModelCaller
	dialogueStore    DialogueStore
	reserveTokens    int
	keepRecentTokens int
	enabled          bool
}

func NewStructuredCompactionService(db *gorm.DB, modelCaller ModelCaller, dialogueStore DialogueStore) *StructuredCompactionService {
	svc := &StructuredCompactionService{
		db:               db,
		modelCaller:      modelCaller,
		dialogueStore:    dialogueStore,
		reserveTokens:    16384,
		keepRecentTokens: 20000,
		enabled:          true,
	}
	if db != nil {
		db.AutoMigrate(&CompactionEntry{})
	}
	return svc
}

func (s *StructuredCompactionService) Compact(ctx context.Context, dialogueID string, customInstructions ...string) (*CompactionResult, error) {
	if !s.enabled {
		return nil, fmt.Errorf("compaction is disabled")
	}

	messages := s.dialogueStore.GetMessages(dialogueID)
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages found for dialogue %s", dialogueID)
	}

	estimator := NewTokenEstimator()
	contextTokens := 0
	for _, msg := range messages {
		contextTokens += estimator.EstimateTokens(msg.Content, "")
	}

	contextWindow := s.getContextWindow()
	if contextTokens <= contextWindow-s.reserveTokens {
		return nil, nil
	}

	var recentTokens int
	firstKeptIdx := len(messages)
	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := estimator.EstimateTokens(messages[i].Content, "")
		recentTokens += msgTokens
		if recentTokens >= s.keepRecentTokens {
			firstKeptIdx = i
			break
		}
	}

	if firstKeptIdx == 0 {
		firstKeptIdx = len(messages) / 2
	}

	if firstKeptIdx >= len(messages) {
		return nil, nil
	}

	messagesToCompact := messages[:firstKeptIdx]
	llmMessages := s.convertToLLMMessages(messagesToCompact)

	summary, err := s.generateSummary(ctx, llmMessages, customInstructions...)
	if err != nil {
		return nil, fmt.Errorf("failed to generate summary: %w", err)
	}

	fileOps := s.ExtractFileOperations(messagesToCompact)

	latestCompaction := s.GetLatestCompaction(dialogueID)
	if latestCompaction != nil {
		fileOps.ReadFiles = mergeUnique(latestCompaction.Details.ReadFiles, fileOps.ReadFiles)
		fileOps.ModifiedFiles = mergeUnique(latestCompaction.Details.ModifiedFiles, fileOps.ModifiedFiles)
	}

	summaryTokens := estimator.EstimateTokens(summary, "")
	remainingTokens := 0
	for i := firstKeptIdx; i < len(messages); i++ {
		remainingTokens += estimator.EstimateTokens(messages[i].Content, "")
	}
	tokensAfter := summaryTokens + remainingTokens

	entry := &CompactionEntry{
		ID:             uuid.New().String(),
		DialogueID:     dialogueID,
		Summary:        summary,
		FirstKeptMsgID: messages[firstKeptIdx].ID,
		TokensBefore:   contextTokens,
		TokensAfter:    tokensAfter,
		Details:        *fileOps,
		CreatedAt:      time.Now(),
	}

	if err := s.db.Create(entry).Error; err != nil {
		return nil, fmt.Errorf("failed to save compaction entry: %w", err)
	}

	slog.Info("Structured compaction completed",
		"dialogue_id", dialogueID,
		"tokens_before", contextTokens,
		"tokens_after", tokensAfter,
		"compacted_messages", len(messagesToCompact),
	)

	return &CompactionResult{
		CompactionID:   entry.ID,
		Summary:        summary,
		TokensBefore:   contextTokens,
		TokensAfter:    tokensAfter,
		FirstKeptMsgID: entry.FirstKeptMsgID,
		CompactedCount: len(messagesToCompact),
		Details:        entry.Details,
	}, nil
}

func (s *StructuredCompactionService) CompactMessages(ctx context.Context, messages []llm.Message, modelID string) ([]llm.Message, *llm.CompactInfo) {
	if !s.enabled || len(messages) <= 10 {
		return messages, nil
	}

	estimator := NewTokenEstimator()
	contextTokens := 0
	for _, msg := range messages {
		contextTokens += estimator.EstimateTokens(msg.Content, modelID)
	}

	contextWindow := s.getContextWindowForModel(modelID)
	if contextTokens <= contextWindow-s.reserveTokens {
		return messages, nil
	}

	var recentTokens int
	splitIdx := len(messages)
	for i := len(messages) - 1; i >= 0; i-- {
		recentTokens += estimator.EstimateTokens(messages[i].Content, modelID)
		if recentTokens >= s.keepRecentTokens {
			splitIdx = i
			break
		}
	}

	if splitIdx <= 0 {
		splitIdx = len(messages) / 2
	}

	if splitIdx >= len(messages) {
		return messages, nil
	}

	oldMessages := messages[:splitIdx]
	recentMessages := messages[splitIdx:]

	serialized := s.SerializeMessages(oldMessages)
	summary, err := s.generateSummaryFromText(ctx, serialized)
	if err != nil {
		slog.Error("Structured compaction failed, using messages as-is", "error", err)
		return messages, nil
	}

	result := []llm.Message{
		{Role: llm.RoleSystem, Content: fmt.Sprintf("[对话历史摘要]\n%s\n[摘要结束 - 从此处继续对话]", summary)},
	}

	for i, msg := range recentMessages {
		if msg.Role == llm.RoleTool {
			if i == 0 {
				continue
			}
			hasAssistantBefore := false
			for j := i - 1; j >= 0; j-- {
				if recentMessages[j].Role == llm.RoleAssistant && len(recentMessages[j].ToolCalls) > 0 {
					hasAssistantBefore = true
					break
				}
				if recentMessages[j].Role != llm.RoleTool {
					break
				}
			}
			if !hasAssistantBefore {
				continue
			}
		}
		result = append(result, msg)
	}

	newTokens := 0
	for _, msg := range result {
		newTokens += estimator.EstimateTokens(msg.Content, modelID)
	}

	info := &llm.CompactInfo{
		Reason:         "structured_compaction",
		BeforeMessages: len(messages),
		AfterMessages:  len(result),
		SavedTokens:    contextTokens - newTokens,
	}

	return result, info
}

func (s *StructuredCompactionService) GetLatestCompaction(dialogueID string) *CompactionEntry {
	var entry CompactionEntry
	if s.db == nil {
		return nil
	}
	if err := s.db.Where("dialogue_id = ?", dialogueID).Order("created_at DESC").First(&entry).Error; err != nil {
		return nil
	}
	return &entry
}

func (s *StructuredCompactionService) BuildCompactionContext(dialogueID string) string {
	entry := s.GetLatestCompaction(dialogueID)
	if entry == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(entry.Summary)

	if len(entry.Details.ReadFiles) > 0 {
		sb.WriteString("\n\n<read-files>\n")
		sb.WriteString(strings.Join(entry.Details.ReadFiles, "\n"))
		sb.WriteString("\n</read-files>")
	}

	if len(entry.Details.ModifiedFiles) > 0 {
		sb.WriteString("\n\n<modified-files>\n")
		sb.WriteString(strings.Join(entry.Details.ModifiedFiles, "\n"))
		sb.WriteString("\n</modified-files>")
	}

	return sb.String()
}

func (s *StructuredCompactionService) ExtractFileOperations(messages []models.Message) *CompactionDetails {
	details := &CompactionDetails{
		ReadFiles:     []string{},
		ModifiedFiles: []string{},
	}

	for _, msg := range messages {
		if msg.Sender != "assistant" {
			continue
		}

		content := msg.Content

		readFiles := extractFilePaths(content, []string{"read_file", "read-file", "cat ", "head ", "tail ", "less "})
		details.ReadFiles = append(details.ReadFiles, readFiles...)

		modifiedFiles := extractFilePaths(content, []string{"write_file", "write-file", "edit_file", "edit-file", "execute_code", "execute-code", "sed ", "patch "})
		details.ModifiedFiles = append(details.ModifiedFiles, modifiedFiles...)
	}

	details.ReadFiles = uniqueStrings(details.ReadFiles)
	details.ModifiedFiles = uniqueStrings(details.ModifiedFiles)

	return details
}

func (s *StructuredCompactionService) SerializeMessages(messages []llm.Message) string {
	var sb strings.Builder
	for _, msg := range messages {
		switch msg.Role {
		case llm.RoleUser:
			sb.WriteString(fmt.Sprintf("[User]: %s\n", msg.Content))
		case llm.RoleAssistant:
			content := msg.Content
			if len(content) > 2000 {
				content = content[:2000] + "..."
			}
			sb.WriteString(fmt.Sprintf("[Assistant]: %s\n", content))
			for _, tc := range msg.ToolCalls {
				sb.WriteString(fmt.Sprintf("[Tool Call]: %s(%s)\n", tc.Function.Name, compactionTruncate(tc.Function.Arguments, 500)))
			}
		case llm.RoleTool:
			content := compactionTruncate(msg.Content, 2000)
			sb.WriteString(fmt.Sprintf("[Tool result]: %s\n", content))
		}
	}
	return sb.String()
}

func (s *StructuredCompactionService) convertToLLMMessages(messages []models.Message) []llm.Message {
	result := make([]llm.Message, 0, len(messages))
	for _, msg := range messages {
		var role string
		switch msg.Sender {
		case "user":
			role = llm.RoleUser
		case "assistant":
			role = llm.RoleAssistant
		case "system":
			role = llm.RoleSystem
		case "tool":
			role = llm.RoleTool
		default:
			role = llm.RoleUser
		}
		result = append(result, llm.Message{
			Role:       role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		})
	}
	return result
}

func (s *StructuredCompactionService) generateSummary(ctx context.Context, messages []llm.Message, customInstructions ...string) (string, error) {
	serialized := s.SerializeMessages(messages)

	var promptContent string
	if len(customInstructions) > 0 && customInstructions[0] != "" {
		promptContent = fmt.Sprintf("Additional instructions: %s\n\nConversation to summarize:\n%s", customInstructions[0], serialized)
	} else {
		promptContent = fmt.Sprintf("Conversation to summarize:\n%s", serialized)
	}

	summaryModelID := s.findFastModel()

	resp, err := s.modelCaller.Chat(summaryModelID, []llm.Message{
		{Role: llm.RoleSystem, Content: SUMMARIZATION_SYSTEM_PROMPT},
		{Role: llm.RoleUser, Content: promptContent},
	}, map[string]interface{}{"max_tokens": float64(4000)})
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty response from LLM")
	}

	return resp.Choices[0].Message.Content, nil
}

func (s *StructuredCompactionService) generateSummaryFromText(ctx context.Context, text string) (string, error) {
	promptContent := fmt.Sprintf("Conversation to summarize:\n%s", text)

	summaryModelID := s.findFastModel()

	resp, err := s.modelCaller.Chat(summaryModelID, []llm.Message{
		{Role: llm.RoleSystem, Content: SUMMARIZATION_SYSTEM_PROMPT},
		{Role: llm.RoleUser, Content: promptContent},
	}, map[string]interface{}{"max_tokens": float64(4000)})
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty response from LLM")
	}

	return resp.Choices[0].Message.Content, nil
}

func (s *StructuredCompactionService) findFastModel() string {
	models, err := s.modelCaller.ListModels()
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

	return models[0].ID
}

func (s *StructuredCompactionService) getContextWindow() int {
	models, err := s.modelCaller.ListModels()
	if err != nil || len(models) == 0 {
		return 128000
	}

	model, err := s.modelCaller.GetModel(models[0].ID)
	if err != nil || model == nil {
		return 128000
	}

	if model.Config != nil {
		if cl, ok := model.Config["context_length"].(float64); ok && cl > 0 {
			return int(cl)
		}
	}

	return 128000
}

func (s *StructuredCompactionService) getContextWindowForModel(modelID string) int {
	model, err := s.modelCaller.GetModel(modelID)
	if err != nil || model == nil {
		return 128000
	}

	if model.Config != nil {
		if cl, ok := model.Config["context_length"].(float64); ok && cl > 0 {
			return int(cl)
		}
	}

	return 128000
}

func extractFilePaths(content string, prefixes []string) []string {
	var paths []string
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		for _, prefix := range prefixes {
			if strings.Contains(trimmed, prefix) {
				path := extractPathFromLine(trimmed, prefix)
				if path != "" {
					paths = append(paths, path)
				}
			}
		}
	}
	return paths
}

func extractPathFromLine(line, prefix string) string {
	idx := strings.Index(line, prefix)
	if idx < 0 {
		return ""
	}
	after := line[idx+len(prefix):]
	after = strings.TrimSpace(after)

	if strings.HasPrefix(after, "{") {
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(after), &args); err == nil {
			for _, key := range []string{"path", "file_path", "filename", "file"} {
				if path, ok := args[key].(string); ok && path != "" {
					return path
				}
			}
		}
	}

	for _, quote := range []string{"\"", "'", "`"} {
		if strings.HasPrefix(after, quote) {
			end := strings.Index(after[1:], quote)
			if end > 0 {
				return after[1 : end+1]
			}
		}
	}

	parts := strings.Fields(after)
	if len(parts) > 0 {
		path := parts[0]
		path = strings.TrimRight(path, ",;)")
		if len(path) > 1 && !strings.HasPrefix(path, "-") {
			return path
		}
	}

	return ""
}

func compactionTruncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func mergeUnique(a, b []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

func uniqueStrings(s []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(s))
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

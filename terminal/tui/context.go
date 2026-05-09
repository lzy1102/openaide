package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ContextBudget struct {
	MaxTokens       int `json:"max_tokens"`
	UsedTokens      int `json:"used_tokens"`
	SystemTokens    int `json:"system_tokens"`
	MemoryTokens    int `json:"memory_tokens"`
	ToolTokens      int `json:"tool_tokens"`
	ConversationTokens int `json:"conversation_tokens"`
	CompletionReserve  int `json:"completion_reserve"`
}

type CompactionRequest struct {
	DialogueID       string `json:"dialogue_id"`
	CustomInstructions string `json:"custom_instructions,omitempty"`
	Strategy         string `json:"strategy,omitempty"`
}

type CompactionResponse struct {
	CompactionID   string `json:"compaction_id"`
	Summary        string `json:"summary"`
	TokensBefore   int    `json:"tokens_before"`
	TokensAfter    int    `json:"tokens_after"`
	CompactedCount int    `json:"compacted_count"`
}

type Chapter struct {
	ID          int
	Title       string
	Summary     string
	StartMsgIdx int
	EndMsgIdx   int
	MsgCount    int
	Tokens      int
	KeyTopics   []string
	Decisions   []string
}

type ChapterOutline struct {
	Chapters    []Chapter
	TotalTokens int
	CreatedAt   time.Time
}

func GetContextBudget(apiURL, dialogueID string) (*ContextBudget, error) {
	reqBody := map[string]interface{}{
		"dialogue_id": dialogueID,
	}
	data, err := makeRequest("POST", apiURL+"/context/budget", reqBody)
	if err != nil {
		return nil, err
	}
	var apiResp APIResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, err
	}
	if apiResp.Data != nil {
		var budget ContextBudget
		if err := json.Unmarshal(apiResp.Data, &budget); err != nil {
			return nil, err
		}
		return &budget, nil
	}
	return nil, nil
}

func RequestCompaction(apiURL, dialogueID, strategy string) (*CompactionResponse, error) {
	reqBody := CompactionRequest{
		DialogueID: dialogueID,
		Strategy:   strategy,
	}
	data, err := makeRequest("POST", apiURL+"/context/compact", reqBody)
	if err != nil {
		return nil, err
	}
	var apiResp APIResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, err
	}
	if apiResp.Data != nil {
		var result CompactionResponse
		if err := json.Unmarshal(apiResp.Data, &result); err != nil {
			return nil, err
		}
		return &result, nil
	}
	return nil, nil
}

func RequestSummarization(apiURL, dialogueID string) (string, error) {
	reqBody := map[string]interface{}{
		"dialogue_id": dialogueID,
	}
	data, err := makeRequest("POST", apiURL+"/context/summarize", reqBody)
	if err != nil {
		return "", err
	}
	var apiResp APIResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return "", err
	}
	if apiResp.Data != nil {
		var result struct {
			Summary string `json:"summary"`
		}
		if err := json.Unmarshal(apiResp.Data, &result); err != nil {
			return "", err
		}
		return result.Summary, nil
	}
	return "", nil
}

func estimateTokensLocal(text string) int {
	if text == "" {
		return 0
	}
	cjkCount := 0
	latinCount := 0
	for _, r := range text {
		if (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3040 && r <= 0x30FF) || (r >= 0xAC00 && r <= 0xD7AF) {
			cjkCount++
		} else if r <= 127 {
			latinCount++
		} else {
			cjkCount++
		}
	}
	return int(float64(cjkCount)*1.5 + float64(latinCount)*0.25)
}

func estimateHistoryTokens(history []Message) int {
	total := 0
	for _, msg := range history {
		total += estimateTokensLocal(msg.Content)
	}
	return total
}

type CompactionStrategy int

const (
	StrategyNone          CompactionStrategy = iota
	StrategyTrimTools
	StrategyChapterOutline
	StrategySummarizeOld
	StrategyFullCompact
)

func DetermineCompactionStrategy(history []Message, maxTokens int) CompactionStrategy {
	if maxTokens <= 0 {
		maxTokens = 128000
	}

	totalTokens := estimateHistoryTokens(history)
	usagePercent := totalTokens * 100 / maxTokens

	if usagePercent < 60 {
		return StrategyNone
	}

	if usagePercent < 75 {
		return StrategyTrimTools
	}

	if usagePercent < 85 {
		return StrategyChapterOutline
	}

	if usagePercent < 95 {
		return StrategySummarizeOld
	}

	return StrategyFullCompact
}

func TrimToolResults(history []Message, maxResultLen int) []Message {
	result := make([]Message, len(history))
	copy(result, history)

	for i := range result {
		if result[i].Sender == "tool" && len(result[i].Content) > maxResultLen {
			truncated := result[i].Content[:maxResultLen]
			result[i].Content = truncated + fmt.Sprintf("\n...[truncated %d chars]", len(result[i].Content)-maxResultLen)
		}
	}
	return result
}

func detectTopicBoundary(msg Message, prevMsgs []Message) bool {
	if msg.Sender != "user" {
		return false
	}

	switch {
	case strings.HasPrefix(msg.Content, "/new"),
		strings.HasPrefix(msg.Content, "/mode"),
		strings.HasPrefix(msg.Content, "/cd "),
		strings.HasPrefix(msg.Content, "/sessions"):
		return true
	}

	shiftSignals := []string{
		"换个话题", "换个话题吧", "说点别的",
		"接下来", "然后", "现在",
		"帮我", "请帮我", "能不能帮我",
		"我想", "我要", "我需要",
		"new task", "switch to", "let's",
		"now", "next", "different",
	}
	contentLower := strings.ToLower(msg.Content)
	for _, sig := range shiftSignals {
		if strings.HasPrefix(contentLower, strings.ToLower(sig)) {
			if len(prevMsgs) >= 2 {
				return true
			}
		}
	}

	return false
}

func extractKeywords(text string, maxKeywords int) []string {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true,
		"have": true, "has": true, "had": true, "do": true,
		"does": true, "did": true, "will": true, "would": true,
		"could": true, "should": true, "may": true, "might": true,
		"can": true, "shall": true, "must": true, "need": true,
		"it": true, "its": true, "this": true, "that": true,
		"these": true, "those": true, "i": true, "you": true,
		"he": true, "she": true, "we": true, "they": true,
		"me": true, "him": true, "her": true, "us": true,
		"my": true, "your": true, "his": true, "our": true,
		"and": true, "or": true, "but": true, "not": true,
		"in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "from": true,
		"by": true, "as": true, "if": true, "so": true,
		"no": true, "yes": true, "ok": true,
		"的": true, "了": true, "在": true, "是": true,
		"我": true, "你": true, "他": true, "她": true,
		"这": true, "那": true, "有": true, "和": true,
		"就": true, "不": true, "也": true, "都": true,
		"要": true, "会": true, "可以": true, "能": true,
		"把": true, "被": true, "让": true, "给": true,
	}

	words := strings.Fields(text)
	freq := make(map[string]int)
	for _, w := range words {
		w = strings.Trim(strings.ToLower(w), ".,;:!?()[]{}\"'`")
		if len(w) < 2 || stopWords[w] {
			continue
		}
		freq[w]++
	}

	type kw struct {
		word string
		freq int
	}
	var keywords []kw
	for w, f := range freq {
		keywords = append(keywords, kw{w, f})
	}

	for i := 0; i < len(keywords); i++ {
		for j := i + 1; j < len(keywords); j++ {
			if keywords[j].freq > keywords[i].freq {
				keywords[i], keywords[j] = keywords[j], keywords[i]
			}
		}
	}

	result := make([]string, 0, maxKeywords)
	for i, kw := range keywords {
		if i >= maxKeywords {
			break
		}
		result = append(result, kw.word)
	}
	return result
}

func generateChapterTitle(msgs []Message, chapterIdx int) string {
	var firstUserMsg string
	for _, m := range msgs {
		if m.Sender == "user" {
			firstUserMsg = m.Content
			break
		}
	}

	if firstUserMsg == "" {
		return fmt.Sprintf("Chapter %d", chapterIdx)
	}

	title := firstUserMsg

	if idx := strings.Index(title, "\n"); idx > 0 {
		title = title[:idx]
	}

	prefixes := []string{"帮我", "请帮我", "能不能", "如何", "怎么", "为什么", "什么是",
		"help", "can you", "how to", "how do", "what is", "why",
		"show", "list", "create", "fix", "update", "add", "remove", "delete"}
	for _, p := range prefixes {
		if strings.HasPrefix(strings.ToLower(title), strings.ToLower(p)) {
			title = title[len(p):]
			break
		}
	}

	title = strings.TrimSpace(title)

	maxTitleLen := 30
	runes := []rune(title)
	if len(runes) > maxTitleLen {
		title = string(runes[:maxTitleLen]) + "..."
	}

	if title == "" {
		return fmt.Sprintf("Chapter %d", chapterIdx)
	}

	return title
}

func generateChapterSummary(msgs []Message) string {
	var userParts []string
	var asstParts []string
	var toolCount int
	var decisions []string

	for _, m := range msgs {
		switch m.Sender {
		case "user":
			userParts = append(userParts, truncateStr(m.Content, 80))
		case "assistant":
			content := m.Content
			if idx := strings.Index(content, "\n"); idx > 0 {
				firstLine := content[:idx]
				if len(firstLine) > 100 {
					firstLine = firstLine[:100]
				}
				asstParts = append(asstParts, firstLine)
			} else {
				asstParts = append(asstParts, truncateStr(content, 100))
			}

			decisionPatterns := []string{
				"决定", "选择", "采用", "使用", "方案",
				"decided", "chose", "using", "approach",
			}
			for _, pat := range decisionPatterns {
				if strings.Contains(strings.ToLower(content), strings.ToLower(pat)) {
					line := truncateStr(content, 120)
					decisions = append(decisions, line)
					break
				}
			}
		case "tool":
			toolCount++
		}
	}

	var sb strings.Builder

	if len(userParts) > 0 {
		sb.WriteString(fmt.Sprintf("用户: %s", userParts[0]))
		if len(userParts) > 1 {
			sb.WriteString(fmt.Sprintf(" (+%d more)", len(userParts)-1))
		}
	}

	if len(asstParts) > 0 {
		sb.WriteString(fmt.Sprintf(" → 助手: %s", asstParts[0]))
		if len(asstParts) > 1 {
			sb.WriteString(fmt.Sprintf(" (+%d more)", len(asstParts)-1))
		}
	}

	if toolCount > 0 {
		sb.WriteString(fmt.Sprintf(" [工具调用×%d]", toolCount))
	}

	if len(decisions) > 0 {
		sb.WriteString(fmt.Sprintf(" | 关键决策: %s", truncateStr(decisions[0], 80)))
	}

	return sb.String()
}

func SplitIntoChapters(history []Message) []Chapter {
	if len(history) == 0 {
		return nil
	}

	var chapters []Chapter
	chapterStart := 0
	chapterIdx := 1

	for i, msg := range history {
		isBoundary := false
		if i > chapterStart && msg.Sender == "user" {
			isBoundary = detectTopicBoundary(msg, history[:i])

			if !isBoundary && i-chapterStart >= 20 {
				hasUser := false
				for j := chapterStart; j < i; j++ {
					if history[j].Sender == "user" {
						hasUser = true
						break
					}
				}
				if hasUser {
					isBoundary = true
				}
			}
		}

		if isBoundary || i == len(history)-1 {
			endIdx := i
			if i == len(history)-1 && !isBoundary {
				endIdx = len(history)
			}

			chapterMsgs := history[chapterStart:endIdx]
			if len(chapterMsgs) > 0 {
				chTokens := 0
				for _, m := range chapterMsgs {
					chTokens += estimateTokensLocal(m.Content)
				}

				ch := Chapter{
					ID:          chapterIdx,
					Title:       generateChapterTitle(chapterMsgs, chapterIdx),
					Summary:     generateChapterSummary(chapterMsgs),
					StartMsgIdx: chapterStart,
					EndMsgIdx:   endIdx - 1,
					MsgCount:    len(chapterMsgs),
					Tokens:      chTokens,
					KeyTopics:   extractKeywords(strings.Join(func() []string {
						var parts []string
						for _, m := range chapterMsgs {
							if m.Sender == "user" || m.Sender == "assistant" {
								parts = append(parts, m.Content)
							}
						}
						return parts
					}(), " "), 5),
					Decisions: nil,
				}

				chapters = append(chapters, ch)
				chapterIdx++
			}
			chapterStart = i
		}
	}

	return chapters
}

func CompactWithChapterOutline(history []Message, keepRecentChapters int) []Message {
	chapters := SplitIntoChapters(history)
	if len(chapters) <= keepRecentChapters {
		return history
	}

	oldChapters := chapters[:len(chapters)-keepRecentChapters]
	recentChapters := chapters[len(chapters)-keepRecentChapters:]

	var sb strings.Builder
	sb.WriteString("[对话目录 — 像小说章节一样概览对话脉络]\n\n")

	for _, ch := range oldChapters {
		sb.WriteString(fmt.Sprintf("§%d %s\n", ch.ID, ch.Title))
		sb.WriteString(fmt.Sprintf("  摘要: %s\n", ch.Summary))
		if len(ch.KeyTopics) > 0 {
			sb.WriteString(fmt.Sprintf("  关键词: %s\n", strings.Join(ch.KeyTopics, ", ")))
		}
		sb.WriteString(fmt.Sprintf("  [%d条消息, ~%s]\n\n", ch.MsgCount, FormatTokenCount(ch.Tokens)))
	}

	sb.WriteString("[以上为历史章节摘要，以下是近期详细对话]\n")

	outlineMsg := Message{
		ID:         GenerateID(),
		DialogueID: history[0].DialogueID,
		Sender:     "system",
		Content:    sb.String(),
	}

	recentStart := recentChapters[0].StartMsgIdx
	recentMsgs := history[recentStart:]

	result := []Message{outlineMsg}
	result = append(result, recentMsgs...)
	return result
}

func SummarizeOldMessages(history []Message, keepRecent int) []Message {
	if len(history) <= keepRecent {
		return history
	}

	oldMessages := history[:len(history)-keepRecent]
	recentMessages := history[len(history)-keepRecent:]

	chapters := SplitIntoChapters(oldMessages)

	var sb strings.Builder
	if len(chapters) > 0 {
		sb.WriteString("[对话历史摘要]\n\n")
		for _, ch := range chapters {
			sb.WriteString(fmt.Sprintf("§%d %s\n", ch.ID, ch.Title))
			sb.WriteString(fmt.Sprintf("  %s\n\n", ch.Summary))
		}
	} else {
		sb.WriteString("[对话历史摘要]\n")
		for _, msg := range oldMessages {
			switch msg.Sender {
			case "user":
				sb.WriteString(fmt.Sprintf("用户: %s\n", truncateStr(msg.Content, 200)))
			case "assistant":
				sb.WriteString(fmt.Sprintf("助手: %s\n", truncateStr(msg.Content, 300)))
			case "tool":
				sb.WriteString(fmt.Sprintf("工具结果: %s\n", truncateStr(msg.Content, 100)))
			}
		}
	}
	sb.WriteString("[摘要结束]\n")

	summaryMsg := Message{
		ID:         GenerateID(),
		DialogueID: history[0].DialogueID,
		Sender:     "system",
		Content:    sb.String(),
	}

	result := []Message{summaryMsg}
	result = append(result, recentMessages...)
	return result
}

func RenderChapterOutline(outline *ChapterOutline) {
	if outline == nil || len(outline.Chapters) == 0 {
		return
	}

	fmt.Println()
	fmt.Printf("  %s\n\n", R.Bold.Render("📖 Chapter Outline"))

	for _, ch := range outline.Chapters {
		fmt.Printf("  %s %s\n",
			R.Accent.Render(fmt.Sprintf("§%d", ch.ID)),
			R.Bold.Render(ch.Title))
		fmt.Printf("  %s %s\n",
			R.Dim.Render("│"),
			R.Dim.Render(truncateStr(ch.Summary, 80)))
		if len(ch.KeyTopics) > 0 {
			fmt.Printf("  %s %s %s\n",
				R.Dim.Render("│"),
				R.Info.Render("topics:"),
				R.Dim.Render(strings.Join(ch.KeyTopics, ", ")))
		}
		fmt.Printf("  %s %s\n",
			R.Dim.Render("│"),
			R.Dim.Render(fmt.Sprintf("%d msgs · ~%s", ch.MsgCount, FormatTokenCount(ch.Tokens))))
	}

	fmt.Printf("\n  %s  %s\n",
		R.Dim.Render("Total:"),
		R.Accent.Render(fmt.Sprintf("%d chapters · ~%s", len(outline.Chapters), FormatTokenCount(outline.TotalTokens))))
}

func RenderContextBudget(budget *ContextBudget) {
	if budget == nil {
		return
	}

	fmt.Println()
	fmt.Printf("  %s\n\n", R.Bold.Render("Context Budget"))

	usagePercent := 0
	if budget.MaxTokens > 0 {
		usagePercent = budget.UsedTokens * 100 / budget.MaxTokens
	}

	barWidth := 30
	filled := usagePercent * barWidth / 100
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	var barStyle func(...string) string
	switch {
	case usagePercent > 90:
		barStyle = R.Error.Render
	case usagePercent > 75:
		barStyle = R.Warning.Render
	default:
		barStyle = R.Success.Render
	}

	fmt.Printf("  %s %d%%\n", barStyle(bar), usagePercent)

	items := []struct {
		label string
		value int
		style func(...string) string
	}{
		{"System", budget.SystemTokens, R.Dim.Render},
		{"Memory", budget.MemoryTokens, R.Info.Render},
		{"Tools", budget.ToolTokens, R.Dim.Render},
		{"Conversation", budget.ConversationTokens, R.Accent.Render},
		{"Reserve", budget.CompletionReserve, R.Dim.Render},
	}

	for _, item := range items {
		if item.value > 0 {
			pct := 0
			if budget.MaxTokens > 0 {
				pct = item.value * 100 / budget.MaxTokens
			}
			fmt.Printf("  %-14s %s\n",
				item.label+":",
				item.style(fmt.Sprintf("%d tokens (%d%%)", item.value, pct)))
		}
	}

	fmt.Printf("\n  %s / %s\n",
		R.Bold.Render(fmt.Sprintf("%d", budget.UsedTokens)),
		R.Dim.Render(fmt.Sprintf("%d tokens", budget.MaxTokens)))
}

func RenderCompactionResult(result *CompactionResponse) {
	if result == nil {
		return
	}

	saved := result.TokensBefore - result.TokensAfter
	savedPercent := 0
	if result.TokensBefore > 0 {
		savedPercent = saved * 100 / result.TokensBefore
	}

	fmt.Println()
	fmt.Printf("  %s\n", R.Bold.Render("Context Compacted"))
	fmt.Printf("  %s %s → %s (%s saved, %d%%)\n",
		R.Success.Render("✓"),
		R.Dim.Render(fmt.Sprintf("%d tokens", result.TokensBefore)),
		R.Accent.Render(fmt.Sprintf("%d tokens", result.TokensAfter)),
		R.Success.Render(fmt.Sprintf("%d", saved)),
		savedPercent)
	fmt.Printf("  %s %s\n",
		R.Dim.Render("Messages compacted:"),
		R.Accent.Render(fmt.Sprintf("%d", result.CompactedCount)))

	if result.Summary != "" {
		lines := strings.Split(result.Summary, "\n")
		preview := 5
		if len(lines) < preview {
			preview = len(lines)
		}
		fmt.Printf("  %s\n", R.Dim.Render("Summary:"))
		for i := 0; i < preview; i++ {
			fmt.Printf("  %s %s\n", R.Dim.Render("│"), R.Dim.Render(truncateStr(lines[i], 80)))
		}
		if len(lines) > preview {
			fmt.Printf("  %s %s\n", R.Dim.Render("│"), R.Dim.Render(fmt.Sprintf("… +%d more lines", len(lines)-preview)))
		}
	}
}

func RenderChapterCompaction(chapters []Chapter, oldTokens, newTokens int) {
	fmt.Println()
	fmt.Printf("  %s\n", R.Bold.Render("📖 Chapter Compaction"))
	fmt.Printf("  %s %s → %s (%s saved)\n",
		R.Success.Render("✓"),
		R.Dim.Render(fmt.Sprintf("%d tokens", oldTokens)),
		R.Accent.Render(fmt.Sprintf("%d tokens", newTokens)),
		R.Success.Render(fmt.Sprintf("%d", oldTokens-newTokens)))

	fmt.Printf("  %s\n", R.Dim.Render("Chapters compacted:"))
	for _, ch := range chapters {
		fmt.Printf("  %s %s\n",
			R.Accent.Render(fmt.Sprintf("§%d", ch.ID)),
			R.Dim.Render(truncateStr(ch.Title, 60)))
	}
}

func AutoCompactIfNeeded(apiURL, dialogueID string, history []Message, maxTokens int) (*[]Message, bool) {
	strategy := DetermineCompactionStrategy(history, maxTokens)

	switch strategy {
	case StrategyTrimTools:
		trimmed := TrimToolResults(history, 2000)
		oldTokens := estimateHistoryTokens(history)
		newTokens := estimateHistoryTokens(trimmed)
		if newTokens < oldTokens {
			RenderInfoLine(fmt.Sprintf("Context >60%%: trimmed tool results (%d → %d tokens)", oldTokens, newTokens))
			return &trimmed, true
		}

	case StrategyChapterOutline:
		chapters := SplitIntoChapters(history)
		if len(chapters) > 2 {
			trimmed := TrimToolResults(history, 1500)
			compacted := CompactWithChapterOutline(trimmed, 2)
			oldTokens := estimateHistoryTokens(history)
			newTokens := estimateHistoryTokens(compacted)
			if newTokens < oldTokens {
				RenderChapterCompaction(chapters[:len(chapters)-2], oldTokens, newTokens)
				return &compacted, true
			}
		}
		fallback := TrimToolResults(history, 1500)
		oldTokens := estimateHistoryTokens(history)
		newTokens := estimateHistoryTokens(fallback)
		if newTokens < oldTokens {
			RenderInfoLine(fmt.Sprintf("Context >85%%: trimmed tool results (%d → %d tokens)", oldTokens, newTokens))
			return &fallback, true
		}

	case StrategySummarizeOld:
		trimmed := TrimToolResults(history, 1000)
		summarized := SummarizeOldMessages(trimmed, 6)
		oldTokens := estimateHistoryTokens(history)
		newTokens := estimateHistoryTokens(summarized)
		if newTokens < oldTokens {
			RenderInfoLine(fmt.Sprintf("Context >90%%: summarized old messages (%d → %d tokens)", oldTokens, newTokens))
			return &summarized, true
		}

	case StrategyFullCompact:
		result, err := RequestCompaction(apiURL, dialogueID, "full")
		if err == nil && result != nil {
			RenderCompactionResult(result)
			return nil, true
		}
		trimmed := TrimToolResults(history, 500)
		summarized := SummarizeOldMessages(trimmed, 4)
		oldTokens := estimateHistoryTokens(history)
		newTokens := estimateHistoryTokens(summarized)
		if newTokens < oldTokens {
			RenderInfoLine(fmt.Sprintf("Context >95%%: emergency compression (%d → %d tokens)", oldTokens, newTokens))
			return &summarized, true
		}
	}

	return nil, false
}

func FormatTokenCount(tokens int) string {
	if tokens >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(tokens)/1000000)
	}
	if tokens >= 1000 {
		return fmt.Sprintf("%.1fk", float64(tokens)/1000)
	}
	return fmt.Sprintf("%d", tokens)
}

func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

type PatternDetectorService struct {
	db       *gorm.DB
	modelSvc *ModelService
	skillSvc *SkillService
	mu       sync.RWMutex
}

func NewPatternDetectorService(
	db *gorm.DB,
	modelSvc *ModelService,
	skillSvc *SkillService,
) *PatternDetectorService {
	return &PatternDetectorService{
		db:       db,
		modelSvc: modelSvc,
		skillSvc: skillSvc,
	}
}

func (s *PatternDetectorService) getLLMClient() (llm.LLMClient, error) {
	client := s.modelSvc.GetLLMClient()
	if client == nil {
		return nil, fmt.Errorf("no LLM client available")
	}
	return client, nil
}

func (s *PatternDetectorService) DetectPatterns(ctx context.Context, userID string) ([]models.RepetitivePattern, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var messages []models.Message
	since := time.Now().AddDate(0, 0, -7)
	dialogueQuery := s.db.Where("sender = ? AND created_at >= ?", "user", since)
	if userID != "" {
		var dialogues []models.Dialogue
		s.db.Where("user_id = ?", userID).Find(&dialogues)
		if len(dialogues) == 0 {
			return nil, nil
		}
		ids := make([]string, len(dialogues))
		for i, d := range dialogues {
			ids[i] = d.ID
		}
		dialogueQuery = s.db.Where("sender = ? AND dialogue_id IN ? AND created_at >= ?", "user", ids, since)
	}

	if err := dialogueQuery.Order("created_at ASC").Limit(500).Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	if len(messages) < 5 {
		return nil, nil
	}

	keywordPatterns := s.detectKeywordPatterns(messages, userID)
	topicPatterns := s.detectTopicPatterns(ctx, messages, userID)

	var allPatterns []models.RepetitivePattern
	allPatterns = append(allPatterns, keywordPatterns...)
	allPatterns = append(allPatterns, topicPatterns...)

	for i := range allPatterns {
		if allPatterns[i].ID == "" {
			allPatterns[i].ID = uuid.New().String()
			allPatterns[i].CreatedAt = time.Now()
			allPatterns[i].UpdatedAt = time.Now()
			s.db.Create(&allPatterns[i])
		}
	}

	return allPatterns, nil
}

func (s *PatternDetectorService) detectKeywordPatterns(messages []models.Message, userID string) []models.RepetitivePattern {
	var patterns []models.RepetitivePattern

	keywordCount := make(map[string]int)
	keywordSamples := make(map[string]string)
	keywordFirstSeen := make(map[string]time.Time)
	keywordLastSeen := make(map[string]time.Time)

	keywords := []string{
		"翻译", "translate",
		"总结", "summarize", "摘要",
		"代码", "code", "编程",
		"分析", "analyze",
		"写", "write",
		"改", "fix", "修改",
		"解释", "explain",
		"生成", "generate",
		"对比", "compare",
		"优化", "optimize",
		"测试", "test",
		"文档", "document",
		"日报", "周报", "报告", "report",
	}

	for _, msg := range messages {
		content := strings.ToLower(msg.Content)
		for _, kw := range keywords {
			if strings.Contains(content, kw) {
				keywordCount[kw]++
				if _, exists := keywordSamples[kw]; !exists || len(msg.Content) < len(keywordSamples[kw]) {
					keywordSamples[kw] = msg.Content
				}
				if _, exists := keywordFirstSeen[kw]; !exists || msg.CreatedAt.Before(keywordFirstSeen[kw]) {
					keywordFirstSeen[kw] = msg.CreatedAt
				}
				if _, exists := keywordLastSeen[kw]; !exists || msg.CreatedAt.After(keywordLastSeen[kw]) {
					keywordLastSeen[kw] = msg.CreatedAt
				}
			}
		}
	}

	for kw, count := range keywordCount {
		if count < 3 {
			continue
		}

		var existing models.RepetitivePattern
		err := s.db.Where("pattern = ? AND user_id = ?", kw, userID).First(&existing).Error
		if err == nil {
			s.db.Model(&existing).Updates(map[string]interface{}{
				"frequency":  count,
				"last_seen":  keywordLastSeen[kw],
				"updated_at": time.Now(),
			})
			patterns = append(patterns, existing)
			continue
		}

		suggestion := s.generatePatternSuggestion(kw, count)
		pattern := models.RepetitivePattern{
			UserID:      userID,
			PatternType: "query_pattern",
			Pattern:     kw,
			Frequency:   count,
			FirstSeen:   keywordFirstSeen[kw],
			LastSeen:    keywordLastSeen[kw],
			SampleQuery: truncate(keywordSamples[kw], 200),
			Suggestion:  suggestion,
			Status:      "detected",
		}
		patterns = append(patterns, pattern)
	}

	return patterns
}

func (s *PatternDetectorService) detectTopicPatterns(ctx context.Context, messages []models.Message, userID string) []models.RepetitivePattern {
	if len(messages) < 10 {
		return nil
	}

	llmClient, err := s.getLLMClient()
	if err != nil {
		log.Printf("[PatternDetector] no LLM client: %v", err)
		return nil
	}

	var sb strings.Builder
	for i, msg := range messages {
		if i >= 50 {
			break
		}
		sb.WriteString(fmt.Sprintf("- %s\n", truncate(msg.Content, 100)))
	}
	messagesSummary := sb.String()

	prompt := fmt.Sprintf(`分析以下用户消息列表，识别重复性的任务模式。

用户消息：
%s

请以 JSON 数组格式返回发现的重复模式（只返回 JSON，不要其他内容）：
[
  {
    "pattern": "模式描述",
    "frequency": 估计出现次数,
    "suggestion": "自动化建议（如：可以创建一个技能来自动处理此任务）"
  }
]

只返回出现3次以上的模式。如果没有发现重复模式，返回空数组 []`, messagesSummary)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个用户行为分析专家，擅长识别重复性任务模式。只返回 JSON 格式。"},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   500,
	}

	resp, err := llmClient.Chat(ctx, req)
	if err != nil {
		log.Printf("[PatternDetector] LLM analysis failed: %v", err)
		return nil
	}

	if len(resp.Choices) == 0 {
		return nil
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var results []struct {
		Pattern    string `json:"pattern"`
		Frequency  int    `json:"frequency"`
		Suggestion string `json:"suggestion"`
	}

	if err := json.Unmarshal([]byte(content), &results); err != nil {
		log.Printf("[PatternDetector] failed to parse LLM response: %v", err)
		return nil
	}

	var patterns []models.RepetitivePattern
	for _, r := range results {
		if r.Frequency < 3 {
			continue
		}

		var existing models.RepetitivePattern
		err := s.db.Where("pattern = ? AND user_id = ? AND pattern_type = ?", r.Pattern, userID, "topic_repetition").First(&existing).Error
		if err == nil {
			s.db.Model(&existing).Updates(map[string]interface{}{
				"frequency":  r.Frequency,
				"suggestion": r.Suggestion,
				"updated_at": time.Now(),
			})
			patterns = append(patterns, existing)
			continue
		}

		pattern := models.RepetitivePattern{
			UserID:      userID,
			PatternType: "topic_repetition",
			Pattern:     r.Pattern,
			Frequency:   r.Frequency,
			FirstSeen:   time.Now().AddDate(0, 0, -7),
			LastSeen:    time.Now(),
			Suggestion:  r.Suggestion,
			Status:      "detected",
		}
		patterns = append(patterns, pattern)
	}

	return patterns
}

func (s *PatternDetectorService) generatePatternSuggestion(keyword string, frequency int) string {
	suggestions := map[string]string{
		"翻译":       "可以创建一个翻译技能，自动检测输入语言并翻译",
		"translate": "可以创建一个翻译技能，自动检测输入语言并翻译",
		"总结":       "可以创建一个摘要技能，自动总结文本内容",
		"summarize": "可以创建一个摘要技能，自动总结文本内容",
		"摘要":       "可以创建一个摘要技能，自动总结文本内容",
		"代码":       "可以创建一个代码助手技能，提供代码生成和审查",
		"code":      "可以创建一个代码助手技能，提供代码生成和审查",
		"日报":       "可以创建一个日报生成技能，自动汇总工作内容",
		"周报":       "可以创建一个周报生成技能，自动汇总一周工作",
		"报告":       "可以创建一个报告生成技能，自动生成结构化报告",
		"优化":       "可以创建一个优化技能，自动分析并提供优化建议",
		"测试":       "可以创建一个测试技能，自动生成测试用例",
		"文档":       "可以创建一个文档技能，自动生成文档",
	}

	if suggestion, ok := suggestions[keyword]; ok {
		return suggestion
	}

	return fmt.Sprintf("检测到用户频繁执行「%s」操作（%d次），建议创建自动化技能来处理此任务", keyword, frequency)
}

func (s *PatternDetectorService) CreateSkillFromPattern(ctx context.Context, patternID string) (*models.Skill, error) {
	var pattern models.RepetitivePattern
	if err := s.db.Where("id = ?", patternID).First(&pattern).Error; err != nil {
		return nil, fmt.Errorf("pattern not found: %w", err)
	}

	llmClient, err := s.getLLMClient()
	if err != nil {
		return nil, err
	}

	prompt := fmt.Sprintf(`基于以下重复模式，生成一个技能定义：

模式类型：%s
模式描述：%s
出现频率：%d次
用户示例：%s
自动化建议：%s

请以 JSON 格式返回技能定义（只返回 JSON，不要其他内容）：
{
  "name": "技能名称（英文，下划线分隔）",
  "description": "技能描述",
  "category": "分类（如：productivity, coding, analysis, writing）",
  "triggers": ["触发关键词1", "触发关键词2"],
  "system_prompt_override": "技能执行时的系统提示词",
  "tools": ["需要的工具列表"]
}`, pattern.PatternType, pattern.Pattern, pattern.Frequency,
		pattern.SampleQuery, pattern.Suggestion)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个技能设计专家。根据用户行为模式自动设计技能。只返回 JSON 格式。"},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.5,
		MaxTokens:   500,
	}

	resp, err := llmClient.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("skill generation LLM call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from skill generation LLM")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var skillDef struct {
		Name                 string   `json:"name"`
		Description          string   `json:"description"`
		Category             string   `json:"category"`
		Triggers             []string `json:"triggers"`
		SystemPromptOverride string   `json:"system_prompt_override"`
		Tools                []string `json:"tools"`
	}

	if err := json.Unmarshal([]byte(content), &skillDef); err != nil {
		return nil, fmt.Errorf("failed to parse skill definition: %w", err)
	}

	triggersJSON, _ := json.Marshal(skillDef.Triggers)
	toolsJSON, _ := json.Marshal(skillDef.Tools)

	skill := &models.Skill{
		Name:                 skillDef.Name,
		Description:          skillDef.Description,
		Category:             skillDef.Category,
		Version:              "1.0.0",
		Author:               "auto-evolution",
		Enabled:              true,
		Triggers:             models.JSONSlice{},
		SystemPromptOverride: skillDef.SystemPromptOverride,
		Tools:                models.JSONSlice{},
		Builtin:              false,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}
	json.Unmarshal(triggersJSON, &skill.Triggers)
	json.Unmarshal(toolsJSON, &skill.Tools)

	if err := s.skillSvc.CreateSkill(skill); err != nil {
		return nil, fmt.Errorf("failed to create skill: %w", err)
	}

	s.db.Model(&pattern).Updates(map[string]interface{}{
		"status":     "skill_created",
		"updated_at": time.Now(),
	})

	return skill, nil
}

func (s *PatternDetectorService) GetPatterns(userID string, patternType string) ([]models.RepetitivePattern, error) {
	var patterns []models.RepetitivePattern
	query := s.db.Order("frequency DESC")
	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if patternType != "" {
		query = query.Where("pattern_type = ?", patternType)
	}
	err := query.Find(&patterns).Error
	return patterns, err
}

func (s *PatternDetectorService) IgnorePattern(patternID string) error {
	return s.db.Model(&models.RepetitivePattern{}).
		Where("id = ?", patternID).
		Updates(map[string]interface{}{
			"status":     "ignored",
			"updated_at": time.Now(),
		}).Error
}

func (s *PatternDetectorService) RunPeriodicDetection(ctx context.Context) {
	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.performPeriodicDetection(ctx)
		}
	}
}

func (s *PatternDetectorService) performPeriodicDetection(ctx context.Context) {
	var users []struct {
		UserID string
	}
	s.db.Table("dialogues").
		Select("DISTINCT user_id").
		Where("created_at >= ?", time.Now().AddDate(0, 0, -7)).
		Scan(&users)

	for _, u := range users {
		if u.UserID == "" {
			continue
		}
		patterns, err := s.DetectPatterns(ctx, u.UserID)
		if err != nil {
			log.Printf("[PatternDetector] detection failed for user %s: %v", u.UserID, err)
			continue
		}

		for _, p := range patterns {
			if p.Frequency >= 5 && p.Status == "detected" {
				log.Printf("[PatternDetector] High-frequency pattern detected: %s (%d times) for user %s",
					p.Pattern, p.Frequency, u.UserID)
			}
		}
	}
}

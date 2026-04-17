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

type CapabilityGapService struct {
	db       *gorm.DB
	modelSvc *ModelService
	skillSvc *SkillService
	mu       sync.RWMutex
}

func NewCapabilityGapService(
	db *gorm.DB,
	modelSvc *ModelService,
	skillSvc *SkillService,
) *CapabilityGapService {
	return &CapabilityGapService{
		db:       db,
		modelSvc: modelSvc,
		skillSvc: skillSvc,
	}
}

func (s *CapabilityGapService) getLLMClient() (llm.LLMClient, error) {
	client := s.modelSvc.GetLLMClient()
	if client == nil {
		return nil, fmt.Errorf("no LLM client available")
	}
	return client, nil
}

func (s *CapabilityGapService) DetectGaps(ctx context.Context, userID string) ([]models.CapabilityGap, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	llmClient, err := s.getLLMClient()
	if err != nil {
		return nil, err
	}

	var messages []models.Message
	since := time.Now().AddDate(0, 0, -7)

	msgQuery := s.db.Where("sender = ? AND created_at >= ?", "user", since)
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
		msgQuery = s.db.Where("sender = ? AND dialogue_id IN ? AND created_at >= ?", "user", ids, since)
	}

	if err := msgQuery.Order("created_at DESC").Limit(100).Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	if len(messages) < 5 {
		return nil, nil
	}

	var sb strings.Builder
	for i, msg := range messages {
		if i >= 30 {
			break
		}
		sb.WriteString(fmt.Sprintf("- %s\n", truncate(msg.Content, 100)))
	}
	messagesSummary := sb.String()

	var skills []models.Skill
	s.db.Where("enabled = ?", true).Find(&skills)
	var skillNames []string
	for _, sk := range skills {
		skillNames = append(skillNames, sk.Name+" ("+sk.Description+")")
	}
	skillsList := strings.Join(skillNames, ", ")
	if skillsList == "" {
		skillsList = "无"
	}

	prompt := fmt.Sprintf(`分析以下用户请求，识别系统当前能力无法很好满足的需求。

用户最近的请求：
%s

系统当前拥有的技能：%s

请以 JSON 数组格式返回能力缺口（只返回 JSON，不要其他内容）：
[
  {
    "gap_type": "missing_skill 或 weak_response 或 no_tool 或 knowledge_lack",
    "description": "能力缺口描述",
    "evidence": "证据（来自用户请求的具体内容）",
    "severity": "low 或 medium 或 high",
    "suggestion": "建议的解决方案"
  }
]

只返回真正存在的能力缺口。如果没有发现缺口，返回空数组 []`, messagesSummary, skillsList)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个 AI 系统能力分析专家，擅长识别系统的能力缺口。只返回 JSON 格式。"},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   500,
	}

	resp, err := llmClient.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gap detection LLM call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, nil
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var results []struct {
		GapType     string `json:"gap_type"`
		Description string `json:"description"`
		Evidence    string `json:"evidence"`
		Severity    string `json:"severity"`
		Suggestion  string `json:"suggestion"`
	}

	if err := json.Unmarshal([]byte(content), &results); err != nil {
		log.Printf("[CapabilityGap] failed to parse LLM response: %v", err)
		return nil, nil
	}

	var gaps []models.CapabilityGap
	for _, r := range results {
		var existing models.CapabilityGap
		err := s.db.Where("description LIKE ? AND status = ?", "%"+r.Description+"%", "detected").First(&existing).Error
		if err == nil {
			s.db.Model(&existing).Updates(map[string]interface{}{
				"frequency":  existing.Frequency + 1,
				"updated_at": time.Now(),
			})
			gaps = append(gaps, existing)
			continue
		}

		gap := models.CapabilityGap{
			ID:          uuid.New().String(),
			UserID:      userID,
			GapType:     r.GapType,
			Description: r.Description,
			Evidence:    r.Evidence,
			Frequency:   1,
			Severity:    r.Severity,
			Suggestion:  r.Suggestion,
			Status:      "detected",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		s.db.Create(&gap)
		gaps = append(gaps, gap)
	}

	return gaps, nil
}

func (s *CapabilityGapService) CreateSkillFromGap(ctx context.Context, gapID string) (*models.Skill, error) {
	var gap models.CapabilityGap
	if err := s.db.Where("id = ?", gapID).First(&gap).Error; err != nil {
		return nil, fmt.Errorf("gap not found: %w", err)
	}

	llmClient, err := s.getLLMClient()
	if err != nil {
		return nil, err
	}

	prompt := fmt.Sprintf(`基于以下能力缺口，生成一个新技能定义来弥补这个缺口。

缺口类型：%s
缺口描述：%s
证据：%s
建议方案：%s

请以 JSON 格式返回技能定义（只返回 JSON，不要其他内容）：
{
  "name": "技能名称（英文，下划线分隔）",
  "description": "技能描述",
  "category": "分类",
  "triggers": ["触发关键词1", "触发关键词2"],
  "system_prompt_override": "技能执行时的系统提示词",
  "tools": ["需要的工具列表"]
}`, gap.GapType, gap.Description, gap.Evidence, gap.Suggestion)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个技能设计专家。根据能力缺口自动设计新技能。只返回 JSON 格式。"},
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

	s.db.Model(&gap).Updates(map[string]interface{}{
		"status":     "skill_created",
		"updated_at": time.Now(),
	})

	return skill, nil
}

func (s *CapabilityGapService) GetGaps(gapType string, severity string) ([]models.CapabilityGap, error) {
	var gaps []models.CapabilityGap
	query := s.db.Order("frequency DESC, created_at DESC")
	if gapType != "" {
		query = query.Where("gap_type = ?", gapType)
	}
	if severity != "" {
		query = query.Where("severity = ?", severity)
	}
	err := query.Find(&gaps).Error
	return gaps, err
}

func (s *CapabilityGapService) IgnoreGap(gapID string) error {
	return s.db.Model(&models.CapabilityGap{}).
		Where("id = ?", gapID).
		Updates(map[string]interface{}{
			"status":     "ignored",
			"updated_at": time.Now(),
		}).Error
}

func (s *CapabilityGapService) RunPeriodicGapDetection(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
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

func (s *CapabilityGapService) performPeriodicDetection(ctx context.Context) {
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
		gaps, err := s.DetectGaps(ctx, u.UserID)
		if err != nil {
			log.Printf("[CapabilityGap] detection failed for user %s: %v", u.UserID, err)
			continue
		}

		for _, g := range gaps {
			if g.Severity == "high" && g.Frequency >= 3 {
				log.Printf("[CapabilityGap] High-severity gap detected: %s (%d times) for user %s",
					g.Description, g.Frequency, u.UserID)
			}
		}
	}
}

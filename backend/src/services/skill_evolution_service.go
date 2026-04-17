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

type SkillEvolutionService struct {
	db       *gorm.DB
	modelSvc *ModelService
	skillSvc *SkillService
	mu       sync.RWMutex
}

func NewSkillEvolutionService(
	db *gorm.DB,
	modelSvc *ModelService,
	skillSvc *SkillService,
) *SkillEvolutionService {
	return &SkillEvolutionService{
		db:       db,
		modelSvc: modelSvc,
		skillSvc: skillSvc,
	}
}

func (s *SkillEvolutionService) getLLMClient() (llm.LLMClient, error) {
	client := s.modelSvc.GetLLMClient()
	if client == nil {
		return nil, fmt.Errorf("no LLM client available")
	}
	return client, nil
}

func (s *SkillEvolutionService) EvolveSkillFromFeedback(ctx context.Context, skillID string) (*models.SkillEvolution, error) {
	var executions []models.SkillExecution
	if err := s.db.Where("skill_id = ? AND status IN ?", skillID, []string{"completed", "failed"}).
		Order("started_at DESC").Limit(20).Find(&executions).Error; err != nil {
		return nil, fmt.Errorf("failed to get executions: %w", err)
	}

	if len(executions) < 3 {
		return nil, nil
	}

	failedCount := 0
	for _, exec := range executions {
		if exec.Status == "failed" {
			failedCount++
		}
	}
	failureRate := float64(failedCount) / float64(len(executions))

	if failureRate < 0.2 {
		return nil, nil
	}

	var skill models.Skill
	if err := s.db.Where("id = ?", skillID).First(&skill).Error; err != nil {
		return nil, fmt.Errorf("skill not found: %w", err)
	}

	var errorMessages []string
	for _, exec := range executions {
		if exec.Status == "failed" && exec.Error != "" {
			errorMessages = append(errorMessages, exec.Error)
		}
	}

	evolution, err := s.generateSkillEvolution(ctx, &skill, failureRate, errorMessages, "feedback")
	if err != nil {
		return nil, err
	}

	return evolution, nil
}

func (s *SkillEvolutionService) EvolveSkillFromReflection(ctx context.Context, skillID string, reflection *models.SelfReflection) (*models.SkillEvolution, error) {
	var skill models.Skill
	if err := s.db.Where("id = ?", skillID).First(&skill).Error; err != nil {
		return nil, fmt.Errorf("skill not found: %w", err)
	}

	evolution, err := s.generateSkillEvolution(ctx, &skill, 1.0-reflection.QualityScore,
		[]string{reflection.Issues}, "reflection")
	if err != nil {
		return nil, err
	}

	return evolution, nil
}

func (s *SkillEvolutionService) generateSkillEvolution(ctx context.Context, skill *models.Skill, problemRate float64, issues []string, trigger string) (*models.SkillEvolution, error) {
	llmClient, err := s.getLLMClient()
	if err != nil {
		return nil, err
	}

	prompt := fmt.Sprintf(`分析以下技能的问题并生成改进方案。

技能名称：%s
技能描述：%s
当前触发词：%v
当前系统提示词：%s
问题发生率：%.1f%%
具体问题：%v

请以 JSON 格式返回改进方案（只返回 JSON，不要其他内容）：
{
  "change_type": "trigger_optimization 或 prompt_improvement 或 parameter_adjustment",
  "change_desc": "变更描述",
  "new_triggers": ["新的触发词列表（如果是 trigger_optimization）"],
  "new_system_prompt": "新的系统提示词（如果是 prompt_improvement）",
  "confidence": 0.0到1.0的置信度
}`, skill.Name, skill.Description, skill.Triggers,
		skill.SystemPromptOverride, problemRate*100, issues)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个技能优化专家。分析技能问题并生成具体的改进方案。只返回 JSON 格式。"},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.5,
		MaxTokens:   500,
	}

	resp, err := llmClient.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("evolution LLM call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from evolution LLM")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var result struct {
		ChangeType      string   `json:"change_type"`
		ChangeDesc      string   `json:"change_desc"`
		NewTriggers     []string `json:"new_triggers"`
		NewSystemPrompt string   `json:"new_system_prompt"`
		Confidence      float64  `json:"confidence"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse evolution result: %w", err)
	}

	beforeState, _ := json.Marshal(map[string]interface{}{
		"triggers":       skill.Triggers,
		"system_prompt":  skill.SystemPromptOverride,
	})

	var afterStateMap map[string]interface{}
	switch result.ChangeType {
	case "trigger_optimization":
		afterStateMap = map[string]interface{}{
			"triggers": result.NewTriggers,
		}
	case "prompt_improvement":
		afterStateMap = map[string]interface{}{
			"system_prompt": result.NewSystemPrompt,
		}
	default:
		afterStateMap = map[string]interface{}{
			"change_desc": result.ChangeDesc,
		}
	}
	afterState, _ := json.Marshal(afterStateMap)

	newVersion := incrementVersion(skill.Version)

	evolution := &models.SkillEvolution{
		ID:          uuid.New().String(),
		SkillID:     skill.ID,
		VersionFrom: skill.Version,
		VersionTo:   newVersion,
		ChangeType:  result.ChangeType,
		ChangeDesc:  result.ChangeDesc,
		BeforeState: string(beforeState),
		AfterState:  string(afterState),
		Trigger:     trigger,
		Confidence:  result.Confidence,
		Status:      "pending",
		CreatedAt:   time.Now(),
	}

	if err := s.db.Create(evolution).Error; err != nil {
		return nil, fmt.Errorf("failed to save evolution: %w", err)
	}

	if result.Confidence > 0.8 {
		if err := s.ApplyEvolution(evolution.ID); err != nil {
			log.Printf("[SkillEvolution] auto-apply failed: %v", err)
		}
	}

	return evolution, nil
}

func (s *SkillEvolutionService) ApplyEvolution(evolutionID string) error {
	var evolution models.SkillEvolution
	if err := s.db.Where("id = ?", evolutionID).First(&evolution).Error; err != nil {
		return fmt.Errorf("evolution not found: %w", err)
	}

	var skill models.Skill
	if err := s.db.Where("id = ?", evolution.SkillID).First(&skill).Error; err != nil {
		return fmt.Errorf("skill not found: %w", err)
	}

	var afterState map[string]interface{}
	if err := json.Unmarshal([]byte(evolution.AfterState), &afterState); err != nil {
		return fmt.Errorf("failed to parse after state: %w", err)
	}

	switch evolution.ChangeType {
	case "trigger_optimization":
		if newTriggers, ok := afterState["triggers"].([]interface{}); ok {
			triggersJSON, _ := json.Marshal(newTriggers)
			var triggers models.JSONSlice
			json.Unmarshal(triggersJSON, &triggers)
			skill.Triggers = triggers
		}
	case "prompt_improvement":
		if newPrompt, ok := afterState["system_prompt"].(string); ok {
			skill.SystemPromptOverride = newPrompt
		}
	}

	skill.Version = evolution.VersionTo
	skill.UpdatedAt = time.Now()

	if err := s.skillSvc.UpdateSkill(&skill); err != nil {
		return fmt.Errorf("failed to update skill: %w", err)
	}

	now := time.Now()
	return s.db.Model(&evolution).Updates(map[string]interface{}{
		"status":     "applied",
		"applied_at": now,
	}).Error
}

func (s *SkillEvolutionService) RollbackEvolution(evolutionID string) error {
	var evolution models.SkillEvolution
	if err := s.db.Where("id = ?", evolutionID).First(&evolution).Error; err != nil {
		return fmt.Errorf("evolution not found: %w", err)
	}

	var skill models.Skill
	if err := s.db.Where("id = ?", evolution.SkillID).First(&skill).Error; err != nil {
		return fmt.Errorf("skill not found: %w", err)
	}

	var beforeState map[string]interface{}
	if err := json.Unmarshal([]byte(evolution.BeforeState), &beforeState); err != nil {
		return fmt.Errorf("failed to parse before state: %w", err)
	}

	switch evolution.ChangeType {
	case "trigger_optimization":
		if oldTriggers, ok := beforeState["triggers"].([]interface{}); ok {
			triggersJSON, _ := json.Marshal(oldTriggers)
			var triggers models.JSONSlice
			json.Unmarshal(triggersJSON, &triggers)
			skill.Triggers = triggers
		}
	case "prompt_improvement":
		if oldPrompt, ok := beforeState["system_prompt"].(string); ok {
			skill.SystemPromptOverride = oldPrompt
		}
	}

	skill.Version = evolution.VersionFrom
	skill.UpdatedAt = time.Now()
	s.skillSvc.UpdateSkill(&skill)

	return s.db.Model(&evolution).Updates(map[string]interface{}{
		"status": "rolled_back",
	}).Error
}

func (s *SkillEvolutionService) GetEvolutionHistory(skillID string) ([]models.SkillEvolution, error) {
	var evolutions []models.SkillEvolution
	err := s.db.Where("skill_id = ?", skillID).Order("created_at DESC").Find(&evolutions).Error
	return evolutions, err
}

func (s *SkillEvolutionService) GetPendingEvolutions() ([]models.SkillEvolution, error) {
	var evolutions []models.SkillEvolution
	err := s.db.Where("status = ?", "pending").Order("created_at DESC").Find(&evolutions).Error
	return evolutions, err
}

func (s *SkillEvolutionService) RunPeriodicEvolution(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.performPeriodicEvolution(ctx)
		}
	}
}

func (s *SkillEvolutionService) performPeriodicEvolution(ctx context.Context) {
	var skills []models.Skill
	s.db.Where("enabled = ? AND builtin = ?", true, false).Find(&skills)

	for _, skill := range skills {
		evolution, err := s.EvolveSkillFromFeedback(ctx, skill.ID)
		if err != nil {
			log.Printf("[SkillEvolution] evolution failed for skill %s: %v", skill.ID, err)
			continue
		}
		if evolution != nil {
			log.Printf("[SkillEvolution] evolution generated for skill %s: %s (%.2f confidence)",
				skill.Name, evolution.ChangeDesc, evolution.Confidence)
		}
	}
}

func incrementVersion(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return "1.0.1"
	}
	minor := 0
	fmt.Sscanf(parts[2], "%d", &minor)
	parts[2] = fmt.Sprintf("%d", minor+1)
	return strings.Join(parts, ".")
}

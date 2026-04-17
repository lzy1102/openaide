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

type SelfReflectionService struct {
	db        *gorm.DB
	modelSvc  *ModelService
	skillSvc  *SkillService
	mu        sync.RWMutex
	promptSvc *PromptService
}

func NewSelfReflectionService(
	db *gorm.DB,
	modelSvc *ModelService,
	skillSvc *SkillService,
	promptSvc *PromptService,
) *SelfReflectionService {
	return &SelfReflectionService{
		db:        db,
		modelSvc:  modelSvc,
		skillSvc:  skillSvc,
		promptSvc: promptSvc,
	}
}

func (s *SelfReflectionService) getLLMClient() (llm.LLMClient, error) {
	client := s.modelSvc.GetLLMClient()
	if client == nil {
		return nil, fmt.Errorf("no LLM client available")
	}
	return client, nil
}

func (s *SelfReflectionService) ReflectOnResponse(ctx context.Context, dialogueID, userID, query, response string) (*models.SelfReflection, error) {
	llmClient, err := s.getLLMClient()
	if err != nil {
		return nil, err
	}

	prompt := fmt.Sprintf(`你是一个 AI 自我反思专家。请评估以下 AI 回复的质量，并给出改进建议。

用户问题：%s

AI 回复：%s

请以 JSON 格式返回（只返回 JSON，不要其他内容）：
{
  "quality_score": 0.0到1.0的评分,
  "issues": "发现的问题（如果没有问题则为空字符串）",
  "improvements": "具体的改进建议",
  "confidence": 0.0到1.0的置信度
}

评估维度：
1. 准确性：回复是否准确回答了用户问题
2. 完整性：回复是否完整覆盖了用户需求
3. 清晰度：回复是否清晰易懂
4. 实用性：回复是否对用户有实际帮助
5. 适当性：回复风格和深度是否适合用户`, query, response)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个 AI 质量评估专家，擅长客观评估 AI 回复质量并给出改进建议。只返回 JSON 格式。"},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   500,
	}

	resp, err := llmClient.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("reflection LLM call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from reflection LLM")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var result struct {
		QualityScore float64 `json:"quality_score"`
		Issues       string  `json:"issues"`
		Improvements string  `json:"improvements"`
		Confidence   float64 `json:"confidence"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		result.QualityScore = 0.5
		result.Issues = "无法解析反思结果"
		result.Improvements = content
		result.Confidence = 0.3
	}

	reflection := &models.SelfReflection{
		ID:           uuid.New().String(),
		DialogueID:   dialogueID,
		UserID:       userID,
		Query:        query,
		Response:     response,
		QualityScore: result.QualityScore,
		Issues:       result.Issues,
		Improvements: result.Improvements,
		Confidence:   result.Confidence,
		Status:       "pending",
		CreatedAt:    time.Now(),
	}

	if err := s.db.Create(reflection).Error; err != nil {
		return nil, fmt.Errorf("failed to save reflection: %w", err)
	}

	if result.QualityScore < 0.5 && result.Confidence > 0.6 {
		go s.triggerImprovementActions(reflection)
	}

	return reflection, nil
}

func (s *SelfReflectionService) triggerImprovementActions(reflection *models.SelfReflection) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if reflection.Improvements != "" {
		now := time.Now()
		optimization := &models.PromptOptimization{
			ID:          uuid.New().String(),
			DialogueID:  reflection.DialogueID,
			TaskType:    "self_reflection",
			Suggestions: reflection.Improvements,
			Status:      "applied",
			Confidence:  reflection.Confidence,
			AppliedAt:   &now,
			CreatedAt:   now,
		}
		if err := s.db.Create(optimization).Error; err != nil {
			log.Printf("[SelfReflection] failed to save optimization: %v", err)
		}
	}

	if reflection.QualityScore < 0.3 {
		s.detectCapabilityGap(ctx, reflection)
	}
}

func (s *SelfReflectionService) detectCapabilityGap(ctx context.Context, reflection *models.SelfReflection) {
	var existing models.CapabilityGap
	err := s.db.Where("description LIKE ? AND status = ?", "%"+reflection.Issues+"%", "detected").
		First(&existing).Error
	if err == nil {
		s.db.Model(&existing).Updates(map[string]interface{}{
			"frequency":  existing.Frequency + 1,
			"updated_at": time.Now(),
		})
		return
	}

	gap := &models.CapabilityGap{
		ID:          uuid.New().String(),
		UserID:      reflection.UserID,
		GapType:     "weak_response",
		Description: reflection.Issues,
		Evidence:    fmt.Sprintf("Quality score: %.2f. Query: %s", reflection.QualityScore, truncate(reflection.Query, 200)),
		Frequency:   1,
		Severity:    "high",
		Suggestion:  reflection.Improvements,
		Status:      "detected",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	s.db.Create(gap)
}

func (s *SelfReflectionService) GetRecentReflections(limit int) ([]models.SelfReflection, error) {
	var reflections []models.SelfReflection
	err := s.db.Order("created_at DESC").Limit(limit).Find(&reflections).Error
	return reflections, err
}

func (s *SelfReflectionService) GetReflectionsByDialogue(dialogueID string) ([]models.SelfReflection, error) {
	var reflections []models.SelfReflection
	err := s.db.Where("dialogue_id = ?", dialogueID).Order("created_at DESC").Find(&reflections).Error
	return reflections, err
}

func (s *SelfReflectionService) GetQualityTrend(days int) ([]QualityTrendPoint, error) {
	since := time.Now().AddDate(0, 0, -days)

	var reflections []models.SelfReflection
	if err := s.db.Where("created_at >= ?", since).Order("created_at ASC").Find(&reflections).Error; err != nil {
		return nil, err
	}

	dailyMap := make(map[string][]float64)
	for _, r := range reflections {
		day := r.CreatedAt.Format("2006-01-02")
		dailyMap[day] = append(dailyMap[day], r.QualityScore)
	}

	var trend []QualityTrendPoint
	for day, scores := range dailyMap {
		avg := 0.0
		for _, sc := range scores {
			avg += sc
		}
		avg /= float64(len(scores))
		trend = append(trend, QualityTrendPoint{
			Date:  day,
			Score: avg,
			Count: len(scores),
		})
	}

	return trend, nil
}

type QualityTrendPoint struct {
	Date  string  `json:"date"`
	Score float64 `json:"score"`
	Count int     `json:"count"`
}

func (s *SelfReflectionService) ApplyReflection(reflectionID string) error {
	return s.db.Model(&models.SelfReflection{}).
		Where("id = ?", reflectionID).
		Updates(map[string]interface{}{
			"status":     "applied",
			"updated_at": time.Now(),
		}).Error
}

func (s *SelfReflectionService) RunPeriodicReflection(ctx context.Context) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.performPeriodicReflection(ctx)
		}
	}
}

func (s *SelfReflectionService) performPeriodicReflection(ctx context.Context) {
	var recentReflections []models.SelfReflection
	since := time.Now().Add(-6 * time.Hour)
	s.db.Where("created_at >= ? AND quality_score < 0.5", since).Find(&recentReflections)

	if len(recentReflections) < 3 {
		return
	}

	lowScoreCount := len(recentReflections)
	avgScore := 0.0
	issueMap := make(map[string]int)
	for _, r := range recentReflections {
		avgScore += r.QualityScore
		if r.Issues != "" {
			issueMap[r.Issues]++
		}
	}
	avgScore /= float64(lowScoreCount)

	var topIssue string
	topCount := 0
	for issue, count := range issueMap {
		if count > topCount {
			topIssue = issue
			topCount = count
		}
	}

	log.Printf("[SelfReflection] Periodic: %d low-quality responses in 6h, avg score %.2f, top issue: %s",
		lowScoreCount, avgScore, topIssue)

	if topCount >= 3 && topIssue != "" {
		s.generateSystemicImprovement(ctx, topIssue, lowScoreCount, avgScore)
	}
}

func (s *SelfReflectionService) generateSystemicImprovement(ctx context.Context, issue string, count int, avgScore float64) {
	llmClient, err := s.getLLMClient()
	if err != nil {
		log.Printf("[SelfReflection] no LLM client for systemic improvement: %v", err)
		return
	}

	prompt := fmt.Sprintf(`系统在过去6小时内有%d次低质量回复，平均质量分%.2f。
最常见的问题：%s

请给出一个系统级的改进建议，可以改善整体回复质量。建议应该是一个具体的系统提示词补充或行为规则。
直接输出改进建议文本（不超过200字）。`, count, avgScore, issue)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个 AI 系统优化专家。根据问题模式给出系统级改进建议。"},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.5,
		MaxTokens:   300,
	}

	resp, err := llmClient.Chat(ctx, req)
	if err != nil {
		log.Printf("[SelfReflection] systemic improvement generation failed: %v", err)
		return
	}

	if len(resp.Choices) > 0 && resp.Choices[0].Message.Content != "" {
		now := time.Now()
		optimization := &models.PromptOptimization{
			ID:          uuid.New().String(),
			TaskType:    "systemic_reflection",
			Suggestions: resp.Choices[0].Message.Content,
			Status:      "applied",
			Confidence:  0.85,
			AppliedAt:   &now,
			CreatedAt:   now,
		}
		s.db.Create(optimization)
		log.Printf("[SelfReflection] Applied systemic improvement: %s", resp.Choices[0].Message.Content)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

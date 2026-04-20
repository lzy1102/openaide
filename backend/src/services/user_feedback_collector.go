package services

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// UserFeedbackCollector 用户满意度收集器
type UserFeedbackCollector struct {
	db            *gorm.DB
	modelSvc      *ModelService
	skillSvc      *SkillService
	memorySvc     *MemoryService
	wsService     *WebSocketService
	enabled       bool
}

// NewUserFeedbackCollector 创建反馈收集器
func NewUserFeedbackCollector(db *gorm.DB, modelSvc *ModelService, skillSvc *SkillService, memorySvc *MemoryService, wsService *WebSocketService, enabled bool) *UserFeedbackCollector {
	return &UserFeedbackCollector{
		db:        db,
		modelSvc:  modelSvc,
		skillSvc:  skillSvc,
		memorySvc: memorySvc,
		wsService: wsService,
		enabled:   enabled,
	}
}

// FeedbackQuestion 反馈问题
type FeedbackQuestion struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
	Type     string   `json:"type"` // satisfaction, usefulness, accuracy
}

// CollectSkillFeedback 收集技能执行后的反馈
func (s *UserFeedbackCollector) CollectSkillFeedback(ctx context.Context, execution *models.SkillExecution) error {
	if !s.enabled {
		return nil
	}

	// 分析最近用户消息中的满意度信号
	var messages []models.Message
	s.db.Where("sender = ? AND created_at > ?", "user", execution.StartedAt).
		Order("created_at ASC").Limit(5).
		Find(&messages)

	if len(messages) == 0 {
		return nil
	}

	// 分析用户回复中的满意度信号
	satisfaction := s.analyzeSatisfaction(messages[0].Content)

	// 更新技能成功率
	if satisfaction < 0.3 {
		memory := &models.Memory{
			UserID:     "system",
			MemoryType: "preference",
			Content:    fmt.Sprintf("用户对技能 '%s' 的执行结果不满意", execution.SkillName),
			Importance: 3,
			Tags:       models.JSONSlice{"feedback", "dissatisfied", execution.SkillName},
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		s.memorySvc.CreateMemory(memory)
	}

	return nil
}

// analyzeSatisfaction 分析用户满意度
func (s *UserFeedbackCollector) analyzeSatisfaction(content string) float64 {
	contentLower := strings.ToLower(content)

	// 正面信号
	positiveSignals := []string{"好的", "谢谢", "不错", "很好", "满意", "完美", "excellent", "good", "thanks", "perfect", "great"}
	// 负面信号
	negativeSignals := []string{"不对", "错了", "不好", "重新", "再试", "不行", "wrong", "bad", "no", "not good", "incorrect"}

	positiveCount := 0
	negativeCount := 0

	for _, signal := range positiveSignals {
		if strings.Contains(contentLower, signal) {
			positiveCount++
		}
	}
	for _, signal := range negativeSignals {
		if strings.Contains(contentLower, signal) {
			negativeCount++
		}
	}

	if positiveCount+negativeCount == 0 {
		return 0.5 // 中性
	}

	satisfaction := float64(positiveCount) / float64(positiveCount+negativeCount)
	return satisfaction
}

// PromptForFeedback 主动询问用户反馈
func (s *UserFeedbackCollector) PromptForFeedback(ctx context.Context, dialogueID, skillName string) {
	if !s.enabled || s.wsService == nil {
		return
	}

	questions := []FeedbackQuestion{
		{
			Question: fmt.Sprintf("技能 '%s' 的执行结果满意吗？", skillName),
			Options:  []string{"非常满意", "满意", "一般", "不满意", "非常不满意"},
			Type:     "satisfaction",
		},
		{
			Question: "这个技能对你有帮助吗？",
			Options:  []string{"非常有帮助", "有帮助", "一般", "不太有帮助", "没有帮助"},
			Type:     "usefulness",
		},
	}

	// 通过 WebSocket 发送反馈请求
	s.wsService.NotifySkillFeedbackRequest(dialogueID, skillName, questions)
}

// ProcessUserFeedback 处理用户反馈
func (s *UserFeedbackCollector) ProcessUserFeedback(ctx context.Context, userID, skillID string, feedback map[string]interface{}) error {
	satisfaction, _ := feedback["satisfaction"].(float64)

	// 更新技能成功率
	s.db.Model(&models.Skill{}).Where("id = ?", skillID).Update("success_rate", satisfaction)

	// 记录反馈到记忆
	if satisfaction < 0.4 {
		var skill models.Skill
		if err := s.db.First(&skill, "id = ?", skillID).Error; err == nil {
			memory := &models.Memory{
				UserID:     userID,
				MemoryType: "preference",
				Content:    fmt.Sprintf("用户对技能 '%s' 不满意，评分: %.1f/5", skill.Name, satisfaction*5),
				Importance: 4,
				Tags:       models.JSONSlice{"feedback", "low_satisfaction", skill.Name},
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}
			s.memorySvc.CreateMemory(memory)
		}
	}

	return nil
}

// RunPeriodicFeedbackAnalysis 定期分析反馈数据
func (s *UserFeedbackCollector) RunPeriodicFeedbackAnalysis(ctx context.Context) {
	if !s.enabled {
		return
	}

	log.Printf("[FeedbackCollector] Starting periodic feedback analysis...")

	// 分析最近 24 小时的技能执行
	var recentExecutions []models.SkillExecution
	s.db.Where("started_at > ?", time.Now().Add(-24*time.Hour)).
		Order("started_at DESC").
		Find(&recentExecutions)

	if len(recentExecutions) == 0 {
		log.Printf("[FeedbackCollector] No recent executions to analyze")
		return
	}

	// 统计成功率
	totalExecs := len(recentExecutions)
	successExecs := 0
	for _, exec := range recentExecutions {
		if exec.Status == "completed" {
			successExecs++
		}
	}

	successRate := float64(successExecs) / float64(totalExecs)
	log.Printf("[FeedbackCollector] 24h stats: %d executions, %.1f%% success rate", totalExecs, successRate*100)

	// 识别低满意度技能
	var skills []models.Skill
	s.db.Find(&skills)

	for _, skill := range skills {
		var execs []models.SkillExecution
		s.db.Where("skill_id = ? AND started_at > ?", skill.ID, time.Now().Add(-7*24*time.Hour)).
			Find(&execs)

		if len(execs) >= 5 {
			successCount := 0
			for _, e := range execs {
				if e.Status == "completed" {
					successCount++
				}
			}

			rate := float64(successCount) / float64(len(execs))
			if rate < 0.5 {
				log.Printf("[FeedbackCollector] Low success rate skill: %s (%.1f%%, %d executions)",
					skill.Name, rate*100, len(execs))
			}
		}
	}

	log.Printf("[FeedbackCollector] Periodic feedback analysis completed")
}

// AnalyzeDialogueSatisfaction 分析对话中的用户满意度
func (s *UserFeedbackCollector) AnalyzeDialogueSatisfaction(ctx context.Context, dialogueID string) (float64, error) {
	var messages []models.Message
	s.db.Where("dialogue_id = ? AND sender = ?", dialogueID, "user").
		Order("created_at ASC").
		Find(&messages)

	if len(messages) == 0 {
		return 0.5, nil
	}

	// 使用 LLM 分析整体满意度
	client := s.modelSvc.GetLLMClient()
	if client == nil {
		return 0.5, fmt.Errorf("no LLM client available")
	}

	var builder strings.Builder
	for _, m := range messages {
		content := m.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		builder.WriteString(fmt.Sprintf("用户: %s\n", content))
	}

	prompt := fmt.Sprintf(`请分析以下用户对话，判断用户对整个对话过程的满意度。

对话内容：
%s

请返回 0-1 之间的满意度评分：
- 0.0-0.2: 非常不满意
- 0.2-0.4: 不满意
- 0.4-0.6: 一般
- 0.6-0.8: 满意
- 0.8-1.0: 非常满意

只返回一个数字，不要其他内容。`, builder.String())

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个满意度分析专家。从用户对话中分析满意度。只返回一个 0-1 之间的数字。"},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.1,
		MaxTokens:   100,
	}

	resp, err := client.Chat(ctx, req)
	if err != nil {
		return 0.5, err
	}

	if len(resp.Choices) == 0 {
		return 0.5, fmt.Errorf("no response from LLM")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	var satisfaction float64
	fmt.Sscanf(content, "%f", &satisfaction)

	return satisfaction, nil
}

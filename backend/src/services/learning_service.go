package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// LearningService 学习服务
type LearningService struct {
	db              *gorm.DB
	llmClient       llm.LLMClient
	feedbackService *FeedbackService
	memoryService   *MemoryService
	mu              sync.RWMutex
}

// NewLearningService 创建学习服务实例
func NewLearningService(
	db *gorm.DB,
	llmClient llm.LLMClient,
	feedbackService *FeedbackService,
	memoryService *MemoryService,
) *LearningService {
	return &LearningService{
		db:              db,
		llmClient:       llmClient,
		feedbackService: feedbackService,
		memoryService:   memoryService,
	}
}

// LearnFromFeedback 从反馈中学习
func (s *LearningService) LearnFromFeedback(ctx context.Context, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 获取任务的反馈
	feedbacks, err := s.feedbackService.GetFeedbackByTask(taskID)
	if err != nil {
		return fmt.Errorf("failed to get feedback: %w", err)
	}

	if len(feedbacks) == 0 {
		return nil
	}

	// 分析反馈数据
	analysis := s.analyzeFeedback(feedbacks)

	// 根据分析结果更新学习数据
	return s.updateLearningData(ctx, analysis)
}

// FeedbackAnalysis 反馈分析结果
type FeedbackAnalysis struct {
	AverageRating    float64            `json:"average_rating"`
	TotalFeedbacks   int                `json:"total_feedbacks"`
	PositiveCount    int                `json:"positive_count"`
	NegativeCount    int                `json:"negative_count"`
	NeutralCount     int                `json:"neutral_count"`
	TaskType         string             `json:"task_type"`
	CommonIssues     []string           `json:"common_issues"`
	CommonPraises    []string           `json:"common_praises"`
	SentimentKeywords map[string]int    `json:"sentiment_keywords"`
	ImprovementAreas []string           `json:"improvement_areas"`
}

// analyzeFeedback 分析反馈数据
func (s *LearningService) analyzeFeedback(feedbacks []models.Feedback) *FeedbackAnalysis {
	analysis := &FeedbackAnalysis{
		SentimentKeywords: make(map[string]int),
		CommonIssues:      make([]string, 0),
		CommonPraises:     make([]string, 0),
		ImprovementAreas:  make([]string, 0),
	}

	if len(feedbacks) == 0 {
		return analysis
	}

	totalRating := 0
	analysis.TaskType = feedbacks[0].TaskType
	analysis.TotalFeedbacks = len(feedbacks)

	for _, feedback := range feedbacks {
		totalRating += feedback.Rating

		// 统计正负面反馈
		switch {
		case feedback.Rating >= 4:
			analysis.PositiveCount++
		case feedback.Rating <= 2:
			analysis.NegativeCount++
		default:
			analysis.NeutralCount++
		}

		// 分析评论内容
		if feedback.Comment != "" {
			s.analyzeComment(feedback.Comment, feedback.Rating, analysis)
		}
	}

	analysis.AverageRating = float64(totalRating) / float64(len(feedbacks))

	return analysis
}

// analyzeComment 分析评论内容
func (s *LearningService) analyzeComment(comment string, rating int, analysis *FeedbackAnalysis) {
	// 简单的关键词提取
	words := strings.Fields(strings.ToLower(comment))

	negativeKeywords := []string{"错误", "失败", "慢", "不好", "问题", "bug", "错误", "不准确", "差"}
	positiveKeywords := []string{"好", "准确", "快", "优秀", "正确", "棒", "不错", "清晰", "有帮助"}

	for _, word := range words {
		for _, neg := range negativeKeywords {
			if strings.Contains(word, neg) {
				analysis.CommonIssues = append(analysis.CommonIssues, word)
				analysis.ImprovementAreas = append(analysis.ImprovementAreas, word)
			}
		}
		for _, pos := range positiveKeywords {
			if strings.Contains(word, pos) {
				analysis.CommonPraises = append(analysis.CommonPraises, word)
			}
		}
		analysis.SentimentKeywords[word]++
	}
}

// updateLearningData 更新学习数据
func (s *LearningService) updateLearningData(ctx context.Context, analysis *FeedbackAnalysis) error {
	// 创建学习记录
	learningRecord := &models.LearningRecord{
		ID:           uuid.New().String(),
		Type:         "feedback_analysis",
		TaskType:     analysis.TaskType,
		Data:         map[string]interface{}{},
		Confidence:   s.calculateConfidence(analysis),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// 将分析结果序列化到 Data 字段
	data, _ := json.Marshal(analysis)
	learningRecord.Data["analysis"] = string(data)

	// 保存学习记录
	if err := s.db.Create(learningRecord).Error; err != nil {
		return fmt.Errorf("failed to save learning record: %w", err)
	}

	// 更新 Prompt 优化建议
	if len(analysis.ImprovementAreas) > 0 {
		go s.generatePromptOptimizations(ctx, analysis)
	}

	return nil
}

// calculateConfidence 计算置信度
func (s *LearningService) calculateConfidence(analysis *FeedbackAnalysis) float64 {
	if analysis.TotalFeedbacks == 0 {
		return 0
	}

	// 基于反馈数量和平均评分计算置信度
	sampleSizeFactor := math.Min(1.0, float64(analysis.TotalFeedbacks)/50.0)
	ratingFactor := analysis.AverageRating / 5.0

	return (sampleSizeFactor + ratingFactor) / 2
}

// generatePromptOptimizations 生成 Prompt 优化建议
func (s *LearningService) generatePromptOptimizations(ctx context.Context, analysis *FeedbackAnalysis) {
	prompt := fmt.Sprintf(`基于以下用户反馈分析，生成 Prompt 优化建议：

平均评分: %.2f
正面反馈: %d
负面反馈: %d
常见问题: %v
改进领域: %v

请提供具体的优化建议，包括：
1. Prompt 模板改进
2. 参数调整建议
3. 工作流优化方向

请以 JSON 格式返回。`,
		analysis.AverageRating,
		analysis.PositiveCount,
		analysis.NegativeCount,
		analysis.CommonIssues,
		analysis.ImprovementAreas,
	)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个 AI 助手优化专家，擅长分析用户反馈并提供改进建议。"},
			{Role: "user", Content: prompt},
		},
		Model:       "gpt-4",
		Temperature: 0.7,
		MaxTokens:   2000,
	}

	resp, err := s.llmClient.Chat(ctx, req)
	if err != nil {
		return
	}

	if len(resp.Choices) > 0 {
		optimization := &models.PromptOptimization{
			ID:          uuid.New().String(),
			TaskType:    analysis.TaskType,
			Suggestions: resp.Choices[0].Message.Content,
			Status:      "pending",
			CreatedAt:   time.Now(),
		}
		s.db.Create(optimization)
	}
}

// UpdateModelFromData 根据数据更新模型
func (s *LearningService) UpdateModelFromData(ctx context.Context, modelID string, data map[string]interface{}) error {
	// 创建学习记录
	record := &models.LearningRecord{
		ID:        uuid.New().String(),
		Type:      "model_update",
		ModelID:   modelID,
		Data:      data,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	return s.db.Create(record).Error
}

// LearnUserPreferences 学习用户偏好
func (s *LearningService) LearnUserPreferences(ctx context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 获取用户的历史反馈
	var feedbacks []models.Feedback
	err := s.db.Where("user_id = ?", userID).Find(&feedbacks).Error
	if err != nil {
		return fmt.Errorf("failed to get user feedbacks: %w", err)
	}

	// 分析用户偏好
	preferences := s.extractUserPreferences(feedbacks)

	// 保存用户偏好
	for key, value := range preferences {
		preference := &models.UserPreference{
			ID:        uuid.New().String(),
			UserID:    userID,
			Key:       key,
			Value:     &models.JSONAny{Data: value},
			UpdatedAt: time.Now(),
		}

		// 更新或创建
		s.db.Where("user_id = ? AND key = ?", userID, key).
			Assign(preference).
			FirstOrCreate(preference)
	}

	return nil
}

// extractUserPreferences 提取用户偏好
func (s *LearningService) extractUserPreferences(feedbacks []models.Feedback) map[string]interface{} {
	preferences := make(map[string]interface{})

	taskTypeCounts := make(map[string]int)
	taskTypeRatings := make(map[string][]int)

	for _, feedback := range feedbacks {
		taskTypeCounts[feedback.TaskType]++
		taskTypeRatings[feedback.TaskType] = append(taskTypeRatings[feedback.TaskType], feedback.Rating)
	}

	// 计算每个任务类型的平均评分
	for taskType, ratings := range taskTypeRatings {
		sum := 0
		for _, r := range ratings {
			sum += r
		}
		avgRating := float64(sum) / float64(len(ratings))
		preferences[fmt.Sprintf("task_type.%s.avg_rating", taskType)] = avgRating
		preferences[fmt.Sprintf("task_type.%s.usage_count", taskType)] = len(ratings)
	}

	return preferences
}

// GetUserPreferences 获取用户偏好
func (s *LearningService) GetUserPreferences(userID string) (map[string]interface{}, error) {
	var preferences []models.UserPreference
	err := s.db.Where("user_id = ?", userID).Find(&preferences).Error
	if err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	for _, pref := range preferences {
		result[pref.Key] = pref.Value
	}

	return result, nil
}

// OptimizeWorkflow 优化工作流
func (s *LearningService) OptimizeWorkflow(ctx context.Context, workflowID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 获取工作流的执行历史
	var executions []models.WorkflowExecution
	err := s.db.Where("workflow_id = ?", workflowID).
		Order("started_at DESC").
		Limit(100).
		Find(&executions).Error
	if err != nil {
		return fmt.Errorf("failed to get workflow executions: %w", err)
	}

	if len(executions) == 0 {
		return nil
	}

	// 分析执行数据
	analysis := s.analyzeWorkflowExecutions(executions)

	// 生成优化建议
	return s.generateWorkflowOptimizations(ctx, workflowID, analysis)
}

// WorkflowExecutionAnalysis 工作流执行分析
type WorkflowExecutionAnalysis struct {
	TotalExecutions     int     `json:"total_executions"`
	SuccessRate         float64 `json:"success_rate"`
	AverageDuration     int64   `json:"average_duration"`
	FailedSteps         map[string]int `json:"failed_steps"`
	SlowSteps           map[string]int64 `json:"slow_steps"`
	CommonErrors        []string `json:"common_errors"`
}

// analyzeWorkflowExecutions 分析工作流执行
func (s *LearningService) analyzeWorkflowExecutions(executions []models.WorkflowExecution) *WorkflowExecutionAnalysis {
	analysis := &WorkflowExecutionAnalysis{
		TotalExecutions: len(executions),
		FailedSteps:     make(map[string]int),
		SlowSteps:       make(map[string]int64),
		CommonErrors:    make([]string, 0),
	}

	successCount := 0
	totalDuration := int64(0)

	for _, exec := range executions {
		if exec.Status == "completed" {
			successCount++
		} else {
			// 收集错误信息
			if exec.Error != "" {
				analysis.CommonErrors = append(analysis.CommonErrors, exec.Error)
			}
		}
		totalDuration += exec.Duration
	}

	analysis.SuccessRate = float64(successCount) / float64(len(executions))
	analysis.AverageDuration = totalDuration / int64(len(executions))

	return analysis
}

// generateWorkflowOptimizations 生成工作流优化建议
func (s *LearningService) generateWorkflowOptimizations(ctx context.Context, workflowID string, analysis *WorkflowExecutionAnalysis) error {
	prompt := fmt.Sprintf(`基于以下工作流执行分析，生成优化建议：

总执行次数: %d
成功率: %.2f%%
平均执行时长: %d ms
常见错误: %v

请提供具体的优化建议，包括：
1. 步骤顺序调整
2. 超时时间优化
3. 重试策略改进
4. 错误处理增强

请以 JSON 格式返回。`,
		analysis.TotalExecutions,
		analysis.SuccessRate*100,
		analysis.AverageDuration,
		analysis.CommonErrors,
	)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个工作流优化专家，擅长分析执行数据并提供改进建议。"},
			{Role: "user", Content: prompt},
		},
		Model:       "gpt-4",
		Temperature: 0.7,
		MaxTokens:   2000,
	}

	resp, err := s.llmClient.Chat(ctx, req)
	if err != nil {
		return err
	}

	if len(resp.Choices) > 0 {
		optimization := &models.WorkflowOptimization{
			ID:          uuid.New().String(),
			WorkflowID:  workflowID,
			Suggestions: resp.Choices[0].Message.Content,
			Status:      "pending",
			CreatedAt:   time.Now(),
		}
		return s.db.Create(optimization).Error
	}

	return nil
}

func (s *LearningService) ApplyWorkflowOptimization(ctx context.Context, optimizationID string) error {
	var optimization models.WorkflowOptimization
	if err := s.db.Where("id = ?", optimizationID).First(&optimization).Error; err != nil {
		return fmt.Errorf("optimization not found: %w", err)
	}

	now := time.Now()
	if err := s.db.Model(&optimization).Updates(map[string]interface{}{
		"status":     "applied",
		"applied_at": now,
	}).Error; err != nil {
		return err
	}

	if len(optimization.Changes) > 0 && optimization.WorkflowID != "" {
		var workflow models.Workflow
		if err := s.db.Where("id = ?", optimization.WorkflowID).First(&workflow).Error; err == nil {
			var steps []models.WorkflowStep
			s.db.Where("workflow_id = ?", workflow.ID).Find(&steps)
			stepMap := make(map[string]*models.WorkflowStep)
			for i := range steps {
				stepMap[steps[i].ID] = &steps[i]
			}
			for _, change := range optimization.Changes {
				if step, ok := stepMap[change.StepID]; ok {
					switch change.Attribute {
					case "timeout":
						s.db.Model(step).Update("timeout", change.NewValue)
					case "retry_policy":
						s.db.Model(step).Update("retry_policy", change.NewValue)
					case "parameters":
						s.db.Model(step).Update("parameters", change.NewValue)
					}
				}
			}
		}
	}

	return nil
}

// EvaluateLearningEffect 评估学习效果
func (s *LearningService) EvaluateLearningEffect(ctx context.Context, period time.Duration) (*LearningEffectReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	since := now.Add(-period)

	// 获取时间范围内的反馈
	var feedbacks []models.Feedback
	err := s.db.Where("created_at >= ?", since).Find(&feedbacks).Error
	if err != nil {
		return nil, err
	}

	// 获取学习记录
	var learningRecords []models.LearningRecord
	err = s.db.Where("created_at >= ?", since).Find(&learningRecords).Error
	if err != nil {
		return nil, err
	}

	report := &LearningEffectReport{
		Period:         period,
		StartTime:      since,
		EndTime:        now,
		TotalFeedbacks: len(feedbacks),
		LearningCount:  len(learningRecords),
	}

	if len(feedbacks) > 0 {
		totalRating := 0
		for _, f := range feedbacks {
			totalRating += f.Rating
		}
		report.AverageRating = float64(totalRating) / float64(len(feedbacks))
	}

	// 计算改进趋势
	report.ImprovementTrend = s.calculateImprovementTrend(feedbacks)

	return report, nil
}

// LearningEffectReport 学习效果报告
type LearningEffectReport struct {
	Period            time.Duration `json:"period"`
	StartTime         time.Time     `json:"start_time"`
	EndTime           time.Time     `json:"end_time"`
	TotalFeedbacks    int           `json:"total_feedbacks"`
	AverageRating     float64       `json:"average_rating"`
	LearningCount     int           `json:"learning_count"`
	ImprovementTrend  float64       `json:"improvement_trend"`
	OptimizedAreas    []string      `json:"optimized_areas"`
}

// calculateImprovementTrend 计算改进趋势
func (s *LearningService) calculateImprovementTrend(feedbacks []models.Feedback) float64 {
	if len(feedbacks) < 2 {
		return 0
	}

	// 按时间排序
	// 简化计算：比较前半部分和后半部分的平均评分
	mid := len(feedbacks) / 2

	firstHalfRating := 0
	secondHalfRating := 0

	for i, f := range feedbacks {
		if i < mid {
			firstHalfRating += f.Rating
		} else {
			secondHalfRating += f.Rating
		}
	}

	firstHalfAvg := float64(firstHalfRating) / float64(mid)
	secondHalfAvg := float64(secondHalfRating) / float64(len(feedbacks)-mid)

	return ((secondHalfAvg - firstHalfAvg) / firstHalfAvg) * 100
}

// GetRecommendedPrompts 获取推荐的 Prompt 模板
func (s *LearningService) GetRecommendedPrompts(ctx context.Context, taskType string) ([]string, error) {
	// 获取历史优化记录
	var optimizations []models.PromptOptimization
	err := s.db.Where("task_type = ? AND status = ?", taskType, "applied").
		Order("created_at DESC").
		Limit(5).
		Find(&optimizations).Error
	if err != nil {
		return nil, err
	}

	prompts := make([]string, 0, len(optimizations))
	for _, opt := range optimizations {
		prompts = append(prompts, opt.Suggestions)
	}

	return prompts, nil
}

// ApplyPromptOptimization 应用 Prompt 优化
func (s *LearningService) ApplyPromptOptimization(ctx context.Context, optimizationID string) error {
	var optimization models.PromptOptimization
	if err := s.db.Where("id = ?", optimizationID).First(&optimization).Error; err != nil {
		return fmt.Errorf("optimization not found: %w", err)
	}

	now := time.Now()
	if err := s.db.Model(&optimization).Updates(map[string]interface{}{
		"status":     "applied",
		"applied_at": now,
	}).Error; err != nil {
		return err
	}

	if optimization.TaskType != "" {
		var template models.PromptTemplate
		err := s.db.Where("category = ? AND is_active = ?", optimization.TaskType, true).First(&template).Error
		if err == nil {
			updatedContent := template.Template + "\n\n【自动优化补充】\n" + optimization.Suggestions
			s.db.Model(&template).Update("template", updatedContent)
		}
	}

	return nil
}

// RecordInteraction 记录交互用于学习
func (s *LearningService) RecordInteraction(ctx context.Context, interaction *models.InteractionRecord) error {
	interaction.ID = uuid.New().String()
	interaction.CreatedAt = time.Now()
	return s.db.Create(interaction).Error
}

// GetLearningInsights 获取学习洞察
func (s *LearningService) GetLearningInsights(ctx context.Context, userID string) (*LearningInsights, error) {
	// 获取用户偏好
	preferences, err := s.GetUserPreferences(userID)
	if err != nil {
		return nil, err
	}

	// 获取最近的反馈
	var recentFeedbacks []models.Feedback
	err = s.db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(20).
		Find(&recentFeedbacks).Error
	if err != nil {
		return nil, err
	}

	insights := &LearningInsights{
		UserID:              userID,
		UserPreferences:     preferences,
		RecentFeedbackCount: len(recentFeedbacks),
		GeneratedAt:         time.Now(),
	}

	if len(recentFeedbacks) > 0 {
		totalRating := 0
		for _, f := range recentFeedbacks {
			totalRating += f.Rating
		}
		insights.AverageSatisfaction = float64(totalRating) / float64(len(recentFeedbacks))
	}

	return insights, nil
}

// LearningInsights 学习洞察
type LearningInsights struct {
	UserID              string                 `json:"user_id"`
	UserPreferences     map[string]interface{} `json:"user_preferences"`
	RecentFeedbackCount int                    `json:"recent_feedback_count"`
	AverageSatisfaction float64                `json:"average_satisfaction"`
	GeneratedAt         time.Time              `json:"generated_at"`
}

// StartContinuousLearning 启动持续学习
func (s *LearningService) StartContinuousLearning(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.performLearningCycle(ctx)
		}
	}
}

// performLearningCycle 执行学习周期
func (s *LearningService) performLearningCycle(ctx context.Context) {
	// 分析所有待处理的反馈
	s.processPendingFeedbacks(ctx)

	// 优化活跃的工作流
	s.optimizeActiveWorkflows(ctx)

	// 更新用户偏好
	s.updateAllUserPreferences(ctx)
}

// processPendingFeedbacks 处理待处理的反馈
func (s *LearningService) processPendingFeedbacks(ctx context.Context) {
	// 获取最近未处理的反馈
	since := time.Now().Add(-24 * time.Hour)

	var tasks []struct {
		TaskID string
	}

	s.db.Table("feedbacks").
		Select("DISTINCT task_id").
		Where("created_at >= ?", since).
		Scan(&tasks)

	for _, task := range tasks {
		s.LearnFromFeedback(ctx, task.TaskID)
	}
}

// optimizeActiveWorkflows 优化活跃的工作流
func (s *LearningService) optimizeActiveWorkflows(ctx context.Context) {
	var workflows []struct {
		ID string
	}

	s.db.Table("workflow_executions").
		Select("DISTINCT workflow_id").
		Where("started_at >= ?", time.Now().Add(-7*24*time.Hour)).
		Scan(&workflows)

	for _, wf := range workflows {
		s.OptimizeWorkflow(ctx, wf.ID)
	}
}

// updateAllUserPreferences 更新所有用户偏好
func (s *LearningService) updateAllUserPreferences(ctx context.Context) {
	var users []struct {
		UserID string
	}

	s.db.Table("feedbacks").
		Select("DISTINCT user_id").
		Where("created_at >= ?", time.Now().Add(-24*time.Hour)).
		Scan(&users)

	for _, user := range users {
		s.LearnUserPreferences(ctx, user.UserID)
	}
}

// GenerateOptimizationFromDialogue 从对话生成 prompt 优化建议
func (s *LearningService) GenerateOptimizationFromDialogue(ctx context.Context, dialogueID string) error {
	// 加载对话消息历史
	var messages []models.Message
	if err := s.db.Where("dialogue_id = ?", dialogueID).Order("created_at ASC").Find(&messages).Error; err != nil {
		return fmt.Errorf("failed to load dialogue messages: %w", err)
	}
	if len(messages) < 4 {
		return nil
	}

	// 构建对话摘要用于分析
	var sb strings.Builder
	for _, msg := range messages {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Sender, msg.Content))
	}
	dialogueText := sb.String()

	prompt := fmt.Sprintf(`分析以下对话，生成 Prompt 改进建议以提升 AI 回复质量。

对话内容：
%s

请分析：
1. 回复是否准确、有帮助
2. 是否有误解用户意图的情况
3. 回复风格和格式是否合适
4. 是否有可以改进的方面

请直接输出改进建议（不超过200字），不要输出 JSON 或其他格式。`, dialogueText)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个 AI 助手质量分析专家。分析对话并给出简洁的 Prompt 改进建议。"},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.7,
		MaxTokens:   500,
	}

	resp, err := s.llmClient.Chat(ctx, req)
	if err != nil {
		return fmt.Errorf("LLM analysis failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil
	}

	suggestions := resp.Choices[0].Message.Content
	if suggestions == "" {
		return nil
	}

	// 计算置信度：基于消息数量和内容质量
	messageCount := len(messages)
	confidence := 0.0
	if messageCount >= 10 {
		confidence = 0.85
	} else if messageCount >= 6 {
		confidence = 0.75
	} else {
		confidence = 0.6
	}

	status := "pending"
	if confidence > 0.8 {
		status = "applied"
	}

	now := time.Now()
	optimization := &models.PromptOptimization{
		ID:          uuid.New().String(),
		DialogueID:  dialogueID,
		TaskType:    "dialogue_quality",
		Suggestions: suggestions,
		Status:      status,
		Confidence:  confidence,
		CreatedAt:   now,
	}
	if status == "applied" {
		optimization.AppliedAt = &now
	}

	return s.db.Create(optimization).Error
}

// AnalyzeAndAutoApply 分析反馈并自动应用高置信度优化
func (s *LearningService) AnalyzeAndAutoApply(ctx context.Context, dialogueID string) error {
	// 获取该对话的反馈记录
	var feedbacks []models.Feedback
	if err := s.db.Where("task_id = ?", dialogueID).
		Order("created_at DESC").
		Limit(10).
		Find(&feedbacks).Error; err != nil {
		return fmt.Errorf("failed to get feedbacks: %w", err)
	}
	if len(feedbacks) == 0 {
		return nil
	}

	// 复用现有 analyzeFeedback 分析
	analysis := s.analyzeFeedback(feedbacks)

	// 如果平均评分较低，生成优化建议
	if analysis.AverageRating < 3.5 && len(analysis.ImprovementAreas) > 0 {
		prompt := fmt.Sprintf(`基于以下反馈分析，给出简短的 Prompt 改进建议：

平均评分: %.1f
负面反馈: %d
改进领域: %v

请直接输出改进建议（不超过150字）。`, analysis.AverageRating, analysis.NegativeCount, analysis.ImprovementAreas)

		req := &llm.ChatRequest{
			Messages: []llm.Message{
				{Role: "system", Content: "你是一个 AI 助手优化专家。根据反馈给出简洁的改进建议。"},
				{Role: "user", Content: prompt},
			},
			Temperature: 0.7,
			MaxTokens:   300,
		}

		resp, err := s.llmClient.Chat(ctx, req)
		if err == nil && len(resp.Choices) > 0 && resp.Choices[0].Message.Content != "" {
			confidence := s.calculateConfidence(analysis)
			now := time.Now()
			status := "pending"
			if confidence > 0.8 {
				status = "applied"
			}

			optimization := &models.PromptOptimization{
				ID:          uuid.New().String(),
				DialogueID:  dialogueID,
				TaskType:    "feedback_driven",
				Suggestions: resp.Choices[0].Message.Content,
				Status:      status,
				Confidence:  confidence,
				CreatedAt:   now,
			}
			if status == "applied" {
				optimization.AppliedAt = &now
			}
			s.db.Create(optimization)
		}
	}

	return nil
}

// AnalyzeInteractionPatterns 分析用户交互模式并更新偏好
func (s *LearningService) AnalyzeInteractionPatterns(ctx context.Context, userID string) error {
	// 获取用户最近的对话
	var dialogues []models.Dialogue
	if err := s.db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(20).
		Find(&dialogues).Error; err != nil {
		return fmt.Errorf("failed to get user dialogues: %w", err)
	}
	if len(dialogues) == 0 {
		return nil
	}

	// 收集最近消息用于分析
	var recentMessages []models.Message
	dialogueIDs := make([]string, len(dialogues))
	for i, d := range dialogues {
		dialogueIDs[i] = d.ID
	}
	if err := s.db.Where("dialogue_id IN ?", dialogueIDs).
		Order("created_at DESC").
		Limit(50).
		Find(&recentMessages).Error; err != nil {
		return fmt.Errorf("failed to get messages: %w", err)
	}
	if len(recentMessages) < 4 {
		return nil
	}

	// 构建消息摘要
	var sb strings.Builder
	for i, msg := range recentMessages {
		if i >= 30 {
			break
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Sender, msg.Content))
	}
	messagesText := sb.String()

	prompt := fmt.Sprintf(`分析以下用户的对话历史，提取用户偏好。

对话历史：
%s

请以 JSON 格式返回以下字段（只返回 JSON，不要其他内容）：
{
  "response_style": "用户偏好的回复风格（如：简洁/详细/技术性/通俗）",
  "preferred_language": "用户主要使用的语言（如：中文/英文/混合）",
  "technical_depth": "用户的技术深度偏好（如：初级/中级/高级）",
  "topics_of_interest": "用户关注的领域（逗号分隔，如：编程,设计,管理）
}`, messagesText)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个用户行为分析专家。只返回 JSON，不要包含其他内容。"},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   300,
	}

	resp, err := s.llmClient.Chat(ctx, req)
	if err != nil {
		return fmt.Errorf("LLM analysis failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil
	}

	// 解析 JSON 响应
	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	// 去除可能的 markdown 代码块标记
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var prefs map[string]interface{}
	if err := json.Unmarshal([]byte(content), &prefs); err != nil {
		// 如果 JSON 解析失败，尝试提取关键信息
		return nil
	}

	// 保存用户偏好
	now := time.Now()
	for key, value := range prefs {
		if value == nil {
			continue
		}
		valStr := fmt.Sprintf("%v", value)
		pref := &models.UserPreference{
			ID:        uuid.New().String(),
			UserID:    userID,
			Key:       key,
			Value:     &models.JSONAny{Data: valStr},
			UpdatedAt: now,
		}
		s.db.Where("user_id = ? AND key = ?", userID, key).
			Assign(pref).
			FirstOrCreate(pref)
	}

	return nil
}

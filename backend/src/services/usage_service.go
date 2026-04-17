package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/smtp"
	"sync"
	"time"

	"openaide/backend/src/config"
	"openaide/backend/src/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UsageService 使用量统计服务
type UsageService struct {
	db        *gorm.DB
	cache     *CacheService
	logger    *LoggerService
	pricing   map[string]*models.ModelPricing
	mu        sync.RWMutex
}

// NewUsageService 创建使用量服务
func NewUsageService(db *gorm.DB, cache *CacheService, logger *LoggerService) *UsageService {
	s := &UsageService{
		db:      db,
		cache:   cache,
		logger:  logger,
		pricing: make(map[string]*models.ModelPricing),
	}

	// 加载定价配置
	s.loadPricing()

	// 初始化默认定价
	s.initDefaultPricing()

	return s
}

// loadPricing 加载定价配置
func (s *UsageService) loadPricing() {
	var pricings []models.ModelPricing
	if err := s.db.Where("is_active = ?", true).Find(&pricings).Error; err != nil {
		s.logger.Error(context.Background(), "Failed to load pricing: %v", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range pricings {
		key := fmt.Sprintf("%s:%s", p.Provider, p.ModelName)
		s.pricing[key] = &p
	}
}

// initDefaultPricing 初始化默认定价
func (s *UsageService) initDefaultPricing() {
	defaultPricings := []models.ModelPricing{
		// OpenAI
		{Provider: "openai", ModelName: "gpt-4", InputPricePer1K: 0.03, OutputPricePer1K: 0.06},
		{Provider: "openai", ModelName: "gpt-4-turbo", InputPricePer1K: 0.01, OutputPricePer1K: 0.03},
		{Provider: "openai", ModelName: "gpt-3.5-turbo", InputPricePer1K: 0.0005, OutputPricePer1K: 0.0015},

		// Anthropic
		{Provider: "anthropic", ModelName: "claude-3-opus", InputPricePer1K: 0.015, OutputPricePer1K: 0.075},
		{Provider: "anthropic", ModelName: "claude-3-sonnet", InputPricePer1K: 0.003, OutputPricePer1K: 0.015},
		{Provider: "anthropic", ModelName: "claude-3-haiku", InputPricePer1K: 0.00025, OutputPricePer1K: 0.00125},

		// DeepSeek
		{Provider: "deepseek", ModelName: "deepseek-chat", InputPricePer1K: 0.0001, OutputPricePer1K: 0.0002},
		{Provider: "deepseek", ModelName: "deepseek-coder", InputPricePer1K: 0.0001, OutputPricePer1K: 0.0002},

		// Qwen
		{Provider: "qwen", ModelName: "qwen-turbo", InputPricePer1K: 0.0004, OutputPricePer1K: 0.0012},
		{Provider: "qwen", ModelName: "qwen-plus", InputPricePer1K: 0.0008, OutputPricePer1K: 0.002},
		{Provider: "qwen", ModelName: "qwen-max", InputPricePer1K: 0.02, OutputPricePer1K: 0.06},

		// Moonshot
		{Provider: "moonshot", ModelName: "moonshot-v1-8k", InputPricePer1K: 0.012, OutputPricePer1K: 0.012},
		{Provider: "moonshot", ModelName: "moonshot-v1-32k", InputPricePer1K: 0.024, OutputPricePer1K: 0.024},

		// Gemini
		{Provider: "gemini", ModelName: "gemini-pro", InputPricePer1K: 0.00025, OutputPricePer1K: 0.0005},
		{Provider: "gemini", ModelName: "gemini-ultra", InputPricePer1K: 0.0025, OutputPricePer1K: 0.0075},

		// Mistral
		{Provider: "mistral", ModelName: "mistral-small", InputPricePer1K: 0.0002, OutputPricePer1K: 0.0002},
		{Provider: "mistral", ModelName: "mistral-medium", InputPricePer1K: 0.0027, OutputPricePer1K: 0.0081},
		{Provider: "mistral", ModelName: "mistral-large", InputPricePer1K: 0.004, OutputPricePer1K: 0.012},

		// 本地模型 (免费)
		{Provider: "ollama", ModelName: "*", InputPricePer1K: 0, OutputPricePer1K: 0},
		{Provider: "vllm", ModelName: "*", InputPricePer1K: 0, OutputPricePer1K: 0},
	}

	for _, p := range defaultPricings {
		key := fmt.Sprintf("%s:%s", p.Provider, p.ModelName)
		if _, exists := s.pricing[key]; !exists {
			p.ID = uuid.New().String()
			p.IsActive = true
			p.CreatedAt = time.Now()
			p.UpdatedAt = time.Now()

			if err := s.db.Create(&p).Error; err != nil {
				s.logger.Error(context.Background(), "Failed to create pricing: %s/%s - %v", p.Provider, p.ModelName, err)
				continue
			}

			s.pricing[key] = &p
		}
	}
}

// RecordUsage 记录使用量
func (s *UsageService) RecordUsage(record *models.UsageRecord) error {
	// 计算成本
	s.calculateCost(record)

	// 保存记录
	record.CreatedAt = time.Now()
	if err := s.db.Create(record).Error; err != nil {
		return fmt.Errorf("failed to record usage: %w", err)
	}

	// 使用带错误处理的 goroutine 更新汇总
	go func() {
		if err := s.updateDailyUsage(record); err != nil {
			s.logger.Error(context.Background(), "Failed to update daily usage: %v", err)
		}
	}()

	go func() {
		if err := s.updateMonthlyUsage(record); err != nil {
			s.logger.Error(context.Background(), "Failed to update monthly usage: %v", err)
		}
	}()

	go func() {
		s.checkBudget(record.UserID)
	}()

	return nil
}

// calculateCost 计算成本
func (s *UsageService) calculateCost(record *models.UsageRecord) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 查找定价
	key := fmt.Sprintf("%s:%s", record.Provider, record.ModelName)
	pricing, ok := s.pricing[key]

	// 尝试通配符匹配
	if !ok {
		key = fmt.Sprintf("%s:*", record.Provider)
		pricing, ok = s.pricing[key]
	}

	if !ok {
		// 默认定价
		pricing = &models.ModelPricing{
			InputPricePer1K:  0.001,
			OutputPricePer1K: 0.002,
		}
	}

	// 计算成本
	record.InputCost = float64(record.PromptTokens) / 1000 * pricing.InputPricePer1K
	record.OutputCost = float64(record.CompletionTokens) / 1000 * pricing.OutputPricePer1K
	record.TotalCost = record.InputCost + record.OutputCost
}

// updateDailyUsage 更新每日汇总
func (s *UsageService) updateDailyUsage(record *models.UsageRecord) error {
	today := time.Now().Format("2006-01-02")
	todayTime, _ := time.Parse("2006-01-02", today)

	var dailyUsage models.DailyUsage
	err := s.db.Where("user_id = ? AND date = ?", record.UserID, todayTime).First(&dailyUsage).Error

	if err == gorm.ErrRecordNotFound {
		// 创建新记录
		modelUsage := map[string]int64{record.ModelID: 1}
		providerUsage := map[string]int64{record.Provider: 1}
		// JSON 序列化保留用于未来扩展
		_, _ = json.Marshal(modelUsage)
		_, _ = json.Marshal(providerUsage)

		dailyUsage = models.DailyUsage{
			ID:                    uuid.New().String(),
			UserID:                record.UserID,
			Date:                  todayTime,
			TotalRequests:         1,
			TotalPromptTokens:     record.PromptTokens,
			TotalCompletionTokens: record.CompletionTokens,
			TotalTokens:           record.TotalTokens,
			TotalCost:             record.TotalCost,
			ModelUsage:            modelUsage,
			ProviderUsage:         providerUsage,
			UpdatedAt:             time.Now(),
		}

		// 使用 Create 而不是 Exec，避免并发问题
		if err := s.db.Create(&dailyUsage).Error; err != nil {
			// 可能是并发插入冲突，尝试更新
			if err := s.db.Where("user_id = ? AND date = ?", record.UserID, todayTime).
				First(&dailyUsage).Error; err == nil {
				goto update
			}
			return fmt.Errorf("failed to create daily usage: %w", err)
		}
		return nil
	}

update:
	// 更新现有记录 - 使用原子操作避免竞争
	updates := map[string]interface{}{
		"total_requests":         gorm.Expr("total_requests + 1"),
		"total_prompt_tokens":    gorm.Expr("total_prompt_tokens + ?", record.PromptTokens),
		"total_completion_tokens": gorm.Expr("total_completion_tokens + ?", record.CompletionTokens),
		"total_tokens":           gorm.Expr("total_tokens + ?", record.TotalTokens),
		"total_cost":             gorm.Expr("total_cost + ?", record.TotalCost),
		"updated_at":             time.Now(),
	}
	if err := s.db.Model(&dailyUsage).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update daily usage: %w", err)
	}

	return nil
}

// updateMonthlyUsage 更新每月汇总
func (s *UsageService) updateMonthlyUsage(record *models.UsageRecord) error {
	now := time.Now()
	year, month := now.Year(), int(now.Month())

	var monthlyUsage models.MonthlyUsage
	err := s.db.Where("user_id = ? AND year = ? AND month = ?", record.UserID, year, month).First(&monthlyUsage).Error

	if err == gorm.ErrRecordNotFound {
		monthlyUsage = models.MonthlyUsage{
			ID:                    uuid.New().String(),
			UserID:                record.UserID,
			Year:                  year,
			Month:                 month,
			TotalRequests:         1,
			TotalPromptTokens:     record.PromptTokens,
			TotalCompletionTokens: record.CompletionTokens,
			TotalTokens:           record.TotalTokens,
			TotalCost:             record.TotalCost,
			UpdatedAt:             time.Now(),
		}
		if err := s.db.Create(&monthlyUsage).Error; err != nil {
			// 可能是并发插入冲突，尝试更新
			if err := s.db.Where("user_id = ? AND year = ? AND month = ?", record.UserID, year, month).
				First(&monthlyUsage).Error; err == nil {
				goto update
			}
			return fmt.Errorf("failed to create monthly usage: %w", err)
		}
		return nil
	}

update:
	updates := map[string]interface{}{
		"total_requests":         gorm.Expr("total_requests + 1"),
		"total_prompt_tokens":    gorm.Expr("total_prompt_tokens + ?", record.PromptTokens),
		"total_completion_tokens": gorm.Expr("total_completion_tokens + ?", record.CompletionTokens),
		"total_tokens":           gorm.Expr("total_tokens + ?", record.TotalTokens),
		"total_cost":             gorm.Expr("total_cost + ?", record.TotalCost),
		"updated_at":             time.Now(),
	}
	if err := s.db.Model(&monthlyUsage).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update monthly usage: %w", err)
	}

	return nil
}

// checkBudget 检查预算
func (s *UsageService) checkBudget(userID string) {
	var budget models.UserBudget
	if err := s.db.Where("user_id = ?", userID).First(&budget).Error; err != nil {
		return // 没有设置预算
	}

	// 防止除零
	if budget.MonthlyBudget <= 0 {
		s.logger.Warn(context.Background(), "Invalid monthly budget for user %s: %.2f", userID, budget.MonthlyBudget)
		return
	}

	// 获取当月使用量
	now := time.Now()
	year, month := now.Year(), int(now.Month())

	var monthlyUsage models.MonthlyUsage
	if err := s.db.Where("user_id = ? AND year = ? AND month = ?", userID, year, month).
		First(&monthlyUsage).Error; err != nil {
		return
	}

	usedPercent := (monthlyUsage.TotalCost / budget.MonthlyBudget) * 100

	// 检查是否达到警报阈值
	for _, threshold := range budget.AlertThresholds {
		if usedPercent >= float64(threshold) && budget.AlertEmail != "" {
			s.sendBudgetAlert(userID, threshold, monthlyUsage.TotalCost, budget.MonthlyBudget, budget.AlertEmail)
		}
	}
}

// sendBudgetAlert 发送预算警报
func (s *UsageService) sendBudgetAlert(userID string, threshold int, used, budget float64, email string) {
	s.logger.Warn(context.Background(), "Budget alert: user=%s, threshold=%d%%, used=%.2f/%.2f, email=%s",
		userID, threshold, used, budget, email)

	if email == "" {
		return
	}

	// 加载邮件配置
	cfg, err := config.Load()
	if err != nil || cfg == nil || !cfg.Email.Enabled || cfg.Email.SMTPHost == "" {
		s.logger.Error(context.Background(), "Email not configured, skipping budget alert")
		return
	}

	subject := fmt.Sprintf("[OpenAIDE] 预算警报 - 已使用 %d%%", threshold)
	body := fmt.Sprintf(
		"用户: %s\n\n您的 OpenAIDE API 使用预算已达到 %d%% 阈值。\n\n已使用: ¥%.2f\n月度预算: ¥%.2f\n剩余: ¥%.2f\n\n请关注使用情况，避免超出预算。",
		userID, threshold, used, budget, budget-used,
	)

	from := cfg.Email.From
	if from == "" {
		from = cfg.Email.Username
	}

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		from, email, subject, body)

	addr := fmt.Sprintf("%s:%d", cfg.Email.SMTPHost, cfg.Email.SMTPPort)
	auth := smtp.PlainAuth("", cfg.Email.Username, cfg.Email.Password, cfg.Email.SMTPHost)

	if err := smtp.SendMail(addr, auth, from, []string{email}, []byte(msg)); err != nil {
		s.logger.Error(context.Background(), "Failed to send budget alert email: %v", err)
		return
	}

	s.logger.Info(context.Background(), "Budget alert email sent to %s", email)
}

// GetDailyUsage 获取每日使用量
func (s *UsageService) GetDailyUsage(userID string, date time.Time) (*models.DailyUsage, error) {
	var usage models.DailyUsage
	err := s.db.Where("user_id = ? AND date = ?", userID, date.Format("2006-01-02")).First(&usage).Error
	if err != nil {
		return nil, err
	}
	return &usage, nil
}

// GetMonthlyUsage 获取每月使用量
func (s *UsageService) GetMonthlyUsage(userID string, year, month int) (*models.MonthlyUsage, error) {
	var usage models.MonthlyUsage
	err := s.db.Where("user_id = ? AND year = ? AND month = ?", userID, year, month).First(&usage).Error
	if err != nil {
		return nil, err
	}
	return &usage, nil
}

// GetUsageHistory 获取使用历史
func (s *UsageService) GetUsageHistory(userID string, startDate, endDate time.Time, limit int) ([]models.UsageRecord, error) {
	var records []models.UsageRecord
	query := s.db.Where("user_id = ?", userID)

	if !startDate.IsZero() {
		query = query.Where("created_at >= ?", startDate)
	}
	if !endDate.IsZero() {
		query = query.Where("created_at <= ?", endDate)
	}

	if err := query.Order("created_at DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}

	return records, nil
}

// GetUsageStats 获取使用统计
func (s *UsageService) GetUsageStats(userID string, period string) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	now := time.Now()
	var startDate time.Time

	switch period {
	case "day":
		startDate = now.Truncate(24 * time.Hour)
	case "week":
		startDate = now.AddDate(0, 0, -7)
	case "month":
		startDate = now.AddDate(0, -1, 0)
	default:
		startDate = now.AddDate(0, -1, 0)
	}

	// 总请求数
	var totalRequests int64
	s.db.Model(&models.UsageRecord{}).
		Where("user_id = ? AND created_at >= ?", userID, startDate).
		Count(&totalRequests)
	stats["total_requests"] = totalRequests

	// 总 Token 数
	var totalTokens int64
	s.db.Model(&models.UsageRecord{}).
		Where("user_id = ? AND created_at >= ?", userID, startDate).
		Select("COALESCE(SUM(total_tokens), 0)").
		Scan(&totalTokens)
	stats["total_tokens"] = totalTokens

	// 总成本
	var totalCost float64
	s.db.Model(&models.UsageRecord{}).
		Where("user_id = ? AND created_at >= ?", userID, startDate).
		Select("COALESCE(SUM(total_cost), 0)").
		Scan(&totalCost)
	stats["total_cost"] = totalCost

	// 按模型分组
	type ModelStats struct {
		ModelID       string
		RequestCount  int64
		TotalTokens   int64
		TotalCost     float64
	}
	var modelStats []ModelStats
	s.db.Model(&models.UsageRecord{}).
		Select("model_id, COUNT(*) as request_count, SUM(total_tokens) as total_tokens, SUM(total_cost) as total_cost").
		Where("user_id = ? AND created_at >= ?", userID, startDate).
		Group("model_id").
		Order("request_count DESC").
		Limit(10).
		Scan(&modelStats)
	stats["by_model"] = modelStats

	// 按提供商分组
	var providerStats []ModelStats
	s.db.Model(&models.UsageRecord{}).
		Select("provider as model_id, COUNT(*) as request_count, SUM(total_tokens) as total_tokens, SUM(total_cost) as total_cost").
		Where("user_id = ? AND created_at >= ?", userID, startDate).
		Group("provider").
		Order("request_count DESC").
		Scan(&providerStats)
	stats["by_provider"] = providerStats

	// 成功率
	var successCount int64
	s.db.Model(&models.UsageRecord{}).
		Where("user_id = ? AND created_at >= ? AND success = ?", userID, startDate, true).
		Count(&successCount)
	if totalRequests > 0 {
		stats["success_rate"] = float64(successCount) / float64(totalRequests) * 100
	} else {
		stats["success_rate"] = 0.0
	}

	// 平均响应时间
	var avgDuration float64
	s.db.Model(&models.UsageRecord{}).
		Where("user_id = ? AND created_at >= ? AND success = ?", userID, startDate, true).
		Select("COALESCE(AVG(duration), 0)").
		Scan(&avgDuration)
	stats["avg_duration_ms"] = avgDuration

	return stats, nil
}

// SetUserBudget 设置用户预算
func (s *UsageService) SetUserBudget(userID string, monthlyBudget, dailyBudget float64, thresholds []int, email string) error {
	var budget models.UserBudget
	err := s.db.Where("user_id = ?", userID).First(&budget).Error

	if err == gorm.ErrRecordNotFound {
		budget = models.UserBudget{
			ID:             uuid.New().String(),
			UserID:         userID,
			MonthlyBudget:  monthlyBudget,
			DailyBudget:    dailyBudget,
			AlertThresholds: thresholds,
			AlertEmail:     email,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		return s.db.Create(&budget).Error
	}

	budget.MonthlyBudget = monthlyBudget
	budget.DailyBudget = dailyBudget
	budget.AlertThresholds = thresholds
	budget.AlertEmail = email
	budget.UpdatedAt = time.Now()

	return s.db.Save(&budget).Error
}

// GetUserBudget 获取用户预算
func (s *UsageService) GetUserBudget(userID string) (*models.UserBudget, error) {
	var budget models.UserBudget
	if err := s.db.Where("user_id = ?", userID).First(&budget).Error; err != nil {
		return nil, err
	}
	return &budget, nil
}

// UpdateModelPricing 更新模型定价
func (s *UsageService) UpdateModelPricing(pricing *models.ModelPricing) error {
	pricing.UpdatedAt = time.Now()

	key := fmt.Sprintf("%s:%s", pricing.Provider, pricing.ModelName)
	if existing, ok := s.pricing[key]; ok {
		pricing.ID = existing.ID
		if err := s.db.Save(pricing).Error; err != nil {
			return err
		}
	} else {
		pricing.ID = uuid.New().String()
		pricing.IsActive = true
		pricing.CreatedAt = time.Now()
		if err := s.db.Create(pricing).Error; err != nil {
			return err
		}
	}

	s.mu.Lock()
	s.pricing[key] = pricing
	s.mu.Unlock()

	return nil
}

// GetModelPricing 获取模型定价
func (s *UsageService) GetModelPricing(provider, modelName string) (*models.ModelPricing, error) {
	key := fmt.Sprintf("%s:%s", provider, modelName)
	s.mu.RLock()
	defer s.mu.RUnlock()

	if pricing, ok := s.pricing[key]; ok {
		return pricing, nil
	}

	// 尝试通配符
	key = fmt.Sprintf("%s:*", provider)
	if pricing, ok := s.pricing[key]; ok {
		return pricing, nil
	}

	return nil, fmt.Errorf("pricing not found for %s/%s", provider, modelName)
}

// ListModelPricing 列出所有定价
func (s *UsageService) ListModelPricing() ([]models.ModelPricing, error) {
	var pricings []models.ModelPricing
	if err := s.db.Where("is_active = ?", true).Find(&pricings).Error; err != nil {
		return nil, err
	}
	return pricings, nil
}

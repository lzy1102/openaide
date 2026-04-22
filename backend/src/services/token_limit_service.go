package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
)

// TokenLimitService Token限制和告警服务
type TokenLimitService struct {
	db           *gorm.DB
	usageService *UsageService
	logger       *LoggerService

	userLimits   map[string]*UserTokenLimit
	limitsMutex  sync.RWMutex

	alertCallbacks []AlertCallback

	stopCh chan struct{}
}

// UserTokenLimit 用户Token限制配置
type UserTokenLimit struct {
	UserID           string  `json:"user_id"`
	DailyLimit       int64   `json:"daily_limit"`        // 每日token限制（0表示无限制）
	MonthlyLimit     int64   `json:"monthly_limit"`      // 每月token限制（0表示无限制）
	DialogueLimit    int64   `json:"dialogue_limit"`     // 单对话token限制（0表示无限制）
	AlertThreshold   float64 `json:"alert_threshold"`    // 告警阈值（百分比，如0.8表示80%）
	AlertEnabled     bool    `json:"alert_enabled"`      // 是否启用告警
	LastAlertTime    time.Time `json:"last_alert_time"`  // 上次告警时间
	AlertCooldown    int     `json:"alert_cooldown"`     // 告警冷却时间（分钟）
}

// AlertCallback 告警回调函数
type AlertCallback func(ctx context.Context, alert *TokenAlert) error

// TokenAlert Token告警信息
type TokenAlert struct {
	UserID        string    `json:"user_id"`
	AlertType     string    `json:"alert_type"`     // daily, monthly, dialogue
	CurrentUsage  int64     `json:"current_usage"`
	Limit         int64     `json:"limit"`
	Percentage    float64   `json:"percentage"`
	Message       string    `json:"message"`
	Timestamp     time.Time `json:"timestamp"`
}

// NewTokenLimitService 创建Token限制服务
func NewTokenLimitService(db *gorm.DB, usageService *UsageService, logger *LoggerService) *TokenLimitService {
	service := &TokenLimitService{
		db:             db,
		usageService:   usageService,
		logger:         logger,
		userLimits:     make(map[string]*UserTokenLimit),
		alertCallbacks: make([]AlertCallback, 0),
		stopCh:         make(chan struct{}),
	}

	go service.startMonitoring()

	return service
}

// Stop 停止监控goroutine
func (s *TokenLimitService) Stop() {
	close(s.stopCh)
}

// SetUserLimit 设置用户Token限制
func (s *TokenLimitService) SetUserLimit(limit *UserTokenLimit) {
	s.limitsMutex.Lock()
	defer s.limitsMutex.Unlock()

	// 设置默认值
	if limit.AlertThreshold == 0 {
		limit.AlertThreshold = 0.8
	}
	if limit.AlertCooldown == 0 {
		limit.AlertCooldown = 30 // 默认30分钟冷却
	}

	s.userLimits[limit.UserID] = limit
}

// GetUserLimit 获取用户Token限制
func (s *TokenLimitService) GetUserLimit(userID string) *UserTokenLimit {
	s.limitsMutex.RLock()
	defer s.limitsMutex.RUnlock()

	if limit, ok := s.userLimits[userID]; ok {
		return limit
	}

	// 返回默认限制
	return &UserTokenLimit{
		UserID:         userID,
		DailyLimit:     100000,  // 默认每日10万token
		MonthlyLimit:   2000000, // 默认每月200万token
		DialogueLimit:  10000,   // 默认单对话1万token
		AlertThreshold: 0.8,
		AlertEnabled:   true,
		AlertCooldown:  30,
	}
}

// CheckLimit 检查用户是否超出Token限制
func (s *TokenLimitService) CheckLimit(ctx context.Context, userID string, estimatedTokens int64) (bool, string) {
	limit := s.GetUserLimit(userID)

	// 检查单对话限制
	if limit.DialogueLimit > 0 && estimatedTokens > limit.DialogueLimit {
		return false, fmt.Sprintf("单对话token数 %d 超过限制 %d", estimatedTokens, limit.DialogueLimit)
	}

	// 检查每日限制
	if limit.DailyLimit > 0 {
		dailyUsage := s.getDailyUsage(userID)
		if dailyUsage+estimatedTokens > limit.DailyLimit {
			return false, fmt.Sprintf("今日token使用量 %d 即将超过每日限制 %d", dailyUsage, limit.DailyLimit)
		}
	}

	// 检查每月限制
	if limit.MonthlyLimit > 0 {
		monthlyUsage := s.getMonthlyUsage(userID)
		if monthlyUsage+estimatedTokens > limit.MonthlyLimit {
			return false, fmt.Sprintf("本月token使用量 %d 即将超过每月限制 %d", monthlyUsage, limit.MonthlyLimit)
		}
	}

	return true, ""
}

// CheckAndAlert 检查并触发告警
func (s *TokenLimitService) CheckAndAlert(ctx context.Context, userID string) {
	limit := s.GetUserLimit(userID)
	if !limit.AlertEnabled {
		return
	}

	// 检查冷却时间
	if time.Since(limit.LastAlertTime) < time.Duration(limit.AlertCooldown)*time.Minute {
		return
	}

	// 检查每日使用量告警
	if limit.DailyLimit > 0 {
		dailyUsage := s.getDailyUsage(userID)
		percentage := float64(dailyUsage) / float64(limit.DailyLimit)

		if percentage >= limit.AlertThreshold {
			alert := &TokenAlert{
				UserID:       userID,
				AlertType:    "daily",
				CurrentUsage: dailyUsage,
				Limit:        limit.DailyLimit,
				Percentage:   percentage,
				Message:      fmt.Sprintf("今日token使用量已达 %.1f%% (%d/%d)", percentage*100, dailyUsage, limit.DailyLimit),
				Timestamp:    time.Now(),
			}
			s.triggerAlert(ctx, alert, limit)
		}
	}

	// 检查每月使用量告警
	if limit.MonthlyLimit > 0 {
		monthlyUsage := s.getMonthlyUsage(userID)
		percentage := float64(monthlyUsage) / float64(limit.MonthlyLimit)

		if percentage >= limit.AlertThreshold {
			alert := &TokenAlert{
				UserID:       userID,
				AlertType:    "monthly",
				CurrentUsage: monthlyUsage,
				Limit:        limit.MonthlyLimit,
				Percentage:   percentage,
				Message:      fmt.Sprintf("本月token使用量已达 %.1f%% (%d/%d)", percentage*100, monthlyUsage, limit.MonthlyLimit),
				Timestamp:    time.Now(),
			}
			s.triggerAlert(ctx, alert, limit)
		}
	}
}

// RegisterAlertCallback 注册告警回调
func (s *TokenLimitService) RegisterAlertCallback(callback AlertCallback) {
	s.alertCallbacks = append(s.alertCallbacks, callback)
}

// GetUserStats 获取用户Token使用统计
func (s *TokenLimitService) GetUserStats(userID string) map[string]interface{} {
	limit := s.GetUserLimit(userID)
	dailyUsage := s.getDailyUsage(userID)
	monthlyUsage := s.getMonthlyUsage(userID)

	dailyPercentage := 0.0
	if limit.DailyLimit > 0 {
		dailyPercentage = float64(dailyUsage) / float64(limit.DailyLimit) * 100
	}

	monthlyPercentage := 0.0
	if limit.MonthlyLimit > 0 {
		monthlyPercentage = float64(monthlyUsage) / float64(limit.MonthlyLimit) * 100
	}

	return map[string]interface{}{
		"user_id":            userID,
		"daily_usage":        dailyUsage,
		"daily_limit":        limit.DailyLimit,
		"daily_percentage":   fmt.Sprintf("%.1f%%", dailyPercentage),
		"monthly_usage":      monthlyUsage,
		"monthly_limit":      limit.MonthlyLimit,
		"monthly_percentage": fmt.Sprintf("%.1f%%", monthlyPercentage),
		"alert_enabled":      limit.AlertEnabled,
		"alert_threshold":    fmt.Sprintf("%.0f%%", limit.AlertThreshold*100),
	}
}

// getDailyUsage 获取用户当日token使用量
func (s *TokenLimitService) getDailyUsage(userID string) int64 {
	if s.usageService == nil {
		return 0
	}

	today := time.Now().Format("2006-01-02")
	usage, err := s.usageService.GetDailyUsage(userID, today)
	if err != nil {
		s.logger.Error(context.Background(), "Failed to get daily usage for user %s: %v", userID, err)
		return 0
	}

	return usage.TotalTokens
}

// getMonthlyUsage 获取用户当月token使用量
func (s *TokenLimitService) getMonthlyUsage(userID string) int64 {
	if s.usageService == nil {
		return 0
	}

	month := time.Now().Format("2006-01")
	usage, err := s.usageService.GetMonthlyUsage(userID, month)
	if err != nil {
		s.logger.Error(context.Background(), "Failed to get monthly usage for user %s: %v", userID, err)
		return 0
	}

	return usage.TotalTokens
}

// triggerAlert 触发告警
func (s *TokenLimitService) triggerAlert(ctx context.Context, alert *TokenAlert, limit *UserTokenLimit) {
	// 更新上次告警时间
	limit.LastAlertTime = time.Now()

	// 记录告警日志
	s.logger.Warn(ctx, "Token alert for user %s: %s", alert.UserID, alert.Message)

	// 执行告警回调
	for _, callback := range s.alertCallbacks {
		go func(cb AlertCallback) {
			if err := cb(ctx, alert); err != nil {
				s.logger.Error(ctx, "Alert callback failed: %v", err)
			}
		}(callback)
	}
}

// startMonitoring 启动后台监控
func (s *TokenLimitService) startMonitoring() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.limitsMutex.RLock()
			userIDs := make([]string, 0, len(s.userLimits))
			for userID := range s.userLimits {
				userIDs = append(userIDs, userID)
			}
			s.limitsMutex.RUnlock()

			ctx := context.Background()
			for _, userID := range userIDs {
				s.CheckAndAlert(ctx, userID)
			}
		}
	}
}

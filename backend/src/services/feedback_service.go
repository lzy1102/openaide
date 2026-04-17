package services

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
)

// FeedbackService 反馈服务
type FeedbackService struct {
	db *gorm.DB
}

// NewFeedbackService 创建反馈服务实例
func NewFeedbackService(db *gorm.DB) *FeedbackService {
	return &FeedbackService{db: db}
}

// CreateFeedback 创建反馈
func (s *FeedbackService) CreateFeedback(feedback *models.Feedback) error {
	feedback.ID = uuid.New().String()
	feedback.CreatedAt = time.Now()
	return s.db.Create(feedback).Error
}

// GetFeedbackByTask 获取任务反馈
func (s *FeedbackService) GetFeedbackByTask(taskID string) ([]models.Feedback, error) {
	var feedbacks []models.Feedback
	err := s.db.Where("task_id = ?", taskID).Find(&feedbacks).Error
	return feedbacks, err
}

// GetAverageRating 获取平均评分
func (s *FeedbackService) GetAverageRating(taskType string) (float64, error) {
	var result struct {
		AvgRating float64
	}
	err := s.db.Model(&models.Feedback{}).
		Where("task_type = ?", taskType).
		Select("AVG(rating) as avg_rating").
		Scan(&result).Error
	return result.AvgRating, err
}

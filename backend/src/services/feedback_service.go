package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
)

type FeedbackService struct {
	db            *gorm.DB
	learningSvc   *LearningService
	eventBus      *EventBus
}

func NewFeedbackService(db *gorm.DB) *FeedbackService {
	return &FeedbackService{db: db}
}

func (s *FeedbackService) SetLearningService(ls *LearningService) {
	s.learningSvc = ls
}

func (s *FeedbackService) SetEventBus(eb *EventBus) {
	s.eventBus = eb
}

func (s *FeedbackService) CreateFeedback(feedback *models.Feedback) error {
	feedback.ID = uuid.New().String()
	feedback.CreatedAt = time.Now()

	if err := s.db.Create(feedback).Error; err != nil {
		return err
	}

	if s.learningSvc != nil && feedback.TaskID != "" {
		go func() {
			if err := s.learningSvc.LearnFromFeedback(context.Background(), feedback.TaskID); err != nil {
				slog.Error("Auto learn from feedback failed", "component", "Feedback", "task_id", feedback.TaskID, "error", err)
			} else {
				slog.Info("Auto learned from feedback", "component", "Feedback", "task_id", feedback.TaskID)
			}
		}()
	}

	if s.eventBus != nil {
		s.eventBus.Publish(context.Background(), models.EventTopicFeedback, "feedback_created", "feedback_service", map[string]interface{}{
			"task_id":   feedback.TaskID,
			"task_type": feedback.TaskType,
			"rating":    feedback.Rating,
		})
	}

	return nil
}

func (s *FeedbackService) GetFeedbackByTask(taskID string) ([]models.Feedback, error) {
	var feedbacks []models.Feedback
	err := s.db.Where("task_id = ?", taskID).Find(&feedbacks).Error
	return feedbacks, err
}

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

package services

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
)

// ConfirmationService 确认服务
type ConfirmationService struct {
	db *gorm.DB
}

// NewConfirmationService 创建确认服务实例
func NewConfirmationService(db *gorm.DB) *ConfirmationService {
	return &ConfirmationService{db: db}
}

// CreateConfirmation 创建确认
func (s *ConfirmationService) CreateConfirmation(confirmation *models.Confirmation) error {
	confirmation.ID = uuid.New().String()
	confirmation.Status = "pending"
	confirmation.CreatedAt = time.Now()
	confirmation.UpdatedAt = time.Now()
	if confirmation.ExpiresAt.IsZero() {
		confirmation.ExpiresAt = time.Now().Add(24 * time.Hour)
	}
	return s.db.Create(confirmation).Error
}

// UpdateConfirmation 更新确认
func (s *ConfirmationService) UpdateConfirmation(confirmation *models.Confirmation) error {
	confirmation.UpdatedAt = time.Now()
	return s.db.Save(confirmation).Error
}

// DeleteConfirmation 删除确认
func (s *ConfirmationService) DeleteConfirmation(id string) error {
	return s.db.Where("id = ?", id).Delete(&models.Confirmation{}).Error
}

// GetConfirmation 获取确认
func (s *ConfirmationService) GetConfirmation(id string) (*models.Confirmation, error) {
	var confirmation models.Confirmation
	err := s.db.First(&confirmation, id).Error
	return &confirmation, err
}

// ListConfirmations 列出所有确认
func (s *ConfirmationService) ListConfirmations() ([]models.Confirmation, error) {
	var confirmations []models.Confirmation
	err := s.db.Find(&confirmations).Error
	return confirmations, err
}

// ConfirmTask 确认任务
func (s *ConfirmationService) ConfirmTask(id string) (*models.Confirmation, error) {
	confirmation, err := s.GetConfirmation(id)
	if err != nil {
		return nil, err
	}

	confirmation.Status = "confirmed"
	confirmation.UpdatedAt = time.Now()
	s.db.Save(confirmation)

	return confirmation, nil
}

// RejectTask 拒绝任务
func (s *ConfirmationService) RejectTask(id string) (*models.Confirmation, error) {
	confirmation, err := s.GetConfirmation(id)
	if err != nil {
		return nil, err
	}

	confirmation.Status = "rejected"
	confirmation.UpdatedAt = time.Now()
	s.db.Save(confirmation)

	return confirmation, nil
}

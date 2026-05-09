package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
)

type SessionBranch struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	DialogueID  string    `json:"dialogue_id" gorm:"index"`
	ParentID    string    `json:"parent_id,omitempty" gorm:"index"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	BranchPoint int       `json:"branch_point"`
	IsActive    bool      `json:"is_active" gorm:"default:true"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
	UserID      string    `json:"user_id" gorm:"index"`
}

func (SessionBranch) TableName() string {
	return "session_branches"
}

type SessionBranchService struct {
	BaseService
	mu       sync.RWMutex
	eventBus *EventBus
}

func NewSessionBranchService(db *gorm.DB, cache *CacheService, eventBus *EventBus) *SessionBranchService {
	s := &SessionBranchService{
		BaseService: BaseService{DB: db, Cache: cache},
		eventBus:    eventBus,
	}
	s.autoMigrate()
	return s
}

func (s *SessionBranchService) autoMigrate() {
	if s.DB != nil {
		s.DB.AutoMigrate(&SessionBranch{})
	}
}

func (s *SessionBranchService) Fork(ctx context.Context, dialogueID, userID, name, description string, branchPoint int) (*SessionBranch, error) {
	branch := &SessionBranch{
		ID:          uuid.New().String(),
		DialogueID:  dialogueID,
		Name:        name,
		Description: description,
		BranchPoint: branchPoint,
		IsActive:    true,
		UserID:      userID,
		CreatedAt:   time.Now(),
	}

	if err := s.DB.Create(branch).Error; err != nil {
		return nil, fmt.Errorf("failed to create branch: %w", err)
	}

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, models.EventTopicMessage, "session_forked", "session_branch", map[string]interface{}{
			"branch_id":    branch.ID,
			"dialogue_id":  dialogueID,
			"branch_point": branchPoint,
			"name":         name,
		})
	}

	slog.Info("Session branched", "component", "SessionBranch", "dialogue_id", dialogueID, "branch", name, "point", branchPoint)
	return branch, nil
}

func (s *SessionBranchService) ListBranches(ctx context.Context, dialogueID string) ([]SessionBranch, error) {
	var branches []SessionBranch
	err := s.DB.Where("dialogue_id = ?", dialogueID).Order("created_at DESC").Find(&branches).Error
	return branches, err
}

func (s *SessionBranchService) GetBranch(ctx context.Context, branchID string) (*SessionBranch, error) {
	var branch SessionBranch
	err := s.DB.Where("id = ?", branchID).First(&branch).Error
	return &branch, err
}

func (s *SessionBranchService) SwitchToBranch(ctx context.Context, dialogueID, branchID string) error {
	s.DB.Model(&SessionBranch{}).Where("dialogue_id = ?", dialogueID).Update("is_active", false)
	return s.DB.Model(&SessionBranch{}).Where("id = ?", branchID).Update("is_active", true).Error
}

func (s *SessionBranchService) GetActiveBranch(ctx context.Context, dialogueID string) (*SessionBranch, error) {
	var branch SessionBranch
	err := s.DB.Where("dialogue_id = ? AND is_active = ?", dialogueID, true).First(&branch).Error
	if err != nil {
		return nil, err
	}
	return &branch, nil
}

func (s *SessionBranchService) DeleteBranch(ctx context.Context, branchID string) error {
	return s.DB.Where("id = ?", branchID).Delete(&SessionBranch{}).Error
}

func (s *SessionBranchService) RenameBranch(ctx context.Context, branchID, name string) error {
	return s.DB.Model(&SessionBranch{}).Where("id = ?", branchID).Update("name", name).Error
}

func (s *SessionBranchService) GetBranchTimeline(ctx context.Context, dialogueID string) ([]SessionBranch, error) {
	var branches []SessionBranch
	err := s.DB.Where("dialogue_id = ?", dialogueID).Order("branch_point ASC, created_at ASC").Find(&branches).Error
	return branches, err
}

func (s *SessionBranchService) BuildBranchContext(ctx context.Context, dialogueID string) string {
	branches, err := s.ListBranches(ctx, dialogueID)
	if err != nil || len(branches) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[Session Branches]\n")
	for _, b := range branches {
		active := ""
		if b.IsActive {
			active = " ← active"
		}
		sb.WriteString(fmt.Sprintf("- %s (point: %d)%s\n", b.Name, b.BranchPoint, active))
		if b.Description != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", b.Description))
		}
	}
	return sb.String()
}

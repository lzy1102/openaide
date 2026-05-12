package services

import (
	"context"
	"errors"
	"time"

	"openaide/backend/src/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrCannotDeleteDefault = errors.New("cannot delete default project")

type ProjectService struct {
	BaseService
}

func NewProjectService(db *gorm.DB, cache *CacheService) *ProjectService {
	s := &ProjectService{
		BaseService: BaseService{DB: db, Cache: cache},
	}
	s.autoMigrate()
	s.ensureDefault()
	return s
}

func (s *ProjectService) autoMigrate() {
	if s.DB != nil {
		s.DB.AutoMigrate(&models.Project{})
	}
}

func (s *ProjectService) ensureDefault() {
	var count int64
	s.DB.Model(&models.Project{}).Where("is_default = ?", true).Count(&count)
	if count == 0 {
		s.DB.Create(&models.Project{
			ID:          uuid.New().String(),
			Name:        "Default",
			Description: "Default project",
			IsDefault:   true,
			SortOrder:   0,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		})
	}
}

func (s *ProjectService) CreateProject(ctx context.Context, name, description, color, icon, systemPrompt, modelID, workingDir string) (*models.Project, error) {
	project := &models.Project{
		ID:           uuid.New().String(),
		Name:         name,
		Description:  description,
		Color:        color,
		Icon:         icon,
		SystemPrompt: systemPrompt,
		ModelID:      modelID,
		WorkingDir:   workingDir,
		SortOrder:    0,
		IsDefault:    false,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := s.DB.Create(project).Error; err != nil {
		return nil, err
	}
	return project, nil
}

func (s *ProjectService) GetProject(id string) (*models.Project, error) {
	var project models.Project
	if err := s.DB.Where("id = ?", id).First(&project).Error; err != nil {
		return nil, err
	}
	return &project, nil
}

func (s *ProjectService) ListProjects() ([]models.Project, error) {
	var projects []models.Project
	err := s.DB.Order("sort_order ASC, created_at ASC").Find(&projects).Error
	return projects, err
}

func (s *ProjectService) UpdateProject(id string, updates map[string]interface{}) (*models.Project, error) {
	updates["updated_at"] = time.Now()
	if err := s.DB.Model(&models.Project{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetProject(id)
}

func (s *ProjectService) DeleteProject(id string) error {
	var project models.Project
	if err := s.DB.Where("id = ?", id).First(&project).Error; err != nil {
		return err
	}
	if project.IsDefault {
		return ErrCannotDeleteDefault
	}
	s.DB.Model(&models.Dialogue{}).Where("project_id = ?", id).Update("project_id", "")
	return s.DB.Delete(&models.Project{}, "id = ?", id).Error
}

func (s *ProjectService) GetDefaultProject() (*models.Project, error) {
	var project models.Project
	if err := s.DB.Where("is_default = ?", true).First(&project).Error; err != nil {
		return nil, err
	}
	return &project, nil
}

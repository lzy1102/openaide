package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
)

// AutomationService 自动化服务
type AutomationService struct {
	db *gorm.DB
}

// NewAutomationService 创建自动化服务实例
func NewAutomationService(db *gorm.DB) *AutomationService {
	return &AutomationService{db: db}
}

// CreateAutomationExecution 创建自动化执行
func (s *AutomationService) CreateAutomationExecution(execution *models.AutomationExecution) error {
	execution.ID = uuid.New().String()
	execution.Status = "pending"
	execution.CreatedAt = time.Now()
	execution.UpdatedAt = time.Now()

	return s.db.Create(execution).Error
}

// UpdateAutomationExecution 更新自动化执行
func (s *AutomationService) UpdateAutomationExecution(execution *models.AutomationExecution) error {
	execution.UpdatedAt = time.Now()
	return s.db.Save(execution).Error
}

// DeleteAutomationExecution 删除自动化执行
func (s *AutomationService) DeleteAutomationExecution(id string) error {
	return s.db.Where("id = ?", id).Delete(&models.AutomationExecution{}).Error
}

// GetAutomationExecution 获取自动化执行
func (s *AutomationService) GetAutomationExecution(id string) (*models.AutomationExecution, error) {
	var execution models.AutomationExecution
	err := s.db.First(&execution, id).Error
	return &execution, err
}

// ListAutomationExecutions 列出所有自动化执行
func (s *AutomationService) ListAutomationExecutions() ([]models.AutomationExecution, error) {
	var executions []models.AutomationExecution
	err := s.db.Find(&executions).Error
	return executions, err
}

// GetExecutionActions 获取自动化执行的动作列表
func GetExecutionActions(e *models.AutomationExecution) ([]models.AutomationActionData, error) {
	if e.Actions == "" {
		return nil, nil
	}
	var actions []models.AutomationActionData
	err := json.Unmarshal([]byte(e.Actions), &actions)
	return actions, err
}

// SetExecutionActions 设置自动化执行的动作列表
func SetExecutionActions(e *models.AutomationExecution, actions []models.AutomationActionData) error {
	data, err := json.Marshal(actions)
	if err != nil {
		return err
	}
	e.Actions = string(data)
	return nil
}

// ExecuteAutomationExecution 执行自动化
func (s *AutomationService) ExecuteAutomationExecution(id string) (*models.AutomationExecution, error) {
	execution, err := s.GetAutomationExecution(id)
	if err != nil {
		return nil, err
	}

	execution.Status = "running"
	execution.LastRunAt = time.Now()
	s.db.Save(execution)

	// 获取动作列表
	actions, err := GetExecutionActions(execution)
	if err != nil {
		execution.Status = "failed"
		execution.UpdatedAt = time.Now()
		s.db.Save(execution)
		return execution, err
	}

	// 执行每个动作
	allCompleted := true
	for i := range actions {
		action := &actions[i]
		action.Status = "running"
		action.UpdatedAt = time.Now()

		// 执行动作
		result, err := s.executeAction(action)
		if err != nil {
			action.Status = "failed"
			action.Error = err.Error()
			allCompleted = false
		} else {
			action.Status = "completed"
			action.Result = result
		}
		action.UpdatedAt = time.Now()
	}

	// 保存更新后的动作列表
	SetExecutionActions(execution, actions)

	if allCompleted {
		execution.Status = "completed"
	} else {
		execution.Status = "failed"
	}
	execution.UpdatedAt = time.Now()
	s.db.Save(execution)

	return execution, nil
}

// executeAction 执行自动化动作
func (s *AutomationService) executeAction(action *models.AutomationActionData) (interface{}, error) {
	switch action.Type {
	case "skill":
		return s.executeSkillAction(action)
	case "plugin":
		return s.executePluginAction(action)
	case "model":
		return s.executeModelAction(action)
	default:
		return nil, fmt.Errorf("action type %s not implemented", action.Type)
	}
}

// executeSkillAction 执行技能动作
func (s *AutomationService) executeSkillAction(action *models.AutomationActionData) (interface{}, error) {
	return map[string]string{
		"message": fmt.Sprintf("Skill action executed: %s", action.TargetID),
	}, nil
}

// executePluginAction 执行插件动作
func (s *AutomationService) executePluginAction(action *models.AutomationActionData) (interface{}, error) {
	return map[string]string{
		"message": fmt.Sprintf("Plugin action executed: %s", action.TargetID),
	}, nil
}

// executeModelAction 执行模型动作
func (s *AutomationService) executeModelAction(action *models.AutomationActionData) (interface{}, error) {
	return map[string]string{
		"message": fmt.Sprintf("Model action executed: %s", action.TargetID),
	}, nil
}

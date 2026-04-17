package services

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
)

// CodeService 代码执行服务
type CodeService struct {
	db *gorm.DB
}

// NewCodeService 创建代码执行服务实例
func NewCodeService(db *gorm.DB) *CodeService {
	return &CodeService{db: db}
}

// CreateCodeExecution 创建代码执行
func (s *CodeService) CreateCodeExecution(execution *models.CodeExecution) error {
	execution.ID = uuid.New().String()
	execution.Status = "pending"
	execution.CreatedAt = time.Now()
	execution.UpdatedAt = time.Now()
	return s.db.Create(execution).Error
}

// UpdateCodeExecution 更新代码执行
func (s *CodeService) UpdateCodeExecution(execution *models.CodeExecution) error {
	execution.UpdatedAt = time.Now()
	return s.db.Save(execution).Error
}

// DeleteCodeExecution 删除代码执行
func (s *CodeService) DeleteCodeExecution(id string) error {
	return s.db.Where("id = ?", id).Delete(&models.CodeExecution{}).Error
}

// GetCodeExecution 获取代码执行
func (s *CodeService) GetCodeExecution(id string) (*models.CodeExecution, error) {
	var execution models.CodeExecution
	err := s.db.First(&execution, id).Error
	return &execution, err
}

// ListCodeExecutions 列出所有代码执行
func (s *CodeService) ListCodeExecutions() ([]models.CodeExecution, error) {
	var executions []models.CodeExecution
	err := s.db.Find(&executions).Error
	return executions, err
}

// ExecuteCodeExecution 执行代码
func (s *CodeService) ExecuteCodeExecution(id string) (*models.CodeExecution, error) {
	execution, err := s.GetCodeExecution(id)
	if err != nil {
		return nil, err
	}

	execution.Status = "running"
	s.db.Save(execution)

	startTime := time.Now()

	// 执行代码
	output, err := s.executeCode(execution.Language, execution.Code, execution.Parameters)
	execution.ExecutionTime = time.Since(startTime).Seconds()

	if err != nil {
		execution.Status = "failed"
		execution.Error = err.Error()
	} else {
		execution.Status = "completed"
		execution.Output = output
	}

	execution.UpdatedAt = time.Now()
	s.db.Save(execution)

	return execution, nil
}

// executeCode 执行代码
func (s *CodeService) executeCode(language, code string, parameters map[string]interface{}) (string, error) {
	// 这里实现代码执行逻辑
	// 实际项目中，这里应该使用安全的沙箱环境执行代码
	switch language {
	case "python":
		return s.executePython(code, parameters)
	case "javascript":
		return s.executeJavaScript(code, parameters)
	case "go":
		return s.executeGo(code, parameters)
	default:
		return "", fmt.Errorf("language %s not supported", language)
	}
}

// executePython 执行Python代码
func (s *CodeService) executePython(code string, parameters map[string]interface{}) (string, error) {
	// 示例Python代码执行
	return fmt.Sprintf("Python code executed:\n%s", code), nil
}

// executeJavaScript 执行JavaScript代码
func (s *CodeService) executeJavaScript(code string, parameters map[string]interface{}) (string, error) {
	// 示例JavaScript代码执行
	return fmt.Sprintf("JavaScript code executed:\n%s", code), nil
}

// executeGo 执行Go代码
func (s *CodeService) executeGo(code string, parameters map[string]interface{}) (string, error) {
	// 示例Go代码执行
	return fmt.Sprintf("Go code executed:\n%s", code), nil
}

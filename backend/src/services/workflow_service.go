package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// WorkflowService 工作流服务
type WorkflowService struct {
	db              *gorm.DB
	llmClient       llm.LLMClient
	executors       map[string]StepExecutor
	templateCache   map[string]*WorkflowTemplate
	templateMutex   sync.RWMutex
	runningInstances map[string]*ExecutionContext
	instanceMutex   sync.RWMutex
}

// StepExecutor 步骤执行器接口
type StepExecutor interface {
	Execute(ctx context.Context, step *models.WorkflowStep, input map[string]interface{}) (map[string]interface{}, error)
	CanHandle(stepType string) bool
}

// ExecutionContext 执行上下文
type ExecutionContext struct {
	Instance      *models.WorkflowInstance
	Workflow      *models.Workflow
	Context       context.Context
	CancelFunc    context.CancelFunc
	Variables     map[string]interface{}
	CompletedSteps map[string]*models.StepInstance
	StartedAt     time.Time
}

// WorkflowTemplate 工作流模板
type WorkflowTemplate struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Category    string               `json:"category"`
	Version     string               `json:"version"`
	Steps       []TemplateStep       `json:"steps"`
	Variables   []TemplateVariable   `json:"variables"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
}

// TemplateStep 模板步骤
type TemplateStep struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Type        string                 `json:"type"`
	Parameters  map[string]interface{} `json:"parameters"`
	Order       int                    `json:"order"`
	Timeout     time.Duration          `json:"timeout"`
	RetryPolicy *RetryPolicy           `json:"retry_policy,omitempty"`
	Condition   *ConditionalBranch     `json:"condition,omitempty"`
	Parallel    bool                   `json:"parallel"`
	DependsOn   []string               `json:"depends_on"`
}

// ConditionalBranch 条件分支
type ConditionalBranch struct {
	Type      string                         `json:"type"` // expression, output, error
	Rules     []ConditionRule                `json:"rules"`
	DefaultRoute string                      `json:"default_route"`
}

// ConditionRule 条件规则
type ConditionRule struct {
	Expression string                        `json:"expression"`
	Operator  string                        `json:"operator"` // equals, contains, greater, less, exists
	Value     interface{}                   `json:"value"`
	NextStep  string                        `json:"next_step"`
}

// TemplateVariable 模板变量
type TemplateVariable struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Default     interface{} `json:"default"`
	Required    bool        `json:"required"`
	Description string      `json:"description"`
}

// RetryPolicy 重试策略
type RetryPolicy struct {
	MaxRetries    int           `json:"max_retries"`
	InitialDelay  time.Duration `json:"initial_delay"`
	MaxDelay      time.Duration `json:"max_delay"`
	BackoffFactor float64       `json:"backoff_factor"`
	RetryOnError []string      `json:"retry_on_error"`
}

// WorkflowStatus 工作流状态
type WorkflowStatus string

const (
	StatusPending   WorkflowStatus = "pending"
	StatusRunning   WorkflowStatus = "running"
	StatusCompleted WorkflowStatus = "completed"
	StatusFailed    WorkflowStatus = "failed"
	StatusCancelled WorkflowStatus = "cancelled"
	StatusPaused    WorkflowStatus = "paused"
)

// StepStatus 步骤状态
type StepStatus string

const (
	StepStatusPending   StepStatus = "pending"
	StepStatusRunning   StepStatus = "running"
	StepStatusCompleted StepStatus = "completed"
	StepStatusFailed    StepStatus = "failed"
	StepStatusSkipped   StepStatus = "skipped"
	StepStatusRetrying  StepStatus = "retrying"
)

// NewWorkflowService 创建工作流服务实例
func NewWorkflowService(db *gorm.DB, llmClient llm.LLMClient) *WorkflowService {
	service := &WorkflowService{
		db:              db,
		llmClient:       llmClient,
		executors:       make(map[string]StepExecutor),
		templateCache:   make(map[string]*WorkflowTemplate),
		runningInstances: make(map[string]*ExecutionContext),
	}

	// 注册默认执行器
	service.RegisterExecutor(&LLMStepExecutor{llmClient: llmClient})
	service.RegisterExecutor(&CodeStepExecutor{})
	service.RegisterExecutor(&PluginStepExecutor{})
	service.RegisterExecutor(&AutomationStepExecutor{})
	service.RegisterExecutor(&ConditionStepExecutor{})
	service.RegisterExecutor(&ParallelStepExecutor{})

	// 加载内置模板
	service.loadBuiltinTemplates()

	return service
}

// RegisterExecutor 注册步骤执行器
func (s *WorkflowService) RegisterExecutor(executor StepExecutor) {
	if executor != nil {
		// 通过执行器的 CanHandle 方法获取支持的类型
		// 这里简化处理，实际使用中可以让执行器自行注册
	}
}

// ListWorkflows 列出所有工作流
func (s *WorkflowService) ListWorkflows() []models.Workflow {
	var workflows []models.Workflow
	s.db.Find(&workflows)
	return workflows
}

// ListWorkflowsByPage 分页列出工作流
func (s *WorkflowService) ListWorkflowsByPage(page, pageSize int, filters map[string]interface{}) ([]models.Workflow, int64) {
	var workflows []models.Workflow
	var total int64

	query := s.db.Model(&models.Workflow{})

	// 应用过滤条件
	if name, ok := filters["name"]; ok {
		query = query.Where("name LIKE ?", "%"+name.(string)+"%")
	}

	// 获取总数
	query.Count(&total)

	// 分页查询
	offset := (page - 1) * pageSize
	query.Offset(offset).Limit(pageSize).Find(&workflows)

	return workflows, total
}

// CreateWorkflow 创建新工作流
func (s *WorkflowService) CreateWorkflow(name, description string, steps []models.WorkflowStep) models.Workflow {
	workflow := models.Workflow{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		Steps:       steps,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// 为每个步骤设置工作流ID和ID
	for i := range workflow.Steps {
		if workflow.Steps[i].ID == "" {
			workflow.Steps[i].ID = uuid.New().String()
		}
		workflow.Steps[i].WorkflowID = workflow.ID
		workflow.Steps[i].Order = i
	}

	s.db.Create(&workflow)
	return workflow
}

// CreateWorkflowFromTemplate 从模板创建工作流
func (s *WorkflowService) CreateWorkflowFromTemplate(templateID string, variables map[string]interface{}) (models.Workflow, error) {
	template, err := s.GetTemplate(templateID)
	if err != nil {
		return models.Workflow{}, fmt.Errorf("template not found: %w", err)
	}

	// 应用变量替换
	steps := make([]models.WorkflowStep, len(template.Steps))
	for i, templateStep := range template.Steps {
		parameters := s.applyVariables(templateStep.Parameters, variables)

		steps[i] = models.WorkflowStep{
			ID:          uuid.New().String(),
			Name:        templateStep.Name,
			Description: templateStep.Description,
			Type:        templateStep.Type,
			Parameters:  parameters,
			Order:       i,
		}
	}

	workflow := models.Workflow{
		ID:          uuid.New().String(),
		Name:        s.applyVariablesToString(template.Name, variables),
		Description: s.applyVariablesToString(template.Description, variables),
		Steps:       steps,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	s.db.Create(&workflow)
	return workflow, nil
}

// applyVariables 应用变量替换
func (s *WorkflowService) applyVariables(params map[string]interface{}, variables map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range params {
		switch val := v.(type) {
		case string:
			result[k] = s.applyVariablesToString(val, variables)
		case map[string]interface{}:
			result[k] = s.applyVariables(val, variables)
		case []interface{}:
			result[k] = s.applyVariablesToArray(val, variables)
		default:
			result[k] = v
		}
	}
	return result
}

// applyVariablesToString 应用变量替换到字符串
func (s *WorkflowService) applyVariablesToString(str string, variables map[string]interface{}) string {
	result := str
	for k, v := range variables {
		placeholder := fmt.Sprintf("{{.%s}}", k)
		result = fmt.Sprintf("%s", replaceAll(result, placeholder, fmt.Sprintf("%v", v)))
	}
	return result
}

// applyVariablesToArray 应用变量替换到数组
func (s *WorkflowService) applyVariablesToArray(arr []interface{}, variables map[string]interface{}) []interface{} {
	result := make([]interface{}, len(arr))
	for i, v := range arr {
		switch val := v.(type) {
		case string:
			result[i] = s.applyVariablesToString(val, variables)
		case map[string]interface{}:
			result[i] = s.applyVariables(val, variables)
		case []interface{}:
			result[i] = s.applyVariablesToArray(val, variables)
		default:
			result[i] = v
		}
	}
	return result
}

// GetWorkflow 获取工作流详情
func (s *WorkflowService) GetWorkflow(id string) (models.Workflow, bool) {
	var workflow models.Workflow
	err := s.db.Preload("Steps").Where("id = ?", id).First(&workflow).Error
	return workflow, err == nil
}

// UpdateWorkflow 更新工作流
func (s *WorkflowService) UpdateWorkflow(id, name, description string, steps []models.WorkflowStep) (models.Workflow, bool) {
	var workflow models.Workflow
	err := s.db.Where("id = ?", id).First(&workflow).Error
	if err != nil {
		return models.Workflow{}, false
	}

	workflow.Name = name
	workflow.Description = description
	workflow.Steps = steps
	workflow.UpdatedAt = time.Now()

	// 为每个步骤设置工作流ID和ID
	for j := range workflow.Steps {
		if workflow.Steps[j].ID == "" {
			workflow.Steps[j].ID = uuid.New().String()
		}
		workflow.Steps[j].WorkflowID = id
		workflow.Steps[j].Order = j
	}

	s.db.Save(&workflow)
	return workflow, true
}

// DeleteWorkflow 删除工作流
func (s *WorkflowService) DeleteWorkflow(id string) bool {
	err := s.db.Where("id = ?", id).Delete(&models.Workflow{}).Error
	return err == nil
}

// CreateWorkflowInstance 创建工作流实例
func (s *WorkflowService) CreateWorkflowInstance(workflowID string, inputVariables map[string]interface{}) (models.WorkflowInstance, bool) {
	// 检查工作流是否存在
	workflow, found := s.GetWorkflow(workflowID)
	if !found {
		return models.WorkflowInstance{}, false
	}

	// 创建工作流实例
	instance := models.WorkflowInstance{
		ID:          uuid.New().String(),
		WorkflowID:  workflowID,
		Status:      string(StatusPending),
		Steps:       make([]models.StepInstance, 0),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// 为每个步骤创建实例
	for _, step := range workflow.Steps {
		stepInstance := models.StepInstance{
			ID:                 uuid.New().String(),
			WorkflowInstanceID: instance.ID,
			StepID:             step.ID,
			Name:               step.Name,
			Status:             string(StepStatusPending),
			Output:             make(map[string]interface{}),
		}
		instance.Steps = append(instance.Steps, stepInstance)
	}

	s.db.Create(&instance)
	return instance, true
}

// ExecuteWorkflowInstance 执行工作流实例（增强版）
func (s *WorkflowService) ExecuteWorkflowInstance(instanceID string) (models.WorkflowInstance, error) {
	return s.ExecuteWorkflowWithContext(context.Background(), instanceID)
}

// ExecuteWorkflowWithContext 使用上下文执行工作流
func (s *WorkflowService) ExecuteWorkflowWithContext(ctx context.Context, instanceID string) (models.WorkflowInstance, error) {
	// 查找工作流实例
	var instance models.WorkflowInstance
	err := s.db.First(&instance, instanceID).Error
	if err != nil {
		return models.WorkflowInstance{}, fmt.Errorf("workflow instance not found: %w", err)
	}

	// 查找工作流定义
	workflow, found := s.GetWorkflow(instance.WorkflowID)
	if !found {
		return models.WorkflowInstance{}, fmt.Errorf("workflow not found")
	}

	// 创建执行上下文
	execCtx, cancel := context.WithCancel(ctx)
	executionCtx := &ExecutionContext{
		Instance:       &instance,
		Workflow:       &workflow,
		Context:        execCtx,
		CancelFunc:     cancel,
		Variables:      make(map[string]interface{}),
		CompletedSteps: make(map[string]*models.StepInstance),
		StartedAt:      time.Now(),
	}

	// 注册运行中的实例
	s.instanceMutex.Lock()
	s.runningInstances[instanceID] = executionCtx
	s.instanceMutex.Unlock()

	// 清理函数
	defer func() {
		s.instanceMutex.Lock()
		delete(s.runningInstances, instanceID)
		s.instanceMutex.Unlock()
	}()

	// 更新实例状态为运行中
	s.updateInstanceStatus(&instance, string(StatusRunning))
	defer func() {
		if instance.Status == string(StatusRunning) {
			// 如果仍处于运行状态，设置为完成
			s.updateInstanceStatus(&instance, string(StatusCompleted))
		}
	}()

	// 执行工作流
	err = s.executeWorkflowSteps(executionCtx)
	if err != nil {
		s.updateInstanceStatus(&instance, string(StatusFailed))
		return instance, fmt.Errorf("workflow execution failed: %w", err)
	}

	// 保存最终状态
	s.db.Save(&instance)
	return instance, nil
}

// executeWorkflowSteps 执行工作流步骤
func (s *WorkflowService) executeWorkflowSteps(execCtx *ExecutionContext) error {
	workflow := execCtx.Workflow

	// 构建步骤依赖图
	stepGraph := s.buildStepGraph(workflow.Steps)

	// 按依赖顺序执行步骤
	executedSteps := make(map[string]bool)
	pendingSteps := s.getRootSteps(stepGraph)

	for len(pendingSteps) > 0 {
		// 检查上下文是否已取消
		select {
		case <-execCtx.Context.Done():
			return fmt.Errorf("workflow execution cancelled")
		default:
		}

		// 批量执行可并行执行的步骤
		var nextBatch []string
		for _, stepID := range pendingSteps {
			if !executedSteps[stepID] && s.allDependenciesExecuted(stepID, stepGraph, executedSteps) {
				nextBatch = append(nextBatch, stepID)
			}
		}

		if len(nextBatch) == 0 {
			// 没有可执行的步骤，可能存在循环依赖
			break
		}

		// 执行这一批步骤
		var wg sync.WaitGroup
		errors := make(chan error, len(nextBatch))

		for _, stepID := range nextBatch {
			step := s.findStepByID(workflow.Steps, stepID)
			if step == nil {
				continue
			}

			// 检查是否是并行步骤
			if s.canExecuteParallel(step) {
				wg.Add(1)
				go func(step *models.WorkflowStep) {
					defer wg.Done()
					if err := s.executeStepWithRetry(execCtx, step); err != nil {
						errors <- err
					}
				}(step)
			} else {
				if err := s.executeStepWithRetry(execCtx, step); err != nil {
					return err
				}
				executedSteps[stepID] = true
			}
		}

		// 等待并行步骤完成
		wg.Wait()
		close(errors)

		// 检查错误
		for err := range errors {
			if err != nil {
				return err
			}
		}

		// 标记步骤为已执行
		for _, stepID := range nextBatch {
			executedSteps[stepID] = true
		}

		// 获取下一批待执行步骤
		pendingSteps = s.getNextPendingSteps(stepGraph, executedSteps)
	}

	return nil
}

// StepGraph 步骤依赖图
type StepGraph struct {
	Nodes map[string]*models.WorkflowStep
	Edges map[string][]string // 步骤ID -> 依赖的步骤ID列表
}

// buildStepGraph 构建步骤依赖图
func (s *WorkflowService) buildStepGraph(steps []models.WorkflowStep) *StepGraph {
	graph := &StepGraph{
		Nodes: make(map[string]*models.WorkflowStep),
		Edges: make(map[string][]string),
	}

	for i := range steps {
		step := &steps[i]
		graph.Nodes[step.ID] = step

		// 从参数中提取依赖关系
		if deps, ok := step.Parameters["depends_on"].([]interface{}); ok {
			for _, dep := range deps {
				if depID, ok := dep.(string); ok {
					graph.Edges[step.ID] = append(graph.Edges[step.ID], depID)
				}
			}
		}
	}

	return graph
}

// getRootSteps 获取根步骤（无依赖的步骤）
func (s *WorkflowService) getRootSteps(graph *StepGraph) []string {
	var roots []string
	for stepID := range graph.Nodes {
		hasDependency := false
		for _, deps := range graph.Edges {
			for _, dep := range deps {
				if dep == stepID {
					hasDependency = true
					break
				}
			}
			if hasDependency {
				break
			}
		}
		if !hasDependency && len(graph.Edges[stepID]) == 0 {
			roots = append(roots, stepID)
		}
	}
	return roots
}

// allDependenciesExecuted 检查所有依赖是否已执行
func (s *WorkflowService) allDependenciesExecuted(stepID string, graph *StepGraph, executed map[string]bool) bool {
	for _, depID := range graph.Edges[stepID] {
		if !executed[depID] {
			return false
		}
	}
	return true
}

// getNextPendingSteps 获取下一批待执行步骤
func (s *WorkflowService) getNextPendingSteps(graph *StepGraph, executed map[string]bool) []string {
	var pending []string
	for stepID := range graph.Nodes {
		if !executed[stepID] {
			pending = append(pending, stepID)
		}
	}
	return pending
}

// findStepByID 根据ID查找步骤
func (s *WorkflowService) findStepByID(steps []models.WorkflowStep, stepID string) *models.WorkflowStep {
	for i := range steps {
		if steps[i].ID == stepID {
			return &steps[i]
		}
	}
	return nil
}

// canExecuteParallel 检查步骤是否可以并行执行
func (s *WorkflowService) canExecuteParallel(step *models.WorkflowStep) bool {
	if parallel, ok := step.Parameters["parallel"].(bool); ok {
		return parallel
	}
	return false
}

// executeStepWithRetry 带重试的步骤执行
func (s *WorkflowService) executeStepWithRetry(execCtx *ExecutionContext, step *models.WorkflowStep) error {
	// 获取重试策略
	retryPolicy := s.getRetryPolicy(step)
	maxRetries := 1
	if retryPolicy != nil {
		maxRetries = retryPolicy.MaxRetries + 1
	}

	var lastErr error
	delay := time.Duration(0)

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// 更新步骤状态为重试中
			s.updateStepStatus(execCtx.Instance, step.ID, string(StepStatusRetrying))
			time.Sleep(delay)

			// 计算下次重试延迟
			if retryPolicy != nil {
				delay = time.Duration(float64(delay) * retryPolicy.BackoffFactor)
				if delay == 0 {
					delay = retryPolicy.InitialDelay
				}
				if delay > retryPolicy.MaxDelay {
					delay = retryPolicy.MaxDelay
				}
			}
		}

		// 执行步骤
		output, err := s.executeStep(execCtx, step)
		if err == nil {
			// 步骤执行成功
			s.updateStepOutput(execCtx.Instance, step.ID, output)
			s.updateStepStatus(execCtx.Instance, step.ID, string(StepStatusCompleted))

			// 保存输出到上下文变量
			if output != nil {
				execCtx.Variables[step.Name] = output
				execCtx.CompletedSteps[step.ID] = s.findStepInstanceByID(execCtx.Instance, step.ID)
			}

			// 处理条件分支
			if step.Type == "condition" {
				nextStep, err := s.evaluateCondition(execCtx, step, output)
				if err != nil {
					return fmt.Errorf("condition evaluation failed: %w", err)
				}
				if nextStep != "" {
					// 修改执行流程
					execCtx.Variables["next_step"] = nextStep
				}
			}

			return nil
		}

		lastErr = err

		// 检查是否应该重试
		if !s.shouldRetry(err, retryPolicy) {
			break
		}
	}

	// 所有重试都失败
	s.updateStepStatus(execCtx.Instance, step.ID, string(StepStatusFailed))
	return fmt.Errorf("step %s failed after %d attempts: %w", step.Name, maxRetries, lastErr)
}

// executeStep 执行单个步骤
func (s *WorkflowService) executeStep(execCtx *ExecutionContext, step *models.WorkflowStep) (map[string]interface{}, error) {
	// 更新步骤状态为运行中
	s.updateStepStatus(execCtx.Instance, step.ID, string(StepStatusRunning))

	// 获取超时设置
	timeout := s.getStepTimeout(step)
	stepCtx, cancel := context.WithTimeout(execCtx.Context, timeout)
	defer cancel()

	// 查找合适的执行器
	executor := s.findExecutor(step.Type)
	if executor == nil {
		return nil, fmt.Errorf("no executor found for step type: %s", step.Type)
	}

	// 准备输入
	input := s.prepareStepInput(execCtx, step)

	// 执行步骤
	output, err := executor.Execute(stepCtx, step, input)
	if err != nil {
		return nil, err
	}

	return output, nil
}

// findExecutor 查找执行器
func (s *WorkflowService) findExecutor(stepType string) StepExecutor {
	for _, exec := range s.executors {
		if exec.CanHandle(stepType) {
			return exec
		}
	}
	return nil
}

// prepareStepInput 准备步骤输入
func (s *WorkflowService) prepareStepInput(execCtx *ExecutionContext, step *models.WorkflowStep) map[string]interface{} {
	input := make(map[string]interface{})

	// 添加上下文变量
	for k, v := range execCtx.Variables {
		input[k] = v
	}

	// 添加步骤参数
	for k, v := range step.Parameters {
		input[k] = v
	}

	// 添加前置步骤的输出
	for _, stepInst := range execCtx.Instance.Steps {
		if stepInst.Status == string(StepStatusCompleted) && stepInst.Output != nil {
			input[stepInst.Name+"_output"] = stepInst.Output
		}
	}

	return input
}

// getStepTimeout 获取步骤超时时间
func (s *WorkflowService) getStepTimeout(step *models.WorkflowStep) time.Duration {
	if timeout, ok := step.Parameters["timeout"].(string); ok {
		if duration, err := time.ParseDuration(timeout); err == nil {
			return duration
		}
	}
	return 5 * time.Minute // 默认5分钟
}

// getRetryPolicy 获取重试策略
func (s *WorkflowService) getRetryPolicy(step *models.WorkflowStep) *RetryPolicy {
	if retryPolicy, ok := step.Parameters["retry_policy"].(map[string]interface{}); ok {
		policy := &RetryPolicy{}
		if data, err := json.Marshal(retryPolicy); err == nil {
			json.Unmarshal(data, policy)
			return policy
		}
	}
	return nil
}

// shouldRetry 判断是否应该重试
func (s *WorkflowService) shouldRetry(err error, policy *RetryPolicy) bool {
	if policy == nil {
		return false
	}

	// 检查错误类型
	for _, errorType := range policy.RetryOnError {
		if contains(err.Error(), errorType) {
			return true
		}
	}

	return false
}

// evaluateCondition 评估条件分支
func (s *WorkflowService) evaluateCondition(execCtx *ExecutionContext, step *models.WorkflowStep, output map[string]interface{}) (string, error) {
	conditionData, ok := step.Parameters["condition"].(map[string]interface{})
	if !ok {
		return "", nil
	}

	condition := &ConditionalBranch{}
	if data, err := json.Marshal(conditionData); err == nil {
		json.Unmarshal(data, condition)
	}

	// 评估每个规则
	for _, rule := range condition.Rules {
		result, err := s.evaluateConditionRule(execCtx, rule, output)
		if err != nil {
			return "", err
		}
		if result {
			return rule.NextStep, nil
		}
	}

	return condition.DefaultRoute, nil
}

// evaluateConditionRule 评估条件规则
func (s *WorkflowService) evaluateConditionRule(execCtx *ExecutionContext, rule ConditionRule, output map[string]interface{}) (bool, error) {
	// 获取表达式的值
	value, err := s.evaluateExpression(execCtx, rule.Expression, output)
	if err != nil {
		return false, err
	}

	// 根据操作符比较
	switch rule.Operator {
	case "equals":
		return fmt.Sprintf("%v", value) == fmt.Sprintf("%v", rule.Value), nil
	case "not_equals":
		return fmt.Sprintf("%v", value) != fmt.Sprintf("%v", rule.Value), nil
	case "contains":
		return contains(fmt.Sprintf("%v", value), fmt.Sprintf("%v", rule.Value)), nil
	case "greater":
		return compareNumbers(value, rule.Value) > 0, nil
	case "less":
		return compareNumbers(value, rule.Value) < 0, nil
	case "exists":
		return value != nil, nil
	default:
		return false, fmt.Errorf("unknown operator: %s", rule.Operator)
	}
}

// evaluateExpression 计算表达式
func (s *WorkflowService) evaluateExpression(execCtx *ExecutionContext, expression string, output map[string]interface{}) (interface{}, error) {
	// 简单表达式解析
	// 支持格式: ${step_name.field}, ${variable}, ${output.field}

	if len(expression) > 2 && expression[:2] == "${" && expression[len(expression)-1:] == "}" {
		expr := expression[2 : len(expression)-1]

		// 检查是否是输出引用
		if len(expr) > 7 && expr[:7] == "output." {
			field := expr[7:]
			if val, ok := output[field]; ok {
				return val, nil
			}
		}

		// 检查是否是变量引用
		if val, ok := execCtx.Variables[expr]; ok {
			return val, nil
		}
	}

	return expression, nil
}

// updateInstanceStatus 更新实例状态
func (s *WorkflowService) updateInstanceStatus(instance *models.WorkflowInstance, status string) {
	instance.Status = status
	instance.UpdatedAt = time.Now()
	s.db.Save(instance)
}

// updateStepStatus 更新步骤状态
func (s *WorkflowService) updateStepStatus(instance *models.WorkflowInstance, stepID string, status string) {
	for i := range instance.Steps {
		if instance.Steps[i].StepID == stepID {
			instance.Steps[i].Status = status
			if status == string(StepStatusRunning) {
				now := time.Now()
				instance.Steps[i].StartedAt = &now
			} else if status == string(StepStatusCompleted) || status == string(StepStatusFailed) {
				now := time.Now()
				instance.Steps[i].CompletedAt = &now
			}
			s.db.Save(&instance.Steps[i])
			break
		}
	}
	instance.UpdatedAt = time.Now()
	s.db.Save(instance)
}

// updateStepOutput 更新步骤输出
func (s *WorkflowService) updateStepOutput(instance *models.WorkflowInstance, stepID string, output map[string]interface{}) {
	for i := range instance.Steps {
		if instance.Steps[i].StepID == stepID {
			instance.Steps[i].Output = output
			s.db.Save(&instance.Steps[i])
			break
		}
	}
}

// findStepInstanceByID 根据步骤ID查找步骤实例
func (s *WorkflowService) findStepInstanceByID(instance *models.WorkflowInstance, stepID string) *models.StepInstance {
	for i := range instance.Steps {
		if instance.Steps[i].StepID == stepID {
			return &instance.Steps[i]
		}
	}
	return nil
}

// PauseWorkflowInstance 暂停工作流实例
func (s *WorkflowService) PauseWorkflowInstance(instanceID string) error {
	s.instanceMutex.RLock()
	execCtx, exists := s.runningInstances[instanceID]
	s.instanceMutex.RUnlock()

	if !exists {
		return fmt.Errorf("workflow instance not running")
	}

	s.updateInstanceStatus(execCtx.Instance, string(StatusPaused))
	execCtx.CancelFunc()

	return nil
}

// ResumeWorkflowInstance 恢复工作流实例
func (s *WorkflowService) ResumeWorkflowInstance(instanceID string) error {
	var instance models.WorkflowInstance
	err := s.db.First(&instance, instanceID).Error
	if err != nil {
		return fmt.Errorf("workflow instance not found: %w", err)
	}

	if instance.Status != string(StatusPaused) {
		return fmt.Errorf("workflow instance is not paused")
	}

	// 重新执行工作流
	_, err = s.ExecuteWorkflowInstance(instanceID)
	return err
}

// CancelWorkflowInstance 取消工作流实例
func (s *WorkflowService) CancelWorkflowInstance(instanceID string) error {
	s.instanceMutex.RLock()
	execCtx, exists := s.runningInstances[instanceID]
	s.instanceMutex.RUnlock()

	if !exists {
		return fmt.Errorf("workflow instance not running")
	}

	s.updateInstanceStatus(execCtx.Instance, string(StatusCancelled))
	execCtx.CancelFunc()

	return nil
}

// GetWorkflowInstance 获取工作流实例详情
func (s *WorkflowService) GetWorkflowInstance(id string) (models.WorkflowInstance, bool) {
	var instance models.WorkflowInstance
	err := s.db.Preload("Steps").First(&instance, id).Error
	return instance, err == nil
}

// ListWorkflowInstances 列出工作流实例
func (s *WorkflowService) ListWorkflowInstances(workflowID string, page, pageSize int) ([]models.WorkflowInstance, int64) {
	var instances []models.WorkflowInstance
	var total int64

	query := s.db.Model(&models.WorkflowInstance{})
	if workflowID != "" {
		query = query.Where("workflow_id = ?", workflowID)
	}

	query.Count(&total)

	offset := (page - 1) * pageSize
	query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&instances)

	return instances, total
}

// ============ 模板管理 ============

// loadBuiltinTemplates 加载内置模板
func (s *WorkflowService) loadBuiltinTemplates() {
	// AI 对话模板
	aiChatTemplate := &WorkflowTemplate{
		ID:          "builtin-ai-chat",
		Name:        "AI 对话工作流",
		Description: "使用 AI 模型进行对话的基础工作流",
		Category:    "ai",
		Version:     "1.0.0",
		Steps: []TemplateStep{
			{
				ID:          "step-1",
				Name:        "输入验证",
				Type:        "validation",
				Description: "验证用户输入",
				Order:       0,
				Timeout:     30 * time.Second,
			},
			{
				ID:          "step-2",
				Name:        "AI 对话",
				Type:        "llm",
				Description: "调用 LLM 进行对话",
				Order:       1,
				Timeout:     2 * time.Minute,
				Parameters: map[string]interface{}{
					"prompt": "{{.user_input}}",
				},
				RetryPolicy: &RetryPolicy{
					MaxRetries:    3,
					InitialDelay:  1 * time.Second,
					MaxDelay:      10 * time.Second,
					BackoffFactor: 2.0,
				},
			},
			{
				ID:          "step-3",
				Name:        "结果格式化",
				Type:        "formatter",
				Description: "格式化 AI 返回结果",
				Order:       2,
				Timeout:     30 * time.Second,
			},
		},
		Variables: []TemplateVariable{
			{Name: "user_input", Type: "string", Required: true, Description: "用户输入的内容"},
		},
		Metadata: map[string]interface{}{
			"author": "system",
			"builtin": true,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 数据处理模板
	dataProcessingTemplate := &WorkflowTemplate{
		ID:          "builtin-data-processing",
		Name:        "数据处理工作流",
		Description: "并行处理数据的工作流模板",
		Category:    "data",
		Version:     "1.0.0",
		Steps: []TemplateStep{
			{
				ID:          "step-1",
				Name:        "数据提取",
				Type:        "extractor",
				Description: "从数据源提取数据",
				Order:       0,
				Timeout:     1 * time.Minute,
			},
			{
				ID:          "step-2",
				Name:        "数据转换",
				Type:        "transformer",
				Description: "转换数据格式",
				Order:       1,
				Parallel:    true,
				Timeout:     2 * time.Minute,
			},
			{
				ID:          "step-3",
				Name:        "数据验证",
				Type:        "validator",
				Description: "验证数据完整性",
				Order:       2,
				Timeout:     1 * time.Minute,
				Condition: &ConditionalBranch{
					Type: "output",
					Rules: []ConditionRule{
						{Expression: "${output.valid}", Operator: "equals", Value: true, NextStep: "step-4"},
					},
					DefaultRoute: "step-error",
				},
			},
		},
		Variables: []TemplateVariable{
			{Name: "data_source", Type: "string", Required: true, Description: "数据源地址"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	s.templateMutex.Lock()
	s.templateCache[aiChatTemplate.ID] = aiChatTemplate
	s.templateCache[dataProcessingTemplate.ID] = dataProcessingTemplate
	s.templateMutex.Unlock()
}

// CreateTemplate 创建模板
func (s *WorkflowService) CreateTemplate(template *WorkflowTemplate) error {
	s.templateMutex.Lock()
	defer s.templateMutex.Unlock()

	template.ID = uuid.New().String()
	template.CreatedAt = time.Now()
	template.UpdatedAt = time.Now()

	s.templateCache[template.ID] = template
	return nil
}

// GetTemplate 获取模板
func (s *WorkflowService) GetTemplate(id string) (*WorkflowTemplate, error) {
	s.templateMutex.RLock()
	defer s.templateMutex.RUnlock()

	if template, ok := s.templateCache[id]; ok {
		return template, nil
	}

	return nil, fmt.Errorf("template not found")
}

// ListTemplates 列出所有模板
func (s *WorkflowService) ListTemplates(category string) []*WorkflowTemplate {
	s.templateMutex.RLock()
	defer s.templateMutex.RUnlock()

	var templates []*WorkflowTemplate
	for _, template := range s.templateCache {
		if category == "" || template.Category == category {
			templates = append(templates, template)
		}
	}

	return templates
}

// UpdateTemplate 更新模板
func (s *WorkflowService) UpdateTemplate(id string, template *WorkflowTemplate) error {
	s.templateMutex.Lock()
	defer s.templateMutex.Unlock()

	if _, exists := s.templateCache[id]; !exists {
		return fmt.Errorf("template not found")
	}

	template.ID = id
	template.UpdatedAt = time.Now()
	s.templateCache[id] = template

	return nil
}

// DeleteTemplate 删除模板
func (s *WorkflowService) DeleteTemplate(id string) error {
	s.templateMutex.Lock()
	defer s.templateMutex.Unlock()

	if _, exists := s.templateCache[id]; !exists {
		return fmt.Errorf("template not found")
	}

	// 不允许删除内置模板
	if s.templateCache[id].Metadata != nil {
		if builtin, ok := s.templateCache[id].Metadata["builtin"].(bool); ok && builtin {
			return fmt.Errorf("cannot delete builtin template")
		}
	}

	delete(s.templateCache, id)
	return nil
}

// ============ 步骤执行器实现 ============

// LLMStepExecutor LLM 步骤执行器
type LLMStepExecutor struct {
	llmClient llm.LLMClient
}

func (e *LLMStepExecutor) CanHandle(stepType string) bool {
	return stepType == "llm" || stepType == "model"
}

func (e *LLMStepExecutor) Execute(ctx context.Context, step *models.WorkflowStep, input map[string]interface{}) (map[string]interface{}, error) {
	if e.llmClient == nil {
		return nil, fmt.Errorf("LLM client not initialized")
	}

	// 构建请求
	prompt, _ := input["prompt"].(string)
	systemPrompt, _ := step.Parameters["system_prompt"].(string)
	model, _ := step.Parameters["model"].(string)
	if model == "" {
		model = "gpt-3.5-turbo"
	}

	messages := []llm.Message{}
	if systemPrompt != "" {
		messages = append(messages, llm.Message{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, llm.Message{Role: "user", Content: prompt})

	req := &llm.ChatRequest{
		Messages:    messages,
		Model:       model,
		Temperature: 0.7,
	}

	// 执行请求
	resp, err := e.llmClient.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from LLM")
	}

	// 返回结果
	result := map[string]interface{}{
		"response":    resp.Choices[0].Message.Content,
		"model":       resp.Model,
		"finish_reason": resp.Choices[0].FinishReason,
	}

	if resp.Usage != nil {
		result["usage"] = map[string]interface{}{
			"prompt_tokens":     resp.Usage.PromptTokens,
			"completion_tokens": resp.Usage.CompletionTokens,
			"total_tokens":      resp.Usage.TotalTokens,
		}
	}

	return result, nil
}

// CodeStepExecutor 代码执行器
type CodeStepExecutor struct{}

func (e *CodeStepExecutor) CanHandle(stepType string) bool {
	return stepType == "code" || stepType == "script"
}

func (e *CodeStepExecutor) Execute(ctx context.Context, step *models.WorkflowStep, input map[string]interface{}) (map[string]interface{}, error) {
	// 简化的代码执行器
	// 实际应用中应该使用沙箱环境执行代码
	code, _ := step.Parameters["code"].(string)
	language, _ := step.Parameters["language"].(string)

	result := map[string]interface{}{
		"output":     fmt.Sprintf("Executed %s code: %s", language, code),
		"success":    true,
		"language":   language,
		"timestamp":  time.Now(),
	}

	return result, nil
}

// PluginStepExecutor 插件执行器
type PluginStepExecutor struct{}

func (e *PluginStepExecutor) CanHandle(stepType string) bool {
	return stepType == "plugin"
}

func (e *PluginStepExecutor) Execute(ctx context.Context, step *models.WorkflowStep, input map[string]interface{}) (map[string]interface{}, error) {
	pluginName, _ := step.Parameters["plugin_name"].(string)
	action, _ := step.Parameters["action"].(string)

	result := map[string]interface{}{
		"plugin":     pluginName,
		"action":     action,
		"output":     fmt.Sprintf("Executed plugin %s with action %s", pluginName, action),
		"success":    true,
	}

	return result, nil
}

// AutomationStepExecutor 自动化执行器
type AutomationStepExecutor struct{}

func (e *AutomationStepExecutor) CanHandle(stepType string) bool {
	return stepType == "automation" || stepType == "skill"
}

func (e *AutomationStepExecutor) Execute(ctx context.Context, step *models.WorkflowStep, input map[string]interface{}) (map[string]interface{}, error) {
	skillName, _ := step.Parameters["skill_name"].(string)
	params, _ := step.Parameters["params"].(map[string]interface{})

	result := map[string]interface{}{
		"skill":      skillName,
		"params":     params,
		"output":     fmt.Sprintf("Executed automation skill %s", skillName),
		"success":    true,
	}

	return result, nil
}

// ConditionStepExecutor 条件执行器
type ConditionStepExecutor struct{}

func (e *ConditionStepExecutor) CanHandle(stepType string) bool {
	return stepType == "condition"
}

func (e *ConditionStepExecutor) Execute(ctx context.Context, step *models.WorkflowStep, input map[string]interface{}) (map[string]interface{}, error) {
	// 条件步骤的执行逻辑已在主工作流中处理
	result := map[string]interface{}{
		"condition_evaluated": true,
		"input": input,
	}

	return result, nil
}

// ParallelStepExecutor 并行执行器
type ParallelStepExecutor struct{}

func (e *ParallelStepExecutor) CanHandle(stepType string) bool {
	return stepType == "parallel"
}

func (e *ParallelStepExecutor) Execute(ctx context.Context, step *models.WorkflowStep, input map[string]interface{}) (map[string]interface{}, error) {
	// 并行步骤的执行逻辑已在主工作流中处理
	result := map[string]interface{}{
		"parallel_executed": true,
		"input": input,
	}

	return result, nil
}

// ============ 辅助函数 ============

func replaceAll(s, old, new string) string {
	result := ""
	for {
		idx := indexOf(s, old)
		if idx == -1 {
			result += s
			break
		}
		result += s[:idx] + new
		s = s[idx+len(old):]
	}
	return result
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func contains(s, substr string) bool {
	return indexOf(s, substr) != -1
}

func compareNumbers(a, b interface{}) int {
	var aFloat, bFloat float64

	switch val := a.(type) {
	case int:
		aFloat = float64(val)
	case int64:
		aFloat = float64(val)
	case float64:
		aFloat = val
	case float32:
		aFloat = float64(val)
	default:
		aFloat = 0
	}

	switch val := b.(type) {
	case int:
		bFloat = float64(val)
	case int64:
		bFloat = float64(val)
	case float64:
		bFloat = val
	case float32:
		bFloat = float64(val)
	default:
		bFloat = 0
	}

	if aFloat < bFloat {
		return -1
	} else if aFloat > bFloat {
		return 1
	}
	return 0
}

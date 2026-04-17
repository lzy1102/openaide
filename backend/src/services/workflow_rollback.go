package services

import (
	"context"
	"fmt"
	"sync"
)

// RollbackHandler 回滚处理器
type RollbackHandler struct {
	workflowService *WorkflowService
	rollbackStacks  map[string][]*RollbackAction
	mutex          sync.RWMutex
}

// RollbackAction 回滚动作
type RollbackAction struct {
	StepID     string
	StepName   string
	ActionType string
	Data       map[string]interface{}
	ExecuteAt  int64
	Executed   bool
}

// NewRollbackHandler 创建回滚处理器
func NewRollbackHandler(workflowService *WorkflowService) *RollbackHandler {
	return &RollbackHandler{
		workflowService: workflowService,
		rollbackStacks:  make(map[string][]*RollbackAction),
	}
}

// RegisterAction 注册回滚动作
func (h *RollbackHandler) RegisterAction(instanceID string, action *RollbackAction) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.rollbackStacks[instanceID] == nil {
		h.rollbackStacks[instanceID] = make([]*RollbackAction, 0)
	}

	h.rollbackStacks[instanceID] = append(h.rollbackStacks[instanceID], action)
}

// ExecuteRollback 执行回滚
func (h *RollbackHandler) ExecuteRollback(ctx context.Context, instanceID string, failedStepID string) error {
	h.mutex.Lock()
	actions, exists := h.rollbackStacks[instanceID]
	if !exists {
		h.mutex.Unlock()
		return fmt.Errorf("no rollback actions found for instance %s", instanceID)
	}

	// 复制一份用于执行
	rollbackActions := make([]*RollbackAction, len(actions))
	copy(rollbackActions, actions)
	h.mutex.Unlock()

	// 从后向前执行回滚（反向执行）
	for i := len(rollbackActions) - 1; i >= 0; i-- {
		action := rollbackActions[i]

		// 如果已经回滚过，跳过
		if action.Executed {
			continue
		}

		// 执行到失败的步骤时停止
		if action.StepID == failedStepID {
			break
		}

		// 执行回滚
		if err := h.executeRollbackAction(ctx, action); err != nil {
			return fmt.Errorf("rollback failed for step %s: %w", action.StepName, err)
		}

		// 标记为已执行
		h.mutex.Lock()
		if h.rollbackStacks[instanceID] != nil {
			for idx, a := range h.rollbackStacks[instanceID] {
				if a.StepID == action.StepID {
					h.rollbackStacks[instanceID][idx].Executed = true
					break
				}
			}
		}
		h.mutex.Unlock()
	}

	// 清理回滚栈
	h.mutex.Lock()
	delete(h.rollbackStacks, instanceID)
	h.mutex.Unlock()

	return nil
}

// executeRollbackAction 执行单个回滚动作
func (h *RollbackHandler) executeRollbackAction(ctx context.Context, action *RollbackAction) error {
	switch action.ActionType {
	case "llm":
		// LLM 调用通常不需要回滚，但可能需要记录
		return nil

	case "database_update":
		// 数据库更新需要回滚
		return h.rollbackDatabaseUpdate(ctx, action)

	case "file_operation":
		// 文件操作需要回滚
		return h.rollbackFileOperation(ctx, action)

	case "api_call":
		// API 调用可能需要补偿操作
		return h.compensateAPICall(ctx, action)

	case "plugin":
		// 插件执行需要回滚
		return h.rollbackPlugin(ctx, action)

	default:
		return fmt.Errorf("unknown rollback action type: %s", action.ActionType)
	}
}

// rollbackDatabaseUpdate 回滚数据库更新
func (h *RollbackHandler) rollbackDatabaseUpdate(ctx context.Context, action *RollbackAction) error {
	// 从回滚数据中获取原始值
	originalData, ok := action.Data["original_value"]
	if !ok {
		return fmt.Errorf("no original value found for rollback")
	}

	table, _ := action.Data["table"].(string)
	recordID, _ := action.Data["record_id"].(string)

	// 执行回滚更新
	// 这里需要实际的数据库操作
	_ = originalData
	_ = table
	_ = recordID

	return nil
}

// rollbackFileOperation 回滚文件操作
func (h *RollbackHandler) rollbackFileOperation(ctx context.Context, action *RollbackAction) error {
	operation, _ := action.Data["operation"].(string)
	filePath, _ := action.Data["file_path"].(string)

	switch operation {
	case "create":
		// 删除创建的文件
		// 实际实现中需要调用文件系统操作
		_ = filePath

	case "update":
		// 恢复原始文件内容
		backupPath, _ := action.Data["backup_path"].(string)
		_ = backupPath

	case "delete":
		// 从备份恢复文件
		backupPath, _ := action.Data["backup_path"].(string)
		_ = backupPath
	}

	return nil
}

// compensateAPICall 补偿 API 调用
func (h *RollbackHandler) compensateAPICall(ctx context.Context, action *RollbackAction) error {
	// 获取补偿端点
	compensateEndpoint, ok := action.Data["compensate_endpoint"].(string)
	if !ok {
		return nil // 没有补偿端点，跳过
	}

	// 执行补偿调用
	_ = compensateEndpoint

	return nil
}

// rollbackPlugin 回滚插件执行
func (h *RollbackHandler) rollbackPlugin(ctx context.Context, action *RollbackAction) error {
	pluginName, _ := action.Data["plugin_name"].(string)
	rollbackAction, _ := action.Data["rollback_action"].(string)

	// 调用插件的回滚方法
	_ = pluginName
	_ = rollbackAction

	return nil
}

// ClearRollbackStack 清理回滚栈
func (h *RollbackHandler) ClearRollbackStack(instanceID string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	delete(h.rollbackStacks, instanceID)
}

// GetRollbackStack 获取回滚栈
func (h *RollbackHandler) GetRollbackStack(instanceID string) []*RollbackAction {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if actions, exists := h.rollbackStacks[instanceID]; exists {
		result := make([]*RollbackAction, len(actions))
		copy(result, actions)
		return result
	}

	return nil
}

// CompensateTransaction 补偿事务处理器
type CompensateTransaction struct {
	steps        []CompensateStep
	currentIndex int
}

// CompensateStep 补偿步骤
type CompensateStep struct {
	Name         string
	Execute      func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error)
	Compensate   func(ctx context.Context, input map[string]interface{}) error
	Input        map[string]interface{}
	Output       map[string]interface{}
	Executed     bool
	Compensated  bool
}

// NewCompensateTransaction 创建补偿事务
func NewCompensateTransaction() *CompensateTransaction {
	return &CompensateTransaction{
		steps:        make([]CompensateStep, 0),
		currentIndex: 0,
	}
}

// AddStep 添加步骤
func (t *CompensateTransaction) AddStep(name string, execute func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error), compensate func(ctx context.Context, input map[string]interface{}) error, input map[string]interface{}) {
	t.steps = append(t.steps, CompensateStep{
		Name:       name,
		Execute:    execute,
		Compensate: compensate,
		Input:      input,
	})
}

// Execute 执行事务
func (t *CompensateTransaction) Execute(ctx context.Context) error {
	for i := range t.steps {
		t.currentIndex = i

		output, err := t.steps[i].Execute(ctx, t.steps[i].Input)
		if err != nil {
			// 执行失败，开始补偿
			return t.Compensate(ctx)
		}

		t.steps[i].Output = output
		t.steps[i].Executed = true
	}

	return nil
}

// Compensate 执行补偿
func (t *CompensateTransaction) Compensate(ctx context.Context) error {
	// 从当前步骤的前一个步骤开始反向补偿
	for i := t.currentIndex - 1; i >= 0; i-- {
		if !t.steps[i].Executed || t.steps[i].Compensated {
			continue
		}

		if err := t.steps[i].Compensate(ctx, t.steps[i].Output); err != nil {
			// 补偿失败，记录错误但继续
			// 实际应用中可能需要更复杂的错误处理
		}

		t.steps[i].Compensated = true
	}

	return fmt.Errorf("transaction failed, compensation completed")
}

// GetStatus 获取事务状态
func (t *CompensateTransaction) GetStatus() map[string]interface{} {
	completedCount := 0
	compensatedCount := 0

	for _, step := range t.steps {
		if step.Executed {
			completedCount++
		}
		if step.Compensated {
			compensatedCount++
		}
	}

	return map[string]interface{}{
		"total_steps":      len(t.steps),
		"completed_steps":  completedCount,
		"compensated_steps": compensatedCount,
		"current_index":    t.currentIndex,
	}
}

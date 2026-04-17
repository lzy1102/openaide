package orchestration

import (
	"fmt"
	"testing"
	"time"

	"openaide/backend/src/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfirmationFlow(t *testing.T) {
	flow := NewConfirmationFlow()
	assert.NotNil(t, flow)
}

func TestCreateConfirmationRequest(t *testing.T) {
	flow := NewConfirmationFlow()

	proposal := &ExecutionProposal{
		TaskSummary:       "实现用户认证 API",
		TaskType:          "coding",
		Priority:          "high",
		EstimatedDuration: 2 * time.Hour,
		EstimatedCost:     10.50,
		TeamConfig: &TeamConfiguration{
			Members: []*TeamMember{
				{
					Role:   "architect",
					Skills:  []string{"架构设计", "系统设计"},
					Model:   "claude-3-opus",
				},
				{
					Role:   "developer",
					Skills:  []string{"编码", "调试"},
					Model:   "claude-3.5-sonnet",
				},
			},
		},
		ExecutionPlan: []*ExecutionStep{
			{
				ID:          "step-1",
				Order:       0,
				Name:        "设计",
				Description: "系统架构设计",
				AssignedTo:  "architect",
				Duration:    30 * time.Minute,
			},
			{
				ID:          "step-2",
				Order:       1,
				Name:        "开发",
				Description: "功能开发",
				AssignedTo:  "developer",
				Duration:    90 * time.Minute,
				Dependencies: []string{"step-1"},
			},
		},
	}

	req, err := flow.CreateConfirmationRequest(nil, "task-123", "user-456", proposal)
	require.NoError(t, err)

	assert.Equal(t, "user-456", req.UserID)
	assert.Equal(t, "task-123", req.TaskID)
	assert.Equal(t, StatePending, req.State)
	assert.NotNil(t, req.Proposal)
}

func TestFormatProposal(t *testing.T) {
	flow := NewConfirmationFlow()

	proposal := &ExecutionProposal{
		TaskSummary:       "实现用户认证 API",
		TaskType:          "coding",
		Priority:          "high",
		EstimatedDuration: 2*time.Hour + 30*time.Minute,
		EstimatedCost:     10.50,
		TeamConfig: &TeamConfiguration{
			Members: []*TeamMember{
				{Role: "architect", Skills: []string{"架构设计"}, Model: "claude-3-opus"},
				{Role: "developer", Skills: []string{"编码", "调试"}, Model: "claude-3.5-sonnet"},
				{Role: "reviewer", Skills: []string{"代码审查"}, Model: "claude-3.5-sonnet"},
			},
		},
		ExecutionPlan: []*ExecutionStep{
			{
				ID:          "step-1",
				Order:       0,
				Name:        "架构设计",
				Description: "系统架构设计",
				AssignedTo:  "architect",
				Duration:    30 * time.Minute,
			},
			{
				ID:          "step-2",
				Order:       1,
				Name:        "编码实现",
				Description: "实现功能",
				AssignedTo:  "developer",
				Duration:    90 * time.Minute,
			},
		},
	}

	output := flow.FormatProposal(proposal)

	assert.Contains(t, output, "实现用户认证 API")
	assert.Contains(t, output, "architect")
	assert.Contains(t, output, "developer")
	assert.Contains(t, output, "claude-3-opus")
	assert.Contains(t, output, "2h30m")
	assert.Contains(t, output, "$10.50")
}

func TestProcessUserAction_Approve(t *testing.T) {
	flow := NewConfirmationFlow()

	proposal := &ExecutionProposal{
		TaskSummary:       "测试任务",
		TaskType:          "analysis",
		EstimatedDuration: 30 * time.Minute,
		TeamConfig: &TeamConfiguration{
			Members: []*TeamMember{
				{Role: "analyst", Model: "claude-3-sonnet"},
			},
		},
		ExecutionPlan: []*ExecutionStep{},
	}

	req, _ := flow.CreateConfirmationRequest(nil, "task-1", "user-1", proposal)

	// 批准
	updatedReq, err := flow.ProcessUserAction(nil, req, ActionApprove, "看起来不错")
	require.NoError(t, err)

	assert.Equal(t, StateApproved, updatedReq.State)
	assert.Equal(t, ActionApprove, updatedReq.Response.Action)
	assert.Equal(t, "看起来不错", updatedReq.Response.Comment)
}

func TestProcessUserAction_Reject(t *testing.T) {
	flow := NewConfirmationFlow()

	proposal := &ExecutionProposal{
		TaskSummary:       "测试任务",
		EstimatedDuration: 30 * time.Minute,
		TeamConfig:        &TeamConfiguration{Members: []*TeamMember{{Role: "agent"}}},
		ExecutionPlan:      []*ExecutionStep{},
	}

	req, _ := flow.CreateConfirmationRequest(nil, "task-1", "user-1", proposal)

	// 拒绝
	updatedReq, err := flow.ProcessUserAction(nil, req, ActionReject, "方向不对")
	require.NoError(t, err)

	assert.Equal(t, StateRejected, updatedReq.State)
	assert.Equal(t, ActionReject, updatedReq.Response.Action)
}

func TestProcessUserAction_Cancel(t *testing.T) {
	flow := NewConfirmationFlow()

	proposal := &ExecutionProposal{
		TaskSummary:       "测试任务",
		EstimatedDuration: 30 * time.Minute,
		TeamConfig:        &TeamConfiguration{Members: []*TeamMember{{Role: "agent"}}},
		ExecutionPlan:      []*ExecutionStep{},
	}

	req, _ := flow.CreateConfirmationRequest(nil, "task-1", "user-1", proposal)

	// 取消
	updatedReq, err := flow.ProcessUserAction(nil, req, ActionCancel, "")
	require.NoError(t, err)

	assert.Equal(t, StateCancelled, updatedReq.State)
	assert.Equal(t, ActionCancel, updatedReq.Response.Action)
}

func TestCreateAdjustment(t *testing.T) {
	flow := NewConfirmationFlow()

	proposal := &ExecutionProposal{
		TaskSummary:       "测试任务",
		EstimatedDuration: 30 * time.Minute,
		TeamConfig:        &TeamConfiguration{Members: []*TeamMember{{Role: "agent"}}},
		ExecutionPlan:      []*ExecutionStep{},
	}

	req, _ := flow.CreateConfirmationRequest(nil, "task-1", "user-1", proposal)

	// 创建调整
	adjustment := flow.CreateAdjustment(req, "priority", "low", "high", "提高优先级")

	assert.Equal(t, "priority", adjustment.Field)
	assert.Equal(t, "low", adjustment.OldValue)
	assert.Equal(t, "high", adjustment.NewValue)
	assert.Equal(t, "提高优先级", adjustment.Reason)
	assert.Equal(t, StatePending, req.State) // 重新进入待确认
}

func TestApplyAdjustment(t *testing.T) {
	flow := NewConfirmationFlow()

	proposal := &ExecutionProposal{
		TaskSummary:       "测试任务",
		TaskType:          "coding",
		Priority:          "low",
		EstimatedDuration: 30 * time.Minute,
		TeamConfig: &TeamConfiguration{
			Members: []*TeamMember{
				{Role: "developer", Model: "gpt-4"},
			},
		},
		ExecutionPlan: []*ExecutionStep{},
	}

	// 调整优先级
	adjustment := &Adjustment{
		Field:    "priority",
		NewValue: "high",
		Reason:   "紧急任务",
	}

	err := flow.ApplyAdjustment(proposal, adjustment)
	assert.NoError(t, err)
	assert.Equal(t, "high", proposal.Priority)

	// 调整模型
	adjustment2 := &Adjustment{
		Field:    "model",
		NewValue: "claude-3-opus",
		Reason:   "需要更强能力",
	}

	err = flow.ApplyAdjustment(proposal, adjustment2)
	assert.NoError(t, err)
	assert.Equal(t, "claude-3-opus", proposal.TeamConfig.Members[0].Model)

	// 调整时间
	adjustment3 := &Adjustment{
		Field:    "estimated_duration",
		NewValue: 45 * time.Minute,
		Reason:   "增加测试时间",
	}

	err = flow.ApplyAdjustment(proposal, adjustment3)
	assert.NoError(t, err)
	assert.Equal(t, 45*time.Minute, proposal.EstimatedDuration)
}

func TestApplyAdjustment_InvalidInput(t *testing.T) {
	flow := NewConfirmationFlow()

	proposal := &ExecutionProposal{
		TaskSummary:       "测试任务",
		EstimatedDuration: 30 * time.Minute,
		TeamConfig:        &TeamConfiguration{Members: []*TeamMember{{Role: "agent"}}},
		ExecutionPlan:      []*ExecutionStep{},
	}

	// 无效的团队规模
	adjustment := &Adjustment{
		Field:    "team_size",
		NewValue: 0,
		}

	err := flow.ApplyAdjustment(proposal, adjustment)
	assert.Error(t, err)

	// 无效的模型
	adjustment2 := &Adjustment{
		Field:    "model",
		NewValue: "",
	}

	err = flow.ApplyAdjustment(proposal, adjustment2)
	assert.Error(t, err)

	// 未知字段
	adjustment3 := &Adjustment{
		Field:    "unknown",
		NewValue: "value",
	}

	err = flow.ApplyAdjustment(proposal, adjustment3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}

func TestValidateProposal_Valid(t *testing.T) {
	flow := NewConfirmationFlow()

	proposal := &ExecutionProposal{
		TaskSummary:       "实现API",
		TaskType:          "coding",
		Priority:          "high",
		EstimatedDuration: 2 * time.Hour,
		TeamConfig: &TeamConfiguration{
			Members: []*TeamMember{
				{Role: "architect", Skills: []string{"design"}, Model: "claude-3-opus"},
			},
		},
		ExecutionPlan: []*ExecutionStep{
			{
				ID:     "step-1",
				Order:  0,
				Name:   "Design",
				AssignedTo: "architect",
				Duration: 30 * time.Minute,
			},
		},
	}

	err := flow.ValidateProposal(proposal)
	assert.NoError(t, err)
}

func TestValidateProposal_Invalid(t *testing.T) {
	flow := NewConfirmationFlow()

	tests := []struct {
		name     string
		proposal *ExecutionProposal
	}{
		{
			name:     "空任务摘要",
			proposal: &ExecutionProposal{TaskType: "coding", EstimatedDuration: time.Hour},
		},
		{
			name:     "空任务类型",
			proposal: &ExecutionProposal{TaskSummary: "task", EstimatedDuration: time.Hour},
		},
		{
			name:     "无效持续时间",
			proposal: &ExecutionProposal{TaskSummary: "task", TaskType: "coding", EstimatedDuration: 0},
		},
		{
			name:     "无团队配置",
			proposal: &ExecutionProposal{TaskSummary: "task", TaskType: "coding", EstimatedDuration: time.Hour},
		},
		{
			name:     "空执行计划",
			proposal: &ExecutionProposal{
				TaskSummary:       "task",
				TaskType:          "coding",
				EstimatedDuration: time.Hour,
				TeamConfig:        &TeamConfiguration{Members: []*TeamMember{{Role: "agent"}}},
				ExecutionPlan:     []*ExecutionStep{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := flow.ValidateProposal(tt.proposal)
			assert.Error(t, err)
		})
	}
}

func TestCloneProposal(t *testing.T) {
	flow := NewConfirmationFlow()

	original := &ExecutionProposal{
		TaskSummary:       "原始任务",
		TaskType:          "coding",
		Priority:          "high",
		EstimatedDuration: 2 * time.Hour,
		EstimatedCost:     100.0,
		TeamConfig: &TeamConfiguration{
			Members: []*TeamMember{
				{Role: "developer", Model: "gpt-4"},
			},
		},
		ExecutionPlan: []*ExecutionStep{
			{
				ID:     "step-1",
				Order:  0,
				Name:   "Step 1",
				Duration: 30 * time.Minute,
			},
		},
		ResourceRequirements: map[string]interface{}{
			"cpu": "4",
			"memory": "8GB",
		},
	}

	cloned := flow.CloneProposal(original)

	// 验证克隆是深拷贝
	assert.Equal(t, cloned.TaskSummary, original.TaskSummary)
	assert.NotSame(t, cloned, original)

	// 修改克隆不应影响原始
	cloned.TaskSummary = "修改后的任务"
	assert.Equal(t, "原始任务", original.TaskSummary)

	// 验证团队配置是深拷贝
	assert.NotSame(t, cloned.TeamConfig, original.TeamConfig)
	// 验证成员是不同的实例
	if len(cloned.TeamConfig.Members) > 0 && len(original.TeamConfig.Members) > 0 {
		// 修改克隆的成员不应影响原始
		cloned.TeamConfig.Members[0].Role = "modified"
		assert.Equal(t, "developer", original.TeamConfig.Members[0].Role)
	}

	// 验证执行计划是深拷贝
	// NotSame doesn't work with slices, so we verify by modification
	// 验证步骤是不同的实例
	if len(cloned.ExecutionPlan) > 0 && len(original.ExecutionPlan) > 0 {
		// 修改克隆的步骤不应影响原始
		cloned.ExecutionPlan[0].Name = "modified"
		assert.Equal(t, "Step 1", original.ExecutionPlan[0].Name)
	}
}

func TestGetAdjustmentOptions(t *testing.T) {
	flow := NewConfirmationFlow()

	proposal := &ExecutionProposal{
		TaskSummary:       "测试任务",
		EstimatedDuration: 30 * time.Minute,
		TeamConfig:        &TeamConfiguration{Members: []*TeamMember{{Role: "agent"}}},
		ExecutionPlan:      []*ExecutionStep{},
	}

	options := flow.GetAdjustmentOptions(proposal)

	assert.Contains(t, options, "调整选项")
	assert.Contains(t, options, "team_size")
	assert.Contains(t, options, "model")
	assert.Contains(t, options, "priority")
	assert.Contains(t, options, "duration")
	assert.Contains(t, options, "show")
	assert.Contains(t, options, "done")
}

func TestGetRequiredRoles(t *testing.T) {
	flow := NewConfirmationFlow()

	tests := []struct {
		taskType  string
		expected []string
	}{
		{"coding", []string{"architect", "developer", "reviewer", "tester"}},
		{"analysis", []string{"analyst", "reviewer"}},
		{"design", []string{"designer", "reviewer"}},
		{"testing", []string{"tester", "developer"}},
		{"documentation", []string{"writer", "reviewer"}},
		{"unknown", []string{"agent", "reviewer"}},
	}

	for _, tt := range tests {
		t.Run(tt.taskType, func(t *testing.T) {
			roles := flow.getRequiredRoles(tt.taskType)
			assert.Equal(t, tt.expected, roles)
		})
	}
}

func TestFormatStatus(t *testing.T) {
	flow := NewConfirmationFlow()

	proposal := &ExecutionProposal{
		TaskSummary:       "测试",
		EstimatedDuration: 30 * time.Minute,
		TeamConfig:        &TeamConfiguration{Members: []*TeamMember{{Role: "agent"}}},
		ExecutionPlan:      []*ExecutionStep{},
	}

	states := []ConfirmationState{StatePending, StateApproved, StateRejected, StateCancelled, StateAdjusting}

	for _, state := range states {
		t.Run(string(state), func(t *testing.T) {
			req := &ConfirmationRequest{
				State:    state,
				Proposal: proposal,
			}
			status := flow.FormatStatus(req)
			assert.NotEmpty(t, status)
		})
	}
}

func TestCalculateProgress(t *testing.T) {
	flow := NewConfirmationFlow()

	proposal := &ExecutionProposal{
		TaskSummary:       "测试",
		EstimatedDuration: 30 * time.Minute,
		TeamConfig:        &TeamConfiguration{Members: []*TeamMember{{Role: "agent"}}},
		ExecutionPlan:      []*ExecutionStep{},
	}

	req := &ConfirmationRequest{
		Proposal: proposal,
	}

	progress := flow.CalculateProgress(req)
	assert.Equal(t, 0.0, progress)
}

func TestGetSummary(t *testing.T) {
	flow := NewConfirmationFlow()

	proposal := &ExecutionProposal{
		TaskSummary:       "测试任务",
		TaskType:          "coding",
		Priority:          "high",
		EstimatedDuration: 90 * time.Minute,
		TeamConfig: &TeamConfiguration{
			Members: []*TeamMember{
				{Role: "developer", Model: "gpt-4"},
				{Role: "reviewer", Model: "claude-3-opus"},
			},
		},
		ExecutionPlan: []*ExecutionStep{
			{ID: "step-1", Order: 0, Name: "Step 1", Duration: 30 * time.Minute},
			{ID: "step-2", Order: 1, Name: "Step 2", Duration: 60 * time.Minute},
		},
	}

	req, _ := flow.CreateConfirmationRequest(nil, "task-1", "user-1", proposal)

	summary := flow.GetSummary(req)

	assert.Equal(t, "task-1", summary["task_id"])
	assert.Equal(t, "测试任务", summary["task_summary"])
	assert.Equal(t, "coding", summary["task_type"])
	assert.Equal(t, "1h30m0s", summary["estimated_duration"])
	assert.Equal(t, 2, summary["team_size"])
	assert.Equal(t, 2, summary["steps_count"])
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly ten!", 12, "exactly ten!"},
		{"this is way too long", 10, "this is..."},
		{"a", 1, "a"},
		{"ab", 2, "ab"},
		{"abc", 2, "ab"},
		{"abcd", 3, "abc"},
		{"abcde", 4, "a..."},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAdjustTeamSize(t *testing.T) {
	original := &TeamConfiguration{
		Members: []*TeamMember{
			{Role: "developer", Model: "gpt-4"},
		},
	}

	// 扩大团队
	expanded := adjustTeamSize(original, 3)
	assert.Equal(t, 3, len(expanded.Members))
	assert.Equal(t, "developer", expanded.Members[1].Role)
	assert.Equal(t, "developer", expanded.Members[2].Role)

	// 缩小团队
	shrunk := adjustTeamSize(original, 1)
	assert.Equal(t, 1, len(shrunk.Members))
	assert.Equal(t, "developer", shrunk.Members[0].Role)
}

func TestUpdateModel(t *testing.T) {
	original := &TeamConfiguration{
		Members: []*TeamMember{
			{Role: "developer", Model: "gpt-4"},
			{Role: "reviewer", Model: "gpt-3.5-turbo"},
		},
	}

	updated := updateModel(original, "claude-3-opus")

	assert.Equal(t, 2, len(updated.Members))
	assert.Equal(t, "claude-3-opus", updated.Members[0].Model)
	assert.Equal(t, "claude-3-opus", updated.Members[1].Model)

	// 验证原始配置未改变
	assert.Equal(t, "gpt-4", original.Members[0].Model)
}

func TestCreateStandardProposal(t *testing.T) {
	flow := NewConfirmationFlow()

	task := &models.Task{
		ID:          "task-1",
		Title:       "实现用户登录功能",
		Description: "实现用户登录功能",
		Type:        "coding",
		Priority:    "high",
		Subtasks:    []models.Subtask{},
	}

	proposal, err := flow.CreateStandardProposal(task)
	require.NoError(t, err)

	assert.Equal(t, "实现用户登录功能", proposal.TaskSummary)
	assert.Equal(t, "coding", proposal.TaskType)
	assert.Equal(t, "high", proposal.Priority)
	assert.Greater(t, proposal.EstimatedDuration, time.Duration(0))
	assert.NotNil(t, proposal.TeamConfig)
	assert.NotEmpty(t, proposal.ExecutionPlan)
}

func TestEstimateDuration(t *testing.T) {
	flow := NewConfirmationFlow()

	tests := []struct {
		priority     string
		taskType     string
		subTaskCount int
		minDuration  time.Duration
	}{
		{"high", "coding", 0, 45 * time.Minute}, // 30 * 1.5 = 45
		{"medium", "coding", 0, 30 * time.Minute}, // 30 * 1.0 = 30
		{"low", "coding", 0, 20 * time.Minute},   // 30 * 2/3 = 20
		{"medium", "analysis", 0, 22*time.Minute + 30*time.Second}, // 30 * 3/4
		{"medium", "testing", 0, 24 * time.Minute}, // 30 * 4/5
		{"medium", "coding", 3, 45 * time.Minute}, // 30 * (1+3)/2 = 45
	}

	for _, tt := range tests {
		t.Run(tt.priority+"_"+tt.taskType, func(t *testing.T) {
			task := &models.Task{
				Priority: tt.priority,
				Type:     tt.taskType,
				Subtasks: make([]models.Subtask, tt.subTaskCount),
			}
			duration := flow.estimateDuration(task)
			assert.True(t, duration >= tt.minDuration,
				fmt.Sprintf("Expected %v >= %v for %s/%s", duration, tt.minDuration, tt.priority, tt.taskType))
		})
	}
}

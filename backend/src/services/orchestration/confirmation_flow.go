package orchestration

import (
	"context"
	"fmt"
	"strings"
	"time"

	"openaide/backend/src/models"

	"github.com/google/uuid"
)

// ConfirmationState 确认状态
type ConfirmationState string

const (
	// StatePending 等待用户确认
	StatePending ConfirmationState = "pending"
	// StateApproved 用户已批准
	StateApproved ConfirmationState = "approved"
	// StateRejected 用户已拒绝
	StateRejected ConfirmationState = "rejected"
	// StateCancelled 用户取消
	StateCancelled ConfirmationState = "cancelled"
	// StateAdjusting 用户要求调整
	StateAdjusting ConfirmationState = "adjusting"
)

// ConfirmationAction 用户确认操作
type ConfirmationAction string

const (
	// ActionApprove 批准执行
	ActionApprove ConfirmationAction = "approve"
	// ActionReject 拒绝执行
	ActionReject ConfirmationAction = "reject"
	// ActionCancel 取消任务
	ActionCancel ConfirmationAction = "cancel"
	// ActionAdjust 调整方案
	ActionAdjust ConfirmationAction = "adjust"
)

// ConfirmationRequest 确认请求
type ConfirmationRequest struct {
	ID        string              `json:"id"`
	TaskID    string              `json:"task_id"`
	UserID    string              `json:"user_id"`
	State     ConfirmationState   `json:"state"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`

	// 方案信息
	Proposal  *ExecutionProposal  `json:"proposal"`
	Response  *ConfirmationResponse `json:"response,omitempty"`

	// 调整历史
	Adjustments []*Adjustment     `json:"adjustments,omitempty"`
}

// ExecutionProposal 执行方案
type ExecutionProposal struct {
	// 任务摘要
	TaskSummary string `json:"task_summary"`
	TaskType    string `json:"task_type"`
	Priority    string `json:"priority"`

	// 时间估算
	EstimatedDuration time.Duration `json:"estimated_duration"`
	EstimatedCost    float64       `json:"estimated_cost,omitempty"`

	// 团队配置
	TeamConfig *TeamConfiguration `json:"team_config"`

	// 执行计划
	ExecutionPlan []*ExecutionStep `json:"execution_plan"`

	// 资源需求
	ResourceRequirements map[string]interface{} `json:"resource_requirements,omitempty"`
}

// TeamConfiguration 团队配置
type TeamConfiguration struct {
	Members []*TeamMember `json:"members"`
}

// TeamMember 团队成员
type TeamMember struct {
	Role      string            `json:"role"`       // architect, developer, reviewer, tester
	Skills    []string          `json:"skills"`
	Model     string            `json:"model"`
	Config    map[string]interface{} `json:"config,omitempty"`
}

// ExecutionStep 执行步骤
type ExecutionStep struct {
	ID          string        `json:"id"`
	Order       int           `json:"order"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	AssignedTo  string        `json:"assigned_to"` // role name
	Duration    time.Duration `json:"duration"`
	Dependencies []string     `json:"dependencies,omitempty"`
}

// ConfirmationResponse 用户响应
type ConfirmationResponse struct {
	Action     ConfirmationAction `json:"action"`
	Comment    string             `json:"comment,omitempty"`
	RespondedAt time.Time          `json:"responded_at"`
}

// Adjustment 方案调整
type Adjustment struct {
	ID         string             `json:"id"`
	RequestedAt time.Time          `json:"requested_at"`
	Field      string             `json:"field"`
	OldValue   interface{}        `json:"old_value"`
	NewValue   interface{}        `json:"new_value"`
	Reason     string             `json:"reason"`
}

// ConfirmationFlow 确认流程服务
type ConfirmationFlow struct {
	// 可以添加依赖服务
	// logger     *LoggerService
	// wsService  *WebSocketService
}

// NewConfirmationFlow 创建确认流程服务
func NewConfirmationFlow() *ConfirmationFlow {
	return &ConfirmationFlow{}
}

// CreateConfirmationRequest 创建确认请求
func (cf *ConfirmationFlow) CreateConfirmationRequest(ctx context.Context, taskID, userID string, proposal *ExecutionProposal) (*ConfirmationRequest, error) {
	req := &ConfirmationRequest{
		ID:        uuid.New().String(),
		TaskID:    taskID,
		UserID:    userID,
		State:     StatePending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Proposal:  proposal,
	}

	return req, nil
}

// FormatProposal 格式化方案展示
func (cf *ConfirmationFlow) FormatProposal(proposal *ExecutionProposal) string {
	var sb strings.Builder

	// 标题
	sb.WriteString("🎯 任务方案\n\n")

	// 任务信息
	sb.WriteString(fmt.Sprintf("📋 任务: %s\n", proposal.TaskSummary))
	sb.WriteString(fmt.Sprintf("🏷️  类型: %s\n", proposal.TaskType))
	sb.WriteString(fmt.Sprintf("⏱️  预估: %s\n", formatDuration(proposal.EstimatedDuration)))
	if proposal.EstimatedCost > 0 {
		sb.WriteString(fmt.Sprintf("💰 成本: $%.2f\n", proposal.EstimatedCost))
	}
	sb.WriteString("\n")

	// 团队配置
	sb.WriteString("👥 团队配置:\n")
	cf.formatTeamConfig(&sb, proposal.TeamConfig)
	sb.WriteString("\n")

	// 执行计划
	sb.WriteString("📝 执行计划:\n")
	for i, step := range proposal.ExecutionPlan {
		duration := formatDuration(step.Duration)
		deps := ""
		if len(step.Dependencies) > 0 {
			deps = fmt.Sprintf(" (依赖: %s)", strings.Join(step.Dependencies, ", "))
		}
		sb.WriteString(fmt.Sprintf("%d. %s (%s) - %s%s\n",
			i+1, step.Name, step.AssignedTo, duration, deps))
	}
	sb.WriteString("\n")

	// 资源需求
	if len(proposal.ResourceRequirements) > 0 {
		sb.WriteString("🔧 资源需求:\n")
		for key, value := range proposal.ResourceRequirements {
			sb.WriteString(fmt.Sprintf("  • %s: %v\n", key, value))
		}
	}

	return sb.String()
}

// formatTeamConfig 格式化团队配置
func (cf *ConfirmationFlow) formatTeamConfig(sb *strings.Builder, config *TeamConfiguration) {
	if config == nil || len(config.Members) == 0 {
		sb.WriteString("  (未配置团队)\n")
		return
	}

	// 表头
	sb.WriteString("┌─────────────┬──────────────┬───────────────┐\n")
	sb.WriteString("│ 角色        │ 技能         │ 模型          │\n")
	sb.WriteString("├─────────────┼──────────────┼───────────────┤\n")

	// 成员行
	for _, member := range config.Members {
		role := truncateString(member.Role, 11)
		skills := truncateString(strings.Join(member.Skills, ", "), 12)
		model := truncateString(member.Model, 13)
		sb.WriteString(fmt.Sprintf("│ %-11s │ %-12s │ %-13s │\n", role, skills, model))
	}

	sb.WriteString("└─────────────┴──────────────┴───────────────┘")
}

// RenderPrompt 渲染确认提示
func (cf *ConfirmationFlow) RenderPrompt(proposal *ExecutionProposal) string {
	var sb strings.Builder

	sb.WriteString(cf.FormatProposal(proposal))
	sb.WriteString("\n确认执行? [Y/n/adjust] ")
	sb.WriteString("\n\n")
	sb.WriteString("  Y - 确认执行\n")
	sb.WriteString("  n - 取消任务\n")
	sb.WriteString("  adjust - 调整方案\n")

	return sb.String()
}

// ProcessUserAction 处理用户操作
func (cf *ConfirmationFlow) ProcessUserAction(ctx context.Context, req *ConfirmationRequest, action ConfirmationAction, comment string) (*ConfirmationRequest, error) {
	req.Response = &ConfirmationResponse{
		Action:     action,
		Comment:    comment,
		RespondedAt: time.Now(),
	}

	switch action {
	case ActionApprove:
		req.State = StateApproved

	case ActionReject:
		req.State = StateRejected

	case ActionCancel:
		req.State = StateCancelled

	case ActionAdjust:
		req.State = StateAdjusting
		// 调整需要进一步处理
	}

	req.UpdatedAt = time.Now()
	return req, nil
}

// CreateAdjustment 创建调整记录
func (cf *ConfirmationFlow) CreateAdjustment(req *ConfirmationRequest, field string, oldValue, newValue interface{}, reason string) *Adjustment {
	adjustment := &Adjustment{
		ID:         uuid.New().String(),
		RequestedAt: time.Now(),
		Field:      field,
		OldValue:   oldValue,
		NewValue:   newValue,
		Reason:     reason,
	}

	req.Adjustments = append(req.Adjustments, adjustment)
	req.State = StatePending // 重新进入待确认状态
	req.UpdatedAt = time.Now()

	return adjustment
}

// ApplyAdjustment 应用调整到方案
func (cf *ConfirmationFlow) ApplyAdjustment(proposal *ExecutionProposal, adjustment *Adjustment) error {
	switch adjustment.Field {
	case "team_size":
		// 调整团队规模
		newSize, ok := adjustment.NewValue.(int)
		if !ok || newSize < 1 {
			return fmt.Errorf("invalid team size")
		}
		proposal.TeamConfig = adjustTeamSize(proposal.TeamConfig, newSize)

	case "model":
		// 更换模型
		newModel, ok := adjustment.NewValue.(string)
		if !ok || newModel == "" {
			return fmt.Errorf("invalid model")
		}
		proposal.TeamConfig = updateModel(proposal.TeamConfig, newModel)

	case "priority":
		// 调整优先级
		newPriority, ok := adjustment.NewValue.(string)
		if !ok {
			return fmt.Errorf("invalid priority")
		}
		proposal.Priority = newPriority

	case "estimated_duration":
		// 调整预估时间
		newDuration, ok := adjustment.NewValue.(time.Duration)
		if !ok {
			return fmt.Errorf("invalid duration")
		}
		proposal.EstimatedDuration = newDuration

	default:
		return fmt.Errorf("unknown field: %s", adjustment.Field)
	}

	return nil
}

// GetAdjustmentOptions 获取可调整选项
func (cf *ConfirmationFlow) GetAdjustmentOptions(proposal *ExecutionProposal) string {
	var sb strings.Builder

	sb.WriteString("\n🔄 调整选项:\n\n")

	sb.WriteString("1. team_size <n> - 调整团队规模\n")
	sb.WriteString("2. model <model_id> - 更换 Agent 模型\n")
	sb.WriteString("3. priority <low|medium|high> - 调整任务优先级\n")
	sb.WriteString("4. duration <minutes> - 调整预估时间\n")
	sb.WriteString("5. show - 显示当前方案\n")
	sb.WriteString("6. done - 完成调整\n")

	return sb.String()
}

// FormatAdjustmentPreview 格式化调整预览
func (cf *ConfirmationFlow) FormatAdjustmentPreview(proposal *ExecutionProposal) string {
	return cf.FormatProposal(proposal)
}

// IsApproved 检查是否已批准
func (cf *ConfirmationFlow) IsApproved(req *ConfirmationRequest) bool {
	return req.State == StateApproved
}

// IsRejected 检查是否已拒绝
func (cf *ConfirmationFlow) IsRejected(req *ConfirmationRequest) bool {
	return req.State == StateRejected
}

// IsCancelled 检查是否已取消
func (cf *ConfirmationFlow) IsCancelled(req *ConfirmationRequest) bool {
	return req.State == StateCancelled
}

// IsPending 检查是否等待确认
func (cf *ConfirmationFlow) IsPending(req *ConfirmationRequest) bool {
	return req.State == StatePending
}

// GetState 获取当前状态
func (cf *ConfirmationFlow) GetState(req *ConfirmationRequest) ConfirmationState {
	return req.State
}

// ValidateProposal 验证方案
func (cf *ConfirmationFlow) ValidateProposal(proposal *ExecutionProposal) error {
	if proposal.TaskSummary == "" {
		return fmt.Errorf("task summary is required")
	}

	if proposal.TaskType == "" {
		return fmt.Errorf("task type is required")
	}

	if proposal.EstimatedDuration <= 0 {
		return fmt.Errorf("estimated duration must be positive")
	}

	if proposal.TeamConfig == nil || len(proposal.TeamConfig.Members) == 0 {
		return fmt.Errorf("team configuration is required")
	}

	if len(proposal.ExecutionPlan) == 0 {
		return fmt.Errorf("execution plan is required")
	}

	// 验证执行步骤
	stepIDs := make(map[string]bool)
	for i, step := range proposal.ExecutionPlan {
		if step.ID == "" {
			return fmt.Errorf("step %d: ID is required", i+1)
		}
		if stepIDs[step.ID] {
			return fmt.Errorf("duplicate step ID: %s", step.ID)
		}
		stepIDs[step.ID] = true

		if step.Order != i {
			return fmt.Errorf("step %d: order mismatch", i+1)
		}

		// 验证依赖
		for _, dep := range step.Dependencies {
			if !stepIDs[dep] {
				return fmt.Errorf("step %s: unknown dependency %s", step.ID, dep)
			}
		}
	}

	return nil
}

// CloneProposal 克隆方案
func (cf *ConfirmationFlow) CloneProposal(proposal *ExecutionProposal) *ExecutionProposal {
	// 深拷贝执行步骤
	steps := make([]*ExecutionStep, len(proposal.ExecutionPlan))
	for i, step := range proposal.ExecutionPlan {
		stepCopy := *step
		// 克隆依赖数组
		if step.Dependencies != nil {
			deps := make([]string, len(step.Dependencies))
			copy(deps, step.Dependencies)
			stepCopy.Dependencies = deps
		}
		steps[i] = &stepCopy
	}

	// 深拷贝团队配置
	var teamConfig *TeamConfiguration
	if proposal.TeamConfig != nil {
		members := make([]*TeamMember, len(proposal.TeamConfig.Members))
		for i, m := range proposal.TeamConfig.Members {
			// 深拷贝每个成员
			memberCopy := *m
			// 深拷贝技能数组
			if m.Skills != nil {
				skills := make([]string, len(m.Skills))
				copy(skills, m.Skills)
				memberCopy.Skills = skills
			}
			members[i] = &memberCopy
		}
		teamConfig = &TeamConfiguration{Members: members}
	}

	// 深拷贝资源需求
	resources := make(map[string]interface{})
	for k, v := range proposal.ResourceRequirements {
		resources[k] = v
	}

	return &ExecutionProposal{
		TaskSummary:          proposal.TaskSummary,
		TaskType:             proposal.TaskType,
		Priority:             proposal.Priority,
		EstimatedDuration:    proposal.EstimatedDuration,
		EstimatedCost:        proposal.EstimatedCost,
		TeamConfig:           teamConfig,
		ExecutionPlan:        steps,
		ResourceRequirements:  resources,
	}
}

// ==================== 辅助函数 ====================

// formatDuration 格式化持续时间
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		if seconds > 0 {
			return fmt.Sprintf("%dm%ds", minutes, seconds)
		}
		return fmt.Sprintf("%dm", minutes)
	} else if d < 24*time.Hour {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		if minutes > 0 {
			return fmt.Sprintf("%dh%dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	} else {
		days := int(d.Hours() / 24)
		hours := int(d.Hours()) % 24
		if hours > 0 {
			return fmt.Sprintf("%dd%dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	}
}

// truncateString 截断字符串到指定长度
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// adjustTeamSize 调整团队规模
func adjustTeamSize(config *TeamConfiguration, newSize int) *TeamConfiguration {
	if config == nil {
		return &TeamConfiguration{
			Members: []*TeamMember{},
		}
	}

	currentSize := len(config.Members)
	if newSize == currentSize {
		return config
	}

	members := make([]*TeamMember, newSize)
	copy(members, config.Members)

	// 如果需要填充，复制现有成员
	for i := currentSize; i < newSize; i++ {
		template := config.Members[currentSize-1]
		memberCopy := *template
		members[i] = &memberCopy
	}

	return &TeamConfiguration{Members: members}
}

// updateModel 更新模型
func updateModel(config *TeamConfiguration, newModel string) *TeamConfiguration {
	if config == nil {
		return config
	}

	members := make([]*TeamMember, len(config.Members))
	for i, member := range config.Members {
		memberCopy := *member
		memberCopy.Model = newModel
		members[i] = &memberCopy
	}

	return &TeamConfiguration{Members: members}
}

// CreateStandardProposal 创建标准方案
func (cf *ConfirmationFlow) CreateStandardProposal(task *models.Task) (*ExecutionProposal, error) {
	// 计算预估时间（基于任务复杂度）
	estimatedDuration := cf.estimateDuration(task)

	// 创建团队配置
	teamConfig := cf.createTeamConfig(task)

	// 创建执行计划
	executionPlan := cf.createExecutionPlan(task, teamConfig)

	return &ExecutionProposal{
		TaskSummary:       task.Description,
		TaskType:          task.Type,
		Priority:          task.Priority,
		EstimatedDuration: estimatedDuration,
		TeamConfig:        teamConfig,
		ExecutionPlan:     executionPlan,
	}, nil
}

// estimateDuration 估算任务持续时间
func (cf *ConfirmationFlow) estimateDuration(task *models.Task) time.Duration {
	// 基础时间：30分钟
	baseTime := 30 * time.Minute

	// 根据优先级调整
	switch task.Priority {
	case "high":
		// 高优先级可能更复杂
		baseTime = baseTime * 3 / 2
	case "low":
		// 低优先级相对简单
		baseTime = baseTime * 2 / 3
	}

	// 根据类型调整
	switch task.Type {
	case "coding":
		// 编码任务需要更多时间
		baseTime = baseTime * 2
	case "analysis":
		// 分析任务相对较快
		baseTime = baseTime * 3 / 4
	case "testing":
		// 测试任务
		baseTime = baseTime * 4 / 5
	}

	// 根据子任务数量调整
	if len(task.Subtasks) > 0 {
		baseTime = baseTime * time.Duration(1+len(task.Subtasks)/2)
	}

	return baseTime
}

// createTeamConfig 创建团队配置
func (cf *ConfirmationFlow) createTeamConfig(task *models.Task) *TeamConfiguration {
	// 根据任务类型确定所需角色
	roles := cf.getRequiredRoles(task.Type)

	members := make([]*TeamMember, len(roles))
	for i, role := range roles {
		members[i] = &TeamMember{
			Role:   role,
			Skills: cf.getRoleSkills(role),
			Model:  cf.getDefaultModel(role),
		}
	}

	return &TeamConfiguration{Members: members}
}

// getRequiredRoles 获取所需角色
func (cf *ConfirmationFlow) getRequiredRoles(taskType string) []string {
	switch taskType {
	case "coding":
		return []string{"architect", "developer", "reviewer", "tester"}
	case "analysis":
		return []string{"analyst", "reviewer"}
	case "design":
		return []string{"designer", "reviewer"}
	case "testing":
		return []string{"tester", "developer"}
	case "documentation":
		return []string{"writer", "reviewer"}
	default:
		return []string{"agent", "reviewer"}
	}
}

// getRoleSkills 获取角色技能
func (cf *ConfirmationFlow) getRoleSkills(role string) []string {
	skills := map[string][]string{
		"architect":  {"架构设计", "系统设计", "技术选型"},
		"developer":  {"编码实现", "调试", "单元测试"},
		"reviewer":   {"代码审查", "质量保证", "最佳实践"},
		"tester":     {"测试设计", "测试执行", "缺陷报告"},
		"analyst":    {"需求分析", "数据分析", "报告撰写"},
		"designer":   {"UI设计", "UX设计", "原型设计"},
		"writer":     {"文档编写", "技术写作", "内容组织"},
		"agent":      {"通用任务", "问题解决", "沟通协作"},
	}

	return skills[role]
}

// getDefaultModel 获取角色默认模型
func (cf *ConfirmationFlow) getDefaultModel(role string) string {
	models := map[string]string{
		"architect": "claude-3-opus",
		"developer":  "claude-3.5-sonnet",
		"reviewer":   "claude-3.5-sonnet",
		"tester":     "claude-3-haiku",
		"analyst":    "claude-3-sonnet",
		"designer":   "claude-3-opus",
		"writer":     "claude-3-sonnet",
		"agent":      "claude-3.5-sonnet",
	}

	if model, ok := models[role]; ok {
		return model
	}
	return "claude-3.5-sonnet"
}

// createExecutionPlan 创建执行计划
func (cf *ConfirmationFlow) createExecutionPlan(task *models.Task, teamConfig *TeamConfiguration) []*ExecutionStep {
	var steps []*ExecutionStep

	switch task.Type {
	case "coding":
		steps = []*ExecutionStep{
			{
				ID:          "step-1",
				Order:       0,
				Name:        "架构设计",
				Description: "分析需求并设计系统架构",
				AssignedTo:  "architect",
				Duration:    20 * time.Minute,
			},
			{
				ID:          "step-2",
				Order:       1,
				Name:        "编码实现",
				Description: "实现核心功能模块",
				AssignedTo:  "developer",
				Duration:    60 * time.Minute,
				Dependencies: []string{"step-1"},
			},
			{
				ID:          "step-3",
				Order:       2,
				Name:        "代码审查",
				Description: "审查代码质量和最佳实践",
				AssignedTo:  "reviewer",
				Duration:    20 * time.Minute,
				Dependencies: []string{"step-2"},
			},
			{
				ID:          "step-4",
				Order:       3,
				Name:        "测试验证",
				Description: "执行测试并修复问题",
				AssignedTo:  "tester",
				Duration:    20 * time.Minute,
				Dependencies: []string{"step-3"},
			},
		}
	case "analysis":
		steps = []*ExecutionStep{
			{
				ID:          "step-1",
				Order:       0,
				Name:        "数据收集",
				Description: "收集相关数据和信息",
				AssignedTo:  "analyst",
				Duration:    30 * time.Minute,
			},
			{
				ID:          "step-2",
				Order:       1,
				Name:        "分析处理",
				Description: "进行数据分析并得出结论",
				AssignedTo:  "analyst",
				Duration:    30 * time.Minute,
				Dependencies: []string{"step-1"},
			},
			{
				ID:          "step-3",
				Order:       2,
				Name:        "报告撰写",
				Description: "撰写分析报告",
				AssignedTo:  "analyst",
				Duration:    20 * time.Minute,
				Dependencies: []string{"step-2"},
			},
		}
	default:
		// 通用执行计划
		steps = []*ExecutionStep{
			{
				ID:          "step-1",
				Order:       0,
				Name:        "任务分析",
				Description: "分析任务需求和目标",
				AssignedTo:  "agent",
				Duration:    15 * time.Minute,
			},
			{
				ID:          "step-2",
				Order:       1,
				Name:        "任务执行",
				Description: "执行主要任务内容",
				AssignedTo:  "agent",
				Duration:    30 * time.Minute,
				Dependencies: []string{"step-1"},
			},
			{
				ID:          "step-3",
				Order:       2,
				Name:        "结果审核",
				Description: "审核和验证任务结果",
				AssignedTo:  "agent",
				Duration:    15 * time.Minute,
				Dependencies: []string{"step-2"},
			},
		}
	}

	return steps
}

// CalculateProgress 计算执行进度
func (cf *ConfirmationFlow) CalculateProgress(req *ConfirmationRequest) float64 {
	if req.Proposal == nil || len(req.Proposal.ExecutionPlan) == 0 {
		return 0
	}

	totalSteps := len(req.Proposal.ExecutionPlan)
	if totalSteps == 0 {
		return 0
	}

	// 这里可以添加实际进度跟踪逻辑
	// 目前返回 0 表示未开始
	return 0
}

// FormatStatus 格式化状态信息
func (cf *ConfirmationFlow) FormatStatus(req *ConfirmationRequest) string {
	var sb strings.Builder

	switch req.State {
	case StatePending:
		sb.WriteString("⏳ 等待用户确认...")
	case StateApproved:
		sb.WriteString("✅ 方案已批准，开始执行...")
	case StateRejected:
		sb.WriteString("❌ 方案已拒绝")
		if req.Response != nil && req.Response.Comment != "" {
			sb.WriteString(fmt.Sprintf("\n原因: %s", req.Response.Comment))
		}
	case StateCancelled:
		sb.WriteString("🚫 任务已取消")
	case StateAdjusting:
		sb.WriteString("🔄 正在调整方案...")
	default:
		sb.WriteString(fmt.Sprintf("未知状态: %s", req.State))
	}

	return sb.String()
}

// GetSummary 获取确认摘要
func (cf *ConfirmationFlow) GetSummary(req *ConfirmationRequest) map[string]interface{} {
	return map[string]interface{}{
		"id":                req.ID,
		"task_id":           req.TaskID,
		"state":             req.State,
		"created_at":        req.CreatedAt,
		"updated_at":        req.UpdatedAt,
		"task_summary":      req.Proposal.TaskSummary,
		"task_type":         req.Proposal.TaskType,
		"estimated_duration": req.Proposal.EstimatedDuration.String(),
		"team_size":         len(req.Proposal.TeamConfig.Members),
		"steps_count":       len(req.Proposal.ExecutionPlan),
	}
}

package services

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"openaide/backend/src/services/llm"
)

// AgentRole Agent 角色定义
type AgentRole string

const (
	RolePlanner  AgentRole = "planner"  // 规划者：分析任务、制定计划
	RoleExecutor AgentRole = "executor" // 执行者：执行具体操作
	RoleReviewer AgentRole = "reviewer" // 审查者：检查输出质量
)

// AgentMessage Agent 间通信消息
type AgentMessage struct {
	From    AgentRole `json:"from"`
	To      AgentRole `json:"to"`
	Content string    `json:"content"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

// AgentOutput Agent 输出结果
type AgentOutput struct {
	Role      AgentRole              `json:"role"`
	Content   string                 `json:"content"`
	Data      map[string]interface{} `json:"data,omitempty"`
	TokensUsed int                   `json:"tokens_used,omitempty"`
	Duration  time.Duration          `json:"duration"`
}

// AgentConfig Agent 配置
type AgentConfig struct {
	Role        AgentRole `json:"role"`
	Name        string    `json:"name"`
	SystemPrompt string   `json:"system_prompt"`
	ModelID     string    `json:"model_id,omitempty"`     // 可选：指定模型
	ModelTag    string    `json:"model_tag,omitempty"`    // 可选：按标签选模型
}

// MultiAgentService 多 Agent 协作服务
// 简化版：Planner → Executor → Reviewer 流水线
type MultiAgentService struct {
	modelSvc *ModelService
	router   *ModelRouter
	logger   *LoggerService
}

// NewMultiAgentService 创建多 Agent 服务
func NewMultiAgentService(modelSvc *ModelService, router *ModelRouter, logger *LoggerService) *MultiAgentService {
	return &MultiAgentService{
		modelSvc: modelSvc,
		router:   router,
		logger:   logger,
	}
}

// DefaultAgentConfigs 默认 Agent 配置
func (s *MultiAgentService) DefaultAgentConfigs() map[AgentRole]AgentConfig {
	return map[AgentRole]AgentConfig{
		RolePlanner: {
			Role:         RolePlanner,
			Name:         "Planner",
			SystemPrompt: `你是一个任务规划专家。你的职责是：
1. 分析用户的请求，理解核心需求
2. 将复杂任务分解为可执行的步骤
3. 为每个步骤明确输入、输出和依赖关系
4. 评估每个步骤的难度和优先级

请以 JSON 格式输出计划：
{
  "goal": "总体目标",
  "steps": [
    {
      "order": 1,
      "action": "具体操作",
      "description": "详细描述",
      "expected_output": "预期输出",
      "dependencies": []
    }
  ],
  "estimated_difficulty": "low/medium/high",
  "notes": "注意事项"
}`,
			ModelTag: "reasoning",
		},
		RoleExecutor: {
			Role:         RoleExecutor,
			Name:         "Executor",
			SystemPrompt: `你是一个任务执行专家。你的职责是：
1. 按照计划逐步执行任务
2. 每个步骤都给出具体、准确的结果
3. 如果遇到问题，清晰描述问题和解决思路
4. 保持输出结构化和可读性

请严格按照给定的计划执行，不要跳过任何步骤。`,
			ModelTag: "code",
		},
		RoleReviewer: {
			Role:         RoleReviewer,
			Name:         "Reviewer",
			SystemPrompt: `你是一个质量审查专家。你的职责是：
1. 检查执行结果是否满足原始需求
2. 评估每个步骤的完成质量
3. 识别潜在的问题和改进空间
4. 给出明确的通过/修改建议

请以以下格式输出审查结果：
{
  "verdict": "pass/needs_revision",
  "quality_score": 1-10,
  "issues": ["问题列表"],
  "suggestions": ["改进建议"],
  "summary": "总体评价"
}`,
			ModelTag: "reasoning",
		},
	}
}

// CollaborativeProcess 协作处理：Planner → Executor → Reviewer
func (s *MultiAgentService) CollaborativeProcess(ctx context.Context, userRequest string, maxRevisionRounds int) (*CollaborativeResult, error) {
	configs := s.DefaultAgentConfigs()
	startTime := time.Now()

	result := &CollaborativeResult{
		Request: userRequest,
		Steps:   make([]*AgentOutput, 0),
	}

	// 1. Planner 分析任务
	planOutput, err := s.runAgent(ctx, configs[RolePlanner], userRequest, nil)
	if err != nil {
		return nil, fmt.Errorf("planner failed: %w", err)
	}
	result.Steps = append(result.Steps, planOutput)

	// 2. Executor 执行计划
	executorInput := fmt.Sprintf("用户请求：%s\n\n计划：%s\n\n请按照以上计划逐步执行。", userRequest, planOutput.Content)
	execOutput, err := s.runAgent(ctx, configs[RoleExecutor], executorInput, nil)
	if err != nil {
		return nil, fmt.Errorf("executor failed: %w", err)
	}
	result.Steps = append(result.Steps, execOutput)

	// 3. Reviewer 审查结果（可多轮）
	reviewerInput := fmt.Sprintf("用户请求：%s\n\n执行结果：%s\n\n请审查执行结果是否满足用户需求。", userRequest, execOutput.Content)

	for round := 0; round <= maxRevisionRounds; round++ {
		reviewOutput, err := s.runAgent(ctx, configs[RoleReviewer], reviewerInput, nil)
		if err != nil {
			log.Printf("[MultiAgent] reviewer failed on round %d: %v", round, err)
			break
		}
		result.Steps = append(result.Steps, reviewOutput)

		// 检查是否通过
		if strings.Contains(strings.ToLower(reviewOutput.Content), `"pass"`) ||
			strings.Contains(strings.ToLower(reviewOutput.Content), "verdict: pass") ||
			strings.Contains(strings.ToLower(reviewOutput.Content), "通过") {
			result.Approved = true
			break
		}

		// 需要修改，重新执行
		if round < maxRevisionRounds {
			revisionInput := fmt.Sprintf("原始请求：%s\n\n上一次执行结果：%s\n\n审查意见（第%d轮）：%s\n\n请根据审查意见修改执行结果。",
				userRequest, execOutput.Content, round+1, reviewOutput.Content)
			execOutput, err = s.runAgent(ctx, configs[RoleExecutor], revisionInput, nil)
			if err != nil {
				break
			}
			result.Steps = append(result.Steps, execOutput)
			reviewerInput = fmt.Sprintf("用户请求：%s\n\n修改后的执行结果：%s\n\n请重新审查。", userRequest, execOutput.Content)
		}
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// CollaborativeResult 协作结果
type CollaborativeResult struct {
	Request  string         `json:"request"`
	Steps    []*AgentOutput `json:"steps"`
	Approved bool           `json:"approved"`
	Duration time.Duration  `json:"duration"`
}

// ==================== Agent 执行 ====================

// runAgent 运行单个 Agent
func (s *MultiAgentService) runAgent(ctx context.Context, config AgentConfig, input string, options map[string]interface{}) (*AgentOutput, error) {
	start := time.Now()

	// 选择模型
	modelID := config.ModelID
	if modelID == "" && config.ModelTag != "" && s.router != nil {
		model, err := s.router.Route(ctx, input, map[string]bool{})
		if err == nil {
			modelID = model.ID
		}
	}
	if modelID == "" {
		defaultModel, err := s.modelSvc.GetDefaultModel()
		if err != nil {
			return nil, fmt.Errorf("no model available: %w", err)
		}
		modelID = defaultModel.ID
	}

	messages := []llm.Message{
		{Role: "system", Content: config.SystemPrompt},
		{Role: "user", Content: input},
	}

	opts := map[string]interface{}{
		"temperature": 0.7,
	}
	if options != nil {
		for k, v := range options {
			opts[k] = v
		}
	}

	resp, err := s.modelSvc.Chat(modelID, messages, opts)
	if err != nil {
		return nil, err
	}

	content := ""
	tokensUsed := 0
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
	}
	if resp.Usage != nil {
		tokensUsed = resp.Usage.TotalTokens
	}

	log.Printf("[MultiAgent] %s completed in %v, tokens=%d", config.Name, time.Since(start), tokensUsed)

	return &AgentOutput{
		Role:       config.Role,
		Content:    content,
		TokensUsed: tokensUsed,
		Duration:   time.Since(start),
	}, nil
}

// RunSingleAgent 单独运行一个 Agent（用于自定义流程）
func (s *MultiAgentService) RunSingleAgent(ctx context.Context, role AgentRole, input string, customPrompt string) (*AgentOutput, error) {
	configs := s.DefaultAgentConfigs()
	config, ok := configs[role]
	if !ok {
		return nil, fmt.Errorf("unknown role: %s", role)
	}
	if customPrompt != "" {
		config.SystemPrompt = customPrompt
	}
	return s.runAgent(ctx, config, input, nil)
}

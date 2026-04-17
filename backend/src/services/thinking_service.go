package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// ThinkingService 思考服务
type ThinkingService struct {
	db        *gorm.DB
	llmClient llm.LLMClient
	mu        sync.RWMutex
}

// NewThinkingService 创建思考服务实例
func NewThinkingService(db *gorm.DB, llmClient llm.LLMClient) *ThinkingService {
	return &ThinkingService{
		db:        db,
		llmClient: llmClient,
	}
}

// ChainOfThoughtRequest 思维链请求
type ChainOfThoughtRequest struct {
	// Query 用户查询
	Query string `json:"query"`
	// Context 上下文信息
	Context string `json:"context,omitempty"`
	// UserID 用户ID
	UserID string `json:"user_id"`
	// ThoughtType 思考类型
	ThoughtType string `json:"thought_type,omitempty"`
	// MaxSteps 最大推理步数
	MaxSteps int `json:"max_steps,omitempty"`
	// Model 模型名称
	Model string `json:"model,omitempty"`
}

// ThoughtID 生成思考ID
func (r *ChainOfThoughtRequest) ThoughtID() string {
	return uuid.New().String()
}

// ChainOfThoughtResponse 思维链响应
type ChainOfThoughtResponse struct {
	// ThoughtID 思考ID
	ThoughtID string `json:"thought_id"`
	// ReasoningSteps 推理步骤
	ReasoningSteps []ReasoningStep `json:"reasoning_steps"`
	// FinalAnswer 最终答案
	FinalAnswer string `json:"final_answer"`
	// Confidence 置信度
	Confidence float64 `json:"confidence"`
	// Status 状态
	Status string `json:"status"`
	// Error 错误信息
	Error string `json:"error,omitempty"`
}

// ReasoningStep 推理步骤
type ReasoningStep struct {
	// StepNumber 步骤编号
	StepNumber int `json:"step_number"`
	// Content 步骤内容
	Content string `json:"content"`
	// StepType 步骤类型
	StepType string `json:"step_type"` // analyze, decompose, infer, verify, conclude
	// Status 状态
	Status string `json:"status"` // pending, in_progress, completed, failed
	// StartTime 开始时间
	StartTime time.Time `json:"start_time"`
	// EndTime 结束时间
	EndTime time.Time `json:"end_time"`
	// TokenUsage Token使用情况
	TokenUsage *llm.Usage `json:"token_usage,omitempty"`
}

// MultiStepReasoningRequest 多步推理请求
type MultiStepReasoningRequest struct {
	// Query 用户查询
	Query string `json:"query"`
	// Context 上下文信息
	Context string `json:"context,omitempty"`
	// UserID 用户ID
	UserID string `json:"user_id"`
	// ReasoningPlan 推理计划
	ReasoningPlan *ReasoningPlan `json:"reasoning_plan,omitempty"`
	// Model 模型名称
	Model string `json:"model,omitempty"`
}

// ThoughtID 生成思考ID
func (r *MultiStepReasoningRequest) ThoughtID() string {
	return uuid.New().String()
}

// ReasoningPlan 推理计划
type ReasoningPlan struct {
	// Steps 计划步骤
	Steps []PlanStep `json:"steps"`
	// Strategy 推理策略
	Strategy string `json:"strategy"` // linear, parallel, recursive, tree_search
}

// PlanStep 计划步骤
type PlanStep struct {
	// StepNumber 步骤编号
	StepNumber int `json:"step_number"`
	// Description 步骤描述
	Description string `json:"description"`
	// StepType 步骤类型
	StepType string `json:"step_type"`
	// Dependencies 依赖的步骤编号
	Dependencies []int `json:"dependencies,omitempty"`
	// PromptTemplate 提示词模板
	PromptTemplate string `json:"prompt_template,omitempty"`
}

// MultiStepReasoningResponse 多步推理响应
type MultiStepReasoningResponse struct {
	// ThoughtID 思考ID
	ThoughtID string `json:"thought_id"`
	// ReasoningPlan 推理计划
	ReasoningPlan *ReasoningPlan `json:"reasoning_plan"`
	// ExecutionResults 执行结果
	ExecutionResults []StepExecutionResult `json:"execution_results"`
	// FinalAnswer 最终答案
	FinalAnswer string `json:"final_answer"`
	// Status 状态
	Status string `json:"status"`
	// Progress 进度
	Progress float64 `json:"progress"`
	// Error 错误信息
	Error string `json:"error,omitempty"`
}

// StepExecutionResult 步骤执行结果
type StepExecutionResult struct {
	// StepNumber 步骤编号
	StepNumber int `json:"step_number"`
	// Content 内容
	Content string `json:"content"`
	// Status 状态
	Status string `json:"status"`
	// StartTime 开始时间
	StartTime time.Time `json:"start_time"`
	// EndTime 结束时间
	EndTime time.Time `json:"end_time"`
	// TokenUsage Token使用情况
	TokenUsage *llm.Usage `json:"token_usage,omitempty"`
	// Error 错误信息
	Error string `json:"error,omitempty"`
}

// ThinkingSessionState 思考会话状态
type ThinkingSessionState struct {
	// SessionID 会话ID
	SessionID string `json:"session_id"`
	// ThoughtID 思考ID
	ThoughtID string `json:"thought_id"`
	// Status 状态
	Status string `json:"status"` // initializing, reasoning, completing, failed, completed
	// CurrentStep 当前步骤
	CurrentStep int `json:"current_step"`
	// TotalSteps 总步骤数
	TotalSteps int `json:"total_steps"`
	// StartTime 开始时间
	StartTime time.Time `json:"start_time"`
	// LastUpdateTime 最后更新时间
	LastUpdateTime time.Time `json:"last_update_time"`
	// Context 上下文数据
	Context map[string]interface{} `json:"context,omitempty"`
	// Error 错误信息
	Error string `json:"error,omitempty"`
}

// ============ Tree-of-Thought 相关类型 ============

// TreeOfThoughtRequest 思维树请求
type TreeOfThoughtRequest struct {
	// Query 用户查询
	Query string `json:"query"`
	// Context 上下文信息
	Context string `json:"context,omitempty"`
	// UserID 用户ID
	UserID string `json:"user_id"`
	// MaxDepth 最大搜索深度
	MaxDepth int `json:"max_depth,omitempty"`
	// MaxBranches 每个节点的最大分支数
	MaxBranches int `json:"max_branches,omitempty"`
	// SearchStrategy 搜索策略
	SearchStrategy string `json:"search_strategy,omitempty"` // bfs, dfs, best_first, monte_carlo
	// EvaluationMethod 评估方法
	EvaluationMethod string `json:"evaluation_method,omitempty"` // heuristic, value_based, vote
	// Model 模型名称
	Model string `json:"model,omitempty"`
}

// TreeOfThoughtResponse 思维树响应
type TreeOfThoughtResponse struct {
	// ThoughtID 思考ID
	ThoughtID string `json:"thought_id"`
	// Tree 思维树结构
	Tree *ThoughtNode `json:"tree"`
	// BestPath 最佳路径
	BestPath []*ThoughtNode `json:"best_path"`
	// FinalAnswer 最终答案
	FinalAnswer string `json:"final_answer"`
	// ExplorationSummary 探索摘要
	ExplorationSummary *ExplorationSummary `json:"exploration_summary"`
	// Status 状态
	Status string `json:"status"`
	// Error 错误信息
	Error string `json:"error,omitempty"`
}

// ThoughtNode 思维节点
type ThoughtNode struct {
	// NodeID 节点ID
	NodeID string `json:"node_id"`
	// ParentID 父节点ID
	ParentID string `json:"parent_id,omitempty"`
	// Depth 深度
	Depth int `json:"depth"`
	// Content 内容
	Content string `json:"content"`
	// NodeType 节点类型
	NodeType string `json:"node_type"` // root, thought, evaluation, solution
	// Score 评分
	Score float64 `json:"score"`
	// VisitCount 访问次数（用于蒙特卡洛树搜索）
	VisitCount int `json:"visit_count"`
	// Children 子节点
	Children []*ThoughtNode `json:"children,omitempty"`
	// Status 状态
	Status string `json:"status"` // pending, exploring, completed, pruned
	// TokenUsage Token使用情况
	TokenUsage *llm.Usage `json:"token_usage,omitempty"`
	// CreateTime 创建时间
	CreateTime time.Time `json:"create_time"`
	// Metadata 元数据
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ExplorationSummary 探索摘要
type ExplorationSummary struct {
	// TotalNodes 总节点数
	TotalNodes int `json:"total_nodes"`
	// MaxDepthReached 达到的最大深度
	MaxDepthReached int `json:"max_depth_reached"`
	// PrunedBranches 剪枝的分支数
	PrunedBranches int `json:"pruned_branches"`
	// EvaluationCount 评估次数
	EvaluationCount int `json:"evaluation_count"`
	// SearchTime 搜索耗时（毫秒）
	SearchTime int64 `json:"search_time"`
}

// VisualizationData 可视化数据
type VisualizationData struct {
	// VisualizationType 可视化类型
	VisualizationType string `json:"visualization_type"` // tree, graph, timeline, flowchart
	// Nodes 节点数据
	Nodes []VisualizationNode `json:"nodes"`
	// Edges 边数据
	Edges []VisualizationEdge `json:"edges"`
	// Metadata 元数据
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// VisualizationNode 可视化节点
type VisualizationNode struct {
	// ID 节点ID
	ID string `json:"id"`
	// Label 标签
	Label string `json:"label"`
	// Type 类型
	Type string `json:"type"`
	// Position 位置
	Position *Position `json:"position,omitempty"`
	// Style 样式
	Style *NodeStyle `json:"style,omitempty"`
	// Data 数据
	Data map[string]interface{} `json:"data,omitempty"`
}

// VisualizationEdge 可视化边
type VisualizationEdge struct {
	// ID 边ID
	ID string `json:"id"`
	// Source 源节点ID
	Source string `json:"source"`
	// Target 目标节点ID
	Target string `json:"target"`
	// Label 标签
	Label string `json:"label,omitempty"`
	// Style 样式
	Style *EdgeStyle `json:"style,omitempty"`
	// Data 数据
	Data map[string]interface{} `json:"data,omitempty"`
}

// Position 位置
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// NodeStyle 节点样式
type NodeStyle struct {
	// Color 颜色
	Color string `json:"color,omitempty"`
	// Size 大小
	Size int `json:"size,omitempty"`
	// Shape 形状
	Shape string `json:"shape,omitempty"`
	// BorderColor 边框颜色
	BorderColor string `json:"border_color,omitempty"`
	// BorderWidth 边框宽度
	BorderWidth int `json:"border_width,omitempty"`
	// BackgroundColor 背景颜色
	BackgroundColor string `json:"background_color,omitempty"`
	// TextColor 文本颜色
	TextColor string `json:"text_color,omitempty"`
}

// EdgeStyle 边样式
type EdgeStyle struct {
	// Color 颜色
	Color string `json:"color,omitempty"`
	// Width 宽度
	Width int `json:"width,omitempty"`
	// Type 类型
	Type string `json:"type,omitempty"` // solid, dashed, dotted
	// Arrow 箭头
	Arrow bool `json:"arrow,omitempty"`
}

// ChainOfThought 实现Chain-of-Thought推理
func (s *ThinkingService) ChainOfThought(ctx context.Context, req *ChainOfThoughtRequest) (*ChainOfThoughtResponse, error) {
	// 创建思考记录
	thought := &models.Thought{
		ID:     uuid.New().String(),
		Type:   req.ThoughtType,
		UserID: req.UserID,
		Status: "draft",
	}
	if thought.Type == "" {
		thought.Type = "problem-solving"
	}

	// 初始化响应
	resp := &ChainOfThoughtResponse{
		ThoughtID:      thought.ID,
		ReasoningSteps: make([]ReasoningStep, 0),
		Status:         "in_progress",
	}

	// 保存初始思考记录
	if err := s.CreateThought(thought); err != nil {
		resp.Status = "failed"
		resp.Error = fmt.Sprintf("创建思考记录失败: %v", err)
		return resp, err
	}

	// 设置默认最大步数
	maxSteps := req.MaxSteps
	if maxSteps == 0 {
		maxSteps = 5
	}

	// 执行思维链推理
	var reasoningChain []string
	var totalTokens int

	// 步骤1: 问题分析
	analysisStep, err := s.analyzeProblem(ctx, req.Query, req.Context, req.Model)
	if err != nil {
		resp.Status = "failed"
		resp.Error = fmt.Sprintf("问题分析失败: %v", err)
		return resp, err
	}
	resp.ReasoningSteps = append(resp.ReasoningSteps, *analysisStep)
	reasoningChain = append(reasoningChain, fmt.Sprintf("分析: %s", analysisStep.Content))
	if analysisStep.TokenUsage != nil {
		totalTokens += analysisStep.TokenUsage.TotalTokens
	}

	// 步骤2: 问题分解
	decomposeStep, err := s.decomposeProblem(ctx, req.Query, analysisStep.Content, req.Model)
	if err != nil {
		resp.Status = "failed"
		resp.Error = fmt.Sprintf("问题分解失败: %v", err)
		return resp, err
	}
	resp.ReasoningSteps = append(resp.ReasoningSteps, *decomposeStep)
	reasoningChain = append(reasoningChain, fmt.Sprintf("分解: %s", decomposeStep.Content))
	if decomposeStep.TokenUsage != nil {
		totalTokens += decomposeStep.TokenUsage.TotalTokens
	}

	// 步骤3-N: 逐步推理
	for i := 3; i <= maxSteps; i++ {
		// 构建推理上下文
		context := strings.Join(reasoningChain, "\n")

		// 执行推理步骤
		reasoningStep, shouldContinue, err := s.performReasoningStep(ctx, req.Query, context, i, req.Model)
		if err != nil {
			resp.Status = "failed"
			resp.Error = fmt.Sprintf("推理步骤%d失败: %v", i, err)
			return resp, err
		}

		resp.ReasoningSteps = append(resp.ReasoningSteps, *reasoningStep)
		reasoningChain = append(reasoningChain, fmt.Sprintf("推理步骤%d: %s", i, reasoningStep.Content))

		if reasoningStep.TokenUsage != nil {
			totalTokens += reasoningStep.TokenUsage.TotalTokens
		}

		// 检查是否应该继续
		if !shouldContinue {
			break
		}
	}

	// 最后步骤: 总结和得出结论
	concludeStep, finalAnswer, err := s.concludeReasoning(ctx, req.Query, reasoningChain, req.Model)
	if err != nil {
		resp.Status = "failed"
		resp.Error = fmt.Sprintf("结论生成失败: %v", err)
		return resp, err
	}
	resp.ReasoningSteps = append(resp.ReasoningSteps, *concludeStep)
	resp.FinalAnswer = finalAnswer
	if concludeStep.TokenUsage != nil {
		totalTokens += concludeStep.TokenUsage.TotalTokens
	}

	// 构建完整思考内容
	thoughtContent := s.buildThoughtContent(resp)

	// 更新思考记录
	thought.Content = thoughtContent
	thought.Status = "published"
	if err := s.UpdateThought(thought); err != nil {
		return resp, err
	}

	// 计算置信度
	resp.Confidence = s.calculateConfidence(resp.ReasoningSteps)
	resp.Status = "completed"

	return resp, nil
}

// analyzeProblem 分析问题
func (s *ThinkingService) analyzeProblem(ctx context.Context, query, contextStr, model string) (*ReasoningStep, error) {
	step := &ReasoningStep{
		StepNumber: 1,
		StepType:   "analyze",
		Status:     "in_progress",
		StartTime:  time.Now(),
	}

	prompt := fmt.Sprintf(`请仔细分析以下问题，识别关键信息、约束条件和目标。

问题: %s

%s

请以结构化的方式分析：
1. 问题的核心是什么？
2. 有哪些关键信息？
3. 有哪些约束条件？
4. 期望的解决方案应该满足什么要求？`,
		query,
		s.buildContextSection(contextStr))

	response, err := s.callLLM(ctx, prompt, model)
	if err != nil {
		step.Status = "failed"
		step.EndTime = time.Now()
		return step, err
	}

	step.Content = response
	step.Status = "completed"
	step.EndTime = time.Now()

	return step, nil
}

// decomposeProblem 分解问题
func (s *ThinkingService) decomposeProblem(ctx context.Context, query, analysis, model string) (*ReasoningStep, error) {
	step := &ReasoningStep{
		StepNumber: 2,
		StepType:   "decompose",
		Status:     "in_progress",
		StartTime:  time.Now(),
	}

	prompt := fmt.Sprintf(`基于以下问题分析，将问题分解为更小的、可解决的子问题。

问题: %s

分析结果:
%s

请提供：
1. 需要解决的关键子问题列表
2. 每个子问题的优先级
3. 子问题之间的依赖关系`,
		query, analysis)

	response, err := s.callLLM(ctx, prompt, model)
	if err != nil {
		step.Status = "failed"
		step.EndTime = time.Now()
		return step, err
	}

	step.Content = response
	step.Status = "completed"
	step.EndTime = time.Now()

	return step, nil
}

// performReasoningStep 执行推理步骤
func (s *ThinkingService) performReasoningStep(ctx context.Context, query, context string, stepNum int, model string) (*ReasoningStep, bool, error) {
	step := &ReasoningStep{
		StepNumber: stepNum,
		StepType:   "reasoning",
		Status:     "in_progress",
		StartTime:  time.Now(),
	}

	prompt := fmt.Sprintf(`基于之前的分析和推理，继续进行深入思考。

原始问题: %s

之前的推理过程:
%s

请继续推理：
1. 基于已有信息，下一步应该考虑什么？
2. 这一推理步骤如何帮助我们接近最终答案？
3. 是否有任何假设需要验证？

如果认为已经有足够的信息得出结论，请在回复开头明确标注"CONCLUSION:"`,
		query, context)

	response, err := s.callLLM(ctx, prompt, model)
	if err != nil {
		step.Status = "failed"
		step.EndTime = time.Now()
		return step, false, err
	}

	// 检查是否应该继续
	shouldContinue := !strings.HasPrefix(response, "CONCLUSION:")

	// 移除结论标记
	if strings.HasPrefix(response, "CONCLUSION:") {
		response = strings.TrimPrefix(response, "CONCLUSION:")
		response = strings.TrimSpace(response)
	}

	step.Content = response
	step.Status = "completed"
	step.EndTime = time.Now()

	return step, shouldContinue, nil
}

// concludeReasoning 得出结论
func (s *ThinkingService) concludeReasoning(ctx context.Context, query string, reasoningChain []string, model string) (*ReasoningStep, string, error) {
	step := &ReasoningStep{
		StepNumber: len(reasoningChain) + 1,
		StepType:   "conclude",
		Status:     "in_progress",
		StartTime:  time.Now(),
	}

	prompt := fmt.Sprintf(`基于完整的推理过程，给出最终的答案和结论。

原始问题: %s

完整推理过程:
%s

请提供：
1. 直接回答原始问题
2. 总结关键推理步骤
3. 说明答案的置信度以及任何不确定性`,
		query, strings.Join(reasoningChain, "\n\n"))

	response, err := s.callLLM(ctx, prompt, model)
	if err != nil {
		step.Status = "failed"
		step.EndTime = time.Now()
		return step, "", err
	}

	step.Content = response
	step.Status = "completed"
	step.EndTime = time.Now()

	return step, response, nil
}

// MultiStepReasoning 多步推理流程编排
func (s *ThinkingService) MultiStepReasoning(ctx context.Context, req *MultiStepReasoningRequest) (*MultiStepReasoningResponse, error) {
	// 如果没有提供推理计划，自动生成一个
	if req.ReasoningPlan == nil {
		plan, err := s.generateReasoningPlan(ctx, req.Query, req.Context, req.Model)
		if err != nil {
			return nil, fmt.Errorf("生成推理计划失败: %w", err)
		}
		req.ReasoningPlan = plan
	}

	// 创建思考记录
	thought := &models.Thought{
		ID:     uuid.New().String(),
		Type:   "multi-step-reasoning",
		UserID: req.UserID,
		Status: "draft",
	}

	// 初始化响应
	resp := &MultiStepReasoningResponse{
		ThoughtID:        thought.ID,
		ReasoningPlan:    req.ReasoningPlan,
		ExecutionResults: make([]StepExecutionResult, 0),
		Status:           "in_progress",
		Progress:         0,
	}

	// 保存初始思考记录
	if err := s.CreateThought(thought); err != nil {
		resp.Status = "failed"
		resp.Error = fmt.Sprintf("创建思考记录失败: %v", err)
		return resp, err
	}

	// 根据策略执行推理
	var err error
	switch req.ReasoningPlan.Strategy {
	case "parallel":
		err = s.executeParallelStrategy(ctx, req, resp)
	case "recursive":
		err = s.executeRecursiveStrategy(ctx, req, resp)
	case "tree_search":
		err = s.executeTreeSearchStrategy(ctx, req, resp)
	default:
		err = s.executeLinearStrategy(ctx, req, resp)
	}

	if err != nil {
		resp.Status = "failed"
		resp.Error = err.Error()
		return resp, err
	}

	// 生成最终答案
	finalAnswer, err := s.generateFinalAnswer(ctx, req.Query, resp.ExecutionResults, req.Model)
	if err != nil {
		resp.Status = "failed"
		resp.Error = fmt.Sprintf("生成最终答案失败: %v", err)
		return resp, err
	}

	resp.FinalAnswer = finalAnswer
	resp.Status = "completed"
	resp.Progress = 100

	// 构建完整思考内容
	thoughtContent := s.buildMultiStepThoughtContent(resp)
	thought.Content = thoughtContent
	thought.Status = "published"
	if err := s.UpdateThought(thought); err != nil {
		return resp, err
	}

	return resp, nil
}

// generateReasoningPlan 生成推理计划
func (s *ThinkingService) generateReasoningPlan(ctx context.Context, query, contextStr, model string) (*ReasoningPlan, error) {
	prompt := fmt.Sprintf(`请为以下问题生成一个推理计划。

问题: %s

%s

请提供：
1. 需要执行的推理步骤列表
2. 每个步骤的类型（分析、分解、推理、验证、结论等）
3. 步骤之间的依赖关系
4. 推荐的策略（linear, parallel, recursive, tree_search）

以JSON格式返回：
{
  "strategy": "推荐的策略",
  "steps": [
    {
      "step_number": 1,
      "description": "步骤描述",
      "step_type": "步骤类型",
      "prompt_template": "可选的提示词模板"
    }
  ]
}`,
		query,
		s.buildContextSection(contextStr))

	response, err := s.callLLM(ctx, prompt, model)
	if err != nil {
		return nil, err
	}

	// 尝试解析JSON响应
	var plan ReasoningPlan
	if err := json.Unmarshal([]byte(response), &plan); err != nil {
		// 如果JSON解析失败，使用默认计划
		plan = ReasoningPlan{
			Strategy: "linear",
			Steps: []PlanStep{
				{StepNumber: 1, Description: "问题分析", StepType: "analyze"},
				{StepNumber: 2, Description: "信息收集", StepType: "gather"},
				{StepNumber: 3, Description: "逻辑推理", StepType: "reasoning"},
				{StepNumber: 4, Description: "结论验证", StepType: "verify"},
				{StepNumber: 5, Description: "总结结论", StepType: "conclude"},
			},
		}
	}

	return &plan, nil
}

// executeLinearStrategy 执行线性策略
func (s *ThinkingService) executeLinearStrategy(ctx context.Context, req *MultiStepReasoningRequest, resp *MultiStepReasoningResponse) error {
	// 存储步骤执行结果用于后续步骤引用
	resultsByStep := make(map[int]string)

	for _, step := range req.ReasoningPlan.Steps {
		result, err := s.executePlanStep(ctx, req.Query, req.Context, step, resultsByStep, req.Model)
		if err != nil {
			return fmt.Errorf("步骤%d执行失败: %w", step.StepNumber, err)
		}

		resp.ExecutionResults = append(resp.ExecutionResults, *result)
		resultsByStep[step.StepNumber] = result.Content

		// 更新进度
		resp.Progress = float64(len(resp.ExecutionResults)) / float64(len(req.ReasoningPlan.Steps)) * 100
	}

	return nil
}

// executeParallelStrategy 执行并行策略
func (s *ThinkingService) executeParallelStrategy(ctx context.Context, req *MultiStepReasoningRequest, resp *MultiStepReasoningResponse) error {
	// 识别可并行执行的步骤（无依赖的步骤）
	stepGroups := s.groupStepsByDependencies(req.ReasoningPlan.Steps)

	resultsByStep := make(map[int]string)

	for _, group := range stepGroups {
		// 并行执行一组步骤
		var wg sync.WaitGroup
		resultChan := make(chan *StepExecutionResult, len(group))
		errorChan := make(chan error, len(group))

		for _, step := range group {
			wg.Add(1)
			go func(ps PlanStep) {
				defer wg.Done()

				// 检查依赖是否满足
				if !s.checkDependencies(ps.Dependencies, resultsByStep) {
					errorChan <- fmt.Errorf("步骤%d的依赖未满足", ps.StepNumber)
					return
				}

				result, err := s.executePlanStep(ctx, req.Query, req.Context, ps, resultsByStep, req.Model)
				if err != nil {
					errorChan <- err
					return
				}
				resultChan <- result
			}(step)
		}

		wg.Wait()
		close(resultChan)
		close(errorChan)

		// 收集结果
		for result := range resultChan {
			resp.ExecutionResults = append(resp.ExecutionResults, *result)
			resultsByStep[result.StepNumber] = result.Content
		}

		// 检查错误
		if err := <-errorChan; err != nil {
			return err
		}

		// 更新进度
		resp.Progress = float64(len(resp.ExecutionResults)) / float64(len(req.ReasoningPlan.Steps)) * 100
	}

	return nil
}

// executeRecursiveStrategy 执行递归策略
func (s *ThinkingService) executeRecursiveStrategy(ctx context.Context, req *MultiStepReasoningRequest, resp *MultiStepReasoningResponse) error {
	resultsByStep := make(map[int]string)
	visited := make(map[int]bool)

	var executeRecursive func(step PlanStep) error
	executeRecursive = func(step PlanStep) error {
		if visited[step.StepNumber] {
			return nil
		}

		// 检查并执行依赖
		for _, dep := range step.Dependencies {
			// 查找依赖步骤
			var depStep *PlanStep
			for _, s := range req.ReasoningPlan.Steps {
				if s.StepNumber == dep {
					depStep = &s
					break
				}
			}
			if depStep != nil {
				if err := executeRecursive(*depStep); err != nil {
					return err
				}
			}
		}

		// 执行当前步骤
		result, err := s.executePlanStep(ctx, req.Query, req.Context, step, resultsByStep, req.Model)
		if err != nil {
			return err
		}

		resp.ExecutionResults = append(resp.ExecutionResults, *result)
		resultsByStep[step.StepNumber] = result.Content
		visited[step.StepNumber] = true

		// 更新进度
		resp.Progress = float64(len(resp.ExecutionResults)) / float64(len(req.ReasoningPlan.Steps)) * 100

		return nil
	}

	// 执行所有步骤
	for _, step := range req.ReasoningPlan.Steps {
		if err := executeRecursive(step); err != nil {
			return err
		}
	}

	return nil
}

// executeTreeSearchStrategy 执行树搜索策略（简化的最佳优先搜索）
func (s *ThinkingService) executeTreeSearchStrategy(ctx context.Context, req *MultiStepReasoningRequest, resp *MultiStepReasoningResponse) error {
	// 使用优先级队列或简单的方式执行
	// 这里简化为按照依赖关系和启发式评分执行

	resultsByStep := make(map[int]string)
	remainingSteps := make([]PlanStep, len(req.ReasoningPlan.Steps))
	copy(remainingSteps, req.ReasoningPlan.Steps)

	for len(remainingSteps) > 0 {
		// 找到可以执行的最佳步骤
		bestIdx := -1
		var bestStep PlanStep

		for i, step := range remainingSteps {
			if s.checkDependencies(step.Dependencies, resultsByStep) {
				if bestIdx == -1 || s.stepPriority(step) > s.stepPriority(bestStep) {
					bestIdx = i
					bestStep = step
				}
			}
		}

		if bestIdx == -1 {
			return fmt.Errorf("无法找到可执行的步骤（可能存在循环依赖）")
		}

		// 执行选中的步骤
		result, err := s.executePlanStep(ctx, req.Query, req.Context, bestStep, resultsByStep, req.Model)
		if err != nil {
			return fmt.Errorf("步骤%d执行失败: %w", bestStep.StepNumber, err)
		}

		resp.ExecutionResults = append(resp.ExecutionResults, *result)
		resultsByStep[bestStep.StepNumber] = result.Content

		// 移除已执行的步骤
		remainingSteps = append(remainingSteps[:bestIdx], remainingSteps[bestIdx+1:]...)

		// 更新进度
		resp.Progress = float64(len(resp.ExecutionResults)) / float64(len(req.ReasoningPlan.Steps)) * 100
	}

	return nil
}

// executePlanStep 执行单个计划步骤
func (s *ThinkingService) executePlanStep(ctx context.Context, query, context string, step PlanStep, previousResults map[int]string, model string) (*StepExecutionResult, error) {
	result := &StepExecutionResult{
		StepNumber: step.StepNumber,
		Status:     "in_progress",
		StartTime:  time.Now(),
	}

	// 构建提示词
	var prompt string
	if step.PromptTemplate != "" {
		prompt = s.fillPromptTemplate(step.PromptTemplate, query, context, previousResults)
	} else {
		prompt = s.buildDefaultStepPrompt(query, context, step, previousResults)
	}

	response, err := s.callLLM(ctx, prompt, model)
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		result.EndTime = time.Now()
		return result, err
	}

	result.Content = response
	result.Status = "completed"
	result.EndTime = time.Now()

	return result, nil
}

// CreateThought 创建思考
func (s *ThinkingService) CreateThought(thought *models.Thought) error {
	if thought.ID == "" {
		thought.ID = uuid.New().String()
	}
	if thought.Status == "" {
		thought.Status = "draft"
	}
	thought.CreatedAt = time.Now()
	thought.UpdatedAt = time.Now()
	return s.db.Create(thought).Error
}

// UpdateThought 更新思考
func (s *ThinkingService) UpdateThought(thought *models.Thought) error {
	thought.UpdatedAt = time.Now()
	return s.db.Save(thought).Error
}

// DeleteThought 删除思考
func (s *ThinkingService) DeleteThought(id string) error {
	return s.db.Delete(&models.Thought{}, "id = ?", id).Error
}

// GetThought 获取思考
func (s *ThinkingService) GetThought(id string) (*models.Thought, error) {
	var thought models.Thought
	err := s.db.Where("id = ?", id).First(&thought).Error
	return &thought, err
}

// ListThoughts 列出所有思考
func (s *ThinkingService) ListThoughts() ([]models.Thought, error) {
	var thoughts []models.Thought
	err := s.db.Find(&thoughts).Error
	return thoughts, err
}

// ListThoughtsByUser 列出用户的所有思考
func (s *ThinkingService) ListThoughtsByUser(userID string) ([]models.Thought, error) {
	var thoughts []models.Thought
	err := s.db.Where("user_id = ?", userID).Find(&thoughts).Error
	return thoughts, err
}

// CreateCorrection 创建修正
func (s *ThinkingService) CreateCorrection(correction *models.Correction) error {
	if correction.ID == "" {
		correction.ID = uuid.New().String()
	}
	if correction.Status == "" {
		correction.Status = "pending"
	}
	correction.CreatedAt = time.Now()
	correction.UpdatedAt = time.Now()
	return s.db.Create(correction).Error
}

// UpdateCorrection 更新修正
func (s *ThinkingService) UpdateCorrection(correction *models.Correction) error {
	correction.UpdatedAt = time.Now()
	return s.db.Save(correction).Error
}

// DeleteCorrection 删除修正
func (s *ThinkingService) DeleteCorrection(id string) error {
	return s.db.Delete(&models.Correction{}, "id = ?", id).Error
}

// GetCorrection 获取修正
func (s *ThinkingService) GetCorrection(id string) (*models.Correction, error) {
	var correction models.Correction
	err := s.db.Where("id = ?", id).First(&correction).Error
	return &correction, err
}

// ListCorrections 列出思考的所有修正
func (s *ThinkingService) ListCorrections(thoughtID string) ([]models.Correction, error) {
	var corrections []models.Correction
	err := s.db.Where("thought_id = ?", thoughtID).Find(&corrections).Error
	return corrections, err
}

// ResolveCorrection 解决修正
func (s *ThinkingService) ResolveCorrection(id string) (*models.Correction, error) {
	correction, err := s.GetCorrection(id)
	if err != nil {
		return nil, err
	}

	correction.Status = "resolved"
	correction.UpdatedAt = time.Now()
	s.db.Save(correction)

	return correction, nil
}

// ============ 辅助方法 ============

// callLLM 调用LLM
func (s *ThinkingService) callLLM(ctx context.Context, prompt, model string) (string, error) {
	if s.llmClient == nil {
		return "", fmt.Errorf("LLM客户端未初始化")
	}

	request := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个专业的分析助手，擅长逻辑推理和问题解决。"},
			{Role: "user", Content: prompt},
		},
		Model:       model,
		Temperature: 0.7,
		MaxTokens:   2000,
	}

	response, err := s.llmClient.Chat(ctx, request)
	if err != nil {
		return "", err
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("未收到响应")
	}

	return response.Choices[0].Message.Content, nil
}

// buildContextSection 构建上下文部分
func (s *ThinkingService) buildContextSection(context string) string {
	if context == "" {
		return ""
	}
	return fmt.Sprintf("上下文信息:\n%s", context)
}

// buildThoughtContent 构建思考内容
func (s *ThinkingService) buildThoughtContent(resp *ChainOfThoughtResponse) string {
	var builder strings.Builder

	builder.WriteString("# 思维链推理\n\n")
	builder.WriteString(fmt.Sprintf("思考ID: %s\n", resp.ThoughtID))
	builder.WriteString(fmt.Sprintf("状态: %s\n", resp.Status))
	builder.WriteString(fmt.Sprintf("置信度: %.2f\n\n", resp.Confidence))

	builder.WriteString("## 推理步骤\n\n")
	for _, step := range resp.ReasoningSteps {
		builder.WriteString(fmt.Sprintf("### 步骤 %d: %s\n", step.StepNumber, step.StepType))
		builder.WriteString(fmt.Sprintf("%s\n\n", step.Content))
	}

	builder.WriteString("## 最终答案\n\n")
	builder.WriteString(resp.FinalAnswer)

	return builder.String()
}

// buildMultiStepThoughtContent 构建多步推理思考内容
func (s *ThinkingService) buildMultiStepThoughtContent(resp *MultiStepReasoningResponse) string {
	var builder strings.Builder

	builder.WriteString("# 多步推理\n\n")
	builder.WriteString(fmt.Sprintf("思考ID: %s\n", resp.ThoughtID))
	builder.WriteString(fmt.Sprintf("策略: %s\n", resp.ReasoningPlan.Strategy))
	builder.WriteString(fmt.Sprintf("状态: %s\n\n", resp.Status))

	builder.WriteString("## 执行结果\n\n")
	for _, result := range resp.ExecutionResults {
		builder.WriteString(fmt.Sprintf("### 步骤 %d\n", result.StepNumber))
		builder.WriteString(fmt.Sprintf("%s\n\n", result.Content))
	}

	builder.WriteString("## 最终答案\n\n")
	builder.WriteString(resp.FinalAnswer)

	return builder.String()
}

// calculateConfidence 计算置信度
func (s *ThinkingService) calculateConfidence(steps []ReasoningStep) float64 {
	if len(steps) == 0 {
		return 0
	}

	// 基于完成的步骤数计算基础置信度
	baseConfidence := float64(len(steps)) / 5.0
	if baseConfidence > 1.0 {
		baseConfidence = 1.0
	}

	// 检查是否有失败的步骤
	for _, step := range steps {
		if step.Status == "failed" {
			baseConfidence *= 0.5
		}
	}

	return baseConfidence
}

// groupStepsByDependencies 按依赖关系分组步骤
func (s *ThinkingService) groupStepsByDependencies(steps []PlanStep) [][]PlanStep {
	// 简单的拓扑排序分组
	remaining := make([]PlanStep, len(steps))
	copy(remaining, steps)

	var groups [][]PlanStep
	completed := make(map[int]bool)

	for len(remaining) > 0 {
		var currentGroup []PlanStep
		var nextRemaining []PlanStep

		for _, step := range remaining {
			if s.checkDependencies(step.Dependencies, completedToResults(completed, steps)) {
				currentGroup = append(currentGroup, step)
			} else {
				nextRemaining = append(nextRemaining, step)
			}
		}

		if len(currentGroup) == 0 {
			// 无法继续，可能存在循环依赖
			break
		}

		groups = append(groups, currentGroup)

		for _, step := range currentGroup {
			completed[step.StepNumber] = true
		}

		remaining = nextRemaining
	}

	return groups
}

// checkDependencies 检查依赖是否满足
func (s *ThinkingService) checkDependencies(dependencies []int, results map[int]string) bool {
	for _, dep := range dependencies {
		if _, exists := results[dep]; !exists {
			return false
		}
	}
	return true
}

// completedToResults 转换完成标记为结果映射
func completedToResults(completed map[int]bool, steps []PlanStep) map[int]string {
	results := make(map[int]string)
	for _, step := range steps {
		if completed[step.StepNumber] {
			results[step.StepNumber] = "completed"
		}
	}
	return results
}

// stepPriority 计算步骤优先级
func (s *ThinkingService) stepPriority(step PlanStep) float64 {
	// 简单的优先级计算
	// 分析类步骤优先级较高
	switch step.StepType {
	case "analyze":
		return 10
	case "decompose":
		return 9
	case "gather":
		return 8
	case "reasoning":
		return 7
	case "verify":
		return 6
	case "conclude":
		return 5
	default:
		return 1
	}
}

// fillPromptTemplate 填充提示词模板
func (s *ThinkingService) fillPromptTemplate(template, query, context string, previousResults map[int]string) string {
	result := template
	result = strings.ReplaceAll(result, "{{query}}", query)
	result = strings.ReplaceAll(result, "{{context}}", context)

	// 替换步骤结果
	for stepNum, content := range previousResults {
		placeholder := fmt.Sprintf("{{step_%d}}", stepNum)
		result = strings.ReplaceAll(result, placeholder, content)
	}

	return result
}

// buildDefaultStepPrompt 构建默认步骤提示词
func (s *ThinkingService) buildDefaultStepPrompt(query, context string, step PlanStep, previousResults map[int]string) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("执行步骤%d: %s\n\n", step.StepNumber, step.Description))
	builder.WriteString(fmt.Sprintf("原始问题: %s\n\n", query))

	if context != "" {
		builder.WriteString(fmt.Sprintf("上下文: %s\n\n", context))
	}

	if len(previousResults) > 0 {
		builder.WriteString("之前步骤的结果:\n")
		for num, content := range previousResults {
			builder.WriteString(fmt.Sprintf("步骤%d: %s\n", num, content))
		}
		builder.WriteString("\n")
	}

	builder.WriteString(fmt.Sprintf("请执行%q类型的步骤。", step.StepType))

	return builder.String()
}

// generateFinalAnswer 生成最终答案
func (s *ThinkingService) generateFinalAnswer(ctx context.Context, query string, results []StepExecutionResult, model string) (string, error) {
	var builder strings.Builder

	builder.WriteString("基于以下步骤执行结果，生成最终答案：\n\n")
	builder.WriteString(fmt.Sprintf("原始问题: %s\n\n", query))
	builder.WriteString("执行结果:\n")

	for _, result := range results {
		builder.WriteString(fmt.Sprintf("步骤%d: %s\n", result.StepNumber, result.Content))
	}

	builder.WriteString("\n请综合以上所有步骤的结果，给出清晰、准确的最终答案。")

	return s.callLLM(ctx, builder.String(), model)
}

// ============ Tree-of-Thought 实现 ============

// TreeOfThought 实现Tree-of-Thought推理
func (s *ThinkingService) TreeOfThought(ctx context.Context, req *TreeOfThoughtRequest) (*TreeOfThoughtResponse, error) {
	startTime := time.Now()

	// 创建思考记录
	thought := &models.Thought{
		ID:     uuid.New().String(),
		Type:   "tree-of-thought",
		UserID: req.UserID,
		Status: "draft",
	}

	// 设置默认参数
	maxDepth := req.MaxDepth
	if maxDepth == 0 {
		maxDepth = 3
	}
	maxBranches := req.MaxBranches
	if maxBranches == 0 {
		maxBranches = 3
	}
	searchStrategy := req.SearchStrategy
	if searchStrategy == "" {
		searchStrategy = "best_first"
	}
	evaluationMethod := req.EvaluationMethod
	if evaluationMethod == "" {
		evaluationMethod = "heuristic"
	}

	// 初始化响应
	resp := &TreeOfThoughtResponse{
		ThoughtID: thought.ID,
		Tree: &ThoughtNode{
			NodeID:  uuid.New().String(),
			Depth:   0,
			Content: req.Query,
			NodeType: "root",
			Status:  "completed",
			CreateTime: time.Now(),
		},
		ExplorationSummary: &ExplorationSummary{},
		Status: "in_progress",
	}

	// 保存初始思考记录
	if err := s.CreateThought(thought); err != nil {
		resp.Status = "failed"
		resp.Error = fmt.Sprintf("创建思考记录失败: %v", err)
		return resp, err
	}

	// 根据搜索策略执行思维树推理
	var err error
	switch searchStrategy {
	case "bfs":
		err = s.totBFS(ctx, req, resp, maxDepth, maxBranches, evaluationMethod)
	case "dfs":
		err = s.totDFS(ctx, req, resp, maxDepth, maxBranches, evaluationMethod)
	case "monte_carlo":
		err = s.totMonteCarlo(ctx, req, resp, maxDepth, maxBranches, evaluationMethod)
	default:
		err = s.totBestFirst(ctx, req, resp, maxDepth, maxBranches, evaluationMethod)
	}

	if err != nil {
		resp.Status = "failed"
		resp.Error = err.Error()
		return resp, err
	}

	// 找到最佳路径
	resp.BestPath = s.findBestPath(resp.Tree)

	// 生成最终答案
	finalAnswer, err := s.generateTOTFinalAnswer(ctx, req.Query, resp.BestPath, req.Model)
	if err != nil {
		resp.Status = "failed"
		resp.Error = fmt.Sprintf("生成最终答案失败: %v", err)
		return resp, err
	}
	resp.FinalAnswer = finalAnswer

	// 更新探索摘要
	resp.ExplorationSummary.SearchTime = time.Since(startTime).Milliseconds()
	resp.ExplorationSummary.TotalNodes = s.countNodes(resp.Tree)
	resp.ExplorationSummary.MaxDepthReached = s.findMaxDepth(resp.Tree)

	// 保存思考结果
	thoughtContent := s.buildTOTThoughtContent(resp)
	thought.Content = thoughtContent
	thought.Status = "published"
	if err := s.UpdateThought(thought); err != nil {
		return resp, err
	}

	resp.Status = "completed"
	return resp, nil
}

// totBFS 广度优先搜索
func (s *ThinkingService) totBFS(ctx context.Context, req *TreeOfThoughtRequest, resp *TreeOfThoughtResponse, maxDepth, maxBranches int, evaluationMethod string) error {
	queue := []*ThoughtNode{resp.Tree}

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		if node.Depth >= maxDepth {
			continue
		}

		// 生成子节点（多个思维分支）
		children, err := s.generateThoughtBranches(ctx, req.Query, node, maxBranches, req.Model)
		if err != nil {
			return err
		}

		// 评估每个子节点
		for _, child := range children {
			score, err := s.evaluateThoughtNode(ctx, req.Query, child, evaluationMethod, req.Model)
			if err != nil {
				return err
			}
			child.Score = score
			child.Status = "completed"

			node.Children = append(node.Children, child)
			resp.ExplorationSummary.EvaluationCount++

			// 将子节点加入队列
			queue = append(queue, child)
		}
	}

	return nil
}

// totDFS 深度优先搜索
func (s *ThinkingService) totDFS(ctx context.Context, req *TreeOfThoughtRequest, resp *TreeOfThoughtResponse, maxDepth, maxBranches int, evaluationMethod string) error {
	return s.totDFSRecursive(ctx, req, resp.Tree, maxDepth, maxBranches, evaluationMethod, req.Model)
}

// totDFSRecursive DFS递归实现
func (s *ThinkingService) totDFSRecursive(ctx context.Context, req *TreeOfThoughtRequest, node *ThoughtNode, maxDepth, maxBranches int, evaluationMethod, model string) error {
	if node.Depth >= maxDepth {
		return nil
	}

	// 生成子节点
	children, err := s.generateThoughtBranches(ctx, req.Query, node, maxBranches, model)
	if err != nil {
		return err
	}

	for _, child := range children {
		// 评估子节点
		score, err := s.evaluateThoughtNode(ctx, req.Query, child, evaluationMethod, model)
		if err != nil {
			return err
		}
		child.Score = score
		child.Status = "completed"

		node.Children = append(node.Children, child)

		// 递归探索
		if err := s.totDFSRecursive(ctx, req, child, maxDepth, maxBranches, evaluationMethod, model); err != nil {
			return err
		}
	}

	return nil
}

// totBestFirst 最佳优先搜索
func (s *ThinkingService) totBestFirst(ctx context.Context, req *TreeOfThoughtRequest, resp *TreeOfThoughtResponse, maxDepth, maxBranches int, evaluationMethod string) error {
	// 使用优先级队列（这里用slice简化实现）
	priorityQueue := &PriorityQueue{
		nodes: make([]*PriorityNode, 0),
	}
	priorityQueue.Push(resp.Tree, 0)

	for priorityQueue.Len() > 0 {
		// 取出评分最高的节点
		pNode := priorityQueue.Pop()
		node := pNode.node

		if node.Depth >= maxDepth {
			continue
		}

		// 生成子节点
		children, err := s.generateThoughtBranches(ctx, req.Query, node, maxBranches, req.Model)
		if err != nil {
			return err
		}

		for _, child := range children {
			// 评估子节点
			score, err := s.evaluateThoughtNode(ctx, req.Query, child, evaluationMethod, req.Model)
			if err != nil {
				return err
			}
			child.Score = score
			child.Status = "completed"

			node.Children = append(node.Children, child)
			resp.ExplorationSummary.EvaluationCount++

			// 加入优先级队列
			priorityQueue.Push(child, score)
		}
	}

	return nil
}

// totMonteCarlo 蒙特卡洛树搜索
func (s *ThinkingService) totMonteCarlo(ctx context.Context, req *TreeOfThoughtRequest, resp *TreeOfThoughtResponse, maxDepth, maxBranches int, evaluationMethod string) error {
	maxIterations := 100 // 最大迭代次数

	for i := 0; i < maxIterations; i++ {
		// 选择：从根节点开始，选择最佳路径
		node := s.selectBestNode(resp.Tree)

		// 扩展：如果节点未达到最大深度，生成子节点
		if node.Depth < maxDepth && len(node.Children) == 0 {
			children, err := s.generateThoughtBranches(ctx, req.Query, node, maxBranches, req.Model)
			if err != nil {
				return err
			}

			for _, child := range children {
				score, err := s.evaluateThoughtNode(ctx, req.Query, child, evaluationMethod, req.Model)
				if err != nil {
					return err
				}
				child.Score = score
				child.Status = "completed"

				node.Children = append(node.Children, child)
				resp.ExplorationSummary.EvaluationCount++
			}

			if len(children) > 0 {
				node = children[0] // 选择第一个子节点进行模拟
			}
		}

		// 模拟：从当前节点随机走到叶子节点
		simulatedScore := s.simulateFromNode(ctx, req.Query, node, maxDepth, req.Model)

		// 回溯：更新路径上所有节点的值
		s.backtrackScore(node, simulatedScore)
	}

	return nil
}

// generateThoughtBranches 生成思维分支
func (s *ThinkingService) generateThoughtBranches(ctx context.Context, query string, parent *ThoughtNode, maxBranches int, model string) ([]*ThoughtNode, error) {
	prompt := fmt.Sprintf(`基于以下问题和之前的思考，生成 %d 个不同的思维分支（perspectives）。

问题: %s

之前的思考路径: %s

请提供 %d 个不同的思考方向或观点，每个方向应该：
1. 从不同的角度分析问题
2. 提出独特的见解或假设
3. 可能导向不同的解决方案

以JSON数组格式返回，每个分支包含：
- thought: 思考内容
- rationale: 选择这个方向的理由

返回格式示例：
[
  {"thought": "思考内容1", "rationale": "理由1"},
  {"thought": "思考内容2", "rationale": "理由2"}
]`,
		maxBranches,
		query,
		s.buildParentPath(parent),
		maxBranches)

	response, err := s.callLLM(ctx, prompt, model)
	if err != nil {
		return nil, err
	}

	// 解析响应
	var branches []struct {
		Thought    string `json:"thought"`
		Rationale  string `json:"rationale"`
	}

	if err := json.Unmarshal([]byte(response), &branches); err != nil {
		// 如果解析失败，简单分割
		branches = s.parseBranchesFallback(response, maxBranches)
	}

	// 创建节点
	nodes := make([]*ThoughtNode, 0, len(branches))
	for i, branch := range branches {
		node := &ThoughtNode{
			NodeID:    uuid.New().String(),
			ParentID:  parent.NodeID,
			Depth:     parent.Depth + 1,
			Content:   branch.Thought,
			NodeType:  "thought",
			Status:    "pending",
			CreateTime: time.Now(),
			Metadata: map[string]interface{}{
				"rationale": branch.Rationale,
				"branch_index": i,
			},
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// evaluateThoughtNode 评估思维节点
func (s *ThinkingService) evaluateThoughtNode(ctx context.Context, query string, node *ThoughtNode, method, model string) (float64, error) {
	switch method {
	case "value_based":
		return s.evaluateValueBased(ctx, query, node, model)
	case "vote":
		return s.evaluateByVote(ctx, query, node, model)
	default:
		return s.evaluateHeuristic(ctx, query, node, model)
	}
}

// evaluateHeuristic 启发式评估
func (s *ThinkingService) evaluateHeuristic(ctx context.Context, query string, node *ThoughtNode, model string) (float64, error) {
	prompt := fmt.Sprintf(`评估以下思考的质量和潜力。

问题: %s

思考内容: %s

请从以下维度评估（0-10分）：
1. 相关性：与问题的相关程度
2. 创新性：观点的新颖程度
3. 可行性：转化为解决方案的可能性
4. 完整性：思考的完整程度

请只返回一个0-1之间的总分（用小数表示，如0.75）。`,
		query, node.Content)

	response, err := s.callLLM(ctx, prompt, model)
	if err != nil {
		return 0.5, nil // 出错时返回中等分数
	}

	// 尝试解析分数
	var score float64
	fmt.Sscanf(response, "%f", &score)
	if score < 0 {
		score = 0
	} else if score > 1 {
		score = 1
	}

	return score, nil
}

// evaluateValueBased 基于价值的评估
func (s *ThinkingService) evaluateValueBased(ctx context.Context, query string, node *ThoughtNode, model string) (float64, error) {
	prompt := fmt.Sprintf(`评估这个思考步骤对最终解决方案的价值。

问题: %s

思考内容: %s

这个思考步骤：
1. 是否帮助我们更好地理解问题？
2. 是否提供了有价值的洞察？
3. 是否接近找到解决方案？

请给出一个0-1之间的价值评分。`,
		query, node.Content)

	response, err := s.callLLM(ctx, prompt, model)
	if err != nil {
		return 0.5, nil
	}

	var score float64
	fmt.Sscanf(response, "%f", &score)
	if score < 0 {
		score = 0
	} else if score > 1 {
		score = 1
	}

	return score, nil
}

// evaluateByVote 投票评估
func (s *ThinkingService) evaluateByVote(ctx context.Context, query string, node *ThoughtNode, model string) (float64, error) {
	// 使用多个提示词进行"投票"，然后综合结果
	votes := make([]float64, 3)

	for i := 0; i < 3; i++ {
		prompt := fmt.Sprintf(`（投票 %d/3）评估以下思考的质量（0-10分）：

问题: %s

思考: %s

请只返回分数。`, i+1, query, node.Content)

		response, err := s.callLLM(ctx, prompt, model)
		if err != nil {
			votes[i] = 5.0
			continue
		}

		var score float64
		fmt.Sscanf(response, "%f", &score)
		votes[i] = score / 10.0 // 转换为0-1
	}

	// 计算平均分
	total := 0.0
	for _, v := range votes {
		total += v
	}
	return total / float64(len(votes)), nil
}

// selectBestNode 选择最佳节点（用于蒙特卡洛树搜索）
func (s *ThinkingService) selectBestNode(root *ThoughtNode) *ThoughtNode {
	node := root

	for len(node.Children) > 0 {
		bestChild := node.Children[0]
		bestScore := s.calculateUCB(bestChild)

		for _, child := range node.Children[1:] {
			score := s.calculateUCB(child)
			if score > bestScore {
				bestScore = score
				bestChild = child
			}
		}

		node = bestChild
	}

	return node
}

// calculateUCB 计算UCB值（Upper Confidence Bound）
func (s *ThinkingService) calculateUCB(node *ThoughtNode) float64 {
	if node.VisitCount == 0 {
		return 1.0 // 未访问的节点优先级最高
	}

	exploitation := node.Score
	exploration := 0.5 * (logOf(float64(node.VisitCount+1)) / logOf(float64(node.VisitCount)))

	return exploitation + exploration
}

// simulateFromNode 从节点模拟
func (s *ThinkingService) simulateFromNode(ctx context.Context, query string, node *ThoughtNode, maxDepth int, model string) float64 {
	// 简化实现：返回当前节点的评分
	return node.Score
}

// backtrackScore 回溯分数
func (s *ThinkingService) backtrackScore(node *ThoughtNode, score float64) {
	for node != nil {
		node.VisitCount++
		// 更新平均分数
		node.Score = (node.Score*float64(node.VisitCount-1) + score) / float64(node.VisitCount)
	}
}

// findBestPath 找到最佳路径
func (s *ThinkingService) findBestPath(root *ThoughtNode) []*ThoughtNode {
	path := []*ThoughtNode{root}

	node := root
	for len(node.Children) > 0 {
		bestChild := node.Children[0]
		bestScore := bestChild.Score

		for _, child := range node.Children[1:] {
			if child.Score > bestScore {
				bestScore = child.Score
				bestChild = child
			}
		}

		path = append(path, bestChild)
		node = bestChild
	}

	return path
}

// generateTOTFinalAnswer 生成思维树最终答案
func (s *ThinkingService) generateTOTFinalAnswer(ctx context.Context, query string, bestPath []*ThoughtNode, model string) (string, error) {
	var builder strings.Builder

	builder.WriteString("基于以下最佳思维路径，给出最终答案：\n\n")
	builder.WriteString(fmt.Sprintf("问题: %s\n\n", query))
	builder.WriteString("最佳思维路径:\n")

	for i, node := range bestPath {
		builder.WriteString(fmt.Sprintf("步骤%d: %s (评分: %.2f)\n", i, node.Content, node.Score))
	}

	builder.WriteString("\n请基于这条思维路径，给出清晰、准确的最终答案。")

	return s.callLLM(ctx, builder.String(), model)
}

// buildTOTThoughtContent 构建思维树思考内容
func (s *ThinkingService) buildTOTThoughtContent(resp *TreeOfThoughtResponse) string {
	var builder strings.Builder

	builder.WriteString("# 思维树推理\n\n")
	builder.WriteString(fmt.Sprintf("思考ID: %s\n", resp.ThoughtID))
	builder.WriteString(fmt.Sprintf("状态: %s\n\n", resp.Status))

	builder.WriteString("## 探索摘要\n\n")
	builder.WriteString(fmt.Sprintf("- 总节点数: %d\n", resp.ExplorationSummary.TotalNodes))
	builder.WriteString(fmt.Sprintf("- 最大深度: %d\n", resp.ExplorationSummary.MaxDepthReached))
	builder.WriteString(fmt.Sprintf("- 评估次数: %d\n", resp.ExplorationSummary.EvaluationCount))
	builder.WriteString(fmt.Sprintf("- 搜索耗时: %dms\n\n", resp.ExplorationSummary.SearchTime))

	builder.WriteString("## 最佳路径\n\n")
	for i, node := range resp.BestPath {
		builder.WriteString(fmt.Sprintf("%d. %s (评分: %.2f)\n", i, node.Content, node.Score))
	}

	builder.WriteString("\n## 最终答案\n\n")
	builder.WriteString(resp.FinalAnswer)

	return builder.String()
}

// countNodes 统计节点数
func (s *ThinkingService) countNodes(root *ThoughtNode) int {
	if root == nil {
		return 0
	}
	count := 1
	for _, child := range root.Children {
		count += s.countNodes(child)
	}
	return count
}

// findMaxDepth 找到最大深度
func (s *ThinkingService) findMaxDepth(root *ThoughtNode) int {
	if root == nil {
		return 0
	}
	maxDepth := root.Depth
	for _, child := range root.Children {
		childDepth := s.findMaxDepth(child)
		if childDepth > maxDepth {
			maxDepth = childDepth
		}
	}
	return maxDepth
}

// buildParentPath 构建父节点路径
func (s *ThinkingService) buildParentPath(node *ThoughtNode) string {
	// 简化：只返回当前节点内容
	// 实际实现可能需要追溯父节点
	return node.Content
}

// parseBranchesFallback 分支解析回退方案
func (s *ThinkingService) parseBranchesFallback(response string, maxBranches int) []struct {
	Thought   string `json:"thought"`
	Rationale string `json:"rationale"`
} {
	// 简单分割响应
	lines := strings.Split(response, "\n")
	branches := make([]struct {
		Thought   string `json:"thought"`
		Rationale string `json:"rationale"`
	}, 0, maxBranches)

	for i, line := range lines {
		if i >= maxBranches {
			break
		}
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, struct {
				Thought   string `json:"thought"`
				Rationale string `json:"rationale"`
			}{
				Thought:   line,
				Rationale: "Generated from fallback parsing",
			})
		}
	}

	return branches
}

// ============ 可视化支持 ============

// GenerateVisualization 生成思考过程的可视化数据
func (s *ThinkingService) GenerateVisualization(thoughtID string, visualizationType string) (*VisualizationData, error) {
	// 获取思考记录
	thought, err := s.GetThought(thoughtID)
	if err != nil {
		return nil, err
	}

	// 根据思考类型生成不同的可视化
	switch thought.Type {
	case "tree-of-thought":
		return s.generateTreeVisualization(thought)
	case "problem-solving":
		return s.generateChainVisualization(thought)
	default:
		return s.generateFlowchartVisualization(thought)
	}
}

// generateTreeVisualization 生成树形可视化
func (s *ThinkingService) generateTreeVisualization(thought *models.Thought) (*VisualizationData, error) {
	viz := &VisualizationData{
		VisualizationType: "tree",
		Nodes: make([]VisualizationNode, 0),
		Edges: make([]VisualizationEdge, 0),
		Metadata: map[string]interface{}{
			"thought_id": thought.ID,
			"layout": "hierarchical",
		},
	}

	// 解析思考内容构建树结构
	// 这里简化实现，实际应该从存储的数据中构建

	// 添加根节点
	rootNode := VisualizationNode{
		ID:    "root",
		Label: "Root",
		Type:  "root",
		Position: &Position{X: 400, Y: 50},
		Style: &NodeStyle{
			Color:           "#4CAF50",
			Size:            50,
			Shape:           "circle",
			BackgroundColor: "#C8E6C9",
			BorderColor:     "#4CAF50",
			BorderWidth:     2,
		},
	}
	viz.Nodes = append(viz.Nodes, rootNode)

	return viz, nil
}

// generateChainVisualization 生成链式可视化
func (s *ThinkingService) generateChainVisualization(thought *models.Thought) (*VisualizationData, error) {
	viz := &VisualizationData{
		VisualizationType: "graph",
		Nodes: make([]VisualizationNode, 0),
		Edges: make([]VisualizationEdge, 0),
		Metadata: map[string]interface{}{
			"thought_id": thought.ID,
			"layout": "force-directed",
		},
	}

	// 简化实现
	return viz, nil
}

// generateFlowchartVisualization 生成流程图可视化
func (s *ThinkingService) generateFlowchartVisualization(thought *models.Thought) (*VisualizationData, error) {
	viz := &VisualizationData{
		VisualizationType: "flowchart",
		Nodes: make([]VisualizationNode, 0),
		Edges: make([]VisualizationEdge, 0),
		Metadata: map[string]interface{}{
			"thought_id": thought.ID,
			"layout": "top-down",
		},
	}

	// 简化实现
	return viz, nil
}

// GenerateTimelineVisualization 生成时间线可视化
func (s *ThinkingService) GenerateTimelineVisualization(thoughtID string) (*VisualizationData, error) {
	thought, err := s.GetThought(thoughtID)
	if err != nil {
		return nil, err
	}

	viz := &VisualizationData{
		VisualizationType: "timeline",
		Nodes: make([]VisualizationNode, 0),
		Edges: make([]VisualizationEdge, 0),
		Metadata: map[string]interface{}{
			"thought_id": thought.ID,
			"start_time": thought.CreatedAt,
			"end_time":   thought.UpdatedAt,
		},
	}

	// 添加时间线节点
	startNode := VisualizationNode{
		ID:    "start",
		Label: "开始",
		Type:  "start",
		Style: &NodeStyle{
			Color:           "#4CAF50",
			Shape:           "circle",
			BackgroundColor: "#C8E6C9",
		},
		Data: map[string]interface{}{
			"time": thought.CreatedAt,
		},
	}
	viz.Nodes = append(viz.Nodes, startNode)

	endNode := VisualizationNode{
		ID:    "end",
		Label: "完成",
		Type:  "end",
		Style: &NodeStyle{
			Color:           "#F44336",
			Shape:           "circle",
			BackgroundColor: "#FFCDD2",
		},
		Data: map[string]interface{}{
			"time": thought.UpdatedAt,
		},
	}
	viz.Nodes = append(viz.Nodes, endNode)

	return viz, nil
}

// ============ 辅助类型和函数 ============

// PriorityNode 优先级队列节点
type PriorityNode struct {
	node  *ThoughtNode
	score float64
}

// PriorityQueue 优先级队列
type PriorityQueue struct {
	nodes []*PriorityNode
}

// Push 添加节点
func (pq *PriorityQueue) Push(node *ThoughtNode, score float64) {
	pq.nodes = append(pq.nodes, &PriorityNode{node: node, score: score})
}

// Pop 弹出最高优先级节点
func (pq *PriorityQueue) Pop() *PriorityNode {
	if len(pq.nodes) == 0 {
		return nil
	}

	// 找到最高分数的节点
	maxIdx := 0
	for i := 1; i < len(pq.nodes); i++ {
		if pq.nodes[i].score > pq.nodes[maxIdx].score {
			maxIdx = i
		}
	}

	node := pq.nodes[maxIdx]
	pq.nodes = append(pq.nodes[:maxIdx], pq.nodes[maxIdx+1:]...)
	return node
}

// Len 返回队列长度
func (pq *PriorityQueue) Len() int {
	return len(pq.nodes)
}

// logOf 简单的对数函数
func logOf(x float64) float64 {
	if x <= 1 {
		return 0
	}
	// 简单近似
	result := 0.0
	for x > 1 {
		x /= 2
		result++
	}
	return result
}

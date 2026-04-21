package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"openaide/backend/src/services/llm"
)

// StructuredPlanner 结构化规划引擎
// 核心思想：先理解 → 再规划 → 依赖分析 → 工具分配 → 风险评估
type StructuredPlanner struct {
	llmClient     llm.LLMClient
	model         string
	memoryService *MemoryService
	skillService  *SkillService
}

// TaskUnderstanding 深度任务理解结果
type TaskUnderstanding struct {
	// UserIntent 用户真正想要什么（不是字面意思）
	UserIntent string `json:"user_intent"`

	// ImplicitNeeds 隐含需求
	ImplicitNeeds []string `json:"implicit_needs"`

	// Constraints 已知约束
	Constraints []string `json:"constraints"`

	// Ambiguities 需要澄清的模糊点
	Ambiguities []string `json:"ambiguities"`

	// Assumptions 模型做出的假设
	Assumptions []string `json:"assumptions"`

	// SuccessCriteria 什么样的结果算成功
	SuccessCriteria string `json:"success_criteria"`

	// RelatedContext 相关的上下文信息
	RelatedContext []string `json:"related_context"`

	// Confidence 理解的可信度
	Confidence float64 `json:"confidence"`
}

// StructuredPlan 结构化规划结果
type StructuredPlan struct {
	ID string `json:"id"`

	Understanding *TaskUnderstanding `json:"understanding"`

	// Phases 阶段（大任务先分阶段，每个阶段包含子任务）
	Phases []PlanPhase `json:"phases"`

	// Dependencies 依赖关系图
	Dependencies []Dependency `json:"dependencies"`

	// ToolPlan 工具/技能分配计划
	ToolPlan []ToolAssignment `json:"tool_plan"`

	// RiskAssessment 风险评估
	RiskAssessment *RiskAssessment `json:"risk_assessment"`

	// FallbackPlans 回退策略
	FallbackPlans []FallbackPlan `json:"fallback_plans"`

	// EstimatedTime 总预估时间（分钟）
	EstimatedTime int `json:"estimated_time"`

	// PlanSummary 计划摘要
	PlanSummary string `json:"plan_summary"`
}

// PlanPhase 计划阶段
type PlanPhase struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Order       int       `json:"order"`
	Subtasks    []Subtask `json:"subtasks"`
}

// Subtask 子任务
type Subtask struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Type        string   `json:"type"` // coding, research, testing, review, documentation
	Skills      []string `json:"skills"`
	Order       int      `json:"order"`
	Estimated   int      `json:"estimated"` // 分钟
	Required    bool     `json:"required"`
	CanParallel bool     `json:"can_parallel"`
}

// Dependency 依赖关系
type Dependency struct {
	From string `json:"from"` // 子任务 ID
	To   string `json:"to"`   // 依赖的子任务 ID
	Type string `json:"type"` // hard, soft
}

// ToolAssignment 工具/技能分配
type ToolAssignment struct {
	SubtaskID  string   `json:"subtask_id"`
	Skills     []string `json:"skills"`
	Tools      []string `json:"tools"`
	Priority   string   `json:"priority"` // primary, secondary
}

// RiskAssessment 风险评估
type RiskAssessment struct {
	OverallRisk    string   `json:"overall_risk"`    // low, medium, high
	TopRisks       []Risk   `json:"top_risks"`
	Mitigations    []string `json:"mitigations"`
}

// Risk 具体风险
type Risk struct {
	Description  string `json:"description"`
	Probability  string `json:"probability"` // low, medium, high
	Impact       string `json:"impact"`      // low, medium, high
	Mitigation   string `json:"mitigation"`
}

// FallbackPlan 回退策略
type FallbackPlan struct {
	Trigger     string `json:"trigger"`     // 什么情况下触发
	Description string `json:"description"` // 回退策略是什么
	Steps       []string `json:"steps"`     // 回退步骤
}

// NewStructuredPlanner 创建结构化规划引擎
func NewStructuredPlanner(llmClient llm.LLMClient, model string, memoryService *MemoryService, skillService *SkillService) *StructuredPlanner {
	if model == "" {
		model = "gpt-4"
	}
	return &StructuredPlanner{
		llmClient:     llmClient,
		model:         model,
		memoryService: memoryService,
		skillService:  skillService,
	}
}

// Plan 完整的结构化规划流程
func (sp *StructuredPlanner) Plan(ctx context.Context, userMessage, userID string, context *PlanContext) (*StructuredPlan, error) {
	log.Printf("[StructuredPlanner] Starting structured planning for user message: %s", truncatePlanStr(userMessage, 50))

	// 步骤 1: 深度理解用户意图
	understanding, err := sp.understandTask(ctx, userMessage, userID, context)
	if err != nil {
		log.Printf("[StructuredPlanner] Task understanding failed, using fallback: %v", err)
		understanding = sp.fallbackUnderstanding(userMessage)
	}
	log.Printf("[StructuredPlanner] Task understood (confidence: %.2f, intent: %s)", understanding.Confidence, truncateStr(understanding.UserIntent, 50))

	// 步骤 2: 结构化规划（分阶段、子任务）
	plan, err := sp.createPlan(ctx, userMessage, understanding, context)
	if err != nil {
		log.Printf("[StructuredPlanner] Planning failed, using fallback: %v", err)
		plan = sp.fallbackPlan(understanding, userMessage)
	}

	plan.Understanding = understanding

	log.Printf("[StructuredPlanner] Plan created: %d phases, %d total subtasks, estimated %d minutes",
		len(plan.Phases), sp.countSubtasks(plan.Phases), plan.EstimatedTime)

	return plan, nil
}

// PlanContext 规划上下文
type PlanContext struct {
	TeamConfig      *TeamConfig            `json:"team_config,omitempty"`
	PreviousTasks   []string               `json:"previous_tasks,omitempty"`
	MemoryContext   string                 `json:"memory_context,omitempty"`
	AvailableSkills []string               `json:"available_skills,omitempty"`
	Options         map[string]interface{} `json:"options,omitempty"`
}

// TeamConfig 团队配置
type TeamConfig struct {
	Members     []MemberCapability `json:"members"`
	TeamType    string             `json:"team_type"`
	MaxParallel int                `json:"max_parallel"`
}

// MemberCapability 成员能力
type MemberCapability struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Role          string   `json:"role"`
	Capabilities  []string `json:"capabilities"`
	Specialization []string `json:"specialization"`
	Availability  string   `json:"availability"`
}

// understandTask 步骤1: 深度理解用户意图
func (sp *StructuredPlanner) understandTask(ctx context.Context, userMessage, userID string, context *PlanContext) (*TaskUnderstanding, error) {
	// 获取相关记忆作为上下文
	var memoryContext string
	if sp.memoryService != nil && userID != "" {
		memories, _ := sp.memoryService.SearchMemories(userID, userMessage)
		if len(memories) > 0 {
			var parts []string
			for _, m := range memories {
				parts = append(parts, fmt.Sprintf("- [%s] %s", m.MemoryType, m.Content))
			}
			memoryContext = strings.Join(parts, "\n")
		}
	}

	prompt := fmt.Sprintf(`你是一个深度任务理解专家。请分析以下用户请求，理解其真正意图和隐含需求。

## 用户请求
%s

## 相关记忆（如有）
%s

## 分析要求

请按以下步骤深度理解：

1. **用户意图**: 用户真正想要什么？（不是字面意思，而是背后的目标）
2. **隐含需求**: 用户没有明说但实际需要的东西
3. **已知约束**: 用户提到的或可以推断的限制条件
4. **模糊点**: 需要进一步澄清的地方
5. **假设**: 为了继续规划，我们做出的合理假设
6. **成功标准**: 什么样的结果用户会满意？
7. **相关上下文**: 有哪些相关的背景信息？

请以 JSON 格式输出：
{
  "user_intent": "一句话描述用户真正想要的",
  "implicit_needs": ["隐含需求1", "隐含需求2"],
  "constraints": ["约束1"],
  "ambiguities": ["模糊点1"],
  "assumptions": ["假设1"],
  "success_criteria": "成功标准描述",
  "related_context": ["相关上下文1"],
  "confidence": 0.0-1.0
}`, userMessage, memoryContext)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "你是一个深度任务理解专家。不要急于行动，先认真理解用户真正需要什么。只返回 JSON。"},
			{Role: llm.RoleUser, Content: prompt},
		},
		Model:       sp.model,
		Temperature: 0.2,
		MaxTokens:   2000,
	}

	resp, err := sp.llmClient.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("task understanding failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from LLM")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	content = extractJSON(content)

	var understanding TaskUnderstanding
	if err := json.Unmarshal([]byte(content), &understanding); err != nil {
		return nil, fmt.Errorf("failed to parse understanding: %w", err)
	}

	return &understanding, nil
}

// createPlan 步骤2: 结构化规划（分阶段、子任务、依赖、工具、风险）
func (sp *StructuredPlanner) createPlan(ctx context.Context, userMessage string, understanding *TaskUnderstanding, context *PlanContext) (*StructuredPlan, error) {
	// 获取可用技能列表
	var skillsContext string
	if sp.skillService != nil {
		skills, _ := sp.skillService.ListSkills()
		if len(skills) > 0 {
			var parts []string
			for _, s := range skills {
				parts = append(parts, fmt.Sprintf("- %s: %s (触发词: %v)", s.Name, s.Description, s.Triggers))
			}
			skillsContext = strings.Join(parts, "\n")
		}
	}

	prompt := fmt.Sprintf(`你是一个任务规划专家。基于以下理解，制定一个结构化的执行计划。

## 用户请求
%s

## 任务理解
- 用户意图: %s
- 隐含需求: %v
- 约束: %v
- 成功标准: %s

## 可用技能
%s

## 规划要求

请按照以下步骤制定计划：

### 第一步：任务拆解
将大任务分解为阶段（Phases），每个阶段包含若干子任务（Subtasks）。
- 阶段应该按逻辑顺序组织（如：分析 → 设计 → 实现 → 测试）
- 每个子任务应该是可独立完成的

### 第二步：依赖分析
确定子任务之间的依赖关系。
- hard: 必须完成后才能开始下一步
- soft: 建议但不强制

### 第三步：工具/技能分配
为每个子任务分配合适的技能和工具。

### 第四步：风险评估
识别潜在风险并给出缓解措施。

### 第五步：回退策略
如果某步失败了，应该怎么办？

请以 JSON 格式输出：
{
  "phases": [
    {
      "name": "阶段名称",
      "description": "阶段描述",
      "order": 1,
      "subtasks": [
        {
          "id": "唯一ID（英文小写下划线）",
          "title": "子任务标题",
          "description": "详细描述",
          "type": "coding|research|testing|review|documentation",
          "skills": ["所需技能"],
          "order": 1,
          "estimated": 预估分钟数,
          "required": true,
          "can_parallel": false
        }
      ]
    }
  ],
  "dependencies": [
    {"from": "子任务ID", "to": "依赖的子任务ID", "type": "hard|soft"}
  ],
  "tool_plan": [
    {"subtask_id": "子任务ID", "skills": ["技能"], "tools": ["工具"], "priority": "primary|secondary"}
  ],
  "risk_assessment": {
    "overall_risk": "low|medium|high",
    "top_risks": [
      {"description": "风险描述", "probability": "low|medium|high", "impact": "low|medium|high", "mitigation": "缓解措施"}
    ],
    "mitigations": ["缓解措施1"]
  },
  "fallback_plans": [
    {"trigger": "触发条件", "description": "回退策略", "steps": ["步骤1"]}
  ],
  "estimated_time": 总预估分钟数,
  "plan_summary": "一段话描述整个计划"
}`, userMessage, understanding.UserIntent, understanding.ImplicitNeeds, understanding.Constraints, understanding.SuccessCriteria, skillsContext)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "你是一个任务规划专家。制定详细、可执行的计划。只返回 JSON。"},
			{Role: llm.RoleUser, Content: prompt},
		},
		Model:       sp.model,
		Temperature: 0.3,
		MaxTokens:   4000,
	}

	resp, err := sp.llmClient.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("planning failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from LLM")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	content = extractJSON(content)

	var plan StructuredPlan
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		return nil, fmt.Errorf("failed to parse plan: %w", err)
	}

	return &plan, nil
}

// fallbackUnderstanding 理解失败时的回退
func (sp *StructuredPlanner) fallbackUnderstanding(userMessage string) *TaskUnderstanding {
	return &TaskUnderstanding{
		UserIntent:    userMessage,
		ImplicitNeeds: []string{},
		Constraints:   []string{},
		Ambiguities:   []string{},
		Assumptions:   []string{"假设用户描述是完整的"},
		SuccessCriteria: "完成用户请求的内容",
		Confidence:    0.5,
	}
}

// fallbackPlan 规划失败时的回退
func (sp *StructuredPlanner) fallbackPlan(understanding *TaskUnderstanding, userMessage string) *StructuredPlan {
	subtaskID := "main_task"
	return &StructuredPlan{
		Phases: []PlanPhase{
			{
				Name:        "执行",
				Description: understanding.UserIntent,
				Order:       1,
				Subtasks: []Subtask{
					{
						ID:          subtaskID,
						Title:       "执行任务",
						Description: userMessage,
						Type:        "research",
						Order:       1,
						Estimated:   30,
						Required:    true,
					},
				},
			},
		},
		RiskAssessment: &RiskAssessment{
			OverallRisk: "medium",
			TopRisks: []Risk{
				{Description: "规划可能不完整", Probability: "medium", Impact: "medium", Mitigation: "执行过程中动态调整"},
			},
			Mitigations: []string{"如果遇到问题，重新规划"},
		},
		FallbackPlans: []FallbackPlan{
			{Trigger: "执行失败", Description: "尝试简化方案", Steps: []string{"分析失败原因", "简化任务", "重新执行"}},
		},
		EstimatedTime: 30,
		PlanSummary:   fmt.Sprintf("简化计划: %s", understanding.UserIntent),
	}
}

// countSubtasks 统计子任务总数
func (sp *StructuredPlanner) countSubtasks(phases []PlanPhase) int {
	count := 0
	for _, phase := range phases {
		count += len(phase.Subtasks)
	}
	return count
}

// extractJSON 从文本中提取 JSON
func extractJSON(s string) string {
	// Try to find JSON object
	start := strings.Index(s, "{")
	if start == -1 {
		return s
	}

	// Count braces to find matching end
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}

	return s[start:]
}

// truncatePlanStr 截断字符串
func truncatePlanStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

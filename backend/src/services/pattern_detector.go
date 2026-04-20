package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// PatternDetector 操作模式检测服务
type PatternDetector struct {
	db       *gorm.DB
	modelSvc *ModelService
	enabled  bool
}

// WorkflowPattern 工作流模式
type WorkflowPattern struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Steps       []string `json:"steps"`
	Triggers    []string `json:"triggers"`
	Frequency   int      `json:"frequency"`
	Confidence  float64  `json:"confidence"`
}

// NewPatternDetector 创建模式检测器
func NewPatternDetector(db *gorm.DB, modelSvc *ModelService, enabled bool) *PatternDetector {
	return &PatternDetector{
		db:       db,
		modelSvc: modelSvc,
		enabled:  enabled,
	}
}

// DetectWorkflowPatterns 检测工作流模式
func (s *PatternDetector) DetectWorkflowPatterns(ctx context.Context, userID string) ([]WorkflowPattern, error) {
	if !s.enabled {
		return nil, nil
	}

	// 获取最近的工具调用序列
	var toolExecs []models.ToolExecution
	s.db.Joins("JOIN messages ON tool_executions.message_id = messages.id").
		Where("messages.dialogue_id IN (?)",
			s.db.Table("messages").Select("dialogue_id").Where("sender = ?", "assistant").Group("dialogue_id").Limit(50)).
		Order("tool_executions.created_at ASC").
		Find(&toolExecs)

	if len(toolExecs) == 0 {
		return nil, nil
	}

	// 按对话分组
	dialogueTools := make(map[string][]models.ToolExecution)
	for _, exec := range toolExecs {
		var msg models.Message
		s.db.First(&msg, "id = ?", exec.MessageID)
		dialogueTools[msg.DialogueID] = append(dialogueTools[msg.DialogueID], exec)
	}

	// 使用 LLM 分析重复的操作序列
	patterns, err := s.extractWorkflowPatterns(ctx, dialogueTools)
	if err != nil {
		return nil, err
	}

	return patterns, nil
}

// extractWorkflowPatterns 使用 LLM 提取工作流模式
func (s *PatternDetector) extractWorkflowPatterns(ctx context.Context, dialogueTools map[string][]models.ToolExecution) ([]WorkflowPattern, error) {
	client := s.modelSvc.GetLLMClient()
	if client == nil {
		return nil, fmt.Errorf("no LLM client available")
	}

	var builder strings.Builder
	builder.WriteString("以下是用户在多个对话中使用的工具调用序列：\n\n")
	for dialogueID, tools := range dialogueTools {
		builder.WriteString(fmt.Sprintf("对话 %s:\n", dialogueID))
		for _, t := range tools {
			builder.WriteString(fmt.Sprintf("  - %s (%s)\n", t.ToolName, t.Status))
		}
		builder.WriteString("\n")
	}

	prompt := fmt.Sprintf(`你是一个工作流分析专家。请分析以下工具调用序列，找出重复出现的操作模式。

要求：
1. 只提取至少出现 2 次的操作序列
2. 为每个模式生成：名称、描述、步骤、触发词
3. 模式应该是可复用的工作流

%s

请以 JSON 数组格式返回：
[
  {
    "name": "工作流名称",
    "description": "工作流描述",
    "steps": ["步骤1", "步骤2"],
    "triggers": ["触发词1"],
    "frequency": 出现次数,
    "confidence": 置信度 (0-1)
  }
]

如果没有发现模式，返回空数组 []。`, builder.String())

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个工作流分析专家。从工具调用序列中发现重复模式。只返回 JSON 数组。"},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.2,
		MaxTokens:   2000,
	}

	resp, err := client.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from LLM")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var patterns []WorkflowPattern
	if err := json.Unmarshal([]byte(content), &patterns); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	return patterns, nil
}

// ConvertPatternToWorkflow 将工作流模式转换为 Workflow
func (s *PatternDetector) ConvertPatternToWorkflow(pattern WorkflowPattern, userID string) (*models.Workflow, error) {
	workflow := &models.Workflow{
		ID:          uuid.New().String(),
		Name:        pattern.Name,
		Description: pattern.Description,
		Version:     "1.0",
		Category:    "custom",
		Enabled:     true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.db.Create(workflow).Error; err != nil {
		return nil, err
	}

	// 创建工作流步骤
	for i, step := range pattern.Steps {
		workflowStep := &models.WorkflowStep{
			ID:           uuid.New().String(),
			WorkflowID:   workflow.ID,
			Name:         fmt.Sprintf("Step %d", i+1),
			Description:  step,
			Type:         "action",
			Order:        i,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		s.db.Create(workflowStep)
	}

	log.Printf("[PatternDetector] Created workflow '%s' with %d steps", workflow.Name, len(pattern.Steps))
	return workflow, nil
}

// RunPeriodicPatternDetection 定期执行模式检测
func (s *PatternDetector) RunPeriodicPatternDetection(ctx context.Context) {
	if !s.enabled {
		return
	}

	log.Printf("[PatternDetector] Starting periodic pattern detection...")

	var users []models.User
	s.db.Find(&users)

	for _, user := range users {
		patterns, err := s.DetectWorkflowPatterns(ctx, user.ID)
		if err != nil {
			log.Printf("[PatternDetector] Failed for user %s: %v", user.ID, err)
			continue
		}

		for _, p := range patterns {
			if p.Frequency >= 2 && p.Confidence >= 0.6 {
				_, err := s.ConvertPatternToWorkflow(p, user.ID)
				if err != nil {
					log.Printf("[PatternDetector] Failed to create workflow '%s': %v", p.Name, err)
				}
			}
		}
	}

	log.Printf("[PatternDetector] Periodic detection completed")
}

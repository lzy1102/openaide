package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"openaide/backend/src/services/llm"
)

// ChainOfThoughtService 思维链推理服务
// 让 AI 在回答前先逐步思考，提高回答质量
type ChainOfThoughtService struct {
	modelSvc  *ModelService
	logger    *LoggerService
}

// CoTRequest 思维链推理请求
type CoTRequest struct {
	Question    string   `json:"question"`
	Context     string   `json:"context,omitempty"`
	Tools       []string `json:"tools,omitempty"`
	ModelID     string   `json:"model_id,omitempty"`
}

// CoTResult 思维链推理结果
type CoTResult struct {
	Thinking    string `json:"thinking"`
	Answer      string `json:"answer"`
	Steps       int    `json:"steps"`
	Duration    time.Duration `json:"duration"`
}

// NewChainOfThoughtService 创建思维链推理服务
func NewChainOfThoughtService(modelSvc *ModelService, logger *LoggerService) *ChainOfThoughtService {
	return &ChainOfThoughtService{
		modelSvc: modelSvc,
		logger:   logger,
	}
}

// Reason 执行思维链推理
func (s *ChainOfThoughtService) Reason(ctx context.Context, req *CoTRequest) (*CoTResult, error) {
	start := time.Now()

	// 确定使用的模型
	modelID := req.ModelID
	if modelID == "" {
		defaultModel, err := s.modelSvc.GetDefaultModel()
		if err != nil {
			return nil, fmt.Errorf("no model available: %w", err)
		}
		modelID = defaultModel.ID
	}

	// 构建思维链提示词
	prompt := s.buildCoTPrompt(req)

	messages := []llm.Message{
		{Role: "system", Content: `你是一个思维链推理专家。请按照以下步骤思考：
1. 理解问题：分析用户问题的核心需求
2. 拆解问题：将复杂问题分解为小步骤
3. 逐步推理：对每个步骤进行详细推理
4. 得出结论：基于推理得出最终答案

请严格按照 JSON 格式返回，不要包含其他内容。`},
		{Role: "user", Content: prompt},
	}

	options := map[string]interface{}{
		"temperature": 0.7,
	}

	resp, err := s.modelSvc.Chat(modelID, messages, options)
	if err != nil {
		return nil, fmt.Errorf("CoT reasoning failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty CoT response")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	
	// 尝试解析 JSON 响应
	var result struct {
		Thinking string `json:"thinking"`
		Answer   string `json:"answer"`
		Steps    int    `json:"steps"`
	}

	// 清理可能的 markdown 代码块标记
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		// 如果解析失败，将原始内容作为 answer 返回
		s.logger.Warn(ctx, "[CoT] Failed to parse JSON response, using raw content")
		result.Answer = content
		result.Thinking = ""
		result.Steps = 0
	}

	s.logger.Info(ctx, "[CoT] Reasoning completed for question: %s (steps=%d, duration=%v)",
		req.Question, result.Steps, time.Since(start))

	return &CoTResult{
		Thinking: result.Thinking,
		Answer:   result.Answer,
		Steps:    result.Steps,
		Duration: time.Since(start),
	}, nil
}

// buildCoTPrompt 构建思维链提示词
func (s *ChainOfThoughtService) buildCoTPrompt(req *CoTRequest) string {
	prompt := fmt.Sprintf("问题：%s\n\n", req.Question)

	if req.Context != "" {
		prompt += fmt.Sprintf("上下文信息：\n%s\n\n", req.Context)
	}

	if len(req.Tools) > 0 {
		prompt += fmt.Sprintf("可用工具：%s\n\n", strings.Join(req.Tools, ", "))
	}

	prompt += `请按照以下格式回答（必须是有效的 JSON）：
{
  "thinking": "你的逐步思考过程",
  "answer": "最终答案",
  "steps": 推理步骤数量
}`

	return prompt
}

// ReasonAndAnswer 简化接口：直接提问并获取带推理的答案
func (s *ChainOfThoughtService) ReasonAndAnswer(ctx context.Context, question, modelID string) (string, error) {
	result, err := s.Reason(ctx, &CoTRequest{
		Question: question,
		ModelID:  modelID,
	})
	if err != nil {
		return "", err
	}

	// 如果有思考过程，可以将其添加到最终答案前
	if result.Thinking != "" {
		return fmt.Sprintf("💭 思考过程：\n%s\n\n---\n\n✅ 答案：\n%s", result.Thinking, result.Answer), nil
	}

	return result.Answer, nil
}

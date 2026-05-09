package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"openaide/backend/src/services/llm"
)

const (
	interviewAnalysisPrompt = `You are analyzing a user's request to identify information gaps that need to be filled before proceeding.

User request: %s

Current context:
%s

Identify what critical information is missing. For each gap, generate a follow-up question.

Respond in JSON format:
{
  "understood": ["What you understood from the request"],
  "gaps": [
    {
      "topic": "What information is missing",
      "question": "A clear, specific question to ask the user",
      "priority": "critical|important|nice_to_have",
      "default_assumption": "If user doesn't answer, what to assume"
    }
  ],
  "readiness_score": 0.0-1.0,
  "recommendation": "proceed|ask_critical|ask_all"
}

Rules:
- Only identify genuinely important gaps, not nice-to-know details
- Questions should be specific and actionable
- If readiness_score >= 0.7 and no critical gaps, recommend "proceed"
- If there are critical gaps, recommend "ask_critical"
- Default assumptions should be reasonable, not risky`

	interviewSynthesisPrompt = `Based on the user's answers to follow-up questions, synthesize the complete context.

Original request: %s

Questions asked and answers received:
%s

Provide a comprehensive context summary that fills in all the gaps.
Respond in JSON format:
{
  "complete_context": "Full context with all gaps filled",
  "remaining_gaps": ["Any still-unresolved gaps"],
  "confidence": 0.0-1.0,
  "ready_to_proceed": true|false
}`
)

type InformationGap struct {
	Topic              string `json:"topic"`
	Question           string `json:"question"`
	Priority           string `json:"priority"`
	DefaultAssumption  string `json:"default_assumption"`
	Answer             string `json:"answer,omitempty"`
	Answered           bool   `json:"answered"`
}

type InterviewAnalysis struct {
	Understood     []string         `json:"understood"`
	Gaps           []InformationGap `json:"gaps"`
	ReadinessScore float64          `json:"readiness_score"`
	Recommendation string           `json:"recommendation"`
}

type InterviewSynthesis struct {
	CompleteContext string   `json:"complete_context"`
	RemainingGaps  []string `json:"remaining_gaps"`
	Confidence     float64  `json:"confidence"`
	ReadyToProceed bool     `json:"ready_to_proceed"`
}

type DeepInterviewService struct {
	modelCaller ModelCaller
}

func NewDeepInterviewService(modelCaller ModelCaller) *DeepInterviewService {
	return &DeepInterviewService{
		modelCaller: modelCaller,
	}
}

func (dis *DeepInterviewService) AnalyzeGaps(ctx context.Context, request string, contextStr string) (*InterviewAnalysis, error) {
	prompt := fmt.Sprintf(interviewAnalysisPrompt, request, contextStr)

	modelID := dis.findInterviewModel()
	resp, err := dis.modelCaller.Chat(modelID, []llm.Message{
		{Role: llm.RoleUser, Content: prompt},
	}, map[string]interface{}{"max_tokens": float64(2000), "temperature": float64(0.2)})
	if err != nil {
		return nil, fmt.Errorf("interview analysis failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from interview LLM")
	}

	content := resp.Choices[0].Message.Content
	content = extractInterviewJSON(content)

	var analysis InterviewAnalysis
	if err := json.Unmarshal([]byte(content), &analysis); err != nil {
		return nil, fmt.Errorf("failed to parse interview analysis: %w", err)
	}

	slog.Info("Interview analysis completed", "component", "DeepInterview",
		"gaps_found", len(analysis.Gaps),
		"readiness_score", analysis.ReadinessScore,
		"recommendation", analysis.Recommendation)

	return &analysis, nil
}

func (dis *DeepInterviewService) GenerateFollowUpQuestions(ctx context.Context, request string, contextStr string) ([]string, error) {
	analysis, err := dis.AnalyzeGaps(ctx, request, contextStr)
	if err != nil {
		return nil, err
	}

	var questions []string
	for _, gap := range analysis.Gaps {
		if gap.Priority == "critical" || gap.Priority == "important" {
			questions = append(questions, gap.Question)
		}
	}

	return questions, nil
}

func (dis *DeepInterviewService) SynthesizeAnswers(ctx context.Context, originalRequest string, qaPairs []InformationGap) (*InterviewSynthesis, error) {
	var qaText strings.Builder
	for i, gap := range qaPairs {
		qaText.WriteString(fmt.Sprintf("Q%d: %s\n", i+1, gap.Question))
		if gap.Answered {
			qaText.WriteString(fmt.Sprintf("A%d: %s\n", i+1, gap.Answer))
		} else {
			qaText.WriteString(fmt.Sprintf("A%d: (not answered, assuming: %s)\n", i+1, gap.DefaultAssumption))
		}
	}

	prompt := fmt.Sprintf(interviewSynthesisPrompt, originalRequest, qaText.String())

	modelID := dis.findInterviewModel()
	resp, err := dis.modelCaller.Chat(modelID, []llm.Message{
		{Role: llm.RoleUser, Content: prompt},
	}, map[string]interface{}{"max_tokens": float64(2000), "temperature": float64(0.3)})
	if err != nil {
		return nil, fmt.Errorf("interview synthesis failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from synthesis LLM")
	}

	content := resp.Choices[0].Message.Content
	content = extractInterviewJSON(content)

	var synthesis InterviewSynthesis
	if err := json.Unmarshal([]byte(content), &synthesis); err != nil {
		return nil, fmt.Errorf("failed to parse interview synthesis: %w", err)
	}

	return &synthesis, nil
}

func (dis *DeepInterviewService) ShouldInterview(request string, contextStr string) bool {
	lowerRequest := strings.ToLower(request)

	ambiguousIndicators := []string{
		"帮我", "弄一下", "搞一下", "处理", "优化", "改进", "fix", "improve",
		"refactor", "update", "change", "help", "make it better",
	}

	for _, indicator := range ambiguousIndicators {
		if strings.Contains(lowerRequest, indicator) {
			wordCount := len(strings.Fields(request))
			if wordCount < 10 {
				return true
			}
		}
	}

	return false
}

func (dis *DeepInterviewService) findInterviewModel() string {
	models, err := dis.modelCaller.ListModels()
	if err != nil || len(models) == 0 {
		return ""
	}

	for _, m := range models {
		for _, tag := range m.Tags {
			if strings.TrimSpace(tag) == "fast" {
				return m.ID
			}
		}
	}

	for _, m := range models {
		if m.Status == "enabled" {
			return m.ID
		}
	}

	return models[0].ID
}

func extractInterviewJSON(content string) string {
	start := strings.Index(content, "{")
	if start < 0 {
		return content
	}

	depth := 0
	for i := start; i < len(content); i++ {
		switch content[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return content[start : i+1]
			}
		}
	}

	return content[start:]
}

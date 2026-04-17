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

// CorrectionService 自动纠错服务
type CorrectionService struct {
	db        *gorm.DB
	llmClient llm.LLMClient
	mu        sync.RWMutex
}

// NewCorrectionService 创建纠错服务实例
func NewCorrectionService(db *gorm.DB, llmClient llm.LLMClient) *CorrectionService {
	return &CorrectionService{
		db:        db,
		llmClient: llmClient,
	}
}

// ============ 纠错请求和响应类型 ============

// OutputEvaluationRequest 输出质量评估请求
type OutputEvaluationRequest struct {
	// OriginalQuery 原始查询
	OriginalQuery string `json:"original_query"`
	// Output 输出内容
	Output string `json:"output"`
	// Context 上下文信息
	Context string `json:"context,omitempty"`
	// ThoughtID 关联的思考ID
	ThoughtID string `json:"thought_id,omitempty"`
	// UserID 用户ID
	UserID string `json:"user_id"`
	// Model 模型名称
	Model string `json:"model,omitempty"`
}

// OutputEvaluationResponse 输出质量评估响应
type OutputEvaluationResponse struct {
	// EvaluationID 评估ID
	EvaluationID string `json:"evaluation_id"`
	// QualityScore 质量分数 (0-100)
	QualityScore float64 `json:"quality_score"`
	// Confidence 置信度
	Confidence float64 `json:"confidence"`
	// IssuesDetected 检测到的问题列表
	IssuesDetected []Issue `json:"issues_detected"`
	// NeedsCorrection 是否需要修正
	NeedsCorrection bool `json:"needs_correction"`
	// EvaluationDetails 评估详情
	EvaluationDetails *EvaluationDetails `json:"evaluation_details"`
	// Error 错误信息
	Error string `json:"error,omitempty"`
}

// Issue 问题定义
type Issue struct {
	// IssueID 问题ID
	IssueID string `json:"issue_id"`
	// IssueType 问题类型
	IssueType string `json:"issue_type"` // factual, logical, linguistic, safety, formatting
	// Severity 严重程度
	Severity string `json:"severity"` // critical, high, medium, low
	// Description 问题描述
	Description string `json:"description"`
	// Location 位置信息
	Location string `json:"location,omitempty"`
	// SuggestedCorrection 建议修正
	SuggestedCorrection string `json:"suggested_correction,omitempty"`
	// Confidence 置信度
	Confidence float64 `json:"confidence"`
}

// EvaluationDetails 评估详情
type EvaluationDetails struct {
	// FactualAccuracy 事实准确性
	FactualAccuracy float64 `json:"factual_accuracy"`
	// LogicalCoherence 逻辑连贯性
	LogicalCoherence float64 `json:"logical_coherence"`
	// LinguisticQuality 语言质量
	LinguisticQuality float64 `json:"linguistic_quality"`
	// Relevance 相关性
	Relevance float64 `json:"relevance"`
	// Completeness 完整性
	Completeness float64 `json:"completeness"`
	// Safety 安全性
	Safety float64 `json:"safety"`
	// Formatting 格式规范性
	Formatting float64 `json:"formatting"`
}

// ErrorDetectionRequest 错误检测请求
type ErrorDetectionRequest struct {
	// Content 待检测内容
	Content string `json:"content"`
	// DetectionType 检测类型
	DetectionType string `json:"detection_type,omitempty"` // all, factual, logical, linguistic, safety
	// Strictness 严格程度 (1-5)
	Strictness int `json:"strictness,omitempty"`
	// Context 上下文信息
	Context string `json:"context,omitempty"`
	// OriginalQuery 原始查询
	OriginalQuery string `json:"original_query,omitempty"`
	// Model 模型名称
	Model string `json:"model,omitempty"`
}

// ErrorDetectionResponse 错误检测响应
type ErrorDetectionResponse struct {
	// DetectionID 检测ID
	DetectionID string `json:"detection_id"`
	// Issues 检测到的问题
	Issues []Issue `json:"issues"`
	// TotalIssues 总问题数
	TotalIssues int `json:"total_issues"`
	// CriticalIssues 严重问题数
	CriticalIssues int `json:"critical_issues"`
	// DetectionSummary 检测摘要
	DetectionSummary string `json:"detection_summary"`
	// Error 错误信息
	Error string `json:"error,omitempty"`
}

// CorrectionSuggestionRequest 修正建议生成请求
type CorrectionSuggestionRequest struct {
	// OriginalContent 原始内容
	OriginalContent string `json:"original_content"`
	// Issues 问题列表
	Issues []Issue `json:"issues"`
	// ThoughtID 关联的思考ID
	ThoughtID string `json:"thought_id,omitempty"`
	// UserID 用户ID
	UserID string `json:"user_id"`
	// CorrectionStrategy 修正策略
	CorrectionStrategy string `json:"correction_strategy,omitempty"` // minimal, comprehensive, interactive
	// Model 模型名称
	Model string `json:"model,omitempty"`
}

// CorrectionSuggestionResponse 修正建议响应
type CorrectionSuggestionResponse struct {
	// SuggestionID 建议ID
	SuggestionID string `json:"suggestion_id"`
	// OriginalContent 原始内容
	OriginalContent string `json:"original_content"`
	// CorrectedContent 修正后内容
	CorrectedContent string `json:"corrected_content"`
	// ChangesApplied 应用的修改
	ChangesApplied []Change `json:"changes_applied"`
	// CorrectionSummary 修正摘要
	CorrectionSummary string `json:"correction_summary"`
	// EstimatedImprovement 预估改进
	EstimatedImprovement float64 `json:"estimated_improvement"`
	// Error 错误信息
	Error string `json:"error,omitempty"`
}

// Change 修改详情
type Change struct {
	// ChangeID 修改ID
	ChangeID string `json:"change_id"`
	// ChangeType 修改类型
	ChangeType string `json:"change_type"` // addition, deletion, replacement, reorganization
	// Original 原始文本
	Original string `json:"original,omitempty"`
	// Replacement 替换文本
	Replacement string `json:"replacement,omitempty"`
	// Position 位置信息
	Position string `json:"position,omitempty"`
	// Reasoning 修改理由
	Reasoning string `json:"reasoning"`
}

// AutoCorrectionRequest 自动修正请求（包含验证循环）
type AutoCorrectionRequest struct {
	// Content 内容
	Content string `json:"content"`
	// OriginalQuery 原始查询
	OriginalQuery string `json:"original_query"`
	// Context 上下文
	Context string `json:"context,omitempty"`
	// ThoughtID 关联的思考ID
	ThoughtID string `json:"thought_id,omitempty"`
	// UserID 用户ID
	UserID string `json:"user_id"`
	// MaxRetries 最大重试次数（默认3）
	MaxRetries int `json:"max_retries,omitempty"`
	// QualityThreshold 质量阈值（默认80）
	QualityThreshold float64 `json:"quality_threshold,omitempty"`
	// CorrectionStrategy 修正策略
	CorrectionStrategy string `json:"correction_strategy,omitempty"`
	// Model 模型名称
	Model string `json:"model,omitempty"`
}

// AutoCorrectionResponse 自动修正响应
type AutoCorrectionResponse struct {
	// CorrectionID 修正ID
	CorrectionID string `json:"correction_id"`
	// OriginalContent 原始内容
	OriginalContent string `json:"original_content"`
	// FinalContent 最终修正内容
	FinalContent string `json:"final_content"`
	// Iterations 迭代历史
	Iterations []CorrectionIteration `json:"iterations"`
	// TotalIterations 总迭代次数
	TotalIterations int `json:"total_iterations"`
	// FinalQualityScore 最终质量分数
	FinalQualityScore float64 `json:"final_quality_score"`
	// Success 是否成功达到阈值
	Success bool `json:"success"`
	// CorrectionSummary 修正摘要
	CorrectionSummary string `json:"correction_summary"`
	// Error 错误信息
	Error string `json:"error,omitempty"`
}

// CorrectionIteration 修正迭代记录
type CorrectionIteration struct {
	// IterationNumber 迭代编号
	IterationNumber int `json:"iteration_number"`
	// InputContent 输入内容
	InputContent string `json:"input_content"`
	// OutputContent 输出内容
	OutputContent string `json:"output_content"`
	// QualityScore 质量分数
	QualityScore float64 `json:"quality_score"`
	// IssuesDetected 检测到的问题
	IssuesDetected []Issue `json:"issues_detected"`
	// ChangesApplied 应用的修改
	ChangesApplied []Change `json:"changes_applied"`
	// StartTime 开始时间
	StartTime time.Time `json:"start_time"`
	// EndTime 结束时间
	EndTime time.Time `json:"end_time"`
	// TokenUsage Token使用情况
	TokenUsage *llm.Usage `json:"token_usage,omitempty"`
}

// CorrectionHistoryRequest 纠错历史请求
type CorrectionHistoryRequest struct {
	// ThoughtID 思考ID
	ThoughtID string `json:"thought_id,omitempty"`
	// UserID 用户ID
	UserID string `json:"user_id,omitempty"`
	// Limit 限制数量
	Limit int `json:"limit,omitempty"`
	// Offset 偏移量
	Offset int `json:"offset,omitempty"`
	// StartTime 开始时间
	StartTime time.Time `json:"start_time,omitempty"`
	// EndTime 结束时间
	EndTime time.Time `json:"end_time,omitempty"`
}

// CorrectionHistoryResponse 纠错历史响应
type CorrectionHistoryResponse struct {
	// HistoryItems 历史记录
	HistoryItems []CorrectionHistoryItem `json:"history_items"`
	// TotalCount 总数
	TotalCount int64 `json:"total_count"`
	// Summary 摘要统计
	Summary *HistorySummary `json:"summary,omitempty"`
}

// CorrectionHistoryItem 纠错历史项
type CorrectionHistoryItem struct {
	// Correction 修正记录
	Correction models.Correction `json:"correction"`
	// ThoughtContent 思考内容
	ThoughtContent string `json:"thought_content,omitempty"`
	// QualityMetrics 质量指标
	QualityMetrics *EvaluationDetails `json:"quality_metrics,omitempty"`
	// IssueCount 问题数量
	IssueCount int `json:"issue_count"`
}

// HistorySummary 历史摘要
type HistorySummary struct {
	// TotalCorrections 总修正次数
	TotalCorrections int64 `json:"total_corrections"`
	// ResolvedCount 已解决数量
	ResolvedCount int64 `json:"resolved_count"`
	// PendingCount 待处理数量
	PendingCount int64 `json:"pending_count"`
	// AverageQualityScore 平均质量分数
	AverageQualityScore float64 `json:"average_quality_score"`
	// MostCommonIssueTypes 最常见的问题类型
	MostCommonIssueTypes []string `json:"most_common_issue_types"`
}

// ============ 核心方法实现 ============

// EvaluateOutputQuality 评估输出质量
func (s *CorrectionService) EvaluateOutputQuality(ctx context.Context, req *OutputEvaluationRequest) (*OutputEvaluationResponse, error) {
	evaluationID := uuid.New().String()

	resp := &OutputEvaluationResponse{
		EvaluationID:     evaluationID,
		IssuesDetected:   make([]Issue, 0),
		NeedsCorrection:  false,
		EvaluationDetails: &EvaluationDetails{},
	}

	// 构建评估提示词
	prompt := s.buildEvaluationPrompt(req)

	// 调用LLM进行评估
	llmResp, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		resp.Error = fmt.Sprintf("LLM调用失败: %v", err)
		return resp, err
	}

	// 解析评估结果
	evalResult, err := s.parseEvaluationResult(llmResp)
	if err != nil {
		resp.Error = fmt.Sprintf("解析评估结果失败: %v", err)
		return resp, err
	}

	resp.QualityScore = evalResult.QualityScore
	resp.Confidence = evalResult.Confidence
	resp.IssuesDetected = evalResult.Issues
	resp.EvaluationDetails = evalResult.Details
	resp.NeedsCorrection = resp.QualityScore < 80 || len(resp.IssuesDetected) > 0

	// 保存评估记录
	if req.ThoughtID != "" {
		s.saveEvaluationRecord(req, resp)
	}

	return resp, nil
}

// DetectErrors 检测错误
func (s *CorrectionService) DetectErrors(ctx context.Context, req *ErrorDetectionRequest) (*ErrorDetectionResponse, error) {
	detectionID := uuid.New().String()

	resp := &ErrorDetectionResponse{
		DetectionID: detectionID,
		Issues:      make([]Issue, 0),
	}

	// 设置默认严格程度
	if req.Strictness == 0 {
		req.Strictness = 3
	}

	// 构建检测提示词
	prompt := s.buildDetectionPrompt(req)

	// 调用LLM进行检测
	llmResp, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		resp.Error = fmt.Sprintf("LLM调用失败: %v", err)
		return resp, err
	}

	// 解析检测结果
	detectionResult, err := s.parseDetectionResult(llmResp)
	if err != nil {
		resp.Error = fmt.Sprintf("解析检测结果失败: %v", err)
		return resp, err
	}

	resp.Issues = detectionResult.Issues
	resp.TotalIssues = len(resp.Issues)
	resp.CriticalIssues = s.countCriticalIssues(resp.Issues)
	resp.DetectionSummary = s.generateDetectionSummary(resp.Issues)

	return resp, nil
}

// GenerateCorrectionSuggestion 生成修正建议
func (s *CorrectionService) GenerateCorrectionSuggestion(ctx context.Context, req *CorrectionSuggestionRequest) (*CorrectionSuggestionResponse, error) {
	suggestionID := uuid.New().String()

	resp := &CorrectionSuggestionResponse{
		SuggestionID:     suggestionID,
		OriginalContent:  req.OriginalContent,
		ChangesApplied:   make([]Change, 0),
		CorrectedContent: req.OriginalContent, // 默认无修改
	}

	// 设置默认修正策略
	if req.CorrectionStrategy == "" {
		req.CorrectionStrategy = "comprehensive"
	}

	// 构建修正提示词
	prompt := s.buildCorrectionPrompt(req)

	// 调用LLM生成修正
	llmResp, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		resp.Error = fmt.Sprintf("LLM调用失败: %v", err)
		return resp, err
	}

	// 解析修正结果
	correctionResult, err := s.parseCorrectionResult(llmResp)
	if err != nil {
		resp.Error = fmt.Sprintf("解析修正结果失败: %v", err)
		return resp, err
	}

	resp.CorrectedContent = correctionResult.CorrectedContent
	resp.ChangesApplied = correctionResult.Changes
	resp.CorrectionSummary = correctionResult.Summary
	resp.EstimatedImprovement = correctionResult.EstimatedImprovement

	// 创建修正记录
	if req.ThoughtID != "" {
		correction := &models.Correction{
			ID:        uuid.New().String(),
			ThoughtID: req.ThoughtID,
			Content:   resp.CorrectedContent,
			UserID:    req.UserID,
			Status:    "pending",
		}
		if err := s.CreateCorrection(correction); err != nil {
			return resp, fmt.Errorf("创建修正记录失败: %w", err)
		}
	}

	return resp, nil
}

// AutoCorrectWithValidation 自动修正并验证（最多3次重试）
func (s *CorrectionService) AutoCorrectWithValidation(ctx context.Context, req *AutoCorrectionRequest) (*AutoCorrectionResponse, error) {
	correctionID := uuid.New().String()

	// 设置默认值
	if req.MaxRetries == 0 {
		req.MaxRetries = 3
	}
	if req.QualityThreshold == 0 {
		req.QualityThreshold = 80
	}
	if req.CorrectionStrategy == "" {
		req.CorrectionStrategy = "comprehensive"
	}

	resp := &AutoCorrectionResponse{
		CorrectionID:     correctionID,
		OriginalContent:  req.Content,
		FinalContent:     req.Content,
		Iterations:       make([]CorrectionIteration, 0),
		TotalIterations:  0,
		Success:          false,
	}

	currentContent := req.Content

	// 修正-验证循环
	for iteration := 0; iteration <= req.MaxRetries; iteration++ {
		iterStart := time.Now()

		// 评估当前内容质量
		evalReq := &OutputEvaluationRequest{
			OriginalQuery: req.OriginalQuery,
			Output:        currentContent,
			Context:       req.Context,
			ThoughtID:     req.ThoughtID,
			UserID:        req.UserID,
			Model:         req.Model,
		}

		evalResp, err := s.EvaluateOutputQuality(ctx, evalReq)
		if err != nil {
			resp.Error = fmt.Sprintf("评估失败(迭代%d): %v", iteration, err)
			return resp, err
		}

		// 创建迭代记录
		iterRecord := CorrectionIteration{
			IterationNumber: iteration,
			InputContent:    currentContent,
			OutputContent:   currentContent,
			QualityScore:    evalResp.QualityScore,
			IssuesDetected:  evalResp.IssuesDetected,
			StartTime:       iterStart,
			EndTime:         time.Now(),
		}

		// 检查是否达到质量阈值
		if evalResp.QualityScore >= req.QualityThreshold && len(evalResp.IssuesDetected) == 0 {
			resp.FinalContent = currentContent
			resp.FinalQualityScore = evalResp.QualityScore
			resp.TotalIterations = iteration
			resp.Success = true
			resp.Iterations = append(resp.Iterations, iterRecord)
			resp.CorrectionSummary = fmt.Sprintf("在%d次迭代后达到质量阈值%.1f", iteration, req.QualityThreshold)
			break
		}

		// 如果是最后一次迭代且未达到阈值，不再尝试修正
		if iteration == req.MaxRetries {
			resp.FinalContent = currentContent
			resp.FinalQualityScore = evalResp.QualityScore
			resp.TotalIterations = iteration
			resp.Iterations = append(resp.Iterations, iterRecord)
			resp.CorrectionSummary = fmt.Sprintf("经过%d次迭代，最终质量分数%.1f未达到阈值%.1f", iteration, evalResp.QualityScore, req.QualityThreshold)
			break
		}

		// 生成修正建议
		correctionReq := &CorrectionSuggestionRequest{
			OriginalContent:    currentContent,
			Issues:             evalResp.IssuesDetected,
			ThoughtID:          req.ThoughtID,
			UserID:             req.UserID,
			CorrectionStrategy: req.CorrectionStrategy,
			Model:              req.Model,
		}

		correctionResp, err := s.GenerateCorrectionSuggestion(ctx, correctionReq)
		if err != nil {
			resp.Error = fmt.Sprintf("修正建议生成失败(迭代%d): %v", iteration, err)
			return resp, err
		}

		// 更新内容
		currentContent = correctionResp.CorrectedContent
		iterRecord.OutputContent = currentContent
		iterRecord.ChangesApplied = correctionResp.ChangesApplied
		iterRecord.EndTime = time.Now()

		resp.Iterations = append(resp.Iterations, iterRecord)
	}

	// 保存最终修正记录
	if req.ThoughtID != "" {
		finalCorrection := &models.Correction{
			ID:        uuid.New().String(),
			ThoughtID: req.ThoughtID,
			Content:   resp.FinalContent,
			UserID:    req.UserID,
			Status:    "resolved",
		}
		if err := s.CreateCorrection(finalCorrection); err != nil {
			return resp, fmt.Errorf("保存最终修正记录失败: %w", err)
		}
	}

	return resp, nil
}

// GetCorrectionHistory 获取纠错历史
func (s *CorrectionService) GetCorrectionHistory(ctx context.Context, req *CorrectionHistoryRequest) (*CorrectionHistoryResponse, error) {
	resp := &CorrectionHistoryResponse{
		HistoryItems: make([]CorrectionHistoryItem, 0),
	}

	query := s.db.Model(&models.Correction{})

	// 应用过滤条件
	if req.ThoughtID != "" {
		query = query.Where("thought_id = ?", req.ThoughtID)
	}
	if req.UserID != "" {
		query = query.Where("user_id = ?", req.UserID)
	}
	if !req.StartTime.IsZero() {
		query = query.Where("created_at >= ?", req.StartTime)
	}
	if !req.EndTime.IsZero() {
		query = query.Where("created_at <= ?", req.EndTime)
	}

	// 获取总数
	var totalCount int64
	query.Count(&totalCount)
	resp.TotalCount = totalCount

	// 分页
	if req.Limit == 0 {
		req.Limit = 50
	}
	query = query.Offset(req.Offset).Limit(req.Limit).Order("created_at DESC")

	// 查询修正记录
	var corrections []models.Correction
	if err := query.Find(&corrections).Error; err != nil {
		return resp, err
	}

	// 构建历史项
	for _, correction := range corrections {
		item := CorrectionHistoryItem{
			Correction: correction,
			IssueCount: 0,
		}

		// 获取关联的思考内容
		if correction.ThoughtID != "" {
			thought, err := s.GetThought(correction.ThoughtID)
			if err == nil {
				item.ThoughtContent = thought.Content
			}
		}

		resp.HistoryItems = append(resp.HistoryItems, item)
	}

	// 生成摘要统计
	resp.Summary = s.generateHistorySummary(resp.HistoryItems)

	return resp, nil
}

// ============ 提示词构建方法 ============

// buildEvaluationPrompt 构建评估提示词
func (s *CorrectionService) buildEvaluationPrompt(req *OutputEvaluationRequest) string {
	return fmt.Sprintf(`请评估以下输出的质量，并给出详细的分析。

原始查询:
%s

输出内容:
%s

%s

请按以下JSON格式返回评估结果:
{
  "quality_score": 质量分数(0-100),
  "confidence": 置信度(0-1),
  "issues": [
    {
      "issue_type": "问题类型(factual/logical/linguistic/safety/formatting)",
      "severity": "严重程度(critical/high/medium/low)",
      "description": "问题描述",
      "location": "位置(可选)",
      "suggested_correction": "建议修正(可选)",
      "confidence": 置信度(0-1)
    }
  ],
  "details": {
    "factual_accuracy": 事实准确性分数(0-1),
    "logical_coherence": 逻辑连贯性分数(0-1),
    "linguistic_quality": 语言质量分数(0-1),
    "relevance": 相关性分数(0-1),
    "completeness": 完整性分数(0-1),
    "safety": 安全性分数(0-1),
    "formatting": 格式规范性分数(0-1)
  }
}`,
		req.OriginalQuery,
		req.Output,
		s.buildContextSection(req.Context))
}

// buildDetectionPrompt 构建检测提示词
func (s *CorrectionService) buildDetectionPrompt(req *ErrorDetectionRequest) string {
	detectionType := req.DetectionType
	if detectionType == "" {
		detectionType = "all"
	}

	strictness := req.Strictness
	strictnessDesc := "中等"
	if strictness <= 2 {
		strictnessDesc = "宽松"
	} else if strictness >= 4 {
		strictnessDesc = "严格"
	}

	return fmt.Sprintf(`请检测以下内容中的错误（严格程度：%s，检测类型：%s）。

内容:
%s

%s

请按以下JSON格式返回检测结果:
{
  "issues": [
    {
      "issue_type": "问题类型",
      "severity": "严重程度",
      "description": "详细描述",
      "location": "具体位置(如段落/句子)",
      "suggested_correction": "修正建议",
      "confidence": 置信度(0-1)
    }
  ]
}

仅报告真正需要修正的问题，避免误报。`,
		strictnessDesc,
		detectionType,
		req.Content,
		s.buildDetectionContext(req))
}

// buildCorrectionPrompt 构建修正提示词
func (s *CorrectionService) buildCorrectionPrompt(req *CorrectionSuggestionRequest) string {
	var issuesDesc strings.Builder
	issuesDesc.WriteString(fmt.Sprintf("发现 %d 个问题需要修正:\n\n", len(req.Issues)))
	for i, issue := range req.Issues {
		issuesDesc.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, issue.IssueType, issue.Description))
		if issue.SuggestedCorrection != "" {
			issuesDesc.WriteString(fmt.Sprintf("   建议: %s\n", issue.SuggestedCorrection))
		}
	}

	strategyDesc := "全面修正"
	switch req.CorrectionStrategy {
	case "minimal":
		strategyDesc = "最小化修改，仅修正严重错误"
	case "interactive":
		strategyDesc = "交互式修正，保留多处选项"
	case "comprehensive":
		strategyDesc = "全面修正，优化整体质量"
	}

	return fmt.Sprintf(`请根据以下问题对内容进行修正（策略：%s）。

原始内容:
%s

%s

请按以下JSON格式返回修正结果:
{
  "corrected_content": "修正后的完整内容",
  "changes": [
    {
      "change_type": "修改类型(addition/deletion/replacement)",
      "original": "原始文本",
      "replacement": "替换文本",
      "position": "位置描述",
      "reasoning": "修改理由"
    }
  ],
  "summary": "修正摘要",
  "estimated_improvement": 预估改进分数(0-100)
}`,
		strategyDesc,
		req.OriginalContent,
		issuesDesc.String())
}

// buildDetectionContext 构建检测上下文
func (s *CorrectionService) buildDetectionContext(req *ErrorDetectionRequest) string {
	var parts []string

	if req.OriginalQuery != "" {
		parts = append(parts, fmt.Sprintf("原始查询: %s", req.OriginalQuery))
	}
	if req.Context != "" {
		parts = append(parts, fmt.Sprintf("上下文: %s", req.Context))
	}

	if len(parts) > 0 {
		return "\n" + strings.Join(parts, "\n")
	}
	return ""
}

// buildContextSection 构建上下文部分
func (s *CorrectionService) buildContextSection(context string) string {
	if context == "" {
		return ""
	}
	return fmt.Sprintf("上下文信息:\n%s", context)
}

// ============ 解析方法 ============

// parseEvaluationResult 解析评估结果
func (s *CorrectionService) parseEvaluationResult(resp string) (*evaluationResult, error) {
	// 尝试提取JSON
	jsonStr := s.extractJSON(resp)
	if jsonStr == "" {
		jsonStr = resp
	}

	var result evaluationResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %w", err)
	}

	return &result, nil
}

// parseDetectionResult 解析检测结果
func (s *CorrectionService) parseDetectionResult(resp string) (*detectionResult, error) {
	jsonStr := s.extractJSON(resp)
	if jsonStr == "" {
		jsonStr = resp
	}

	var result detectionResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %w", err)
	}

	return &result, nil
}

// parseCorrectionResult 解析修正结果
func (s *CorrectionService) parseCorrectionResult(resp string) (*correctionResult, error) {
	jsonStr := s.extractJSON(resp)
	if jsonStr == "" {
		jsonStr = resp
	}

	var result correctionResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %w", err)
	}

	return &result, nil
}

// extractJSON 从响应中提取JSON
func (s *CorrectionService) extractJSON(resp string) string {
	// 查找JSON开始和结束
	start := strings.Index(resp, "{")
	if start == -1 {
		start = strings.Index(resp, "[")
	}
	if start == -1 {
		return ""
	}

	// 计算括号匹配
	depth := 0
	inString := false
	escape := false

	for i := start; i < len(resp); i++ {
		c := resp[i]

		if escape {
			escape = false
			continue
		}

		if c == '\\' {
			escape = true
			continue
		}

		if c == '"' {
			inString = !inString
			continue
		}

		if !inString {
			if c == '{' || c == '[' {
				depth++
			} else if c == '}' || c == ']' {
				depth--
				if depth == 0 {
					return resp[start : i+1]
				}
			}
		}
	}

	return resp[start:]
}

// ============ 辅助方法 ============

// countCriticalIssues 统计严重问题
func (s *CorrectionService) countCriticalIssues(issues []Issue) int {
	count := 0
	for _, issue := range issues {
		if issue.Severity == "critical" {
			count++
		}
	}
	return count
}

// generateDetectionSummary 生成检测摘要
func (s *CorrectionService) generateDetectionSummary(issues []Issue) string {
	if len(issues) == 0 {
		return "未检测到问题"
	}

	byType := make(map[string]int)
	bySeverity := make(map[string]int)

	for _, issue := range issues {
		byType[issue.IssueType]++
		bySeverity[issue.Severity]++
	}

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("检测到 %d 个问题。", len(issues)))

	if len(bySeverity) > 0 {
		summary.WriteString(" 按严重程度: ")
		var parts []string
		for severity, count := range bySeverity {
			parts = append(parts, fmt.Sprintf("%s:%d", severity, count))
		}
		summary.WriteString(strings.Join(parts, ", "))
	}

	if len(byType) > 0 {
		summary.WriteString(" 按类型: ")
		var parts []string
		for issueType, count := range byType {
			parts = append(parts, fmt.Sprintf("%s:%d", issueType, count))
		}
		summary.WriteString(strings.Join(parts, ", "))
	}

	return summary.String()
}

// generateHistorySummary 生成历史摘要
func (s *CorrectionService) generateHistorySummary(items []CorrectionHistoryItem) *HistorySummary {
	summary := &HistorySummary{
		TotalCorrections:       int64(len(items)),
		MostCommonIssueTypes:   make([]string, 0),
	}

	resolved := 0
	pending := 0
	totalQuality := 0.0
	qualityCount := 0
	typeCount := make(map[string]int)

	for _, item := range items {
		switch item.Correction.Status {
		case "resolved":
			resolved++
		case "pending":
			pending++
		}

		if item.QualityMetrics != nil {
			totalQuality += item.QualityMetrics.FactualAccuracy
			qualityCount++
		}

		typeCount[strings.ToLower(item.Correction.Status)]++
	}

	summary.ResolvedCount = int64(resolved)
	summary.PendingCount = int64(pending)

	if qualityCount > 0 {
		summary.AverageQualityScore = totalQuality / float64(qualityCount)
	}

	// 找出最常见的问题类型
	maxCount := 0
	for issueType, count := range typeCount {
		if count > maxCount {
			maxCount = count
			summary.MostCommonIssueTypes = []string{issueType}
		} else if count == maxCount {
			summary.MostCommonIssueTypes = append(summary.MostCommonIssueTypes, issueType)
		}
	}

	return summary
}

// saveEvaluationRecord 保存评估记录
func (s *CorrectionService) saveEvaluationRecord(req *OutputEvaluationRequest, resp *OutputEvaluationResponse) error {
	// 创建修正记录来存储评估结果
	correction := &models.Correction{
		ID:        uuid.New().String(),
		ThoughtID: req.ThoughtID,
		Content:   fmt.Sprintf("质量评估: %.1f分, 检测到%d个问题", resp.QualityScore, len(resp.IssuesDetected)),
		UserID:    req.UserID,
		Status:    "pending",
	}

	return s.CreateCorrection(correction)
}

// callLLM 调用LLM
func (s *CorrectionService) callLLM(ctx context.Context, prompt, model string) (string, error) {
	if s.llmClient == nil {
		return "", fmt.Errorf("LLM客户端未初始化")
	}

	request := &llm.ChatRequest{
		Messages: []llm.Message{
			{
				Role:    "system",
				Content: "你是一个专业的质量评估和内容修正专家。请严格按照JSON格式返回结果，确保可以正确解析。",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Model:       model,
		Temperature: 0.3,
		MaxTokens:   3000,
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

// ============ 与ThinkingService集成的方法 ============

// GetThought 获取思考（复用ThinkingService的方法）
func (s *CorrectionService) GetThought(id string) (*models.Thought, error) {
	var thought models.Thought
	err := s.db.Where("id = ?", id).First(&thought).Error
	return &thought, err
}

// CreateCorrection 创建修正
func (s *CorrectionService) CreateCorrection(correction *models.Correction) error {
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
func (s *CorrectionService) UpdateCorrection(correction *models.Correction) error {
	correction.UpdatedAt = time.Now()
	return s.db.Save(correction).Error
}

// DeleteCorrection 删除修正
func (s *CorrectionService) DeleteCorrection(id string) error {
	return s.db.Delete(&models.Correction{}, "id = ?", id).Error
}

// GetCorrection 获取修正
func (s *CorrectionService) GetCorrection(id string) (*models.Correction, error) {
	var correction models.Correction
	err := s.db.Where("id = ?", id).First(&correction).Error
	return &correction, err
}

// ListCorrections 列出思考的所有修正
func (s *CorrectionService) ListCorrections(thoughtID string) ([]models.Correction, error) {
	var corrections []models.Correction
	err := s.db.Where("thought_id = ?", thoughtID).Find(&corrections).Error
	return corrections, err
}

// ResolveCorrection 解决修正
func (s *CorrectionService) ResolveCorrection(id string) (*models.Correction, error) {
	correction, err := s.GetCorrection(id)
	if err != nil {
		return nil, err
	}

	correction.Status = "resolved"
	correction.UpdatedAt = time.Now()
	s.db.Save(correction)

	return correction, nil
}

// ============ 内部数据结构 ============

// evaluationResult 评估结果
type evaluationResult struct {
	QualityScore float64             `json:"quality_score"`
	Confidence   float64             `json:"confidence"`
	Issues       []Issue             `json:"issues"`
	Details      *EvaluationDetails  `json:"details"`
}

// detectionResult 检测结果
type detectionResult struct {
	Issues []Issue `json:"issues"`
}

// correctionResult 修正结果
type correctionResult struct {
	CorrectedContent      string  `json:"corrected_content"`
	Changes               []Change `json:"changes"`
	Summary               string  `json:"summary"`
	EstimatedImprovement  float64 `json:"estimated_improvement"`
}

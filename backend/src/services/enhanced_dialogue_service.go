package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// GenerateUUID 生成 UUID
func GenerateUUID() string {
	return uuid.New().String()
}

// EnhancedDialogueService 增强对话服务 - 在 DialogueService 之上添加 prompt 组装和后置钩子
type EnhancedDialogueService struct {
	dialogueSvc      *DialogueService
	modelSvc         *ModelService
	cacheSvc         *CacheService
	loggerSvc        *LoggerService
	toolCallingSvc   *ToolCallingService
	router           *ModelRouter
	planSvc          *PlanService
	skillSvc         *SkillService
	eventBus         *EventBus
	promptSvc        *PromptService
	postHookSvc      *PostHookService
	localKnowledge   *LocalKnowledgeFirst
	orchestrator     *RequestOrchestrator
	memoryExtractSvc *MemoryExtractionService
}

// NewEnhancedDialogueService 创建增强对话服务
func NewEnhancedDialogueService(
	dialogueSvc *DialogueService,
	modelSvc *ModelService,
	cacheSvc *CacheService,
	loggerSvc *LoggerService,
	toolCallingSvc *ToolCallingService,
	router *ModelRouter,
	planSvc *PlanService,
	skillSvc *SkillService,
	eventBus *EventBus,
	promptSvc *PromptService,
	postHookSvc *PostHookService,
) *EnhancedDialogueService {
	return &EnhancedDialogueService{
		dialogueSvc:    dialogueSvc,
		modelSvc:       modelSvc,
		cacheSvc:       cacheSvc,
		loggerSvc:      loggerSvc,
		toolCallingSvc: toolCallingSvc,
		router:         router,
		planSvc:        planSvc,
		skillSvc:       skillSvc,
		eventBus:       eventBus,
		promptSvc:      promptSvc,
		postHookSvc:    postHookSvc,
		orchestrator:   NewRequestOrchestrator(router, skillSvc, planSvc, toolCallingSvc, eventBus),
	}
}

func (s *EnhancedDialogueService) SetLocalKnowledge(lk *LocalKnowledgeFirst) {
	s.localKnowledge = lk
}

func (s *EnhancedDialogueService) SetMemoryExtractionService(svc *MemoryExtractionService) {
	s.memoryExtractSvc = svc
}

// ComposeSystemPrompt 组装 system prompt（委托给 PromptService）
func (s *EnhancedDialogueService) ComposeSystemPrompt(ctx context.Context, userID, dialogueID, query string, options map[string]interface{}) string {
	return s.promptSvc.Compose(ctx, userID, dialogueID, query, options)
}

// SendMessageStreamEnhanced 增强版流式消息发送
func (s *EnhancedDialogueService) SendMessageStreamEnhanced(
	ctx context.Context,
	dialogueID, userID, content, modelID string,
	options map[string]interface{},
) (<-chan llm.ChatStreamChunk, error) {
	if s.eventBus != nil {
		s.eventBus.Publish(ctx, models.EventTopicMessage, models.EventTypeMessageReceived, "dialogue", map[string]interface{}{
			"dialogue_id": dialogueID,
			"user_id":     userID,
			"content":     content,
		})
	}

	if s.localKnowledge != nil && s.localKnowledge.ShouldTryLocal(content) {
		localResult, err := s.localKnowledge.Query(ctx, content, 3)
		if err == nil && localResult != nil && localResult.FromLocal {
			slog.Info("local knowledge hit: score=, saved_tokens=", "component", "EnhancedDialogue", "score", localResult.Score, "score", localResult.SavedTokens)
			if s.eventBus != nil {
				s.eventBus.Publish(ctx, "knowledge", "local_hit", "local_knowledge", map[string]interface{}{
					"query":        content,
					"score":        localResult.Score,
					"saved_tokens": localResult.SavedTokens,
					"sources":      len(localResult.Sources),
				})
			}
			return s.localKnowledge.ToStreamChunks(localResult.Answer), nil
		}

		if localResult != nil && !localResult.FromLocal && localResult.Score >= LocalKnowledgeMediumThreshold {
			if options == nil {
				options = make(map[string]interface{})
			}
			options["local_knowledge_context"] = localResult.Answer
			slog.Info("local knowledge partial match: score=, injecting as context", "component", "EnhancedDialogue", "score", localResult.Score)
		}
	}

	composedPrompt := s.ComposeSystemPrompt(ctx, userID, dialogueID, content, options)
	if options == nil {
		options = make(map[string]interface{})
	}
	options["system"] = composedPrompt

	chunkChan, err := s.dialogueSvc.SendMessageStream(ctx, dialogueID, userID, content, modelID, options)
	if err != nil {
		return nil, err
	}

	return s.postHookSvc.WrapStream(chunkChan, dialogueID, userID, content), nil
}

// OnResponseComplete 响应完成后的后置处理（委托给 PostHookService）
func (s *EnhancedDialogueService) OnResponseComplete(ctx context.Context, dialogueID, userID, content string) {
	s.postHookSvc.OnResponseCompleteLegacy(ctx, dialogueID, userID, content)

	if s.memoryExtractSvc != nil && dialogueID != "" && userID != "" {
		go func() {
			extractCtx := context.Background()
			if err := s.memoryExtractSvc.ExtractMemoriesFromDialogue(extractCtx, dialogueID, userID); err != nil {
				slog.Error("Auto memory extraction failed", "component", "EnhancedDialogue", "error", err)
			}
		}()
	}
}

// GetDialogue 获取对话（代理方法）
func (s *EnhancedDialogueService) GetDialogue(id string) (models.Dialogue, bool) {
	return s.dialogueSvc.GetDialogue(id)
}

// CreateDialogue 创建对话（代理方法）
func (s *EnhancedDialogueService) CreateDialogue(userID, title string) models.Dialogue {
	return s.dialogueSvc.CreateDialogue(userID, title)
}

// GetMessages 获取消息列表（代理方法）
func (s *EnhancedDialogueService) GetMessages(dialogueID string) []models.Message {
	return s.dialogueSvc.GetMessages(dialogueID)
}

// SaveStreamMessage 保存流式消息（代理方法）
func (s *EnhancedDialogueService) SaveStreamMessage(dialogueID, content string, reasoningContent ...string) (models.Message, error) {
	return s.dialogueSvc.SaveStreamMessage(dialogueID, content, reasoningContent...)
}

// SendMessageStream 流式消息发送（自动路由+工具检测）
func (s *EnhancedDialogueService) SendMessageStream(
	ctx context.Context, dialogueID, userID, content, modelID string,
	options map[string]interface{},
) (<-chan llm.ChatStreamChunk, error) {
	return s.SendMessageStreamRouted(ctx, dialogueID, userID, content, modelID, options)
}

// SendMessage 非流式消息发送（始终传工具定义，LLM自主决策）
func (s *EnhancedDialogueService) SendMessage(
	ctx context.Context, dialogueID, userID, content, modelID string,
	options map[string]interface{},
) (*models.Message, error) {
	if s.toolCallingSvc != nil {
		return s.SendMessageWithTools(ctx, dialogueID, userID, content, modelID, options)
	}
	return s.dialogueSvc.SendMessage(ctx, dialogueID, userID, content, modelID, options)
}

// AddMessage 添加消息（代理方法）
func (s *EnhancedDialogueService) AddMessage(dialogueID, sender, content string) error {
	_, err := s.dialogueSvc.AddMessage(dialogueID, sender, content)
	return err
}

// SendMessageWithTools 带工具调用的增强消息发送
func (s *EnhancedDialogueService) SendMessageWithTools(
	ctx context.Context,
	dialogueID, userID, content, modelID string,
	options map[string]interface{},
) (*models.Message, error) {
	if s.toolCallingSvc == nil {
		return nil, fmt.Errorf("tool calling service not available")
	}

	// 发布消息接收事件
	if s.eventBus != nil {
		s.eventBus.Publish(ctx, models.EventTopicMessage, models.EventTypeMessageReceived, "dialogue", map[string]interface{}{
			"dialogue_id": dialogueID,
			"user_id":     userID,
			"content":     content,
		})
	}

	// 保存用户消息
	if _, err := s.dialogueSvc.AddMessage(dialogueID, "user", content); err != nil {
		if s.loggerSvc != nil {
			s.loggerSvc.Error(ctx, "Failed to save user message: %v", err)
		}
	}

	if options == nil {
		options = make(map[string]interface{})
	}
	options["has_tools"] = true

	composedPrompt := s.ComposeSystemPrompt(ctx, userID, dialogueID, content, options)
	options["system"] = composedPrompt

	// 技能工具过滤：将 skill_tools 转为 tool_filter 传给 ToolCallingService
	if toolsRaw, ok := options["skill_tools"]; ok {
		if names := toStringSlice(toolsRaw); len(names) > 0 {
			options["tool_filter"] = names
		}
	}

	// 执行工具调用循环
	msg, err := s.toolCallingSvc.SendMessageWithTools(ctx, dialogueID, userID, content, modelID, options)
	if err != nil {
		return nil, err
	}

	// 发布消息发送事件
	if s.eventBus != nil && msg != nil {
		s.eventBus.Publish(ctx, models.EventTopicMessage, models.EventTypeMessageSent, "dialogue", map[string]interface{}{
			"dialogue_id": dialogueID,
			"content":     msg.Content,
		})
	}

	// 触发后置钩子
	go s.OnResponseComplete(context.Background(), dialogueID, userID, msg.Content)

	return msg, nil
}

// SendMessageStreamRouted 自动路由的流式消息发送（modelID 可为空，由 router 自动选择）
func (s *EnhancedDialogueService) SendMessageStreamRouted(
	ctx context.Context,
	dialogueID, userID, content, modelID string,
	options map[string]interface{},
) (<-chan llm.ChatStreamChunk, error) {
	decision := s.orchestrator.Route(ctx, content, modelID, options)

	if options == nil {
		options = make(map[string]interface{})
	}

	if decision.SkillMatch != nil {
		s.applySkillContext(ctx, decision.SkillMatch, content, userID, options)
	}

	modelID = decision.ModelID

	if s.eventBus != nil && s.router != nil {
		s.eventBus.Publish(ctx, models.EventTopicModel, models.EventTypeModelRouted, "orchestrator", map[string]interface{}{
			"content":    content,
			"intent":     string(decision.Intent),
			"model_id":   modelID,
			"confidence": decision.Confidence,
			"reason":     decision.Reason,
		})
	}

	if decision.NeedsPlan && s.planSvc != nil {
		slog.Info("Using planning path", "component", "EnhancedDialogue", "reason", decision.Reason)
		planResult, err := s.planSvc.ChatWithPlan(ctx, content, modelID, userID, dialogueID, options)
		if err != nil {
			slog.Error("Planning failed, falling back to tool calling", "component", "EnhancedDialogue", "error", err)
		} else if planResult != nil {
			ch := make(chan llm.ChatStreamChunk, 1)
			go func() {
				defer close(ch)
				ch <- llm.ChatStreamChunk{
					Choices: []llm.StreamChoice{{
						Delta: llm.MessageDelta{Content: planResult.Result},
					}},
				}
			}()
			go s.OnResponseComplete(context.Background(), dialogueID, userID, content)
			return ch, nil
		}
	}

	if decision.NeedsTools && s.toolCallingSvc != nil {
		slog.Info("Using tool-calling path", "component", "EnhancedDialogue", "reason", decision.Reason)
		return s.SendMessageWithToolsStream(ctx, dialogueID, userID, content, modelID, options)
	}

	if decision.NeedsRAG {
		options["force_rag"] = true
	}

	slog.Info("Using enhanced chat path", "component", "EnhancedDialogue", "reason", decision.Reason)
	return s.SendMessageStreamEnhanced(ctx, dialogueID, userID, content, modelID, options)
}

func (s *EnhancedDialogueService) applySkillContext(ctx context.Context, match *SkillMatchResult, content, userID string, options map[string]interface{}) {
	skillContext := map[string]interface{}{
		"skill_name": match.Skill.Name,
	}
	if match.Skill.SystemPromptOverride != "" {
		skillContext["system_prompt"] = match.Skill.SystemPromptOverride
	}

	finalParams := map[string]interface{}{
		"content": content,
	}
	if userID != "" {
		finalParams["user_id"] = userID
	}

	defs, err := s.skillSvc.GetSkillParameters(match.Skill.ID)
	if err != nil {
		slog.Error("load skill parameters failed", "component", "EnhancedDialogue", "error", err)
	} else if len(defs) > 0 {
		extracted, err := s.skillSvc.ExtractParametersFromContent(ctx, match.Skill, defs, content)
		if err != nil {
			slog.Error("parameter extraction failed, fallback to prompt-only skill", "component", "EnhancedDialogue", "error", err)
		} else {
			for key, value := range extracted {
				if _, exists := finalParams[key]; !exists {
					finalParams[key] = value
				}
			}
			normalized, err := normalizeParameters(defs, finalParams)
			if err != nil {
				slog.Error("parameter normalization failed, fallback to prompt-only skill", "component", "EnhancedDialogue", "error", err)
			} else {
				finalParams = normalized
				skillContext["parameters"] = filterDeclaredParameters(defs, finalParams)
			}
		}
	}
	options["skill_context"] = skillContext

	if s.eventBus != nil {
		matchedPayload := map[string]interface{}{
			"skill_name": match.Skill.Name,
			"trigger":    match.MatchedTrigger,
			"confidence": match.Confidence,
		}
		if params, ok := skillContext["parameters"].(map[string]interface{}); ok && len(params) > 0 {
			matchedPayload["parameters"] = params
		}
		s.eventBus.Publish(ctx, models.EventTopicSkill, models.EventTypeSkillMatched, "skill_service", matchedPayload)
	}

	if len(match.Skill.Tools) > 0 {
		options["skill_tools"] = []string(match.Skill.Tools)
	}

	go func(skillID, skillName string, parameters map[string]interface{}) {
		s.skillSvc.TrackSkillExecution(skillID, skillName, parameters, "completed")
	}(match.Skill.ID, match.Skill.Name, finalParams)

	if s.eventBus != nil {
		executedPayload := map[string]interface{}{
			"skill_id":   match.Skill.ID,
			"skill_name": match.Skill.Name,
			"trigger":    match.MatchedTrigger,
		}
		if params, ok := skillContext["parameters"].(map[string]interface{}); ok && len(params) > 0 {
			executedPayload["parameters"] = params
		}
		s.eventBus.Publish(ctx, models.EventTopicSkill, models.EventTypeSkillExecuted, "skill_service", executedPayload)
	}
}

func (s *EnhancedDialogueService) SendMessageWithToolsStream(
	ctx context.Context,
	dialogueID, userID, content, modelID string,
	options map[string]interface{},
) (<-chan llm.ChatStreamChunk, error) {
	if s.toolCallingSvc == nil {
		return s.SendMessageStreamEnhanced(ctx, dialogueID, userID, content, modelID, options)
	}

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, models.EventTopicMessage, models.EventTypeMessageReceived, "dialogue", map[string]interface{}{
			"dialogue_id": dialogueID,
			"user_id":     userID,
			"content":     content,
		})
	}

	if _, err := s.dialogueSvc.AddMessage(dialogueID, "user", content); err != nil {
		if s.loggerSvc != nil {
			s.loggerSvc.Error(ctx, "Failed to save user message: %v", err)
		}
	}

	if options == nil {
		options = make(map[string]interface{})
	}
	options["has_tools"] = true

	composedPrompt := s.ComposeSystemPrompt(ctx, userID, dialogueID, content, options)
	options["system"] = composedPrompt

	if toolsRaw, ok := options["skill_tools"]; ok {
		if names := toStringSlice(toolsRaw); len(names) > 0 {
			options["tool_filter"] = names
		}
	}

	chunkChan, err := s.toolCallingSvc.SendMessageWithToolsStream(ctx, dialogueID, userID, content, modelID, options)
	if err != nil {
		return nil, err
	}

	go s.OnResponseComplete(context.Background(), dialogueID, userID, content)

	return chunkChan, nil
}

// SendMessageWithPlan 带规划的聊天（自动判断是否需要规划）
func (s *EnhancedDialogueService) SendMessageWithPlan(
	ctx context.Context,
	dialogueID, userID, content, modelID string,
	options map[string]interface{},
) (*PlanResult, error) {
	if s.planSvc == nil {
		return nil, fmt.Errorf("plan service not available")
	}

	return s.planSvc.ChatWithPlan(ctx, content, modelID, userID, dialogueID, options)
}

// ExecutePendingPlan 执行待确认的计划
func (s *EnhancedDialogueService) ExecutePendingPlan(
	ctx context.Context,
	sessionID string,
) (*PlanResult, error) {
	if s.planSvc == nil {
		return nil, fmt.Errorf("plan service not available")
	}

	return s.planSvc.ExecutePlan(ctx, sessionID)
}

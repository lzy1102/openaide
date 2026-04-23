package services

import (
	"context"
	"fmt"
	"log"

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
	dialogueSvc     *DialogueService
	modelSvc        *ModelService
	cacheSvc        *CacheService
	loggerSvc       *LoggerService
	toolCallingSvc  *ToolCallingService
	router          *ModelRouter
	planSvc         *PlanService
	skillSvc        *SkillService
	eventBus        *EventBus
	promptSvc       *PromptService
	postHookSvc     *PostHookService
	localKnowledge  *LocalKnowledgeFirst
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
	}
}

func (s *EnhancedDialogueService) SetLocalKnowledge(lk *LocalKnowledgeFirst) {
	s.localKnowledge = lk
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
			log.Printf("[EnhancedDialogue] local knowledge hit: score=%.2f, saved_tokens=%d", localResult.Score, localResult.SavedTokens)
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
			log.Printf("[EnhancedDialogue] local knowledge partial match: score=%.2f, injecting as context", localResult.Score)
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
func (s *EnhancedDialogueService) SaveStreamMessage(dialogueID, content string) models.Message {
	return s.dialogueSvc.SaveStreamMessage(dialogueID, content)
}

// SendMessageStream 流式消息发送（代理方法，供 FeishuService 等使用）
func (s *EnhancedDialogueService) SendMessageStream(
	ctx context.Context, dialogueID, userID, content, modelID string,
	options map[string]interface{},
) (<-chan llm.ChatStreamChunk, error) {
	return s.dialogueSvc.SendMessageStream(ctx, dialogueID, userID, content, modelID, options)
}

// AddMessage 添加消息（代理方法）
func (s *EnhancedDialogueService) AddMessage(dialogueID, sender, content string) {
	s.dialogueSvc.AddMessage(dialogueID, sender, content)
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
	s.dialogueSvc.AddMessage(dialogueID, "user", content)

	// 组装 system prompt
	composedPrompt := s.ComposeSystemPrompt(ctx, userID, dialogueID, content, options)
	if options == nil {
		options = make(map[string]interface{})
	}
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
	// modelID 为空时自动路由
	if (modelID == "") && s.router != nil {
		routed, err := s.router.Route(ctx, content, nil)
		if err != nil {
			log.Printf("[EnhancedDialogue] auto route failed: %v", err)
		} else {
			modelID = routed.ID
			log.Printf("[EnhancedDialogue] auto routed to model: %s (task matched)", routed.Name)
			if s.eventBus != nil {
				info := s.router.GetRouteInfo(ctx, content)
				s.eventBus.Publish(ctx, models.EventTopicModel, models.EventTypeModelRouted, "model_router", map[string]interface{}{
					"content":    content,
					"task_type":  info.TaskType,
					"model_name": routed.Name,
					"confidence": info.Confidence,
				})
			}
		}
	}

	if s.skillSvc != nil && s.skillSvc.NeedsSkillExecution(content) {
		match := s.skillSvc.MatchSkill(content)
		if match != nil {
			log.Printf("[EnhancedDialogue] skill matched: %s (confidence=%.2f)", match.Skill.Name, match.Confidence)

			if options == nil {
				options = make(map[string]interface{})
			}

			skillContext := map[string]interface{}{
				"skill_name": match.Skill.Name,
			}
			if match.Skill.SystemPromptOverride != "" {
				skillContext["system_prompt"] = match.Skill.SystemPromptOverride
				log.Printf("[EnhancedDialogue] skill %s: system prompt injected", match.Skill.Name)
			}

			finalParams := map[string]interface{}{
				"content": content,
			}
			if userID != "" {
				finalParams["user_id"] = userID
			}

			defs, err := s.skillSvc.GetSkillParameters(match.Skill.ID)
			if err != nil {
				log.Printf("[EnhancedDialogue] load skill parameters failed: %v", err)
			} else if len(defs) > 0 {
				extracted, err := s.skillSvc.ExtractParametersFromContent(ctx, match.Skill, defs, content)
				if err != nil {
					log.Printf("[EnhancedDialogue] parameter extraction failed, fallback to prompt-only skill: %v", err)
				} else {
					for key, value := range extracted {
						if _, exists := finalParams[key]; !exists {
							finalParams[key] = value
						}
					}
					normalized, err := normalizeParameters(defs, finalParams)
					if err != nil {
						log.Printf("[EnhancedDialogue] parameter normalization failed, fallback to prompt-only skill: %v", err)
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

			if match.Skill.ModelPreference != "" {
				skillModelID, err := s.skillSvc.ResolveModelID(ctx, match.Skill.ModelPreference)
				if err != nil {
					log.Printf("[EnhancedDialogue] skill model resolution failed: %v, using routed model", err)
				} else {
					modelID = skillModelID
					log.Printf("[EnhancedDialogue] skill %s: model overridden to preference %s", match.Skill.Name, match.Skill.ModelPreference)
				}
			}

			if len(match.Skill.Tools) > 0 {
				options["skill_tools"] = []string(match.Skill.Tools)
				log.Printf("[EnhancedDialogue] skill %s: %d tools bound", match.Skill.Name, len(match.Skill.Tools))
			}

			go func(skillID, skillName string, parameters map[string]interface{}) {
				s.skillSvc.TrackSkillExecution(skillID, skillName, parameters, "completed")
				log.Printf("[EnhancedDialogue] skill %s: execution tracked", skillName)
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
	}

	if s.toolCallingSvc != nil && s.needsToolExecution(content) {
		log.Printf("[EnhancedDialogue] tool execution needed, using tool-calling path")
		return s.SendMessageWithToolsStream(ctx, dialogueID, userID, content, modelID, options)
	}

	return s.SendMessageStreamEnhanced(ctx, dialogueID, userID, content, modelID, options)
}

func (s *EnhancedDialogueService) needsToolExecution(content string) bool {
	lower := strings.ToLower(content)
	toolIndicators := []string{
		"执行", "运行", "跑一下", "跑个", "curl", "wget", "ping ",
		"ls ", "cat ", "查看ip", "公网ip", "查ip", "ip地址",
		"执行命令", "运行命令", "跑命令", "shell", "终端",
		"docker", "git ", "npm ", "pip ", "go run",
		"python ", "node ", "java ",
		"读文件", "写文件", "创建文件", "删除文件",
		"查一下", "查询", "调用", "请求", "api",
		"format", "lint", "test", "build",
	}
	for _, indicator := range toolIndicators {
		if strings.Contains(lower, indicator) {
			return true
		}
	}
	return false
}

func (s *EnhancedDialogueService) SendMessageWithToolsStream(
	ctx context.Context,
	dialogueID, userID, content, modelID string,
	options map[string]interface{},
) (<-chan llm.ChatStreamChunk, error) {
	if s.toolCallingSvc == nil {
		return s.SendMessageStreamEnhanced(ctx, dialogueID, userID, content, modelID, options)
	}

	msg, err := s.toolCallingSvc.SendMessageWithTools(ctx, dialogueID, userID, content, modelID, options)
	if err != nil {
		return nil, err
	}

	go s.OnResponseComplete(context.Background(), dialogueID, userID, content)

	ch := make(chan llm.ChatStreamChunk, 1)
	go func() {
		defer close(ch)
		ch <- llm.ChatStreamChunk{
			Choices: []llm.StreamChoice{
				{
					Delta: llm.StreamDelta{
						Content: msg.Content,
						Role:    "assistant",
					},
				},
			},
		}
	}()
	return ch, nil
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

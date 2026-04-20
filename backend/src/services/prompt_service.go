package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"openaide/backend/src/models"
)

// PromptService 系统提示词组装服务
// 负责 5 层 system prompt 的组装：基础 prompt → 优化建议 → 用户偏好 → 记忆上下文 → RAG 上下文
type PromptService struct {
	BaseService
	memorySvc *MemoryService
	ragSvc    RAGService
}

// NewPromptService 创建提示词服务
func NewPromptService(db *gorm.DB, memorySvc *MemoryService, ragSvc RAGService) *PromptService {
	return &PromptService{
		BaseService: BaseService{DB: db},
		memorySvc:   memorySvc,
		ragSvc:      ragSvc,
	}
}

// Compose 组装 5 层 system prompt
func (s *PromptService) Compose(ctx context.Context, userID, dialogueID, query string, options map[string]interface{}) string {
	var parts []string

	// 1. 基础 prompt: options["system"] 或默认模板
	basePrompt := ""
	if v, ok := options["system"]; ok {
		if str, ok := v.(string); ok {
			basePrompt = str
		}
	}
	if basePrompt == "" {
		basePrompt = "你是一个有帮助的 AI 助手。"
	}
	parts = append(parts, basePrompt)

	// 1.5 技能层：如果 options 中有活跃技能，注入技能系统提示和结构化参数
	if skillCtx, ok := options["skill_context"]; ok {
		if skillMap, ok := skillCtx.(map[string]interface{}); ok {
			skillName, _ := skillMap["skill_name"].(string)
			if skillName == "" {
				skillName = "未知技能"
			}
			if prompt, ok := skillMap["system_prompt"].(string); ok && prompt != "" {
				parts = append(parts, fmt.Sprintf("【当前激活技能：%s】\n%s\n请在回复时严格遵循以上技能指令。", skillName, prompt))
			}
			if parameters, ok := skillMap["parameters"].(map[string]interface{}); ok && len(parameters) > 0 {
				if payload, err := json.MarshalIndent(parameters, "", "  "); err == nil {
					parts = append(parts, fmt.Sprintf("【技能参数：%s】\n以下 JSON 为已提取的结构化参数，请优先基于这些参数完成当前任务：\n%s", skillName, string(payload)))
				}
			}
		}
	}

	// 2. 已应用优化
	var optimizations []models.PromptOptimization
	if err := s.DB.Where("status = ?", "applied").
		Order("created_at DESC").
		Limit(5).
		Find(&optimizations).Error; err == nil && len(optimizations) > 0 {
		var optParts []string
		for _, opt := range optimizations {
			optParts = append(optParts, opt.Suggestions)
		}
		optBlock := "以下是已应用的优化建议，请在回复时参考：\n" + strings.Join(optParts, "\n---\n")
		parts = append(parts, optBlock)
	}

	// 3. 用户偏好
	var preferences []models.UserPreference
	if userID != "" {
		if err := s.DB.Where("user_id = ?", userID).Find(&preferences).Error; err == nil && len(preferences) > 0 {
			var prefInstructions []string
			for _, pref := range preferences {
				if pref.Value == nil {
					continue
				}
				val := fmt.Sprintf("%v", pref.Value.Data)
				switch pref.Key {
				case "response_style":
					prefInstructions = append(prefInstructions, fmt.Sprintf("请以%s的风格回复", val))
				case "preferred_language":
					prefInstructions = append(prefInstructions, fmt.Sprintf("请使用%s回复", val))
				case "technical_depth":
					prefInstructions = append(prefInstructions, fmt.Sprintf("请使用%s的技术深度", val))
				case "topics_of_interest":
					prefInstructions = append(prefInstructions, fmt.Sprintf("用户关注的领域：%s", val))
				}
			}
			if len(prefInstructions) > 0 {
				parts = append(parts, "用户偏好：\n"+strings.Join(prefInstructions, "\n"))
			}
		}
	}

	// 4. 记忆上下文
	if userID != "" && s.memorySvc != nil {
		memoryCtx := s.memorySvc.BuildMemoryContext(userID, query, 1000)
		if memoryCtx != "" {
			parts = append(parts, "用户相关信息（请在回复时参考）：\n"+memoryCtx)
		}
	}

	// 5. RAG 上下文
	if query != "" && s.ragSvc != nil {
		ragCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		results, err := s.ragSvc.Retrieve(ragCtx, query, 3)
		if err == nil && len(results) > 0 {
			contextStr := s.ragSvc.BuildContext(results, 2000)
			if contextStr != "" {
				parts = append(parts, "相关知识库内容（请在回复时参考）：\n"+contextStr)
			}
		}
	}

	// 6. 思维链 (Chain of Thought) - 让模型先思考再回答
	if !s.shouldSkipThinking(options) {
		thinkingTemplate := s.buildThinkingTemplate(userID, query, options)
		if thinkingTemplate != "" {
			parts = append(parts, thinkingTemplate)
		}
	}

	return strings.Join(parts, "\n\n")
}

// shouldSkipThinking 判断是否跳过思考过程
func (s *PromptService) shouldSkipThinking(options map[string]interface{}) bool {
	if options == nil {
		return false
	}
	if v, ok := options["skip_thinking"]; ok {
		if b, ok := v.(bool); ok && b {
			return true
		}
	}
	return false
}

// buildThinkingTemplate 构建思维链思考模板
func (s *PromptService) buildThinkingTemplate(userID, query string, options map[string]interface{}) string {
	hasTools := false
	if options != nil {
		if _, ok := options["skill_tools"]; ok {
			hasTools = true
		}
		if _, ok := options["tool_filter"]; ok {
			hasTools = true
		}
	}

	if hasTools {
		return `【思维链 - 请先进行结构化思考，再用 <thinking>...</thinking> 标签输出思考过程】
在回复前，请按以下步骤思考：
1. **理解意图** - 用户的真正需求是什么？核心问题是什么？
2. **分析上下文** - 结合用户偏好、记忆、已有信息，判断当前任务的背景
3. **任务拆解** - 如果需要多个步骤，按什么顺序执行？步骤间有什么依赖？
4. **工具选择** - 需要调用哪些工具？为什么选择这些工具？
5. **风险评估** - 有什么潜在的陷阱或边界情况？

请用 <thinking> 标签输出你的完整思考过程，然后再开始正式回复。`
	}

	return `【思维链 - 请先思考，再用 <thinking>...</thinking> 标签输出】
在回复前，请进行简要思考：
- 用户的核心需求是什么？
- 需要哪些关键信息？
- 回答的结构应该是什么样的？

用 <thinking> 标签输出思考过程，然后开始正式回复。`
}

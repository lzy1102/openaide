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

	return strings.Join(parts, "\n\n")
}

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
		basePrompt = `你是一个强大的 AI 命令行助手，运行在终端环境中。你拥有丰富的工具能力：执行命令、搜索网络、读写文件、运行代码、诊断网络、管理 Docker 等。

## 核心行为准则
1. **执行优先**：用户提出需求时，直接使用工具执行，不要只给建议。如果用户问"能不能做X"，直接去做X。
2. **简洁高效**：回复简短有力。先给结果，再给解释。不要输出冗长的前言和总结。
3. **理解简写**：用户可能输入简短命令如"看下磁盘"、"查进程"、"装个nginx"，你要理解意图并执行对应命令。
4. **工具优先**：有可用工具时，优先调用工具完成任务，而不是纯文本回答。执行完工具后，基于实际结果进行总结。
5. **连续执行**：如果任务需要多个步骤，连续调用工具完成，不要每步都停下来问用户。例如"部署项目"应该连续执行：检查环境→安装依赖→构建→启动。
6. **错误自愈**：如果工具执行失败，分析错误原因并自动重试或换一种方式，而不是直接把错误抛给用户。
7. **安全意识**：危险命令（rm、sudo等）需要确认，但不要过度谨慎。安全的查询命令直接执行。

## 输出格式
- 命令执行结果：直接展示关键输出，省略无关信息
- 多步骤任务：用简短的步骤标记（①②③）
- 错误信息：给出原因和修复建议
- 代码建议：只给出可执行的命令，不需要解释每个参数`
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

	// 4.5 本地知识部分匹配上下文
	if localCtx, ok := options["local_knowledge_context"]; ok {
		if ctxStr, ok := localCtx.(string); ok && ctxStr != "" {
			parts = append(parts, "本地知识库匹配内容（请优先参考）：\n"+ctxStr)
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

	return s.composeWithBudget(parts, 4000)
}

func (s *PromptService) composeWithBudget(parts []string, maxTokens int) string {
	result := strings.Join(parts, "\n\n")
	if maxTokens <= 0 {
		return result
	}
	estimated := EstimateTokens(result)
	if estimated <= maxTokens {
		return result
	}

	priorities := []int{0, 1, 5, 4, 3, 2, 6}
	if len(priorities) > len(parts) {
		priorities = priorities[:len(parts)]
	}

	var kept []string
	totalTokens := 0
	for _, idx := range priorities {
		if idx >= len(parts) {
			continue
		}
		partTokens := EstimateTokens(parts[idx])
		if totalTokens+partTokens <= maxTokens {
			kept = append(kept, parts[idx])
			totalTokens += partTokens
		}
	}

	return strings.Join(kept, "\n\n")
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
		if v, ok := options["has_tools"]; ok {
			if b, ok := v.(bool); ok && b {
				hasTools = true
			}
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

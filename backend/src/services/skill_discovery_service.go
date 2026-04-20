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

// SkillDiscoveryService 技能发现服务 - 从对话中发现新模式和新技能
type SkillDiscoveryService struct {
	db            *gorm.DB
	skillSvc      *SkillService
	modelSvc      *ModelService
	memorySvc     *MemoryService
	enabled       bool
	minOccurrences int // 最小出现次数才认为是有效模式
}

// NewSkillDiscoveryService 创建技能发现服务
func NewSkillDiscoveryService(db *gorm.DB, skillSvc *SkillService, modelSvc *ModelService, memorySvc *MemoryService, enabled bool) *SkillDiscoveryService {
	return &SkillDiscoveryService{
		db:             db,
		skillSvc:       skillSvc,
		modelSvc:       modelSvc,
		memorySvc:      memorySvc,
		enabled:        enabled,
		minOccurrences: 3, // 至少出现3次才认为是模式
	}
}

// SkillPattern 发现的模式
type SkillPattern struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Triggers    []string `json:"triggers"`
	SystemPrompt string `json:"system_prompt"`
	Occurrences int      `json:"occurrences"`
	Examples    []string `json:"examples"`
	Confidence  float64  `json:"confidence"`
}

// DiscoverNewSkills 从对话中发现新技能
func (s *SkillDiscoveryService) DiscoverNewSkills(ctx context.Context, userID string) ([]SkillPattern, error) {
	if !s.enabled {
		return nil, nil
	}

	// 获取最近100条已完成对话
	var dialogues []models.Dialogue
	s.db.Where("user_id = ? AND status = ? AND messages_extracted = ?", userID, "completed", true).
		Order("created_at DESC").
		Limit(100).
		Find(&dialogues)

	if len(dialogues) == 0 {
		return nil, nil
	}

	// 分析对话模式
	patterns, err := s.analyzeDialoguePatterns(ctx, dialogues)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze patterns: %w", err)
	}

	// 过滤已有技能
	var newPatterns []SkillPattern
	for _, p := range patterns {
		existing, _ := s.skillSvc.GetSkillByName(p.Name)
		if existing == nil && p.Confidence >= 0.7 {
			newPatterns = append(newPatterns, p)
		}
	}

	return newPatterns, nil
}

// analyzeDialoguePatterns 分析对话中的模式
func (s *SkillDiscoveryService) analyzeDialoguePatterns(ctx context.Context, dialogues []models.Dialogue) ([]SkillPattern, error) {
	// 构建分析文本
	var analysisText strings.Builder
	for _, d := range dialogues {
		var messages []models.Message
		s.db.Where("dialogue_id = ?", d.ID).Order("created_at ASC").Limit(10).Find(&messages)
		if len(messages) > 0 {
			analysisText.WriteString(fmt.Sprintf("\n=== 对话 %s ===\n", d.ID))
			for _, m := range messages {
				sender := "用户"
				if m.Sender == "assistant" {
					sender = "助手"
				}
				content := m.Content
				if len(content) > 200 {
					content = content[:200] + "..."
				}
				analysisText.WriteString(fmt.Sprintf("%s: %s\n", sender, content))
			}
		}
	}

	// 使用 LLM 分析模式
	patterns, err := s.extractPatternWithLLM(ctx, analysisText.String())
	if err != nil {
		return nil, err
	}

	// 验证模式的重复次数
	for i, p := range patterns {
		count := s.countPatternOccurrences(dialogues, p.Triggers)
		patterns[i].Occurrences = count
		patterns[i].Confidence = float64(count) / float64(len(dialogues))
		if patterns[i].Confidence > 1.0 {
			patterns[i].Confidence = 1.0
		}
	}

	return patterns, nil
}

// extractPatternWithLLM 使用 LLM 提取模式
func (s *SkillDiscoveryService) extractPatternWithLLM(ctx context.Context, dialogueText string) ([]SkillPattern, error) {
	client := s.modelSvc.GetLLMClient()
	if client == nil {
		return nil, fmt.Errorf("no LLM client available")
	}

	prompt := fmt.Sprintf(`你是一个技能发现专家。请分析以下对话历史，找出用户反复请求但系统没有专门技能支持的模式。

要求：
1. 只提取至少出现 3 次的模式
2. 忽略已有内置技能能处理的内容（翻译、代码审查、摘要、数据分析、日报）
3. 为每个模式生成：名称、描述、触发词、系统提示词、示例

对话内容：
%s

请以 JSON 数组格式返回发现的模式：
[
  {
    "name": "技能名称（英文小写下划线）",
    "description": "技能描述",
    "category": "分类（language/development/content/analytics/productivity/custom）",
    "triggers": ["触发词1", "触发词2"],
    "system_prompt": "系统提示词",
    "examples": ["示例1", "示例2"]
  }
]

如果没有发现新模式，返回空数组 []。`, dialogueText)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个技能发现专家。从对话历史中发现用户反复请求的模式。只返回 JSON 数组。"},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.2,
		MaxTokens:   3000,
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

	var patterns []SkillPattern
	if err := json.Unmarshal([]byte(content), &patterns); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	return patterns, nil
}

// countPatternOccurrences 统计模式出现次数
func (s *SkillDiscoveryService) countPatternOccurrences(dialogues []models.Dialogue, triggers []string) int {
	count := 0
	for _, d := range dialogues {
		var messages []models.Message
		s.db.Where("dialogue_id = ? AND sender = ?", d.ID, "user").Find(&messages)
		for _, m := range messages {
			contentLower := strings.ToLower(m.Content)
			for _, trigger := range triggers {
				if strings.Contains(contentLower, strings.ToLower(trigger)) {
					count++
					break
				}
			}
		}
	}
	return count
}

// CreateSkillFromPattern 从模式创建新技能
func (s *SkillDiscoveryService) CreateSkillFromPattern(pattern SkillPattern, userID string) (*models.Skill, error) {
	skill := &models.Skill{
		ID:          uuid.New().String(),
		Name:        pattern.Name,
		Description: pattern.Description,
		Category:    pattern.Category,
		Version:     "1.0",
		Author:      userID,
		Enabled:     true,
		Builtin:     false,
		Triggers:    models.JSONSlice(pattern.Triggers),
		SystemPromptOverride: pattern.SystemPrompt,
		Level0Summary: fmt.Sprintf("自动发现的新技能，已出现 %d 次", pattern.Occurrences),
		Level1Content: pattern.SystemPrompt,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := s.skillSvc.CreateSkill(skill); err != nil {
		return nil, err
	}

	// 记录发现日志
	log.Printf("[SkillDiscovery] Created new skill '%s' from pattern (occurrences: %d, confidence: %.2f)",
		skill.Name, pattern.Occurrences, pattern.Confidence)

	return skill, nil
}

// RunPeriodicDiscovery 定期执行技能发现
func (s *SkillDiscoveryService) RunPeriodicDiscovery(ctx context.Context) {
	if !s.enabled {
		return
	}

	log.Printf("[SkillDiscovery] Starting periodic skill discovery...")

	// 获取所有用户
	var users []models.User
	s.db.Find(&users)

	for _, user := range users {
		patterns, err := s.DiscoverNewSkills(ctx, user.ID)
		if err != nil {
			log.Printf("[SkillDiscovery] Failed for user %s: %v", user.ID, err)
			continue
		}

		for _, p := range patterns {
			if p.Occurrences >= s.minOccurrences {
				skill, err := s.CreateSkillFromPattern(p, user.ID)
				if err != nil {
					log.Printf("[SkillDiscovery] Failed to create skill '%s': %v", p.Name, err)
				} else {
					log.Printf("[SkillDiscovery] Created skill '%s' with %d occurrences", skill.Name, p.Occurrences)
				}
			}
		}
	}

	log.Printf("[SkillDiscovery] Periodic discovery completed")
}

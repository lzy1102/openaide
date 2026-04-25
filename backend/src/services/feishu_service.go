package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"gorm.io/gorm"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// FeishuService 飞书机器人服务
type FeishuService struct {
	db                  *gorm.DB
	enhancedDialogueSvc *EnhancedDialogueService
	modelSvc            *ModelService
	skillSvc            *SkillService
	voiceSvc            *VoiceService
	feedbackSvc         *FeedbackService
	cacheSvc            *CacheService
	loggerSvc           *LoggerService
	config              FeishuConfig
	client              *larkws.Client
	tenantToken         string
	tokenExpireAt       time.Time
	httpClient         *http.Client
	mu                 sync.RWMutex
	running            bool
	cancelFunc         context.CancelFunc
	lastMessageAt      map[string]time.Time
}

// FeishuConfig 飞书配置
type FeishuConfig struct {
	Enabled        bool   `json:"enabled"`
	AppID          string `json:"app_id"`
	AppSecret      string `json:"app_secret"`
	DefaultModel   string `json:"default_model"`
	SystemPrompt   string `json:"system_prompt"`
	StreamInterval int    `json:"stream_interval"`
}

// NewFeishuService 创建飞书服务实例
func NewFeishuService(db *gorm.DB, enhancedDialogueSvc *EnhancedDialogueService, modelSvc *ModelService, skillSvc *SkillService, voiceSvc *VoiceService, feedbackSvc *FeedbackService, cacheSvc *CacheService, loggerSvc *LoggerService, cfg FeishuConfig) *FeishuService {
	return &FeishuService{
		db:                  db,
		enhancedDialogueSvc: enhancedDialogueSvc,
		modelSvc:            modelSvc,
		skillSvc:            skillSvc,
		voiceSvc:            voiceSvc,
		feedbackSvc:         feedbackSvc,
		cacheSvc:            cacheSvc,
		loggerSvc:           loggerSvc,
		config:              cfg,
		httpClient:          &http.Client{Timeout: 30 * time.Second},
		lastMessageAt:       make(map[string]time.Time),
	}
}

// Start 启动 WebSocket 长连接（非阻塞）
func (s *FeishuService) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("feishu service already running")
	}
	if s.config.AppID == "" || s.config.AppSecret == "" {
		return fmt.Errorf("feishu app_id or app_secret is empty")
	}

	// 获取 tenant_access_token
	token, err := s.getTenantToken()
	if err != nil {
		return fmt.Errorf("failed to get tenant_access_token: %w", err)
	}
	s.tenantToken = token

	// 创建事件分发器并注册消息接收回调
	eventDispatcher := dispatcher.NewEventDispatcher("", "")
	eventDispatcher.OnP2MessageReceiveV1(s.handleMessage)

	// 创建 WebSocket 客户端
	s.client = larkws.NewClient(
		s.config.AppID,
		s.config.AppSecret,
		larkws.WithEventHandler(eventDispatcher),
	)

	// 在 goroutine 中启动（Start 内部会 select {} 阻塞）
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelFunc = cancel
	s.running = true

	go func() {
		defer func() {
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
			if r := recover(); r != nil {
				log.Printf("[Feishu] WebSocket goroutine panic: %v", r)
			}
		}()
		log.Println("[Feishu] WebSocket client starting...")
		if err := s.client.Start(ctx); err != nil {
			log.Printf("[Feishu] WebSocket client stopped: %v", err)
		}
	}()

	log.Println("[Feishu] WebSocket client started")
	return nil
}

// Stop 停止 WebSocket 连接
func (s *FeishuService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}
	if s.cancelFunc != nil {
		s.cancelFunc()
		s.cancelFunc = nil
	}
	s.running = false
	log.Println("[Feishu] WebSocket client stopped")
}

// IsRunning 返回运行状态
func (s *FeishuService) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetStatus 获取状态信息
func (s *FeishuService) GetStatus() map[string]interface{} {
	var sessionCount int64
	s.db.Model(&models.FeishuSession{}).Count(&sessionCount)
	var userCount int64
	s.db.Model(&models.FeishuUser{}).Count(&userCount)

	return map[string]interface{}{
		"enabled":  s.config.Enabled,
		"running":  s.IsRunning(),
		"app_id":   maskString(s.config.AppID),
		"sessions": sessionCount,
		"users":    userCount,
	}
}

// getTenantToken 获取 tenant_access_token
func (s *FeishuService) getTenantToken() (string, error) {
	body := map[string]string{
		"app_id":     s.config.AppID,
		"app_secret": s.config.AppSecret,
	}
	data, _ := json.Marshal(body)

	resp, err := s.httpClient.Post(
		"https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal",
		"application/json; charset=utf-8",
		bytes.NewReader(data),
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Code         int    `json:"code"`
		Msg          string `json:"msg"`
		TenantToken  string `json:"tenant_access_token"`
		Expire       int    `json:"expire"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Code != 0 {
		return "", fmt.Errorf("feishu auth error: code=%d msg=%s", result.Code, result.Msg)
	}
	return result.TenantToken, nil
}

// refreshTenantToken 刷新 token（过期前刷新）
func (s *FeishuService) refreshTenantToken() error {
	s.mu.RLock()
	if time.Now().Before(s.tokenExpireAt.Add(-5 * time.Minute)) {
		s.mu.RUnlock()
		return nil
	}
	s.mu.RUnlock()

	token, err := s.getTenantToken()
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.tenantToken = token
	s.tokenExpireAt = time.Now().Add(2 * time.Hour) // 飞书 token 有效期约2小时
	s.mu.Unlock()
	return nil
}

// handleMessage 处理飞书消息事件（SDK 回调）
func (s *FeishuService) handleMessage(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	if event == nil || event.Event == nil || event.Event.Message == nil || event.Event.Sender == nil {
		return nil
	}

	msg := event.Event.Message
	sender := event.Event.Sender

	// 只处理文本消息
	if msg.MessageType == nil || *msg.MessageType != "text" {
		return nil
	}

	// 忽略机器人自己发的消息
	if sender.SenderType != nil && *sender.SenderType == "app" {
		return nil
	}

	messageID := stringValue(msg.MessageId)
	chatID := stringValue(msg.ChatId)
	chatType := stringValue(msg.ChatType)
	content := stringValue(msg.Content)
	openID := ""
	unionID := ""
	if sender.SenderId != nil {
		openID = stringValue(sender.SenderId.OpenId)
		unionID = stringValue(sender.SenderId.UnionId)
	}

	if messageID == "" || chatID == "" {
		return nil
	}

	// 防重复处理
	s.mu.Lock()
	if last, ok := s.lastMessageAt[messageID]; ok && time.Since(last) < 5*time.Minute {
		s.mu.Unlock()
		return nil
	}
	s.lastMessageAt[messageID] = time.Now()
	for k, v := range s.lastMessageAt {
		if time.Since(v) > 5*time.Minute {
			delete(s.lastMessageAt, k)
		}
	}
	s.mu.Unlock()

	// 解析文本内容
	textContent := parseTextContent(content)
	if textContent == "" {
		return nil
	}

	// 处理命令
	if strings.HasPrefix(textContent, "/") {
		s.handleCommand(ctx, chatID, chatType, openID, textContent)
		return nil
	}

	// 更新用户记录
	s.upsertFeishuUser(openID, unionID, chatType)

	// 获取或创建会话
	session, err := s.resolveSession(ctx, chatType, openID, chatID)
	if err != nil {
		log.Printf("[Feishu] failed to resolve session: %v", err)
		s.sendCardMessage(ctx, chatID, buildErrorCard(fmt.Sprintf("会话创建失败: %v", err)))
		return nil
	}

	// 记录入站消息
	s.saveMessageLog(messageID, chatID, textContent, "inbound", "success", 0)

	// 发送初始"思考中"卡片
	cardResp, err := s.sendCardMessage(ctx, chatID, buildThinkingCard())
	if err != nil {
		log.Printf("[Feishu] failed to send thinking card: %v", err)
		return nil
	}
	cardMessageID := ""
	if cardResp != nil {
		cardMessageID = cardResp.Data.MessageID
	}

	// 流式调用 LLM（使用增强服务）
	modelID := session.ModelID
	if modelID == "" {
		modelID = s.config.DefaultModel
	}
	options := map[string]interface{}{}
	if s.config.SystemPrompt != "" {
		options["system"] = s.config.SystemPrompt
	}

	startTime := time.Now()
	var chunkChan <-chan llm.ChatStreamChunk
	chunkChan, err = s.enhancedDialogueSvc.SendMessageStreamEnhanced(ctx, session.DialogueID, openID, textContent, modelID, options)
	if err != nil {
		duration := time.Since(startTime)
		log.Printf("[Feishu] LLM call failed: %v", err)
		if cardMessageID != "" {
			s.patchCardMessage(ctx, chatID, cardMessageID, buildErrorCard(fmt.Sprintf("AI 调用失败: %v", err)))
		} else {
			s.sendCardMessage(ctx, chatID, buildErrorCard(fmt.Sprintf("AI 调用失败: %v", err)))
		}
		s.saveMessageLog(messageID, chatID, textContent, "inbound", "error", duration.Milliseconds())
		return nil
	}

	// 流式更新卡片
	var fullContent strings.Builder
	interval := time.Duration(s.config.StreamInterval) * time.Millisecond
	if interval <= 0 {
		interval = 200 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	streamDone := false
	pendingContent := ""

	for !streamDone {
		select {
		case chunk, ok := <-chunkChan:
			if !ok {
				streamDone = true
				continue
			}
			if chunk.Error != nil {
				streamDone = true
				continue
			}
			if len(chunk.Choices) > 0 {
				pendingContent += chunk.Choices[0].Delta.Content
				fullContent.WriteString(chunk.Choices[0].Delta.Content)
			}
		case <-ticker.C:
			if pendingContent != "" && cardMessageID != "" {
				currentContent := fullContent.String()
				if err := s.patchCardMessage(ctx, chatID, cardMessageID, buildStreamCard(currentContent)); err != nil {
					log.Printf("[Feishu] patch card failed: %v", err)
				}
				pendingContent = ""
			}
		}
	}

	// 发送最终完成卡片（带反馈按钮）
	duration := time.Since(startTime)
	finalContent := fullContent.String()
	finalCard := buildFeedbackCard(finalContent, fmt.Sprintf("耗时 %.1fs", duration.Seconds()), session.DialogueID)

	if cardMessageID != "" {
		if err := s.patchCardMessage(ctx, chatID, cardMessageID, finalCard); err != nil {
			log.Printf("[Feishu] patch final card failed, sending new message: %v", err)
			s.sendCardMessage(ctx, chatID, finalCard)
		}
	} else {
		s.sendCardMessage(ctx, chatID, finalCard)
	}

	// 保存完整回复到对话
	if finalContent != "" {
		s.enhancedDialogueSvc.SaveStreamMessage(session.DialogueID, finalContent)
	}

	// 更新会话消息计数
	s.db.Model(&models.FeishuSession{}).Where("id = ?", session.ID).
		Update("message_count", gorm.Expr("message_count + ?", 1))

	// 记录出站消息
	s.saveMessageLog(messageID, chatID, finalContent, "outbound", "success", duration.Milliseconds())

	return nil
}

// handleCommand 处理斜杠命令
func (s *FeishuService) handleCommand(ctx context.Context, chatID, chatType, openID, textContent string) {
	parts := strings.Fields(textContent)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/reset":
		sessionKey := buildSessionKey(chatType, openID, chatID)
		s.db.Where("session_key = ?", sessionKey).Delete(&models.FeishuSession{})
		s.sendCardMessage(ctx, chatID, buildFinalCard("会话已重置，开始新的对话。", ""))

	case "/model":
		if len(parts) < 2 {
			currentModel := s.config.DefaultModel
			if currentModel == "" {
				currentModel = "未配置"
			}
			s.sendCardMessage(ctx, chatID, buildFinalCard(fmt.Sprintf("当前默认模型: %s\n\n使用 `/model <模型名称>` 切换模型", currentModel), ""))
			return
		}
		newModel := parts[1]
		sessionKey := buildSessionKey(chatType, openID, chatID)
		s.db.Model(&models.FeishuSession{}).Where("session_key = ?", sessionKey).
			Update("model_id", newModel)
		s.sendCardMessage(ctx, chatID, buildFinalCard(fmt.Sprintf("已切换模型为: %s", newModel), ""))

	case "/help":
		helpText := "**可用命令:**\n\n" +
			"/reset - 重置当前会话\n" +
			"/model - 查看当前模型\n" +
			"/model <名称> - 切换模型\n" +
			"/tools <问题> - 使用工具回答（如: /tools 现在几点了）\n" +
			"/plan <任务描述> - 任务规划（如: /plan 帮我开发用户管理系统）\n" +
			"/skill <内容> - 使用技能（如: /skill 帮我翻译这段话）\n" +
			"/voice <文本> - 文字转语音（如: /voice 你好）\n" +
			"/help - 显示帮助"
		s.sendCardMessage(ctx, chatID, buildFinalCard(helpText, ""))

	case "/tools":
		if len(parts) < 2 {
			s.sendCardMessage(ctx, chatID, buildFinalCard("用法: /tools <你的问题>\n\n示例: /tools 现在几点了？", ""))
			return
		}
		startTime := time.Now()
		question := strings.Join(parts[1:], " ")

		// 获取会话
		session, err := s.resolveSession(ctx, chatType, openID, chatID)
		if err != nil {
			s.sendCardMessage(ctx, chatID, buildErrorCard(fmt.Sprintf("会话创建失败: %v", err)))
			return
		}

		modelID := session.ModelID
		if modelID == "" {
			modelID = s.config.DefaultModel
		}
		options := map[string]interface{}{}
		if s.config.SystemPrompt != "" {
			options["system"] = s.config.SystemPrompt
		}

		if s.enhancedDialogueSvc != nil {
			msg, err := s.enhancedDialogueSvc.SendMessageWithTools(ctx, session.DialogueID, openID, question, modelID, options)
			if err != nil {
				s.sendCardMessage(ctx, chatID, buildErrorCard(fmt.Sprintf("工具调用失败: %v", err)))
				return
			}
			s.sendCardMessage(ctx, chatID, buildFeedbackCard(msg.Content, fmt.Sprintf("%dms", time.Since(startTime).Milliseconds()), session.DialogueID))
		} else {
			s.sendCardMessage(ctx, chatID, buildErrorCard("增强对话服务不可用"))
		}
		return
	case "/plan":
		if len(parts) < 2 {
			s.sendCardMessage(ctx, chatID, buildFinalCard("用法: /plan <任务描述>\n\n示例: /plan 帮我开发一个用户管理系统", ""))
			return
		}
		startTime := time.Now()
		taskDesc := strings.Join(parts[1:], " ")

		session, err := s.resolveSession(ctx, chatType, openID, chatID)
		if err != nil {
			s.sendCardMessage(ctx, chatID, buildErrorCard(fmt.Sprintf("会话创建失败: %v", err)))
			return
		}

		if s.enhancedDialogueSvc != nil {
			result, err := s.enhancedDialogueSvc.SendMessageWithPlan(ctx, session.DialogueID, openID, taskDesc, s.config.DefaultModel, nil)
			if err != nil {
				s.sendCardMessage(ctx, chatID, buildErrorCard(fmt.Sprintf("规划失败: %v", err)))
				return
			}
			if result.NeedsPlanning {
				summary := result.PlanSummary
				if summary == "" {
					summary = fmt.Sprintf("任务类型: %s\n复杂度: %s\n子任务数: %d\n状态: %s",
						result.TaskType, result.Complexity, result.SubtaskCount, result.Status)
				}
				s.sendCardMessage(ctx, chatID, buildFeedbackCard(summary, fmt.Sprintf("%dms", time.Since(startTime).Milliseconds()), session.DialogueID))
			} else {
				s.sendCardMessage(ctx, chatID, buildFinalCard("该任务不需要规划，已走普通对话流程。", ""))
			}
		} else {
			s.sendCardMessage(ctx, chatID, buildErrorCard("增强对话服务不可用"))
		}
		return

	case "/skill":
		if len(parts) < 2 {
			s.sendCardMessage(ctx, chatID, buildFinalCard("用法: /skill <内容>\n\n系统会自动匹配最佳技能并执行。\n示例: /skill 帮我翻译这段话\n示例: /skill 帮我总结一下", ""))
			return
		}
		startTime := time.Now()
		skillContent := strings.Join(parts[1:], " ")

		if s.skillSvc == nil {
			s.sendCardMessage(ctx, chatID, buildErrorCard("技能服务不可用"))
			return
		}
		if s.enhancedDialogueSvc == nil {
			s.sendCardMessage(ctx, chatID, buildErrorCard("增强对话服务不可用"))
			return
		}

		session, err := s.resolveSession(ctx, chatType, openID, chatID)
		if err != nil {
			s.sendCardMessage(ctx, chatID, buildErrorCard(fmt.Sprintf("会话创建失败: %v", err)))
			return
		}

		cardMessageID := ""
		if cardResp, err := s.sendCardMessage(ctx, chatID, buildThinkingCard()); err != nil {
			log.Printf("[Feishu] failed to send skill thinking card: %v", err)
		} else if cardResp != nil {
			cardMessageID = cardResp.Data.MessageID
		}

		match := s.skillSvc.MatchSkill(skillContent)
		if match == nil {
			finalCard := buildFinalCard("未找到匹配的技能，请调整描述后重试。\n输入 /help 查看可用命令。", "")
			if cardMessageID != "" {
				if err := s.patchCardMessage(ctx, chatID, cardMessageID, finalCard); err != nil {
					log.Printf("[Feishu] patch skill no-match card failed: %v", err)
					s.sendCardMessage(ctx, chatID, finalCard)
				}
			} else {
				s.sendCardMessage(ctx, chatID, finalCard)
			}
			return
		}

		if err := s.enhancedDialogueSvc.AddMessage(session.DialogueID, "user", skillContent); err != nil {
			log.Printf("[Feishu] Failed to add user message: %v", err)
		}

		execution, err := s.skillSvc.ExecuteSkillWithContent(ctx, match.Skill, skillContent, openID)
		duration := time.Since(startTime)
		if err != nil {
			errMsg := fmt.Sprintf("技能执行失败: %v", err)
			if IsSkillParameterError(err) {
				errMsg = fmt.Sprintf("技能参数错误: %v", err)
			}
			finalCard := buildErrorCard(errMsg)
			if cardMessageID != "" {
				if patchErr := s.patchCardMessage(ctx, chatID, cardMessageID, finalCard); patchErr != nil {
					log.Printf("[Feishu] patch skill error card failed: %v", patchErr)
					s.sendCardMessage(ctx, chatID, finalCard)
				}
			} else {
				s.sendCardMessage(ctx, chatID, finalCard)
			}
			return
		}

		finalContent := formatSkillExecutionCardContent(match, execution)
		if err := s.enhancedDialogueSvc.AddMessage(session.DialogueID, "assistant", finalContent); err != nil {
			log.Printf("[Feishu] Failed to add assistant message: %v", err)
		}
		s.db.Model(&models.FeishuSession{}).Where("id = ?", session.ID).
			Update("message_count", gorm.Expr("message_count + ?", 1))

		finalCard := buildFeedbackCard(finalContent, fmt.Sprintf("%dms", duration.Milliseconds()), session.DialogueID)
		if cardMessageID != "" {
			if err := s.patchCardMessage(ctx, chatID, cardMessageID, finalCard); err != nil {
				log.Printf("[Feishu] patch skill final card failed: %v", err)
				s.sendCardMessage(ctx, chatID, finalCard)
			}
		} else {
			s.sendCardMessage(ctx, chatID, finalCard)
		}
		return

	case "/voice":
		if len(parts) < 2 {
			s.sendCardMessage(ctx, chatID, buildFinalCard("用法: /voice <文本>\n\n系统会将文本转为语音发送。\n示例: /voice 你好，这是语音测试", ""))
			return
		}
		voiceText := strings.Join(parts[1:], " ")
		startTime := time.Now()
		if s.voiceSvc != nil && s.voiceSvc.IsEnabled() {
			_, format, err := s.voiceSvc.TextToSpeechBase64(ctx, voiceText)
			if err != nil {
				s.sendCardMessage(ctx, chatID, buildErrorCard(fmt.Sprintf("语音合成失败: %v", err)))
				return
			}
			s.sendCardMessage(ctx, chatID, buildFeedbackCard(
				fmt.Sprintf("语音合成成功 (格式: %s, 耗时 %dms)", format, time.Since(startTime).Milliseconds()),
				fmt.Sprintf("%dms", time.Since(startTime).Milliseconds()), ""))
		} else {
			s.sendCardMessage(ctx, chatID, buildErrorCard("语音服务未启用或未配置 TTS API"))
		}
		return
	default:
		s.sendCardMessage(ctx, chatID, buildFinalCard(fmt.Sprintf("未知命令: %s\n输入 /help 查看帮助", cmd), ""))
	}
}

// upsertFeishuUser 更新或创建飞书用户
func (s *FeishuService) upsertFeishuUser(openID, unionID, chatType string) {
	var user models.FeishuUser
	err := s.db.Where("open_id = ?", openID).First(&user).Error
	if err != nil {
		user = models.FeishuUser{
			OpenID:       openID,
			UnionID:      unionID,
			ChatType:     chatType,
			LastActiveAt: time.Now(),
		}
		s.db.Create(&user)
	} else {
		s.db.Model(&user).Updates(map[string]interface{}{
			"union_id":       unionID,
			"chat_type":      chatType,
			"last_active_at": time.Now(),
		})
	}
}

// resolveSession 获取或创建会话
func (s *FeishuService) resolveSession(ctx context.Context, chatType, openID, chatID string) (*models.FeishuSession, error) {
	sessionKey := buildSessionKey(chatType, openID, chatID)

	var session models.FeishuSession
	err := s.db.Where("session_key = ?", sessionKey).First(&session).Error
	if err == nil {
		_, found := s.enhancedDialogueSvc.GetDialogue(session.DialogueID)
		if found {
			return &session, nil
		}
		s.db.Delete(&session)
	}

	dialogue := s.enhancedDialogueSvc.CreateDialogue(openID, fmt.Sprintf("飞书会话 %s", sessionKey))

	session = models.FeishuSession{
		SessionKey:   sessionKey,
		DialogueID:   dialogue.ID,
		ModelID:      s.config.DefaultModel,
		MessageCount: 0,
	}
	if err := s.db.Create(&session).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

// sendCardMessage 发送卡片消息
func (s *FeishuService) sendCardMessage(ctx context.Context, chatID, cardContent string) (*feishuSendResp, error) {
	if err := s.refreshTenantToken(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	token := s.tenantToken
	s.mu.RUnlock()

	body := map[string]interface{}{
		"receive_id": chatID,
		"msg_type":   "interactive",
		"content":    cardContent,
	}
	data, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id",
		bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respData, _ := io.ReadAll(resp.Body)
	var result feishuSendResp
	if err := json.Unmarshal(respData, &result); err != nil {
		return nil, err
	}
	if result.Code != 0 {
		return &result, fmt.Errorf("feishu api error: code=%d msg=%s", result.Code, result.Msg)
	}
	return &result, nil
}

// patchCardMessage 更新卡片消息
func (s *FeishuService) patchCardMessage(ctx context.Context, chatID, messageID, cardContent string) error {
	if err := s.refreshTenantToken(); err != nil {
		return err
	}

	s.mu.RLock()
	token := s.tenantToken
	s.mu.RUnlock()

	body := map[string]interface{}{
		"content": cardContent,
	}
	data, _ := json.Marshal(body)

	url := fmt.Sprintf("https://open.feishu.cn/open-apis/im/v1/messages/%s?receive_id_type=chat_id", messageID)
	req, err := http.NewRequestWithContext(ctx, "PATCH", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respData, _ := io.ReadAll(resp.Body)
	var result feishuAPIResp
	if err := json.Unmarshal(respData, &result); err != nil {
		return err
	}
	if result.Code != 0 {
		return fmt.Errorf("feishu patch error: code=%d msg=%s", result.Code, result.Msg)
	}
	return nil
}

// saveMessageLog 保存消息日志
func (s *FeishuService) saveMessageLog(messageID, chatID, content, direction, status string, duration int64) {
	msgLog := models.FeishuMessageLog{
		MessageID: messageID,
		ChatID:    chatID,
		Content:   content,
		Direction: direction,
		Status:    status,
		Duration:  duration,
	}
	s.db.Create(&msgLog)
}

func formatSkillExecutionCardContent(match *SkillMatchResult, execution *models.SkillExecution) string {
	var parts []string

	skillName := ""
	if execution != nil {
		skillName = execution.SkillName
	}
	if skillName == "" && match != nil && match.Skill != nil {
		skillName = match.Skill.Name
	}
	if skillName != "" {
		parts = append(parts, fmt.Sprintf("已执行技能: %s", skillName))
	}
	if match != nil && match.Skill != nil && strings.TrimSpace(match.Skill.Description) != "" {
		parts = append(parts, fmt.Sprintf("描述: %s", match.Skill.Description))
	}
	if match != nil && match.Confidence > 0 {
		parts = append(parts, fmt.Sprintf("匹配置信度: %.2f", match.Confidence))
	}

	resultText := extractSkillExecutionResultText(execution)
	if resultText == "" {
		resultText = "技能执行完成。"
	}
	parts = append(parts, "", resultText)

	return strings.Join(parts, "\n")
}

func extractSkillExecutionResultText(execution *models.SkillExecution) string {
	if execution == nil || execution.Result == nil || execution.Result.Data == nil {
		return ""
	}

	switch v := execution.Result.Data.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]interface{}:
		if output, ok := v["output"].(string); ok && strings.TrimSpace(output) != "" {
			return strings.TrimSpace(output)
		}
		payload, err := json.MarshalIndent(v, "", "  ")
		if err == nil {
			return "```json\n" + string(payload) + "\n```"
		}
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	default:
		payload, err := json.MarshalIndent(v, "", "  ")
		if err == nil {
			return "```json\n" + string(payload) + "\n```"
		}
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

// ListSessions 列出所有飞书会话
func (s *FeishuService) ListSessions() []models.FeishuSession {
	var sessions []models.FeishuSession
	s.db.Order("updated_at DESC").Find(&sessions)
	return sessions
}

// DeleteSession 删除飞书会话（重置）
func (s *FeishuService) DeleteSession(sessionKey string) error {
	return s.db.Where("session_key = ?", sessionKey).Delete(&models.FeishuSession{}).Error
}

// ListUsers 列出所有飞书用户
func (s *FeishuService) ListUsers() []models.FeishuUser {
	var users []models.FeishuUser
	s.db.Order("last_active_at DESC").Find(&users)
	return users
}

// --- 辅助类型和函数 ---

type feishuAPIResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type feishuSendResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		MessageID string `json:"message_id"`
	} `json:"data"`
}

func stringValue(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func buildSessionKey(chatType, openID, chatID string) string {
	if chatType == "p2p" {
		return fmt.Sprintf("p2p:%s", openID)
	}
	return fmt.Sprintf("group:%s", chatID)
}

func parseTextContent(rawContent string) string {
	var msg struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(rawContent), &msg); err != nil {
		return strings.TrimSpace(rawContent)
	}
	return strings.TrimSpace(msg.Text)
}

func maskString(s string) string {
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "***" + s[len(s)-4:]
}

package services

import (
	"encoding/json"
	"log"
	"sync"
	"time"
)

// WebSocketMessageType WebSocket 消息类型
type WebSocketMessageType string

const (
	// 客户端 -> 服务端
	WSTypeAuth         WebSocketMessageType = "auth"          // 认证
	WSTypePing         WebSocketMessageType = "ping"          // 心跳请求
	WSTypeSubscribe    WebSocketMessageType = "subscribe"     // 订阅频道
	WSTypeUnsubscribe  WebSocketMessageType = "unsubscribe"   // 取消订阅
	WSTypeChat         WebSocketMessageType = "chat"          // 聊天消息
	WSTypeChatStream   WebSocketMessageType = "chat_stream"   // 流式聊天
	WSTypeTaskStatus   WebSocketMessageType = "task_status"   // 任务状态查询
	WSTypeWorkflowExec WebSocketMessageType = "workflow_exec" // 工作流执行

	// 服务端 -> 客户端
	WSTypePong           WebSocketMessageType = "pong"            // 心跳响应
	WSTypeConnected      WebSocketMessageType = "connected"       // 连接成功
	WSTypeError          WebSocketMessageType = "error"           // 错误消息
	WSTypeChatResponse   WebSocketMessageType = "chat_response"   // 聊天响应
	WSTypeChatChunk      WebSocketMessageType = "chat_chunk"      // 流式聊天块
	WSTypeChatComplete   WebSocketMessageType = "chat_complete"   // 流式聊天完成
	WSTypeTaskUpdate     WebSocketMessageType = "task_update"     // 任务状态更新
	WSTypeWorkflowUpdate WebSocketMessageType = "workflow_update" // 工作流状态更新
	WSTypeNotification   WebSocketMessageType = "notification"    // 通知消息
	WSTypeTaskCompleted  WebSocketMessageType = "task_completed"  // 后台任务完成通知 (Hermes Agent)
)

// WebSocketMessage WebSocket 消息结构
type WebSocketMessage struct {
	Type      WebSocketMessageType `json:"type"`
	ID        string               `json:"id,omitempty"`        // 消息 ID（用于请求-响应匹配）
	Timestamp int64                `json:"timestamp,omitempty"` // 时间戳
	Channel   string               `json:"channel,omitempty"`   // 频道/房间
	Payload   interface{}          `json:"payload,omitempty"`   // 消息内容
}

// WebSocketClient WebSocket 客户端接口
type WebSocketClient interface {
	// GetID 获取客户端 ID
	GetID() string
	// GetUserID 获取用户 ID
	GetUserID() string
	// GetChannels 获取订阅的频道
	GetChannels() map[string]bool
	// SendMessage 发送消息
	SendMessage(message []byte) error
	// CloseConn 关闭连接
	CloseConn() error
	// SetLastActivity 设置最后活动时间
	SetLastActivity(t time.Time)
	// GetLastActivity 获取最后活动时间
	GetLastActivity() time.Time
}

// WebSocketClientImpl WebSocket 客户端实现
type WebSocketClientImpl struct {
	ID           string
	UserID       string
	Channels     map[string]bool
	sendFunc     func(message []byte) error
	closeFunc    func() error
	LastActivity time.Time
	mu           sync.RWMutex
}

// 确保 WebSocketClientImpl 实现 WebSocketClient 接口
var _ WebSocketClient = (*WebSocketClientImpl)(nil)

func (c *WebSocketClientImpl) GetID() string {
	return c.ID
}

func (c *WebSocketClientImpl) GetUserID() string {
	return c.UserID
}

func (c *WebSocketClientImpl) GetChannels() map[string]bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	// 返回副本避免并发问题
	result := make(map[string]bool, len(c.Channels))
	for k, v := range c.Channels {
		result[k] = v
	}
	return result
}

// AddChannel 添加频道订阅
func (c *WebSocketClientImpl) AddChannel(channel string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Channels == nil {
		c.Channels = make(map[string]bool)
	}
	c.Channels[channel] = true
}

// RemoveChannel 移除频道订阅
func (c *WebSocketClientImpl) RemoveChannel(channel string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Channels != nil {
		delete(c.Channels, channel)
	}
}

func (c *WebSocketClientImpl) SetLastActivity(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastActivity = t
}

func (c *WebSocketClientImpl) GetLastActivity() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.LastActivity
}

// SendMessage 发送消息方法
func (c *WebSocketClientImpl) SendMessage(message []byte) error {
	if c.sendFunc != nil {
		return c.sendFunc(message)
	}
	return nil
}

// CloseConn 关闭连接方法
func (c *WebSocketClientImpl) CloseConn() error {
	if c.closeFunc != nil {
		return c.closeFunc()
	}
	return nil
}

// SetSendFunc 设置发送函数
func (c *WebSocketClientImpl) SetSendFunc(fn func(message []byte) error) {
	c.sendFunc = fn
}

// SetCloseFunc 设置关闭函数
func (c *WebSocketClientImpl) SetCloseFunc(fn func() error) {
	c.closeFunc = fn
}

// WebSocketService WebSocket 服务
type WebSocketService struct {
	clients     map[string]WebSocketClient
	userClients map[string]map[string]bool // userID -> clientIDs
	channels    map[string]map[string]bool // channel -> clientIDs
	mu          sync.RWMutex

	// 依赖服务
	dialogueService *DialogueService
	modelService    *ModelService
	taskService     *TaskService
	workflowService *WorkflowService

	// 基于活动的超时跟踪器 (Hermes Agent)
	ActivityTracker *ActivityTracker

	// 配置
	pingInterval   time.Duration
	pongWait       time.Duration
	maxMessageSize int64
}

// WebSocketConfig WebSocket 配置
type WebSocketConfig struct {
	PingInterval   time.Duration
	PongWait       time.Duration
	MaxMessageSize int64
}

// NewWebSocketService 创建 WebSocket 服务
func NewWebSocketService(
	dialogueService *DialogueService,
	modelService *ModelService,
	taskService *TaskService,
	workflowService *WorkflowService,
	config *WebSocketConfig,
) *WebSocketService {
	if config == nil {
		config = &WebSocketConfig{
			PingInterval:   30 * time.Second,
			PongWait:       60 * time.Second,
			MaxMessageSize: 1024 * 1024, // 1MB
		}
	}

	svc := &WebSocketService{
		clients:         make(map[string]WebSocketClient),
		userClients:     make(map[string]map[string]bool),
		channels:        make(map[string]map[string]bool),
		dialogueService: dialogueService,
		modelService:    modelService,
		taskService:     taskService,
		workflowService: workflowService,
		pingInterval:    config.PingInterval,
		pongWait:        config.PongWait,
		maxMessageSize:  config.MaxMessageSize,
	}

	// 启动心跳检查
	go svc.heartbeatChecker()

	return svc
}

// RegisterClient 注册客户端
func (s *WebSocketService) RegisterClient(client WebSocketClient) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.clients[client.GetID()] = client

	// 关联用户
	userID := client.GetUserID()
	if userID != "" {
		if s.userClients[userID] == nil {
			s.userClients[userID] = make(map[string]bool)
		}
		s.userClients[userID][client.GetID()] = true
	}

	log.Printf("[WS] Client registered: %s (user: %s)", client.GetID(), userID)

	// 发送连接成功消息
	s.sendToClient(client, WebSocketMessage{
		Type:      WSTypeConnected,
		Timestamp: time.Now().Unix(),
		Payload: map[string]interface{}{
			"client_id": client.GetID(),
		},
	})
}

// UnregisterClient 注销客户端
func (s *WebSocketService) UnregisterClient(clientID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	client, ok := s.clients[clientID]
	if !ok {
		return
	}

	// 移除用户关联
	userID := client.GetUserID()
	if userID != "" && s.userClients[userID] != nil {
		delete(s.userClients[userID], clientID)
		if len(s.userClients[userID]) == 0 {
			delete(s.userClients, userID)
		}
	}

	// 移除频道订阅
	for channel := range client.GetChannels() {
		if s.channels[channel] != nil {
			delete(s.channels[channel], clientID)
			if len(s.channels[channel]) == 0 {
				delete(s.channels, channel)
			}
		}
	}

	delete(s.clients, clientID)
	log.Printf("[WS] Client unregistered: %s", clientID)
}

// GetClient 获取客户端
func (s *WebSocketService) GetClient(clientID string) (WebSocketClient, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	client, ok := s.clients[clientID]
	return client, ok
}

// HandleMessage 处理消息
func (s *WebSocketService) HandleMessage(client WebSocketClient, data []byte) {
	client.SetLastActivity(time.Now())

	var msg WebSocketMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		s.sendError(client, "", "invalid message format")
		return
	}

	msg.Timestamp = time.Now().Unix()

	switch msg.Type {
	case WSTypeAuth:
		s.handleAuth(client, msg)
	case WSTypePing:
		s.handlePing(client, msg)
	case WSTypeSubscribe:
		s.handleSubscribe(client, msg)
	case WSTypeUnsubscribe:
		s.handleUnsubscribe(client, msg)
	case WSTypeChat:
		s.handleChat(client, msg)
	case WSTypeChatStream:
		s.handleChatStream(client, msg)
	case WSTypeTaskStatus:
		s.handleTaskStatus(client, msg)
	case WSTypeWorkflowExec:
		s.handleWorkflowExec(client, msg)
	default:
		s.sendError(client, msg.ID, "unknown message type: "+string(msg.Type))
	}
}

// handleAuth 处理认证
func (s *WebSocketService) handleAuth(client WebSocketClient, msg WebSocketMessage) {
	// 这里可以添加更复杂的认证逻辑
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		s.sendError(client, msg.ID, "invalid auth payload")
		return
	}

	userID, _ := payload["user_id"].(string)
	if userID != "" {
		s.mu.Lock()
		// 更新用户关联
		if s.userClients[userID] == nil {
			s.userClients[userID] = make(map[string]bool)
		}
		s.userClients[userID][client.GetID()] = true
		s.mu.Unlock()
	}

	s.sendToClient(client, WebSocketMessage{
		Type: WSTypePong,
		ID:   msg.ID,
		Payload: map[string]interface{}{
			"authenticated": true,
		},
	})
}

// handlePing 处理心跳
func (s *WebSocketService) handlePing(client WebSocketClient, msg WebSocketMessage) {
	s.sendToClient(client, WebSocketMessage{
		Type: WSTypePong,
		ID:   msg.ID,
	})
}

// handleSubscribe 处理订阅
func (s *WebSocketService) handleSubscribe(client WebSocketClient, msg WebSocketMessage) {
	channel, ok := msg.Payload.(string)
	if !ok {
		if m, ok := msg.Payload.(map[string]interface{}); ok {
			channel, _ = m["channel"].(string)
		}
	}
	if channel == "" {
		s.sendError(client, msg.ID, "channel is required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 添加频道订阅
	if s.channels[channel] == nil {
		s.channels[channel] = make(map[string]bool)
	}
	s.channels[channel][client.GetID()] = true

	// 更新客户端的频道列表
	if impl, ok := client.(*WebSocketClientImpl); ok {
		impl.AddChannel(channel)
	}

	s.sendToClient(client, WebSocketMessage{
		Type: WSTypePong,
		ID:   msg.ID,
		Payload: map[string]interface{}{
			"subscribed": channel,
		},
	})
}

// handleUnsubscribe 处理取消订阅
func (s *WebSocketService) handleUnsubscribe(client WebSocketClient, msg WebSocketMessage) {
	channel, ok := msg.Payload.(string)
	if !ok {
		if m, ok := msg.Payload.(map[string]interface{}); ok {
			channel, _ = m["channel"].(string)
		}
	}
	if channel == "" {
		s.sendError(client, msg.ID, "channel is required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.channels[channel] != nil {
		delete(s.channels[channel], client.GetID())
		if len(s.channels[channel]) == 0 {
			delete(s.channels, channel)
		}
	}

	// 更新客户端的频道列表
	if impl, ok := client.(*WebSocketClientImpl); ok {
		impl.RemoveChannel(channel)
	}

	s.sendToClient(client, WebSocketMessage{
		Type: WSTypePong,
		ID:   msg.ID,
		Payload: map[string]interface{}{
			"unsubscribed": channel,
		},
	})
}

// handleChat 处理聊天
func (s *WebSocketService) handleChat(client WebSocketClient, msg WebSocketMessage) {
	// 记录活动 (Hermes Agent 基于活动超时)
	if s.ActivityTracker != nil {
		s.ActivityTracker.RecordActivity(client.GetID())
	}

	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		s.sendError(client, msg.ID, "invalid chat payload")
		return
	}

	dialogueID, _ := payload["dialogue_id"].(string)
	content, _ := payload["content"].(string)
	modelID, _ := payload["model_id"].(string)

	if dialogueID == "" || content == "" {
		s.sendError(client, msg.ID, "dialogue_id and content are required")
		return
	}

	// 发送消息并获取响应
	message, err := s.dialogueService.SendMessage(nil, dialogueID, client.GetUserID(), content, modelID, nil)
	if err != nil {
		s.sendError(client, msg.ID, err.Error())
		return
	}

	s.sendToClient(client, WebSocketMessage{
		Type: WSTypeChatResponse,
		ID:   msg.ID,
		Payload: map[string]interface{}{
			"message": message,
		},
	})
}

// handleChatStream 处理流式聊天
func (s *WebSocketService) handleChatStream(client WebSocketClient, msg WebSocketMessage) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		s.sendError(client, msg.ID, "invalid chat payload")
		return
	}

	dialogueID, _ := payload["dialogue_id"].(string)
	content, _ := payload["content"].(string)
	modelID, _ := payload["model_id"].(string)

	if dialogueID == "" || content == "" {
		s.sendError(client, msg.ID, "dialogue_id and content are required")
		return
	}

	// 获取流式响应
	chunkChan, err := s.dialogueService.SendMessageStream(nil, dialogueID, client.GetUserID(), content, modelID, nil)
	if err != nil {
		s.sendError(client, msg.ID, err.Error())
		return
	}

	// 异步发送流式数据
	go func() {
		for chunk := range chunkChan {
			if chunk.Error != nil {
				s.sendToClient(client, WebSocketMessage{
					Type: WSTypeError,
					ID:   msg.ID,
					Payload: map[string]interface{}{
						"error": chunk.Error.Error(),
					},
				})
				return
			}

			if len(chunk.Choices) > 0 {
				isDone := chunk.Choices[0].FinishReason != ""
				s.sendToClient(client, WebSocketMessage{
					Type: WSTypeChatChunk,
					ID:   msg.ID,
					Payload: map[string]interface{}{
						"content": chunk.Choices[0].Delta.Content,
						"done":    isDone,
					},
				})

				if isDone {
					s.sendToClient(client, WebSocketMessage{
						Type: WSTypeChatComplete,
						ID:   msg.ID,
						Payload: map[string]interface{}{
							"model": chunk.Model,
						},
					})
				}
			}
		}
	}()
}

// handleTaskStatus 处理任务状态查询
func (s *WebSocketService) handleTaskStatus(client WebSocketClient, msg WebSocketMessage) {
	taskID, ok := msg.Payload.(string)
	if !ok {
		if m, ok := msg.Payload.(map[string]interface{}); ok {
			taskID, _ = m["task_id"].(string)
		}
	}

	if taskID == "" {
		s.sendError(client, msg.ID, "task_id is required")
		return
	}

	task, err := s.taskService.GetTask(taskID)
	if err != nil {
		s.sendError(client, msg.ID, err.Error())
		return
	}

	s.sendToClient(client, WebSocketMessage{
		Type: WSTypeTaskUpdate,
		ID:   msg.ID,
		Payload: map[string]interface{}{
			"task": task,
		},
	})
}

// handleWorkflowExec 处理工作流执行
func (s *WebSocketService) handleWorkflowExec(client WebSocketClient, msg WebSocketMessage) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		s.sendError(client, msg.ID, "invalid workflow payload")
		return
	}

	workflowID, _ := payload["workflow_id"].(string)
	inputVars, _ := payload["input_variables"].(map[string]interface{})

	if workflowID == "" {
		s.sendError(client, msg.ID, "workflow_id is required")
		return
	}

	// 创建工作流实例
	instance, found := s.workflowService.CreateWorkflowInstance(workflowID, inputVars)
	if !found {
		s.sendError(client, msg.ID, "workflow not found")
		return
	}

	// 异步执行工作流
	go func() {
		executedInstance, err := s.workflowService.ExecuteWorkflowInstance(instance.ID)
		if err != nil {
			s.sendToClient(client, WebSocketMessage{
				Type: WSTypeError,
				ID:   msg.ID,
				Payload: map[string]interface{}{
					"error": err.Error(),
				},
			})
			return
		}

		s.sendToClient(client, WebSocketMessage{
			Type: WSTypeWorkflowUpdate,
			ID:   msg.ID,
			Payload: map[string]interface{}{
				"instance": executedInstance,
			},
		})
	}()

	// 立即返回实例创建结果
	s.sendToClient(client, WebSocketMessage{
		Type: WSTypeWorkflowUpdate,
		ID:   msg.ID,
		Payload: map[string]interface{}{
			"instance": instance,
			"status":   "started",
		},
	})
}

// Broadcast 广播消息到所有客户端
func (s *WebSocketService) Broadcast(msg WebSocketMessage) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	for _, client := range s.clients {
		client.SendMessage(data)
	}
}

// BroadcastToChannel 广播消息到指定频道
func (s *WebSocketService) BroadcastToChannel(channel string, msg WebSocketMessage) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clientIDs, ok := s.channels[channel]
	if !ok {
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	for clientID := range clientIDs {
		if client, ok := s.clients[clientID]; ok {
			client.SendMessage(data)
		}
	}
}

// SendToUser 发送消息给指定用户
func (s *WebSocketService) SendToUser(userID string, msg WebSocketMessage) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clientIDs, ok := s.userClients[userID]
	if !ok {
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	for clientID := range clientIDs {
		if client, ok := s.clients[clientID]; ok {
			client.SendMessage(data)
		}
	}
}

// sendToClient 发送消息给指定客户端
func (s *WebSocketService) sendToClient(client WebSocketClient, msg WebSocketMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	client.SendMessage(data)
}

// sendError 发送错误消息
func (s *WebSocketService) sendError(client WebSocketClient, msgID string, errMsg string) {
	s.sendToClient(client, WebSocketMessage{
		Type: WSTypeError,
		ID:   msgID,
		Payload: map[string]interface{}{
			"error": errMsg,
		},
	})
}

// heartbeatChecker 心跳检查
func (s *WebSocketService) heartbeatChecker() {
	ticker := time.NewTicker(s.pingInterval)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.RLock()
		now := time.Now()
		for _, client := range s.clients {
			if now.Sub(client.GetLastActivity()) > s.pongWait {
				// 超时断开
				go func(c WebSocketClient) {
					log.Printf("[WS] Client timeout: %s", c.GetID())
					c.CloseConn()
				}(client)
			} else {
				// 发送心跳
				s.sendToClient(client, WebSocketMessage{
					Type: WSTypePing,
				})
			}
		}
		s.mu.RUnlock()
	}
}

// GetClientCount 获取客户端数量
func (s *WebSocketService) GetClientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}

// GetChannelCount 获取频道数量
func (s *WebSocketService) GetChannelCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.channels)
}

// NotifyTaskUpdate 通知任务状态更新
func (s *WebSocketService) NotifyTaskUpdate(taskID string, status string, data interface{}) {
	s.Broadcast(WebSocketMessage{
		Type: WSTypeTaskUpdate,
		Payload: map[string]interface{}{
			"task_id": taskID,
			"status":  status,
			"data":    data,
		},
	})
}

// NotifyWorkflowUpdate 通知工作流状态更新
func (s *WebSocketService) NotifyWorkflowUpdate(instanceID string, status string, data interface{}) {
	s.Broadcast(WebSocketMessage{
		Type: WSTypeWorkflowUpdate,
		Payload: map[string]interface{}{
			"instance_id": instanceID,
			"status":      status,
			"data":        data,
		},
	})
}

// SendNotification 发送通知
func (s *WebSocketService) SendNotification(userID string, title string, content string) {
	s.SendToUser(userID, WebSocketMessage{
		Type: WSTypeNotification,
		Payload: map[string]interface{}{
			"title":   title,
			"content": content,
			"time":    time.Now().Unix(),
		},
	})
}

// NotifyTaskCompleted 通知后台任务完成 (Hermes Agent notify_on_complete)
func (s *WebSocketService) NotifyTaskCompleted(userID string, taskID string, status string, result interface{}) {
	s.SendToUser(userID, WebSocketMessage{
		Type: WSTypeTaskCompleted,
		Timestamp: time.Now().Unix(),
		Payload: map[string]interface{}{
			"task_id":      taskID,
			"status":       status,
			"result":       result,
			"completed_at": time.Now().Format(time.RFC3339),
		},
	})
}

// ActivityTracker 基于活动的超时跟踪器 (Hermes Agent 智能超时)
type ActivityTracker struct {
	mu              sync.RWMutex
	lastActivity    map[string]time.Time // sessionID -> 最后活动时间
	activityTimeout time.Duration
	onTimeout       func(sessionID string)
}

// NewActivityTracker 创建活动跟踪器
func NewActivityTracker(activityTimeout time.Duration, onTimeout func(sessionID string)) *ActivityTracker {
	tracker := &ActivityTracker{
		lastActivity:    make(map[string]time.Time),
		activityTimeout: activityTimeout,
		onTimeout:       onTimeout,
	}
	go tracker.startCleanupLoop(5 * time.Minute)
	return tracker
}

// RecordActivity 记录活动 (工具调用/消息发送)
func (t *ActivityTracker) RecordActivity(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastActivity[sessionID] = time.Now()
}

// ShouldTimeout 检查是否应超时
func (t *ActivityTracker) ShouldTimeout(sessionID string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	lastActive, exists := t.lastActivity[sessionID]
	if !exists {
		return true
	}
	return time.Since(lastActive) > t.activityTimeout
}

// RemoveSession 移除会话
func (t *ActivityTracker) RemoveSession(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.lastActivity, sessionID)
}

// startCleanupLoop 定期清理超时会话
func (t *ActivityTracker) startCleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		t.cleanupExpired()
	}
}

// cleanupExpired 清理过期会话
func (t *ActivityTracker) cleanupExpired() {
	t.mu.Lock()
	defer t.mu.Unlock()

	var timedOut []string
	now := time.Now()
	for sessionID, lastActive := range t.lastActivity {
		if now.Sub(lastActive) > t.activityTimeout {
			timedOut = append(timedOut, sessionID)
		}
	}

	for _, sessionID := range timedOut {
		delete(t.lastActivity, sessionID)
		if t.onTimeout != nil {
			go t.onTimeout(sessionID)
		}
	}
}

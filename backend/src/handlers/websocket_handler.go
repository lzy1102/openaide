package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"openaide/backend/src/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// 生产环境应该检查 Origin
		return true
	},
}

// WebSocketHandler WebSocket 处理器
type WebSocketHandler struct {
	wsService *services.WebSocketService
}

// NewWebSocketHandler 创建 WebSocket 处理器
func NewWebSocketHandler(wsService *services.WebSocketService) *WebSocketHandler {
	return &WebSocketHandler{
		wsService: wsService,
	}
}

// HandleWebSocket 处理 WebSocket 连接
func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	// 升级 HTTP 连接为 WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WS] Upgrade error: %v", err)
		return
	}

	// 创建客户端
	clientID := uuid.New().String()
	userID := c.Query("user_id")
	if userID == "" {
		userID = c.GetHeader("X-User-ID")
	}

	client := &services.WebSocketClientImpl{
		ID:           clientID,
		UserID:       userID,
		Channels:     make(map[string]bool),
		LastActivity: time.Now(),
	}
	client.SetSendFunc(func(message []byte) error {
		return conn.WriteMessage(websocket.TextMessage, message)
	})
	client.SetCloseFunc(func() error {
		return conn.Close()
	})

	// 注册客户端
	h.wsService.RegisterClient(client)

	// 读取消息
	go h.readMessages(client, conn)

	// 发送心跳
	go h.sendHeartbeat(client, conn)
}

// readMessages 读取客户端消息
func (h *WebSocketHandler) readMessages(client *services.WebSocketClientImpl, conn *websocket.Conn) {
	defer func() {
		h.wsService.UnregisterClient(client.GetID())
		conn.Close()
	}()

	conn.SetReadLimit(1024 * 1024) // 1MB
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		client.SetLastActivity(time.Now())
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WS] Read error: %v", err)
			}
			break
		}

		client.SetLastActivity(time.Now())
		h.wsService.HandleMessage(client, message)
	}
}

// sendHeartbeat 发送心跳
func (h *WebSocketHandler) sendHeartbeat(client *services.WebSocketClientImpl, conn *websocket.Conn) {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		conn.Close()
	}()

	for range ticker.C {
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
			log.Printf("[WS] Ping error: %v", err)
			break
		}
	}
}

// HandleWebSocketStats 获取 WebSocket 统计信息
func (h *WebSocketHandler) HandleWebSocketStats(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"client_count":  h.wsService.GetClientCount(),
		"channel_count": h.wsService.GetChannelCount(),
	})
}

// HandleBroadcast 广播消息 (管理员接口)
func (h *WebSocketHandler) HandleBroadcast(c *gin.Context) {
	var req struct {
		Type    string      `json:"type" binding:"required"`
		Channel string      `json:"channel"`
		Payload interface{} `json:"payload" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	msg := services.WebSocketMessage{
		Type:      services.WebSocketMessageType(req.Type),
		Payload:   req.Payload,
		Timestamp: time.Now().Unix(),
	}

	if req.Channel != "" {
		h.wsService.BroadcastToChannel(req.Channel, msg)
	} else {
		h.wsService.Broadcast(msg)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "broadcast sent",
	})
}

// HandleSendToUser 发送消息给指定用户 (管理员接口)
func (h *WebSocketHandler) HandleSendToUser(c *gin.Context) {
	var req struct {
		UserID  string      `json:"user_id" binding:"required"`
		Type    string      `json:"type" binding:"required"`
		Payload interface{} `json:"payload" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.wsService.SendToUser(req.UserID, services.WebSocketMessage{
		Type:      services.WebSocketMessageType(req.Type),
		Payload:   req.Payload,
		Timestamp: time.Now().Unix(),
	})

	c.JSON(http.StatusOK, gin.H{
		"message": "message sent",
	})
}

// HandleNotifyTask 通知任务更新 (内部接口)
func (h *WebSocketHandler) HandleNotifyTask(c *gin.Context) {
	taskID := c.Param("id")
	var req struct {
		Status string      `json:"status" binding:"required"`
		Data   interface{} `json:"data"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.wsService.NotifyTaskUpdate(taskID, req.Status, req.Data)

	c.JSON(http.StatusOK, gin.H{
		"message": "notification sent",
	})
}

// DialogueStreamHandler 对话流式处理 (使用 WebSocket)
func (h *WebSocketHandler) DialogueStreamHandler(dialogueService *services.DialogueService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 升级连接
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("[WS] Upgrade error: %v", err)
			return
		}
		defer conn.Close()

		// 读取请求
		_, message, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var req struct {
			DialogueID string                 `json:"dialogue_id"`
			UserID     string                 `json:"user_id"`
			Content    string                 `json:"content"`
			ModelID    string                 `json:"model_id"`
			Options    map[string]interface{} `json:"options"`
		}

		if err := parseJSON(message, &req); err != nil {
			sendWSError(conn, "", "invalid request")
			return
		}

		// 发送流式消息
		ctx := context.Background()
		chunkChan, err := dialogueService.SendMessageStream(ctx, req.DialogueID, req.UserID, req.Content, req.ModelID, req.Options)
		if err != nil {
			sendWSError(conn, "", err.Error())
			return
		}

		for chunk := range chunkChan {
			if chunk.Error != nil {
				sendWSError(conn, "", chunk.Error.Error())
				return
			}

			if len(chunk.Choices) > 0 {
				msg := services.WebSocketMessage{
					Type: services.WSTypeChatChunk,
					Payload: map[string]interface{}{
						"content":       chunk.Choices[0].Delta.Content,
						"finish_reason": chunk.Choices[0].FinishReason,
						"model":         chunk.Model,
					},
				}
				data, _ := encodeJSON(msg)
				conn.WriteMessage(websocket.TextMessage, data)
			}
		}

		// 发送完成消息
		completeMsg := services.WebSocketMessage{
			Type:    services.WSTypeChatComplete,
			Payload: map[string]interface{}{"done": true},
		}
		data, _ := encodeJSON(completeMsg)
		conn.WriteMessage(websocket.TextMessage, data)
	}
}

// 辅助函数
func parseJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func encodeJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func sendWSError(conn *websocket.Conn, id string, errMsg string) {
	msg := services.WebSocketMessage{
		Type: services.WSTypeError,
		ID:   id,
		Payload: map[string]interface{}{
			"error": errMsg,
		},
	}
	data, _ := encodeJSON(msg)
	conn.WriteMessage(websocket.TextMessage, data)
}

package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// TelegramBot Telegram机器人渠道
//
// 工作流：
//
//	Telegram → Webhook POST → /api/v1/channels/{id}/webhook
//	    → 解析 Update → 提取 Message → MessageHandler
//	    → Send() → Telegram sendMessage API
//
// 依赖 Telegram Bot API:
//   - setWebhook: https://api.telegram.org/bot{token}/setWebhook
//   - sendMessage: https://api.telegram.org/bot{token}/sendMessage
type TelegramBot struct {
	id          string
	name        string
	token       string
	handler     MessageHandler
	handlerMu   sync.RWMutex
	started     bool
	startedMu   sync.RWMutex
	regFn       HTTPHandler
	prefix      string
	apiBaseURL  string
	client      *http.Client
}

// TelegramConfig Telegram机器人配置
type TelegramConfig struct {
	ID    string `json:"id" yaml:"id"`
	Name  string `json:"name" yaml:"name"`
	Token string `json:"token" yaml:"token"`
}

// NewTelegramBot 创建Telegram机器人渠道
func NewTelegramBot(cfg TelegramConfig, regFn HTTPHandler, prefix string) *TelegramBot {
	if prefix == "" {
		prefix = "/api/v1/channels"
	}
	return &TelegramBot{
		id:         cfg.ID,
		name:       cfg.Name,
		token:      cfg.Token,
		apiBaseURL: fmt.Sprintf("https://api.telegram.org/bot%s", cfg.Token),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		regFn:  regFn,
		prefix: strings.TrimRight(prefix, "/"),
	}
}

// ID 实现 Channel 接口
func (t *TelegramBot) ID() string { return t.id }

// Name 实现 Channel 接口
func (t *TelegramBot) Name() string { return t.name }

// Type 实现 Channel 接口
func (t *TelegramBot) Type() ChannelType { return TypeTelegram }

// Send 实现 Channel 接口 — 通过Telegram API发送消息
func (t *TelegramBot) Send(ctx context.Context, targetID string, resp *Response) error {
	body := map[string]interface{}{
		"chat_id": targetID,
		"text":    resp.Content,
		"parse_mode": "HTML",
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal telegram request body: %w", err)
	}
	url := fmt.Sprintf("%s/sendMessage", t.apiBaseURL)

	req, err2 := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err2 != nil {
		return fmt.Errorf("create telegram request: %w", err2)
	}
	req.Header.Set("Content-Type", "application/json")

	respHTTP, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram send message failed: %w", err)
	}
	defer respHTTP.Body.Close()

	if respHTTP.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(respHTTP.Body)
		return fmt.Errorf("telegram API returned %d: %s", respHTTP.StatusCode, string(bodyBytes))
	}
	return nil
}

// Start 实现 Channel 接口 — 注册Telegram Webhook端点
func (t *TelegramBot) Start(ctx context.Context, handler MessageHandler) error {
	t.startedMu.Lock()
	defer t.startedMu.Unlock()

	if t.started {
		return fmt.Errorf("telegram bot %q already started", t.id)
	}

	t.handlerMu.Lock()
	t.handler = handler
	t.handlerMu.Unlock()

	pattern := fmt.Sprintf("%s/%s/webhook", t.prefix, t.id)
	if t.regFn != nil {
		t.regFn(pattern, t.serveHTTP)
		slog.Info("Telegram bot endpoint registered",
			"channel", t.id,
			"pattern", pattern,
		)
	}

	t.started = true
	return nil
}

// Stop 实现 Channel 接口
func (t *TelegramBot) Stop(ctx context.Context) error {
	t.startedMu.Lock()
	defer t.startedMu.Unlock()
	t.started = false
	t.handlerMu.Lock()
	t.handler = nil
	t.handlerMu.Unlock()
	return nil
}

// Status 实现 Channel 接口
func (t *TelegramBot) Status(ctx context.Context) Status {
	t.startedMu.RLock()
	running := t.started
	t.startedMu.RUnlock()
	return Status{
		ID:      t.id,
		Type:    TypeTelegram,
		Running: running,
		Healthy: running,
	}
}

// SetWebhook 向Telegram API注册Webhook地址
// 外部地址如: https://your-domain.com/api/v1/channels/{id}/webhook
func (t *TelegramBot) SetWebhook(ctx context.Context, webhookURL string) error {
	body := map[string]string{
		"url": webhookURL,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal setWebhook body: %w", err)
	}

	url := fmt.Sprintf("%s/setWebhook", t.apiBaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram setWebhook: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("parse setWebhook response: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("telegram setWebhook failed: %s", result.Description)
	}
	return nil
}

// serveHTTP 处理Telegram Webhook回调
//
// Telegram Update 结构:
//
//	{
//	  "update_id": 12345,
//	  "message": {
//	    "message_id": 1,
//	    "from": { "id": 67890, "first_name": "User" },
//	    "chat": { "id": 67890 },
//	    "text": "hello"
//	  }
//	}
func (t *TelegramBot) serveHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(rw, "Failed to read body", http.StatusBadRequest)
		return
	}

	// 解析Telegram Update
	var update struct {
		UpdateID int `json:"update_id"`
		Message  *struct {
			MessageID int `json:"message_id"`
			From      struct {
				ID        int64  `json:"id"`
				FirstName string `json:"first_name"`
				Username  string `json:"username,omitempty"`
			} `json:"from"`
			Chat struct {
				ID int64 `json:"id"`
			} `json:"chat"`
			Text string `json:"text,omitempty"`
		} `json:"message"`
	}
	if err := json.Unmarshal(body, &update); err != nil {
		http.Error(rw, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// 忽略非文本消息（如图片、贴纸）
	if update.Message == nil || update.Message.Text == "" {
		rw.WriteHeader(http.StatusOK)
		return
	}

	msg := &Message{
		ID:         fmt.Sprintf("tg_%d_%d", update.UpdateID, update.Message.MessageID),
		ChannelID:  t.id,
		Type:       TypeTelegram,
		UserID:     fmt.Sprintf("%d", update.Message.Chat.ID),
		Content:    update.Message.Text,
		Raw:        body,
		ReceivedAt: time.Now(),
	}

	t.handlerMu.RLock()
	handler := t.handler
	t.handlerMu.RUnlock()

	if handler == nil {
		slog.Error("Telegram handler not initialized", "channel", t.id)
		http.Error(rw, "Service not ready", http.StatusServiceUnavailable)
		return
	}

	resp, err := handler(r.Context(), msg)
	if err != nil {
		slog.Error("Message handler failed", "channel", t.id, "error", err)
		rw.WriteHeader(http.StatusOK)
		return
	}

	// 自动发送回复
	if resp != nil && resp.Content != "" {
		if sendErr := t.Send(r.Context(), msg.UserID, resp); sendErr != nil {
			slog.Warn("Failed to send telegram reply", "channel", t.id, "error", sendErr)
		}
	}

	rw.WriteHeader(http.StatusOK)
}



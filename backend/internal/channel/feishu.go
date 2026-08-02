package channel

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// FeishuBot 飞书机器人渠道
//
// 工作流：
//
//	飞书 → POST 回调 → /api/v1/channels/{id}/webhook
//	    → 解密事件 → 解析消息 → MessageHandler
//	    → Send() → 飞书 SendMessage API
//
// 依赖飞书开放平台:
//   - 事件订阅（Webhook）
//   - 发送消息 API（https://open.feishu.cn/open-apis/im/v1/messages）
type FeishuBot struct {
	id          string
	name        string
	appID       string
	appSecret   string
	verifyToken string // 飞书事件验证令牌
	aesKey      string // 飞书事件推送加密密钥（空=不加密）
	handler     MessageHandler
	handlerMu   sync.RWMutex
	started     bool
	startedMu   sync.RWMutex
	client      *http.Client
	regFn       HTTPHandler
	prefix      string

	// 飞书API token缓存
	tokenCache struct {
		mu        sync.RWMutex
		token     string
		expiresAt time.Time
	}
}

// FeishuConfig 飞书机器人配置
type FeishuConfig struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	AppID       string `json:"app_id" yaml:"app_id"`
	AppSecret   string `json:"app_secret" yaml:"app_secret"`
	VerifyToken string `json:"verify_token" yaml:"verify_token"`
	AESKey      string `json:"aes_key" yaml:"aes_key"`
}

// NewFeishuBot 创建飞书机器人渠道
func NewFeishuBot(cfg FeishuConfig, regFn HTTPHandler, prefix string) *FeishuBot {
	if prefix == "" {
		prefix = "/api/v1/channels"
	}
	return &FeishuBot{
		id:          cfg.ID,
		name:        cfg.Name,
		appID:       cfg.AppID,
		appSecret:   cfg.AppSecret,
		verifyToken: cfg.VerifyToken,
		aesKey:      cfg.AESKey,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		regFn:  regFn,
		prefix: strings.TrimRight(prefix, "/"),
	}
}

// ID 实现 Channel 接口
func (f *FeishuBot) ID() string { return f.id }

// Name 实现 Channel 接口
func (f *FeishuBot) Name() string { return f.name }

// Type 实现 Channel 接口
func (f *FeishuBot) Type() ChannelType { return TypeFeishu }

// Send 实现 Channel 接口 — 通过飞书API发送消息
func (f *FeishuBot) Send(ctx context.Context, targetID string, resp *Response) error {
	token, err := f.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("get feishu token: %w", err)
	}

	// 飞书消息格式
	msgContent, err := json.Marshal(map[string]string{
		"text": resp.Content,
	})
	if err != nil {
		return fmt.Errorf("marshal feishu message content: %w", err)
	}

	body := map[string]interface{}{
		"receive_id": targetID,
		"msg_type":   "text",
		"content":    string(msgContent),
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal feishu request body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx,
		http.MethodPost,
		"https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=open_id",
		bytes.NewReader(data),
	)
	if err != nil {
		return fmt.Errorf("create feishu send request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	respHTTP, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("feishu send request failed: %w", err)
	}
	defer respHTTP.Body.Close()

	if respHTTP.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(respHTTP.Body)
		return fmt.Errorf("feishu API returned %d: %s", respHTTP.StatusCode, string(bodyBytes))
	}
	return nil
}

// Start 实现 Channel 接口 — 注册飞书事件回调端点
func (f *FeishuBot) Start(ctx context.Context, handler MessageHandler) error {
	f.startedMu.Lock()
	defer f.startedMu.Unlock()

	if f.started {
		return fmt.Errorf("feishu bot %q already started", f.id)
	}

	f.handlerMu.Lock()
	f.handler = handler
	f.handlerMu.Unlock()

	pattern := fmt.Sprintf("%s/%s/webhook", f.prefix, f.id)
	if f.regFn != nil {
		f.regFn(pattern, f.serveHTTP)
		slog.Info("Feishu bot endpoint registered",
			"channel", f.id,
			"pattern", pattern,
		)
	}

	f.started = true
	return nil
}

// Stop 实现 Channel 接口
func (f *FeishuBot) Stop(ctx context.Context) error {
	f.startedMu.Lock()
	defer f.startedMu.Unlock()
	f.started = false
	f.handlerMu.Lock()
	f.handler = nil
	f.handlerMu.Unlock()
	return nil
}

// Status 实现 Channel 接口
func (f *FeishuBot) Status(ctx context.Context) Status {
	f.startedMu.RLock()
	running := f.started
	f.startedMu.RUnlock()
	return Status{
		ID:      f.id,
		Type:    TypeFeishu,
		Running: running,
		Healthy: running,
		Info: map[string]interface{}{
			"app_id": f.appID,
		},
	}
}

// serveHTTP 处理飞书事件回调
//
// 飞书事件结构:
//
//	{
//	  "encrypt": "encrypted_data",     // 启用了加密时有
//	  "challenge": "xxx",              // URL验证
//	  "token": "verify_token",
//	  "type": "event_callback" | "url_verification",
//	  "event": {
//	    "type": "im.message.receive_v1",
//	    "message": {
//	      "chat_type": "p2p" | "group",
//	      "message_type": "text",
//	      "content": "{\"text\":\"hello\"}"
//	    },
//	    "sender": { "sender_id": { "open_id": "xxx" } }
//	  }
//	}
func (f *FeishuBot) serveHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(rw, "Failed to read body", http.StatusBadRequest)
		return
	}

	// 解析飞书事件
	var feishuEvent struct {
		Encrypt   string          `json:"encrypt"`
		Challenge string          `json:"challenge"`
		Token     string          `json:"token"`
		Type      string          `json:"type"`
		Event     json.RawMessage `json:"event,omitempty"`
	}
	if err := json.Unmarshal(body, &feishuEvent); err != nil {
		http.Error(rw, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// URL验证挑战
	if feishuEvent.Type == "url_verification" {
		rw.Header().Set("Content-Type", "application/json")
		json.NewEncoder(rw).Encode(map[string]string{
			"challenge": feishuEvent.Challenge,
		})
		return
	}

	// Token验证
	if f.verifyToken != "" && feishuEvent.Token != f.verifyToken {
		slog.Warn("Invalid feishu verify token", "channel", f.id)
		http.Error(rw, "Invalid token", http.StatusUnauthorized)
		return
	}

	// 解密事件数据（如果加密）
	eventData := feishuEvent.Event
	if feishuEvent.Encrypt != "" && f.aesKey != "" {
		decrypted, err := f.decryptEvent(feishuEvent.Encrypt)
		if err != nil {
			slog.Error("Feishu decrypt failed", "channel", f.id, "error", err)
			http.Error(rw, "Decrypt failed", http.StatusBadRequest)
			return
		}
		eventData = decrypted
	}

	// 解析消息内容
	msg, err := f.parseMessage(eventData)
	if err != nil {
		slog.Debug("Feishu non-message event", "channel", f.id, "error", err)
		rw.WriteHeader(http.StatusOK)
		return
	}

	// 调用消息处理器
	f.handlerMu.RLock()
	handler := f.handler
	f.handlerMu.RUnlock()

	if handler == nil {
		slog.Error("Feishu handler not initialized", "channel", f.id)
		http.Error(rw, "Service not ready", http.StatusServiceUnavailable)
		return
	}

	resp, err := handler(r.Context(), msg)
	if err != nil {
		slog.Error("Message handler failed", "channel", f.id, "error", err)
		// 飞书要求200响应，即使处理失败
		rw.WriteHeader(http.StatusOK)
		return
	}

	// 自动发送回复
	if resp != nil && resp.Content != "" {
		if sendErr := f.Send(r.Context(), msg.UserID, resp); sendErr != nil {
			slog.Warn("Failed to send feishu reply", "channel", f.id, "error", sendErr)
		}
	}

	rw.WriteHeader(http.StatusOK)
}

// parseMessage 从飞书事件中提取消息
func (f *FeishuBot) parseMessage(eventData json.RawMessage) (*Message, error) {
	var event struct {
		Type   string `json:"type"`
		Sender struct {
			SenderID struct {
				OpenID string `json:"open_id"`
			} `json:"sender_id"`
		} `json:"sender"`
		Message struct {
			MessageID   string `json:"message_id"`
			MessageType string `json:"message_type"`
			Content     string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(eventData, &event); err != nil {
		return nil, err
	}

	// 只处理文本消息
	if event.Type != "im.message.receive_v1" || event.Message.MessageType != "text" {
		return nil, fmt.Errorf("unhandled event type: %s/%s", event.Type, event.Message.MessageType)
	}

	// 飞书文本内容格式: {"text":"hello"}
	var textContent struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(event.Message.Content), &textContent); err != nil {
		return nil, fmt.Errorf("parse text content: %w", err)
	}

	return &Message{
		ID:         event.Message.MessageID,
		ChannelID:  f.id,
		Type:       TypeFeishu,
		UserID:     event.Sender.SenderID.OpenID,
		Content:    textContent.Text,
		Raw:        eventData,
		ReceivedAt: time.Now(),
	}, nil
}

// getAccessToken 获取飞书API访问令牌（自动缓存刷新）
func (f *FeishuBot) getAccessToken(ctx context.Context) (string, error) {
	f.tokenCache.mu.RLock()
	if f.tokenCache.token != "" && time.Now().Before(f.tokenCache.expiresAt) {
		token := f.tokenCache.token
		f.tokenCache.mu.RUnlock()
		return token, nil
	}
	f.tokenCache.mu.RUnlock()

	f.tokenCache.mu.Lock()
	defer f.tokenCache.mu.Unlock()

	// 双重检查
	if f.tokenCache.token != "" && time.Now().Before(f.tokenCache.expiresAt) {
		return f.tokenCache.token, nil
	}

	body := map[string]string{
		"app_id":     f.appID,
		"app_secret": f.appSecret,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal feishu auth body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal",
		bytes.NewReader(data),
	)
	if err != nil {
		return "", fmt.Errorf("create feishu auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("feishu auth request: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("parse feishu auth response: %w", err)
	}
	if result.Code != 0 {
		return "", fmt.Errorf("feishu auth error: %s", result.Msg)
	}

	f.tokenCache.token = result.TenantAccessToken
	f.tokenCache.expiresAt = time.Now().Add(time.Duration(result.Expire-60) * time.Second)

	return f.tokenCache.token, nil
}

// decryptEvent AES解密飞书事件
func (f *FeishuBot) decryptEvent(encrypted string) (json.RawMessage, error) {
	key := sha256.Sum256([]byte(f.aesKey))
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("aes new cipher: %w", err)
	}

	if len(ciphertext) < aes.BlockSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	stream := cipher.NewCBCDecrypter(block, iv)
	stream.CryptBlocks(ciphertext, ciphertext)

	// PKCS7Unpadding — validate ALL padding bytes
	padLen := int(ciphertext[len(ciphertext)-1])
	if padLen < 1 || padLen > aes.BlockSize || padLen > len(ciphertext) {
		return nil, fmt.Errorf("invalid padding length %d", padLen)
	}
	for i := len(ciphertext) - padLen; i < len(ciphertext); i++ {
		if int(ciphertext[i]) != padLen {
			return nil, fmt.Errorf("invalid PKCS7 padding at byte %d", i)
		}
	}
	ciphertext = ciphertext[:len(ciphertext)-padLen]

	return ciphertext, nil
}

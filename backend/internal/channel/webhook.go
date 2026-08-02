package channel

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// HTTPHandler 注册HTTP处理器的函数类型
// 由API Server调用，将渠道的Webhook端点挂载到主Mux
type HTTPHandler func(pattern string, handler http.HandlerFunc)

// WebhookReceiver 通用HTTP Webhook渠道
//
// 工作流:
//
//	外部系统 POST → /api/v1/channels/{id}/webhook
//	   → 签名验证 → 解析消息 → MessageHandler
//	   → Send() 回传响应（POST到 configured callback_url）
type WebhookReceiver struct {
	id          string
	name        string
	secretToken string // HMAC-SHA256 签名密钥（空=不验证）
	callbackURL string // 响应回调地址（空=不回传）
	client      *http.Client
	handler     MessageHandler
	handlerMu   sync.RWMutex
	started     bool
	startedMu   sync.RWMutex
	regFn       HTTPHandler // 注册HTTP处理器的函数
	prefix      string      // URL路径前缀
}

// WebhookConfig Webhook渠道配置
type WebhookConfig struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	SecretToken string `json:"secret_token" yaml:"secret_token"`
	CallbackURL string `json:"callback_url" yaml:"callback_url"`
}

// NewWebhookReceiver 创建Webhook渠道
//
//	regFn: HTTP处理器注册函数（由API Server提供）
//	prefix: URL前缀，如 "/api/v1/channels"
func NewWebhookReceiver(cfg WebhookConfig, regFn HTTPHandler, prefix string) *WebhookReceiver {
	if prefix == "" {
		prefix = "/api/v1/channels"
	}
	return &WebhookReceiver{
		id:          cfg.ID,
		name:        cfg.Name,
		secretToken: cfg.SecretToken,
		callbackURL: cfg.CallbackURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		regFn:  regFn,
		prefix: strings.TrimRight(prefix, "/"),
	}
}

// ID 实现 Channel 接口
func (w *WebhookReceiver) ID() string { return w.id }

// Name 实现 Channel 接口
func (w *WebhookReceiver) Name() string { return w.name }

// Type 实现 Channel 接口
func (w *WebhookReceiver) Type() ChannelType { return TypeWebhook }

const (
	// 总重试次数 = defaultMaxRetries（最后一次尝试+重试回溯）
	defaultMaxRetries    = 3
	defaultRetryBaseWait = time.Second
)

// maxSendAttempts 最大发送尝试次数（1次初始 + defaultMaxRetries次重试）
const maxSendAttempts = defaultMaxRetries + 1

// Send 实现 Channel 接口 — POST回调结果到 callbackURL（带指数退避重试）
func (w *WebhookReceiver) Send(ctx context.Context, targetID string, resp *Response) error {
	if w.callbackURL == "" {
		return nil
	}

	body := map[string]interface{}{
		"target_id": targetID,
		"content":   resp.Content,
		"data":      resp.Data,
		"timestamp": time.Now().Unix(),
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal callback body: %w", err)
	}

	return w.retrySend(ctx, data)
}

// retrySend 带指数退避重试的HTTP回调
func (w *WebhookReceiver) retrySend(ctx context.Context, data []byte) error {
	var lastErr error

	for attempt := 0; attempt < maxSendAttempts; attempt++ {
		if attempt > 0 {
			backoff := defaultRetryBaseWait * (1 << (attempt - 1)) // 1s, 2s, 4s
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.callbackURL, bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("create callback request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		if w.secretToken != "" {
			sig := signPayload(data, w.secretToken)
			req.Header.Set("X-Signature", sig)
		}

		respHTTP, err := w.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("callback request failed (attempt %d/%d): %w", attempt+1, maxSendAttempts, err)
			slog.Warn("Webhook callback failed, will retry", "url", w.callbackURL, "attempt", attempt+1, "error", err)
			continue
		}

		if respHTTP.StatusCode >= 300 {
			bodyBytes, _ := io.ReadAll(respHTTP.Body)
			respHTTP.Body.Close()
			lastErr = fmt.Errorf("callback returned %d (attempt %d/%d): %s", respHTTP.StatusCode, attempt+1, maxSendAttempts, string(bodyBytes))
			slog.Warn("Webhook callback non-2xx, will retry", "url", w.callbackURL, "attempt", attempt+1, "status", respHTTP.StatusCode)
			continue
		}

		respHTTP.Body.Close()
		return nil
	}

	return fmt.Errorf("webhook callback failed after %d retries: %w", defaultMaxRetries, lastErr)
}

// Start 实现 Channel 接口 — 注册HTTP端点
func (w *WebhookReceiver) Start(ctx context.Context, handler MessageHandler) error {
	w.startedMu.Lock()
	defer w.startedMu.Unlock()

	if w.started {
		return fmt.Errorf("webhook receiver %q already started", w.id)
	}

	w.handlerMu.Lock()
	w.handler = handler
	w.handlerMu.Unlock()

	w.started = true

	pattern := fmt.Sprintf("%s/%s/webhook", w.prefix, w.id)
	if w.regFn != nil {
		w.regFn(pattern, w.serveHTTP)
		slog.Info("Webhook endpoint registered",
			"channel", w.id,
			"pattern", pattern,
		)
	}

	return nil
}

// Stop 实现 Channel 接口
func (w *WebhookReceiver) Stop(ctx context.Context) error {
	w.startedMu.Lock()
	defer w.startedMu.Unlock()
	w.started = false
	w.handlerMu.Lock()
	w.handler = nil
	w.handlerMu.Unlock()
	return nil
}

// Status 实现 Channel 接口
func (w *WebhookReceiver) Status(ctx context.Context) Status {
	w.startedMu.RLock()
	running := w.started
	w.startedMu.RUnlock()
	return Status{
		ID:      w.id,
		Type:    TypeWebhook,
		Running: running,
		Healthy: running,
		Info: map[string]interface{}{
			"callback_url": w.callbackURL,
		},
	}
}

// serveHTTP 处理入站Webhook请求
func (w *WebhookReceiver) serveHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(rw, "Failed to read body", http.StatusBadRequest)
		return
	}

	// 签名验证
	if w.secretToken != "" {
		sig := r.Header.Get("X-Signature")
		if sig == "" || !verifySignature(body, w.secretToken, sig) {
			slog.Warn("Invalid webhook signature", "channel", w.id)
			http.Error(rw, "Invalid signature", http.StatusUnauthorized)
			return
		}
	}

	// 解析消息
	msg, err := w.parseMessage(body)
	if err != nil {
		slog.Error("Failed to parse webhook message", "channel", w.id, "error", err)
		http.Error(rw, "Invalid message format", http.StatusBadRequest)
		return
	}

	// 调用消息处理器
	w.handlerMu.RLock()
	handler := w.handler
	w.handlerMu.RUnlock()

	if handler == nil {
		slog.Error("Webhook handler not initialized", "channel", w.id)
		http.Error(rw, "Service not ready", http.StatusServiceUnavailable)
		return
	}

	resp, err := handler(r.Context(), msg)
	if err != nil {
		slog.Error("Message handler failed", "channel", w.id, "error", err)
		w.writeJSON(rw, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	w.writeJSON(rw, http.StatusOK, resp)
}

// parseMessage 从HTTP请求体构建渠道消息
func (w *WebhookReceiver) parseMessage(body []byte) (*Message, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	content, _ := raw["content"].(string)
	if content == "" {
		// 尝试从 message/content/text 字段读取
		if msg, ok := raw["message"].(map[string]interface{}); ok {
			content, _ = msg["content"].(string)
		}
	}

	userID, _ := raw["user_id"].(string)
	if userID == "" {
		userID = "webhook-" + w.id
	}

	return &Message{
		ID:         uuid.New().String(),
		ChannelID:  w.id,
		Type:       TypeWebhook,
		UserID:     userID,
		Content:    content,
		Raw:        body,
		ReceivedAt: time.Now(),
	}, nil
}

func (w *WebhookReceiver) writeJSON(rw http.ResponseWriter, status int, data interface{}) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	json.NewEncoder(rw).Encode(data)
}

// signPayload HMAC-SHA256 签名
func signPayload(data []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

// verifySignature 验证HMAC-SHA256签名
func verifySignature(data []byte, secret, signature string) bool {
	expected := signPayload(data, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}

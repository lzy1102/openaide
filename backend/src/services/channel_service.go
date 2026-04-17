package services

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// ChannelType 渠道类型
type ChannelType string

const (
	ChannelFeishu    ChannelType = "feishu"
	ChannelCLI       ChannelType = "cli"
	ChannelWeb       ChannelType = "web"
	ChannelAPI       ChannelType = "api"
	ChannelWeChat    ChannelType = "wechat"
	ChannelTelegram  ChannelType = "telegram"
	ChannelSlack     ChannelType = "slack"
)

// ChannelMessage 统一渠道消息格式
type ChannelMessage struct {
	ID        string                 `json:"id"`
	Channel   ChannelType            `json:"channel"`
	UserID    string                 `json:"user_id"`
	Content   string                 `json:"content"`
	MediaType string                 `json:"media_type,omitempty"` // text, image, audio, file
	MediaURL  string                 `json:"media_url,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ChannelResponse 统一渠道响应格式
type ChannelResponse struct {
	Content   string                 `json:"content"`
	MediaType string                 `json:"media_type,omitempty"` // text, markdown, card, audio
	MediaData []byte                 `json:"-"`                   // 二进制数据（音频等）
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// MessageChannel 消息渠道接口
type MessageChannel interface {
	// 基本信息
	Type() ChannelType
	Name() string
	IsEnabled() bool

	// 消息处理
	Send(ctx context.Context, msg *ChannelResponse, targetID string) error
	Receive(ctx context.Context, msg *ChannelMessage) error

	// 生命周期
	Start() error
	Stop() error
}

// ChannelRegistry 渠道注册中心
type ChannelRegistry struct {
	channels map[ChannelType]MessageChannel
	mu       sync.RWMutex
}

// NewChannelRegistry 创建渠道注册中心
func NewChannelRegistry() *ChannelRegistry {
	return &ChannelRegistry{
		channels: make(map[ChannelType]MessageChannel),
	}
}

// Register 注册渠道
func (r *ChannelRegistry) Register(ch MessageChannel) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.channels[ch.Type()]; exists {
		return fmt.Errorf("channel %s already registered", ch.Type())
	}
	r.channels[ch.Type()] = ch
	log.Printf("[Channel] registered: %s (%s)", ch.Name(), ch.Type())
	return nil
}

// Unregister 注销渠道
func (r *ChannelRegistry) Unregister(channelType ChannelType) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.channels, channelType)
}

// Get 获取渠道
func (r *ChannelRegistry) Get(channelType ChannelType) (MessageChannel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ch, ok := r.channels[channelType]
	return ch, ok
}

// List 列出所有已注册渠道
func (r *ChannelRegistry) List() []MessageChannel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]MessageChannel, 0, len(r.channels))
	for _, ch := range r.channels {
		result = append(result, ch)
	}
	return result
}

// ListEnabled 列出已启用的渠道
func (r *ChannelRegistry) ListEnabled() []MessageChannel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []MessageChannel
	for _, ch := range r.channels {
		if ch.IsEnabled() {
			result = append(result, ch)
		}
	}
	return result
}

// Broadcast 广播消息到所有已启用渠道
func (r *ChannelRegistry) Broadcast(ctx context.Context, msg *ChannelResponse, excludeChannels ...ChannelType) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	excludeSet := make(map[ChannelType]bool)
	for _, ch := range excludeChannels {
		excludeSet[ch] = true
	}

	for channelType, ch := range r.channels {
		if !ch.IsEnabled() || excludeSet[channelType] {
			continue
		}
		go func(ct ChannelType, c MessageChannel) {
			if err := c.Send(ctx, msg, ""); err != nil {
				log.Printf("[Channel] broadcast to %s failed: %v", ct, err)
			}
		}(channelType, ch)
	}
}

// SendToChannel 向指定渠道发送消息
func (r *ChannelRegistry) SendToChannel(ctx context.Context, channelType ChannelType, msg *ChannelResponse, targetID string) error {
	r.mu.RLock()
	ch, ok := r.channels[channelType]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("channel %s not registered", channelType)
	}
	if !ch.IsEnabled() {
		return fmt.Errorf("channel %s is not enabled", channelType)
	}
	return ch.Send(ctx, msg, targetID)
}

// StartAll 启动所有已注册渠道
func (r *ChannelRegistry) StartAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, ch := range r.channels {
		if err := ch.Start(); err != nil {
			log.Printf("[Channel] failed to start %s: %v", ch.Name(), err)
		}
	}
}

// StopAll 停止所有已注册渠道
func (r *ChannelRegistry) StopAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, ch := range r.channels {
		if err := ch.Stop(); err != nil {
			log.Printf("[Channel] failed to stop %s: %v", ch.Name(), err)
		}
	}
}

// GetStatus 获取所有渠道状态
func (r *ChannelRegistry) GetStatus() []map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	status := make([]map[string]interface{}, 0, len(r.channels))
	for _, ch := range r.channels {
		status = append(status, map[string]interface{}{
			"type":     ch.Type(),
			"name":     ch.Name(),
			"enabled":  ch.IsEnabled(),
		})
	}
	return status
}

// ==================== CLI 渠道适配器 ====================

// CLIChannel CLI 渠道
type CLIChannel struct {
	enabled bool
	onMsg   func(ctx context.Context, msg *ChannelMessage) error
}

// NewCLIChannel 创建 CLI 渠道
func NewCLIChannel(onMsg func(ctx context.Context, msg *ChannelMessage) error) *CLIChannel {
	return &CLIChannel{
		enabled: true,
		onMsg:   onMsg,
	}
}

func (c *CLIChannel) Type() ChannelType { return ChannelCLI }
func (c *CLIChannel) Name() string       { return "CLI Terminal" }
func (c *CLIChannel) IsEnabled() bool     { return c.enabled }
func (c *CLIChannel) Start() error        { return nil }
func (c *CLIChannel) Stop() error         { return nil }

func (c *CLIChannel) Send(ctx context.Context, msg *ChannelResponse, targetID string) error {
	log.Printf("[CLI] %s", msg.Content)
	return nil
}

func (c *CLIChannel) Receive(ctx context.Context, msg *ChannelMessage) error {
	if c.onMsg != nil {
		return c.onMsg(ctx, msg)
	}
	return nil
}

// ==================== API 渠道适配器 ====================

// APIChannel API 渠道
type APIChannel struct {
	enabled bool
}

// NewAPIChannel 创建 API 渠道
func NewAPIChannel() *APIChannel {
	return &APIChannel{enabled: true}
}

func (c *APIChannel) Type() ChannelType { return ChannelAPI }
func (c *APIChannel) Name() string       { return "REST API" }
func (c *APIChannel) IsEnabled() bool     { return c.enabled }
func (c *APIChannel) Start() error        { return nil }
func (c *APIChannel) Stop() error         { return nil }

func (c *APIChannel) Send(ctx context.Context, msg *ChannelResponse, targetID string) error {
	// API 渠道的消息通过 HTTP 响应返回，不需要主动推送
	return nil
}

func (c *APIChannel) Receive(ctx context.Context, msg *ChannelMessage) error {
	// API 渠道的消息通过 HTTP 请求接收
	return nil
}

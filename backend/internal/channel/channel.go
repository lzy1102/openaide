// Package channel 提供外部消息渠道抽象层
//
// 架构：
//
//	Channel (接口)         ← 每个外部渠道实现此接口
//	    ↑                        ↑              ↑
//	WebhookReceiver    FeishuBot    TelegramBot
//	    ↑                        ↑              ↑
//	└── 注册HTTP端点     └── 飞书回调     └── Telegram Webhook
//	      到主API Server         + API调用       + sendMessage API
//
// Registry 管理所有渠道生命周期
//
//	MessageHandler 由编排器实现，将渠道消息转发到 Kernel
package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"openaide/backend/internal/kernel/actor"
)

// ChannelType 渠道类型
type ChannelType string

const (
	TypeWebhook  ChannelType = "webhook"
	TypeFeishu   ChannelType = "feishu"
	TypeTelegram ChannelType = "telegram"
)

// Message 来自外部渠道的消息
type Message struct {
	ID         string          `json:"id"`
	ChannelID  string          `json:"channel_id"`
	Type       ChannelType     `json:"type"`
	UserID     string          `json:"user_id"`
	Content    string          `json:"content"`
	Raw        json.RawMessage `json:"raw,omitempty"`
	ReceivedAt time.Time       `json:"received_at"`
}

// Response 发送到渠道的响应
type Response struct {
	Content string                 `json:"content"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

// MessageHandler 消息处理器函数
// 由编排器实现：解析消息 → Kernel.Process → 返回响应
type MessageHandler func(ctx context.Context, msg *Message) (*Response, error)

// Channel 渠道接口
// 每个外部通信渠道（Webhook/飞书/Telegram）实现此接口
type Channel interface {
	// ID 渠道唯一标识
	ID() string

	// Name 渠道可读名称
	Name() string

	// Type 渠道类型
	Type() ChannelType

	// Send 发送消息到渠道
	// targetID: 渠道内的用户或群聊标识
	Send(ctx context.Context, targetID string, resp *Response) error

	// Start 启动渠道监听
	// handler 由编排器注入，处理所有入站消息
	Start(ctx context.Context, handler MessageHandler) error

	// Stop 停止渠道
	Stop(ctx context.Context) error

	// Status 返回渠道运行状态
	Status(ctx context.Context) Status
}

// Status 渠道运行状态
type Status struct {
	ID      string                 `json:"id"`
	Type    ChannelType            `json:"type"`
	Running bool                   `json:"running"`
	Healthy bool                   `json:"healthy"`
	Info    map[string]interface{} `json:"info,omitempty"`
}

// Registry 渠道注册表 — lock-free via SafeMap
type Registry struct {
	channels *actor.SafeMap[string, Channel]
}

// NewRegistry 创建渠道注册表
func NewRegistry() *Registry {
	return &Registry{
		channels: actor.NewSafeMap[string, Channel](8),
	}
}

// Register 注册渠道
func (r *Registry) Register(ch Channel) error {
	if _, ok := r.channels.Load(ch.ID()); ok {
		return fmt.Errorf("channel %q already registered", ch.ID())
	}
	r.channels.Store(ch.ID(), ch)
	return nil
}

// Unregister 注销渠道
func (r *Registry) Unregister(id string) {
	if ch, ok := r.channels.Load(id); ok {
		_ = ch.Stop(context.Background())
		r.channels.Delete(id)
	}
}

// Get 获取渠道
func (r *Registry) Get(id string) Channel {
	ch, _ := r.channels.Load(id)
	return ch
}

// List 列出所有注册的渠道
func (r *Registry) List() []Channel {
	result := make([]Channel, 0)
	r.channels.Range(func(_ string, ch Channel) bool {
		result = append(result, ch)
		return true
	})
	return result
}

// StartAll 启动所有已注册渠道
func (r *Registry) StartAll(ctx context.Context, handler MessageHandler) error {
	var lastErr error
	r.channels.Range(func(id string, ch Channel) bool {
		if err := ch.Start(ctx, handler); err != nil {
			lastErr = fmt.Errorf("start channel %q failed: %w", id, err)
			return false
		}
		return true
	})
	return lastErr
}

// StopAll 停止所有已注册渠道
func (r *Registry) StopAll(ctx context.Context) error {
	var lastErr error
	r.channels.Range(func(id string, ch Channel) bool {
		if err := ch.Stop(ctx); err != nil {
			lastErr = err
		}
		return true
	})
	return lastErr
}

// StatusAll 获取所有渠道运行状态
func (r *Registry) StatusAll(ctx context.Context) []Status {
	result := make([]Status, 0)
	r.channels.Range(func(_ string, ch Channel) bool {
		result = append(result, ch.Status(ctx))
		return true
	})
	return result
}

// Len 返回已注册渠道数量
func (r *Registry) Len() int { return r.channels.Len() }

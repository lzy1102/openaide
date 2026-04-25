package services

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
)

// EventHandler 事件处理函数类型
type EventHandler func(ctx context.Context, event *models.Event)

// EventSubscription 事件订阅
type EventSubscription struct {
	ID      string
	Topic   string         // 订阅的主题（空字符串表示订阅所有）
	Handler EventHandler
	Async   bool           // 是否异步处理
}

// EventBus 事件总线 - 进程内发布/订阅
type EventBus struct {
	db           *gorm.DB
	subscriptions map[string][]*EventSubscription // topic -> handlers
	allHandlers   []*EventSubscription            // 订阅所有事件的处理器
	mu            sync.RWMutex
	logger        *LoggerService
	persistEvents bool // 是否持久化事件到数据库
}

// NewEventBus 创建事件总线
func NewEventBus(db *gorm.DB, logger *LoggerService, persistEvents bool) *EventBus {
	return &EventBus{
		db:            db,
		subscriptions: make(map[string][]*EventSubscription),
		allHandlers:   make([]*EventSubscription, 0),
		logger:        logger,
		persistEvents: persistEvents,
	}
}

// Subscribe 订阅事件
func (bus *EventBus) Subscribe(topic string, handler EventHandler) string {
	return bus.SubscribeWithOptions(topic, handler, false)
}

// SubscribeAsync 异步订阅事件
func (bus *EventBus) SubscribeAsync(topic string, handler EventHandler) string {
	return bus.SubscribeWithOptions(topic, handler, true)
}

// SubscribeWithOptions 带选项订阅事件
func (bus *EventBus) SubscribeWithOptions(topic string, handler EventHandler, async bool) string {
	sub := &EventSubscription{
		ID:      uuid.New().String(),
		Topic:   topic,
		Handler: handler,
		Async:   async,
	}

	bus.mu.Lock()
	defer bus.mu.Unlock()

	if topic == "" {
		bus.allHandlers = append(bus.allHandlers, sub)
	} else {
		bus.subscriptions[topic] = append(bus.subscriptions[topic], sub)
	}

	return sub.ID
}

// Unsubscribe 取消订阅
func (bus *EventBus) Unsubscribe(subID string) {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	for topic, subs := range bus.subscriptions {
		for i, sub := range subs {
			if sub.ID == subID {
				bus.subscriptions[topic] = append(subs[:i], subs[i+1:]...)
				return
			}
		}
	}

	for i, sub := range bus.allHandlers {
		if sub.ID == subID {
			bus.allHandlers = append(bus.allHandlers[:i], bus.allHandlers[i+1:]...)
			return
		}
	}
}

// Publish 发布事件
func (bus *EventBus) Publish(ctx context.Context, topic, eventType, source string, data map[string]interface{}) {
	bus.PublishWithMetadata(ctx, topic, eventType, source, data, nil)
}

// PublishWithMetadata 带元数据发布事件
func (bus *EventBus) PublishWithMetadata(ctx context.Context, topic, eventType, source string, data, metadata map[string]interface{}) {
	event := &models.Event{
		ID:        uuid.New().String(),
		Topic:     topic,
		Type:      eventType,
		Source:    source,
		Data:      data,
		Metadata:  metadata,
		Status:    "pending",
		CreatedAt: time.Now(),
	}

	// 持久化到数据库
	if bus.persistEvents && bus.db != nil {
		go func() {
			if err := bus.db.Create(event).Error; err != nil {
				log.Printf("[EventBus] failed to persist event %s: %v", event.ID, err)
			}
		}()
	}

	// 分发给订阅者
	bus.dispatchEvent(ctx, event)
}

// dispatchEvent 分发事件给订阅者
func (bus *EventBus) dispatchEvent(ctx context.Context, event *models.Event) {
	bus.mu.RLock()
	defer bus.mu.RUnlock()

	// 主题匹配的处理器
	if subs, ok := bus.subscriptions[event.Topic]; ok {
		for _, sub := range subs {
			bus.invokeHandler(ctx, sub, event)
		}
	}

	// 订阅所有事件的处理器
	for _, sub := range bus.allHandlers {
		bus.invokeHandler(ctx, sub, event)
	}

	// 更新事件状态
	if bus.persistEvents && bus.db != nil {
		go func() {
			now := time.Now()
			bus.db.Model(&models.Event{}).Where("id = ?", event.ID).
				Updates(map[string]interface{}{"status": "processed", "processed_at": &now})
		}()
	}
}

// invokeHandler 调用事件处理器
func (bus *EventBus) invokeHandler(ctx context.Context, sub *EventSubscription, event *models.Event) {
	if sub.Async {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[EventBus] panic in async handler for %s: %v", event.Type, r)
				}
			}()
			// 使用带超时的 context 避免 handler 永久阻塞
			execCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			sub.Handler(execCtx, event)
		}()
	} else {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[EventBus] panic in handler for %s: %v", event.Type, r)
				}
			}()
			sub.Handler(ctx, event)
		}()
	}
}

// ==================== 事件查询 ====================

// GetEvents 查询事件列表
func (bus *EventBus) GetEvents(topic string, limit int) ([]models.Event, error) {
	if bus.db == nil {
		return nil, fmt.Errorf("database not available")
	}
	var events []models.Event
	query := bus.db.Order("created_at DESC")
	if topic != "" {
		query = query.Where("topic = ?", topic)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&events).Error
	return events, err
}

// GetEvent 获取单个事件
func (bus *EventBus) GetEvent(id string) (*models.Event, error) {
	if bus.db == nil {
		return nil, fmt.Errorf("database not available")
	}
	var event models.Event
	err := bus.db.First(&event, id).Error
	return &event, err
}

// CleanupOldEvents 清理旧事件（保留最近 N 天）
func (bus *EventBus) CleanupOldEvents(days int) error {
	if bus.db == nil {
		return nil
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	return bus.db.Where("created_at < ?", cutoff).Delete(&models.Event{}).Error
}

// ==================== 事件统计 ====================

// EventStats 事件统计
type EventStats struct {
	TotalEvents    int64            `json:"total_events"`
	ByTopic        map[string]int64 `json:"by_topic"`
	ByType         map[string]int64 `json:"by_type"`
	PendingEvents  int64            `json:"pending_events"`
	FailedEvents   int64            `json:"failed_events"`
}

// GetStats 获取事件统计
func (bus *EventBus) GetStats() (*EventStats, error) {
	stats := &EventStats{
		ByTopic: make(map[string]int64),
		ByType:  make(map[string]int64),
	}

	if bus.db == nil {
		return stats, nil
	}

	bus.db.Model(&models.Event{}).Count(&stats.TotalEvents)
	bus.db.Model(&models.Event{}).Where("status = ?", "pending").Count(&stats.PendingEvents)
	bus.db.Model(&models.Event{}).Where("status = ?", "failed").Count(&stats.FailedEvents)

	type countResult struct {
		Column string
		Count  int64
	}

	var topicCounts []countResult
	bus.db.Model(&models.Event{}).Select("topic as column, count(*) as count").Group("topic").Scan(&topicCounts)
	for _, c := range topicCounts {
		stats.ByTopic[c.Column] = c.Count
	}

	var typeCounts []countResult
	bus.db.Model(&models.Event{}).Select("type as column, count(*) as count").Group("type").Scan(&typeCounts)
	for _, c := range typeCounts {
		stats.ByType[c.Column] = c.Count
	}

	return stats, nil
}

// ==================== JSON helper ====================

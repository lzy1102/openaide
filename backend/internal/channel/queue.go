package channel

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// Task 异步任务
type Task struct {
	ID        string                 `json:"id"`
	ChannelID string                 `json:"channel_id"`
	UserID    string                 `json:"user_id"`
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt time.Time              `json:"created_at"`

	// OnResult 任务完成后的回调函数
	// 由 enqueue 方设置，用于将处理结果发送回对应渠道
	OnResult func(result *TaskResult) `json:"-"`
}

// TaskResult 任务执行结果
type TaskResult struct {
	TaskID    string        `json:"task_id"`
	Content   string        `json:"content"`
	Error     string        `json:"error,omitempty"`
	Completed bool          `json:"completed"`
	Duration  time.Duration `json:"duration"`
}

// TaskHandler 任务处理函数
// 由编排器实现: 调用 Kernel.Process 执行任务
type TaskHandler func(ctx context.Context, task *Task) *TaskResult

// TaskQueue 异步任务队列
//
// 用途: 将外部渠道的长耗时任务异步化，不阻塞渠道Webhook响应
// 工作流:
//
//	渠道接收消息 → TaskQueue.Enqueue → 工作池异步处理
//	                                                 ↓
//	                             处理完成后可选通过Channel.Send 回传
type TaskQueue struct {
	workerCount int
	queueSize   int
	tasks       chan *Task
	handler     TaskHandler
	stopped     atomic.Bool
	wg          sync.WaitGroup
	done        chan struct{}
	closeOnce   sync.Once

	// 统计
	enqueued  atomic.Int64
	completed atomic.Int64
	failed    atomic.Int64
}

// QueueConfig 任务队列配置
type QueueConfig struct {
	WorkerCount int `json:"worker_count" yaml:"worker_count"`
	QueueSize   int `json:"queue_size" yaml:"queue_size"`
}

// NewTaskQueue 创建异步任务队列
//
//	默认: 4个worker, 128缓冲区
func NewTaskQueue(cfg QueueConfig) *TaskQueue {
	if cfg.WorkerCount < 1 {
		cfg.WorkerCount = 4
	}
	if cfg.QueueSize < 1 {
		cfg.QueueSize = 128
	}
	return &TaskQueue{
		workerCount: cfg.WorkerCount,
		queueSize:   cfg.QueueSize,
		tasks:       make(chan *Task, cfg.QueueSize),
		done:        make(chan struct{}),
	}
}

// Enqueue 入队任务（非阻塞）
// 队列满时返回 error
func (q *TaskQueue) Enqueue(task *Task) error {
	if q.stopped.Load() {
		return fmt.Errorf("task queue stopped")
	}
	if task.ID == "" {
		task.ID = uuid.New().String()
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}

	select {
	case q.tasks <- task:
		q.enqueued.Add(1)
		return nil
	default:
		return fmt.Errorf("task queue full (%d pending)", len(q.tasks))
	}
}

// Start 启动工作池
func (q *TaskQueue) Start(ctx context.Context, handler TaskHandler) error {
	if handler == nil {
		return fmt.Errorf("task queue handler must not be nil")
	}
	q.handler = handler

	for i := 0; i < q.workerCount; i++ {
		q.wg.Add(1)
		go q.worker(ctx, i)
	}

	slog.Info("Task queue started",
		"workers", q.workerCount,
		"queue_size", q.queueSize,
	)
	return nil
}

// Stop 停止工作池（等待所有运行中任务完成）
func (q *TaskQueue) Stop(ctx context.Context) error {
	q.stopped.Store(true)
	q.closeOnce.Do(func() {
		close(q.tasks)
	})

	doneCh := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
		close(q.done)
		slog.Info("Task queue stopped",
			"enqueued", q.enqueued.Load(),
			"completed", q.completed.Load(),
			"failed", q.failed.Load(),
		)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stats 获取队列统计
func (q *TaskQueue) Stats() map[string]interface{} {
	return map[string]interface{}{
		"enqueued":  q.enqueued.Load(),
		"completed": q.completed.Load(),
		"failed":    q.failed.Load(),
		"pending":   len(q.tasks),
		"workers":   q.workerCount,
	}
}

// worker 工作协程
func (q *TaskQueue) worker(ctx context.Context, id int) {
	defer q.wg.Done()
	slog.Debug("Task worker started", "worker_id", id)

	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-q.tasks:
			if !ok {
				return
			}

			start := time.Now()
			result := q.handler(ctx, task)
			duration := time.Since(start)

			if result.Error != "" {
				q.failed.Add(1)
				slog.Warn("Task failed",
					"task_id", task.ID,
					"worker", id,
					"duration", duration,
					"error", result.Error,
				)
			} else {
				q.completed.Add(1)
				slog.Debug("Task completed",
					"task_id", task.ID,
					"worker", id,
					"duration", duration,
				)
			}
		}
	}
}

package channel

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestTaskQueue_EnqueueDequeue(t *testing.T) {
	q := NewTaskQueue(QueueConfig{WorkerCount: 1, QueueSize: 10})

	task := &Task{ID: "test-1", Content: "hello"}
	if err := q.Enqueue(task); err != nil {
		t.Fatal(err)
	}

	var received *Task
	var mu sync.Mutex
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q.Start(ctx, func(ctx context.Context, t *Task) *TaskResult {
		mu.Lock()
		received = t
		mu.Unlock()
		return &TaskResult{TaskID: t.ID, Completed: true}
	})

	time.Sleep(50 * time.Millisecond)
	q.Stop(ctx)

	mu.Lock()
	defer mu.Unlock()
	if received == nil {
		t.Error("expected task to be received")
	} else if received.ID != "test-1" {
		t.Errorf("expected test-1, got %s", received.ID)
	}
	cancel()
}

func TestTaskQueue_QueueFull(t *testing.T) {
	q := NewTaskQueue(QueueConfig{WorkerCount: 1, QueueSize: 1})
	q.Enqueue(&Task{ID: "first"})
	err := q.Enqueue(&Task{ID: "second"})
	if err == nil {
		t.Error("expected queue full error")
	}
}

func TestTaskQueue_StartStop(t *testing.T) {
	q := NewTaskQueue(QueueConfig{WorkerCount: 2, QueueSize: 10})
	ctx := context.Background()

	q.Start(ctx, func(ctx context.Context, t *Task) *TaskResult { return &TaskResult{Completed: true} })
	time.Sleep(10 * time.Millisecond)

	if err := q.Stop(ctx); err != nil {
		t.Error(err)
	}
}

func TestTaskQueue_StoppedEnqueue(t *testing.T) {
	q := NewTaskQueue(QueueConfig{WorkerCount: 1, QueueSize: 10})
	ctx, cancel := context.WithCancel(context.Background())
	q.Start(ctx, func(ctx context.Context, t *Task) *TaskResult { return &TaskResult{Completed: true} })
	q.Stop(ctx)
	cancel()
	if err := q.Enqueue(&Task{Content: "after stop"}); err == nil {
		t.Error("expected error after stop")
	}
}

func TestTaskQueue_AutoID(t *testing.T) {
	q := NewTaskQueue(QueueConfig{WorkerCount: 1, QueueSize: 10})
	task := &Task{Content: "no-id"}
	q.Enqueue(task)
	if task.ID == "" {
		t.Error("expected auto-generated ID")
	}
}

func TestTaskQueue_Stats(t *testing.T) {
	q := NewTaskQueue(QueueConfig{WorkerCount: 3, QueueSize: 10})
	s := q.Stats()
	if w, ok := s["workers"].(int); !ok || w != 3 {
		t.Errorf("expected 3 workers, got %v", s["workers"])
	}
}

func TestTaskQueue_DefaultConfig(t *testing.T) {
	q := NewTaskQueue(QueueConfig{})
	if q.workerCount != 4 || q.queueSize != 128 {
		t.Errorf("expected defaults (4/128), got (%d/%d)", q.workerCount, q.queueSize)
	}
}

// ============ Registry tests ============

type mockChannel struct{ id, name string; ctype ChannelType; started bool }

func (m *mockChannel) ID() string     { return m.id }
func (m *mockChannel) Name() string   { return m.name }
func (m *mockChannel) Type() ChannelType { return m.ctype }
func (m *mockChannel) Send(ctx context.Context, tID string, r *Response) error { return nil }
func (m *mockChannel) Start(ctx context.Context, h MessageHandler) error { m.started = true; return nil }
func (m *mockChannel) Stop(ctx context.Context) error { m.started = false; return nil }
func (m *mockChannel) Status(ctx context.Context) Status { return Status{ID: m.id, Running: m.started, Healthy: true} }

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&mockChannel{id: "ch1", ctype: TypeWebhook}); err != nil {
		t.Fatal(err)
	}
	if r.Len() != 1 {
		t.Errorf("expected 1, got %d", r.Len())
	}
}

func TestRegistry_Duplicate(t *testing.T) {
	r := NewRegistry()
	ch := &mockChannel{id: "ch1", ctype: TypeWebhook}
	r.Register(ch)
	if err := r.Register(ch); err == nil {
		t.Error("expected error for duplicate")
	}
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockChannel{id: "ch1", ctype: TypeFeishu})
	if r.Get("ch1") == nil || r.Get("nonexistent") != nil {
		t.Error("Get failed")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockChannel{id: "ch1", ctype: TypeWebhook})
	r.Unregister("ch1")
	if r.Get("ch1") != nil || r.Len() != 0 {
		t.Error("Unregister failed")
	}
}

func TestSignPayload(t *testing.T) {
	s1 := signPayload([]byte("hello"), "secret")
	s2 := signPayload([]byte("hello"), "secret")
	s3 := signPayload([]byte("world"), "secret")
	if s1 != s2 || s1 == s3 || len(s1) != 64 {
		t.Error("signPayload failed")
	}
}

func TestVerifySignature(t *testing.T) {
	sig := signPayload([]byte("msg"), "key")
	if !verifySignature([]byte("msg"), "key", sig) {
		t.Error("valid signature rejected")
	}
	if verifySignature([]byte("msg"), "key", "bad") {
		t.Error("invalid signature accepted")
	}
}

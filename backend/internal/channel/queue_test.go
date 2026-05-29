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

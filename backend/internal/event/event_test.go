package event

import (
	"sync"
	"testing"
	"time"

	"openaide/backend/core"
)

func TestBus_SubscribeAndPublish(t *testing.T) {
	bus := NewBus()
	var received []kernel.Event
	var mu sync.Mutex

	bus.Subscribe("test.event", kernel.EventHandlerFunc(func(e kernel.Event) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	}))

	bus.Publish(kernel.Event{Type: "test.event", Source: "test", Data: map[string]interface{}{"key": "val"}})
	time.Sleep(50 * time.Millisecond) // Publish dispatches in goroutines

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Type != "test.event" {
		t.Errorf("expected 'test.event', got %s", received[0].Type)
	}
}

func TestBus_WildcardSubscriber(t *testing.T) {
	bus := NewBus()
	var count int
	var mu sync.Mutex

	bus.Subscribe("*", kernel.EventHandlerFunc(func(e kernel.Event) {
		mu.Lock()
		count++
		mu.Unlock()
	}))

	bus.Publish(kernel.Event{Type: "foo.bar", Source: "test"})
	bus.Publish(kernel.Event{Type: "baz.qux", Source: "test"})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count != 2 {
		t.Fatalf("expected 2 events, got %d", count)
	}
}

func TestBus_GetEvents(t *testing.T) {
	bus := NewBus()
	bus.EnablePersistence(t.TempDir())
	defer bus.Shutdown()

	for i := 0; i < 5; i++ {
		bus.Publish(kernel.Event{Type: "evt", Source: "test"})
	}

	events := bus.GetEvents("evt", 10)
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}
}

func TestBus_MaxEvents(t *testing.T) {
	bus := NewBus()
	// Don't enable persistence — the disk I/O from 10K goroutines races
	// with temp dir cleanup and causes flaky failures.
	for i := 0; i < 11000; i++ {
		bus.Publish(kernel.Event{Type: "test", Source: "test"})
	}

	events := bus.GetEvents("test", 20000)
	if len(events) > 10000 {
		t.Fatalf("expected <= 10000 events (capped), got %d", len(events))
	}
	if len(events) == 0 {
		t.Error("expected some events, got 0")
	}
}

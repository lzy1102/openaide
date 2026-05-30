package kernel

import (
	"sync"
)

// Actor is a CSP-style autonomous goroutine that owns its data.
// All access goes through the command channel — no locks needed.
// The actor processes commands sequentially, ensuring linear data access.
type Actor struct {
	commands chan actorCmd
	stop     chan struct{}
	stopped  chan struct{}
	once     sync.Once // ensures stop is signaled only once
}

// actorCmd wraps a function to be executed in the actor's goroutine.
// reply is nil for fire-and-forget commands.
type actorCmd struct {
	fn    func()
	reply chan<- struct{} // signaled when fn completes (nil = async)
}

// NewActor creates and starts an actor goroutine.
// bufSize is the command channel buffer size (0 = synchronous, >0 = buffered).
func NewActor(bufSize int) *Actor {
	a := &Actor{
		commands: make(chan actorCmd, bufSize),
		stop:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}
	go a.loop()
	return a
}

// Send dispatches a command to the actor synchronously.
// Blocks until the command is processed. Safe for concurrent callers.
func (a *Actor) Send(fn func()) {
	reply := make(chan struct{}, 1)
	select {
	case a.commands <- actorCmd{fn: fn, reply: reply}:
		<-reply
	case <-a.stop:
	}
}

// SendAsync dispatches a command without waiting for completion.
// The actor will process it eventually. Safe for concurrent callers.
func (a *Actor) SendAsync(fn func()) {
	select {
	case a.commands <- actorCmd{fn: fn}:
	case <-a.stop:
	}
}

// Stop gracefully shuts down the actor.
// After Stop returns, no more commands will be processed.
func (a *Actor) Stop() {
	a.once.Do(func() {
		close(a.stop)
	})
	<-a.stopped
}

func (a *Actor) loop() {
	defer close(a.stopped)
	for {
		select {
		case cmd := <-a.commands:
			cmd.fn()
			if cmd.reply != nil {
				close(cmd.reply)
			}
		case <-a.stop:
			// Drain remaining commands before shutting down
			for {
				select {
				case cmd := <-a.commands:
					cmd.fn()
					if cmd.reply != nil {
						close(cmd.reply)
					}
				default:
					return
				}
			}
		}
	}
}

// ── Actor-backed module helpers ──────────────────────────

// ActorStore is an actor-owned map[string]interface{} for simple key-value storage.
// Useful for registries, caches, and simple state holders.
type ActorStore[A any] struct {
	actor *Actor
	items map[string]A
}

// NewActorStore creates an actor-backed typed map.
func NewActorStore[A any](bufSize int) *ActorStore[A] {
	s := &ActorStore[A]{
		actor: NewActor(bufSize),
		items: make(map[string]A),
	}
	return s
}

// Get retrieves a value (synchronous).
func (s *ActorStore[A]) Get(key string) (A, bool) {
	var val A
	var ok bool
	s.actor.Send(func() {
		val, ok = s.items[key]
	})
	return val, ok
}

// Set stores a value (synchronous).
func (s *ActorStore[A]) Set(key string, value A) {
	s.actor.Send(func() {
		s.items[key] = value
	})
}

// Delete removes a key (synchronous).
func (s *ActorStore[A]) Delete(key string) {
	s.actor.Send(func() {
		delete(s.items, key)
	})
}

// Len returns the number of entries.
func (s *ActorStore[A]) Len() int {
	var n int
	s.actor.Send(func() {
		n = len(s.items)
	})
	return n
}

// Range iterates over all entries (synchronous).
// The callback runs inside the actor's goroutine — keep it fast.
func (s *ActorStore[A]) Range(fn func(key string, val A) bool) {
	s.actor.Send(func() {
		for k, v := range s.items {
			if !fn(k, v) {
				break
			}
		}
	})
}

// Values returns a copy of all values (synchronous).
func (s *ActorStore[A]) Values() []A {
	var vals []A
	s.actor.Send(func() {
		vals = make([]A, 0, len(s.items))
		for _, v := range s.items {
			vals = append(vals, v)
		}
	})
	return vals
}

// Keys returns all keys (synchronous).
func (s *ActorStore[A]) Keys() []string {
	var keys []string
	s.actor.Send(func() {
		keys = make([]string, 0, len(s.items))
		for k := range s.items {
			keys = append(keys, k)
		}
	})
	return keys
}

// Stop shuts down the actor store.
func (s *ActorStore[A]) Stop() {
	s.actor.Stop()
}

// Actor sends fire-and-forget to the underlying actor.
// Useful for async operations like persistence that shouldn't block callers.
func (s *ActorStore[A]) Actor() *Actor {
	return s.actor
}


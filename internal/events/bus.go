package events

import (
	"log/slog"
	"sync"
)

type Event struct {
	Type    string
	Payload map[string]string
}

type Handler func(Event)

// UnsubscribeFunc removes the associated subscription from the bus.
type UnsubscribeFunc func()

type subscription struct {
	eventType string
	id        uint64
	handler   Handler
}

type Bus struct {
	mu       sync.RWMutex
	subs     map[string][]subscription
	nextID   uint64
	wg       sync.WaitGroup
}

func NewBus() *Bus {
	return &Bus{subs: make(map[string][]subscription)}
}

// Subscribe registers a handler for the given event type and returns an
// unsubscribe function that removes it. Callers must call the returned
// function when they no longer need to receive events (e.g. on SSE disconnect).
func (b *Bus) Subscribe(eventType string, h Handler) UnsubscribeFunc {
	if h == nil {
		return func() {}
	}
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	b.subs[eventType] = append(b.subs[eventType], subscription{
		eventType: eventType,
		id:        id,
		handler:   h,
	})
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		subs := b.subs[eventType]
		for i, s := range subs {
			if s.id == id {
				b.subs[eventType] = append(subs[:i], subs[i+1:]...)
				return
			}
		}
	}
}

func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	subs := b.subs[e.Type]
	handlers := make([]Handler, len(subs))
	for i, s := range subs {
		handlers[i] = s.handler
	}
	b.wg.Add(len(handlers))
	b.mu.RUnlock()

	for _, h := range handlers {
		go func(fn Handler) {
			defer b.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("event handler panicked", "event", e.Type, "panic", r)
				}
			}()
			fn(e)
		}(h)
	}
}

// Wait blocks until all in-flight handlers have completed.
func (b *Bus) Wait() {
	b.wg.Wait()
}

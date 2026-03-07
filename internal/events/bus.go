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

type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
	wg       sync.WaitGroup
}

func NewBus() *Bus {
	return &Bus{handlers: make(map[string][]Handler)}
}

func (b *Bus) Subscribe(eventType string, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], h)
}

func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	handlers := make([]Handler, len(b.handlers[e.Type]))
	copy(handlers, b.handlers[e.Type])
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

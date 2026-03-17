package events

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestPublishToSubscriber(t *testing.T) {
	bus := NewBus()

	var received atomic.Value
	bus.Subscribe("test.event", func(e Event) {
		received.Store(e)
	})

	bus.Publish(Event{Type: "test.event", Payload: map[string]string{"key": "val"}})
	bus.Wait()

	evt, ok := received.Load().(Event)
	if !ok {
		t.Fatal("handler was not called")
	}
	if evt.Payload["key"] != "val" {
		t.Fatalf("expected payload key=val, got %v", evt.Payload)
	}
}

func TestMultipleSubscribers(t *testing.T) {
	bus := NewBus()
	var count atomic.Int32

	for range 3 {
		bus.Subscribe("multi", func(e Event) {
			count.Add(1)
		})
	}

	bus.Publish(Event{Type: "multi"})
	bus.Wait()

	if count.Load() != 3 {
		t.Fatalf("expected 3 handlers called, got %d", count.Load())
	}
}

func TestNoSubscriberNoPanic(t *testing.T) {
	bus := NewBus()
	// Should not panic
	bus.Publish(Event{Type: "nobody.listening"})
	bus.Wait()
}

func TestSubscribersIsolatedByType(t *testing.T) {
	bus := NewBus()

	var called atomic.Bool
	bus.Subscribe("type.a", func(e Event) {
		called.Store(true)
	})

	bus.Publish(Event{Type: "type.b"})
	bus.Wait()

	if called.Load() {
		t.Fatal("handler for type.a should not be called for type.b event")
	}
}

func TestHandlerPanicRecovery(t *testing.T) {
	bus := NewBus()

	var afterPanic atomic.Bool
	bus.Subscribe("panic.test", func(e Event) {
		panic("test panic")
	})
	bus.Subscribe("panic.test", func(e Event) {
		afterPanic.Store(true)
	})

	bus.Publish(Event{Type: "panic.test"})
	bus.Wait()

	// The second handler should still execute despite the first panicking
	if !afterPanic.Load() {
		t.Fatal("second handler should execute despite first handler panic")
	}
}

func TestWaitBlocksUntilComplete(t *testing.T) {
	bus := NewBus()
	var done atomic.Bool

	bus.Subscribe("slow", func(e Event) {
		time.Sleep(50 * time.Millisecond)
		done.Store(true)
	})

	bus.Publish(Event{Type: "slow"})
	bus.Wait()

	if !done.Load() {
		t.Fatal("Wait should block until handler completes")
	}
}

package event

import "sync"

const subscriberBufferSize = 64

// Bus is an in-memory per-task event pub/sub system.
// Events are delivered on a best-effort basis — if a subscriber's channel
// is full, that delivery is dropped. The SQLite store provides durability.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[string][]*subscriber
}

type subscriber struct {
	ch     chan *Event
	closed bool
}

// NewBus creates a new event bus.
func NewBus() *Bus {
	return &Bus{
		subscribers: make(map[string][]*subscriber),
	}
}

// Publish sends an event to all subscribers for the given task.
// Non-blocking: if a subscriber's channel is full, the event is dropped
// for that subscriber (it remains in the SQLite store for replay).
func (b *Bus) Publish(taskID string, evt *Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, sub := range b.subscribers[taskID] {
		if sub.closed {
			continue
		}
		select {
		case sub.ch <- evt:
		default:
			// Subscriber is full; drop this delivery.
		}
	}
}

// Subscribe returns a channel that receives events for the given task,
// and an unsubscribe function that must be called when done.
func (b *Bus) Subscribe(taskID string) (events <-chan *Event, unsubscribe func()) {
	ch := make(chan *Event, subscriberBufferSize)
	sub := &subscriber{ch: ch}

	b.mu.Lock()
	b.subscribers[taskID] = append(b.subscribers[taskID], sub)
	b.mu.Unlock()

	unsubscribe = func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		sub.closed = true
		close(ch)

		// Remove from the subscriber list.
		subs := b.subscribers[taskID]
		for i, s := range subs {
			if s == sub {
				b.subscribers[taskID] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		if len(b.subscribers[taskID]) == 0 {
			delete(b.subscribers, taskID)
		}
	}

	events = ch
	return events, unsubscribe
}

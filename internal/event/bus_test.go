package event

import (
	"encoding/json"
	"testing"
	"time"
)

func testEvent(typ string, id int64) *Event {
	return &Event{
		ID:        id,
		TaskID:    "task-1",
		Type:      typ,
		Timestamp: time.Now(),
		Data:      json.RawMessage(`{}`),
	}
}

func TestBusPublishSubscribe(t *testing.T) {
	bus := NewBus()
	ch, unsub := bus.Subscribe("task-1")
	defer unsub()

	evt := testEvent("step.started", 1)
	bus.Publish("task-1", evt)

	select {
	case got := <-ch:
		if got.ID != 1 {
			t.Fatalf("expected event ID 1, got %d", got.ID)
		}
		if got.Type != "step.started" {
			t.Fatalf("expected type step.started, got %s", got.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBusMultipleSubscribers(t *testing.T) {
	bus := NewBus()
	ch1, unsub1 := bus.Subscribe("task-1")
	defer unsub1()
	ch2, unsub2 := bus.Subscribe("task-1")
	defer unsub2()

	evt := testEvent("step.output", 1)
	bus.Publish("task-1", evt)

	for i, ch := range []<-chan *Event{ch1, ch2} {
		select {
		case got := <-ch:
			if got.ID != 1 {
				t.Fatalf("subscriber %d: expected event ID 1, got %d", i, got.ID)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out waiting for event", i)
		}
	}
}

func TestBusUnsubscribe(t *testing.T) {
	bus := NewBus()
	ch, unsub := bus.Subscribe("task-1")
	unsub()

	// Channel should be closed after unsubscribe.
	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed after unsubscribe")
	}

	// Publishing after unsubscribe should not panic.
	bus.Publish("task-1", testEvent("step.started", 1))
}

func TestBusNoSubscribers(t *testing.T) {
	bus := NewBus()
	// Should not panic when no subscribers exist.
	bus.Publish("task-1", testEvent("step.started", 1))
}

func TestBusDifferentTasks(t *testing.T) {
	bus := NewBus()
	ch1, unsub1 := bus.Subscribe("task-1")
	defer unsub1()
	ch2, unsub2 := bus.Subscribe("task-2")
	defer unsub2()

	bus.Publish("task-1", testEvent("step.started", 1))

	// task-1 subscriber should receive the event.
	select {
	case <-ch1:
	case <-time.After(time.Second):
		t.Fatal("task-1 subscriber timed out")
	}

	// task-2 subscriber should NOT receive the event.
	select {
	case <-ch2:
		t.Fatal("task-2 subscriber should not receive task-1 events")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBusDropsOnFullChannel(t *testing.T) {
	bus := NewBus()
	ch, unsub := bus.Subscribe("task-1")
	defer unsub()

	// Fill the subscriber channel.
	for i := range subscriberBufferSize {
		bus.Publish("task-1", testEvent("step.output", int64(i)))
	}

	// This publish should be dropped (not block).
	done := make(chan struct{})
	go func() {
		bus.Publish("task-1", testEvent("step.output", 999))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on full channel")
	}

	// Drain and verify we got the buffered events.
	count := 0
	for range subscriberBufferSize {
		<-ch
		count++
	}
	if count != subscriberBufferSize {
		t.Fatalf("expected %d events, got %d", subscriberBufferSize, count)
	}
}

package event

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("creating sqlite store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("closing store: %v", err)
		}
	})
	return store
}

func TestSQLiteStoreAppendAndFetch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	evt := &Event{
		Type:      "step.started",
		Timestamp: time.Now().Truncate(time.Microsecond),
		Data:      json.RawMessage(`{"step":"clone"}`),
	}

	id, err := store.Append(ctx, "task-1", evt)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if id != 1 {
		t.Fatalf("expected id 1, got %d", id)
	}
	if evt.ID != 1 {
		t.Fatalf("expected event ID to be set to 1, got %d", evt.ID)
	}

	events, err := store.Fetch(ctx, "task-1", 0, 100)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "step.started" {
		t.Fatalf("expected type step.started, got %s", events[0].Type)
	}
	if string(events[0].Data) != `{"step":"clone"}` {
		t.Fatalf("unexpected data: %s", events[0].Data)
	}
}

func TestSQLiteStoreFetchAfterID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for i := range 5 {
		evt := &Event{
			Type:      "step.output",
			Timestamp: time.Now(),
			Data:      json.RawMessage(`{}`),
		}
		if _, err := store.Append(ctx, "task-1", evt); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	// Fetch after ID 3 should return events 4 and 5.
	events, err := store.Fetch(ctx, "task-1", 3, 100)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].ID != 4 {
		t.Fatalf("expected first event ID 4, got %d", events[0].ID)
	}
}

func TestSQLiteStoreFetchLimit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for i := range 10 {
		evt := &Event{
			Type:      "step.output",
			Timestamp: time.Now(),
			Data:      json.RawMessage(`{}`),
		}
		if _, err := store.Append(ctx, "task-1", evt); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	events, err := store.Fetch(ctx, "task-1", 0, 3)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
}

func TestSQLiteStoreFetchEmpty(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	events, err := store.Fetch(ctx, "task-1", 0, 100)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

func TestSQLiteStoreIsolatesTasks(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	evt1 := &Event{Type: "step.started", Timestamp: time.Now(), Data: json.RawMessage(`{}`)}
	evt2 := &Event{Type: "step.started", Timestamp: time.Now(), Data: json.RawMessage(`{}`)}

	if _, err := store.Append(ctx, "task-1", evt1); err != nil {
		t.Fatalf("append task-1: %v", err)
	}
	if _, err := store.Append(ctx, "task-2", evt2); err != nil {
		t.Fatalf("append task-2: %v", err)
	}

	events, err := store.Fetch(ctx, "task-1", 0, 100)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event for task-1, got %d", len(events))
	}
}

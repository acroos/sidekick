package event

import "context"

// Store persists events for a task and enables replay.
type Store interface {
	// Append adds an event to the store and assigns it a monotonic ID.
	Append(ctx context.Context, taskID string, evt *Event) (int64, error)

	// Fetch returns events for a task with ID greater than afterID,
	// up to the given limit. Use afterID=0 to fetch from the beginning.
	Fetch(ctx context.Context, taskID string, afterID int64, limit int) ([]*Event, error)
}

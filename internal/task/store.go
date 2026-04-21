package task

import "context"

// Store persists tasks and enables querying.
type Store interface {
	// Create inserts a new task.
	Create(ctx context.Context, t *Task) error

	// Get returns a task by ID, or nil if not found.
	Get(ctx context.Context, id string) (*Task, error)

	// List returns tasks matching the filter.
	List(ctx context.Context, filter ListFilter) ([]*Task, error)

	// Update overwrites the mutable fields of a task (status, steps, timestamps, error).
	Update(ctx context.Context, t *Task) error
}

// ListFilter controls task listing queries.
type ListFilter struct {
	Status      Status // Empty = all statuses
	WorkflowRef string // Empty = all workflows
	Limit       int    // 0 = default (50)
	Offset      int
}

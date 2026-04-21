package task

import (
	"context"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestSQLiteStore_CreateAndGet(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Microsecond)
	task := &Task{
		ID:          "task_abc123",
		WorkflowRef: "fix-issue",
		Variables:   map[string]string{"REPO_URL": "https://github.com/test/repo"},
		Status:      StatusPending,
		Steps:       []StepResult{},
		WebhookURL:  "https://example.com/hooks",
		CreatedAt:   now,
	}

	if err := store.Create(ctx, task); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := store.Get(ctx, "task_abc123")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected task, got nil")
	}

	if got.ID != task.ID {
		t.Errorf("ID = %q, want %q", got.ID, task.ID)
	}
	if got.WorkflowRef != task.WorkflowRef {
		t.Errorf("WorkflowRef = %q, want %q", got.WorkflowRef, task.WorkflowRef)
	}
	if got.Status != StatusPending {
		t.Errorf("Status = %q, want %q", got.Status, StatusPending)
	}
	if got.Variables["REPO_URL"] != "https://github.com/test/repo" {
		t.Errorf("Variables[REPO_URL] = %q", got.Variables["REPO_URL"])
	}
	if got.WebhookURL != "https://example.com/hooks" {
		t.Errorf("WebhookURL = %q", got.WebhookURL)
	}
}

func TestSQLiteStore_GetNotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	got, err := store.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestSQLiteStore_Update(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Microsecond)
	task := &Task{
		ID:          "task_update",
		WorkflowRef: "fix-issue",
		Variables:   map[string]string{},
		Status:      StatusPending,
		Steps:       []StepResult{},
		CreatedAt:   now,
	}
	if err := store.Create(ctx, task); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Update to running.
	startedAt := now.Add(time.Second)
	task.Status = StatusRunning
	task.StartedAt = &startedAt
	task.Steps = []StepResult{
		{Name: "clone", Status: StatusSucceeded, Duration: 3 * time.Second},
	}
	if err := store.Update(ctx, task); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := store.Get(ctx, "task_update")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != StatusRunning {
		t.Errorf("Status = %q, want %q", got.Status, StatusRunning)
	}
	if got.StartedAt == nil {
		t.Fatal("StartedAt is nil")
	}
	if len(got.Steps) != 1 || got.Steps[0].Name != "clone" {
		t.Errorf("Steps = %+v", got.Steps)
	}
}

func TestSQLiteStore_List(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Microsecond)
	tasks := []*Task{
		{ID: "task_1", WorkflowRef: "fix-issue", Variables: map[string]string{}, Status: StatusPending, Steps: []StepResult{}, CreatedAt: now},
		{ID: "task_2", WorkflowRef: "fix-issue", Variables: map[string]string{}, Status: StatusRunning, Steps: []StepResult{}, CreatedAt: now.Add(time.Second)},
		{ID: "task_3", WorkflowRef: "code-review", Variables: map[string]string{}, Status: StatusSucceeded, Steps: []StepResult{}, CreatedAt: now.Add(2 * time.Second)},
	}
	for _, task := range tasks {
		if err := store.Create(ctx, task); err != nil {
			t.Fatalf("create %s: %v", task.ID, err)
		}
	}

	// List all.
	got, err := store.List(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("list all: got %d, want 3", len(got))
	}

	// List by status.
	got, err = store.List(ctx, ListFilter{Status: StatusRunning})
	if err != nil {
		t.Fatalf("list running: %v", err)
	}
	if len(got) != 1 || got[0].ID != "task_2" {
		t.Errorf("list running: got %+v", got)
	}

	// List by workflow.
	got, err = store.List(ctx, ListFilter{WorkflowRef: "code-review"})
	if err != nil {
		t.Fatalf("list code-review: %v", err)
	}
	if len(got) != 1 || got[0].ID != "task_3" {
		t.Errorf("list code-review: got %+v", got)
	}

	// List with limit.
	got, err = store.List(ctx, ListFilter{Limit: 2})
	if err != nil {
		t.Fatalf("list limit: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("list limit: got %d, want 2", len(got))
	}
}

func TestSQLiteStore_StepsRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Microsecond)
	task := &Task{
		ID:          "task_steps",
		WorkflowRef: "fix-issue",
		Variables:   map[string]string{},
		Status:      StatusSucceeded,
		Steps: []StepResult{
			{Name: "clone", Status: StatusSucceeded, ExitCode: 0, Duration: 2 * time.Second, StartedAt: now},
			{Name: "solve", Status: StatusSucceeded, ExitCode: 0, Duration: 30 * time.Second, TokensUsed: 5000, StartedAt: now.Add(3 * time.Second)},
		},
		CreatedAt: now,
	}
	if err := store.Create(ctx, task); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := store.Get(ctx, "task_steps")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Steps) != 2 {
		t.Fatalf("got %d steps, want 2", len(got.Steps))
	}
	if got.Steps[1].TokensUsed != 5000 {
		t.Errorf("Steps[1].TokensUsed = %d, want 5000", got.Steps[1].TokensUsed)
	}
}

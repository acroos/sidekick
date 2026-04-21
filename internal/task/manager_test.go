package task

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManager_ResolveWorkflowPath(t *testing.T) {
	m := &Manager{cfg: ManagerConfig{WorkflowDir: "/tmp/workflows"}}

	tests := []struct {
		name    string
		ref     string
		wantErr bool
	}{
		{"valid", "fix-issue", false},
		{"nested", "category/fix-issue", false},
		{"empty", "", true},
		{"traversal dots", "../etc/passwd", true},
		{"absolute path", "/etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := m.resolveWorkflowPath(tt.ref)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveWorkflowPath(%q) error = %v, wantErr %v", tt.ref, err, tt.wantErr)
			}
		})
	}
}

// setupTestWorkflow creates a temp dir with a valid workflow file and returns the dir path.
func setupTestWorkflow(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	content := []byte(`name: test-workflow
timeout: 1m
sandbox:
  image: alpine:latest
steps:
  - name: hello
    type: deterministic
    run: echo hello
`)
	if err := os.WriteFile(filepath.Join(dir, "test-workflow.yaml"), content, 0644); err != nil {
		t.Fatalf("writing test workflow: %v", err)
	}
	return dir
}

func TestManager_SubmitAndGet(t *testing.T) {
	store := newTestStore(t)
	wfDir := setupTestWorkflow(t)

	m := NewManager(store, nil, ManagerConfig{
		MaxConcurrent: 2,
		WorkflowDir:   wfDir,
	})

	ctx := context.Background()
	task, err := m.Submit(ctx, SubmitRequest{
		WorkflowRef: "test-workflow",
		Variables:   map[string]string{"FOO": "bar"},
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	if task.Status != StatusPending {
		t.Errorf("status = %q, want %q", task.Status, StatusPending)
	}

	// The task should be retrievable from the store.
	got, err := m.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected task, got nil")
	}
	if got.WorkflowRef != "test-workflow" {
		t.Errorf("workflow = %q, want %q", got.WorkflowRef, "test-workflow")
	}

	// Cleanup — goroutine will fail because executor is nil, but that's fine for this test.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = m.Shutdown(shutdownCtx)
}

func TestManager_Cancel(t *testing.T) {
	store := newTestStore(t)
	wfDir := setupTestWorkflow(t)

	m := NewManager(store, nil, ManagerConfig{
		MaxConcurrent: 1,
		WorkflowDir:   wfDir,
	})

	ctx := context.Background()
	task, err := m.Submit(ctx, SubmitRequest{
		WorkflowRef: "test-workflow",
		Variables:   map[string]string{},
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Cancel immediately.
	canceled, err := m.Cancel(ctx, task.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if canceled == nil {
		t.Fatal("expected task, got nil")
	}

	// Wait for goroutine to finish.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = m.Shutdown(shutdownCtx)
}

func TestManager_CancelNotFound(t *testing.T) {
	store := newTestStore(t)
	m := NewManager(store, nil, ManagerConfig{MaxConcurrent: 1})

	got, err := m.Cancel(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestManager_List(t *testing.T) {
	store := newTestStore(t)
	wfDir := setupTestWorkflow(t)

	m := NewManager(store, nil, ManagerConfig{
		MaxConcurrent: 1,
		WorkflowDir:   wfDir,
	})

	ctx := context.Background()
	for range 3 {
		_, err := m.Submit(ctx, SubmitRequest{
			WorkflowRef: "test-workflow",
			Variables:   map[string]string{},
		})
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
	}

	tasks, err := m.List(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tasks) != 3 {
		t.Errorf("got %d tasks, want 3", len(tasks))
	}

	// Cleanup.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = m.Shutdown(shutdownCtx)
}

func TestManager_SubmitInvalidWorkflow(t *testing.T) {
	store := newTestStore(t)
	m := NewManager(store, nil, ManagerConfig{
		MaxConcurrent: 1,
		WorkflowDir:   t.TempDir(), // Empty dir, no workflow files.
	})

	_, err := m.Submit(context.Background(), SubmitRequest{
		WorkflowRef: "nonexistent",
		Variables:   map[string]string{},
	})
	if err == nil {
		t.Fatal("expected error for nonexistent workflow")
	}
}

func TestManager_SubmitPathTraversal(t *testing.T) {
	store := newTestStore(t)
	m := NewManager(store, nil, ManagerConfig{
		MaxConcurrent: 1,
		WorkflowDir:   t.TempDir(),
	})

	_, err := m.Submit(context.Background(), SubmitRequest{
		WorkflowRef: "../../../etc/passwd",
		Variables:   map[string]string{},
	})
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

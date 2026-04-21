package task

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ManagerConfig configures the task manager.
type ManagerConfig struct {
	MaxConcurrent int    // Worker pool size (default 4)
	WorkflowDir   string // Directory containing workflow YAML files
}

// Manager coordinates task lifecycle: submission, dispatch, cancellation, and webhooks.
type Manager struct {
	store    Store
	executor Executor
	cfg      ManagerConfig

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
	sem     chan struct{}
	wg      sync.WaitGroup
}

// NewManager creates a new task manager.
func NewManager(store Store, executor Executor, cfg ManagerConfig) *Manager {
	maxConcurrent := cfg.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 4
	}
	return &Manager{
		store:    store,
		executor: executor,
		cfg:      cfg,
		cancels:  make(map[string]context.CancelFunc),
		sem:      make(chan struct{}, maxConcurrent),
	}
}

// SubmitRequest holds the parameters for submitting a new task.
type SubmitRequest struct {
	WorkflowRef string
	Variables   map[string]string
	WebhookURL  string
}

// Submit creates a new task and dispatches it for execution.
// The task is returned immediately in pending state; execution is asynchronous.
func (m *Manager) Submit(ctx context.Context, req SubmitRequest) (*Task, error) {
	// Resolve and validate workflow path.
	wfPath, err := m.resolveWorkflowPath(req.WorkflowRef)
	if err != nil {
		return nil, fmt.Errorf("invalid workflow: %w", err)
	}

	// Verify the executor can load this workflow (fail fast on parse/validation errors).
	// We do a dry-run parse by calling RunWorkflow with a canceled context — but that's
	// wasteful. Instead, just check the file exists.
	if !fileExists(wfPath) {
		return nil, fmt.Errorf("workflow file not found: %s", req.WorkflowRef)
	}

	// Create task.
	t := &Task{
		ID:          "task_" + uuid.New().String()[:12],
		WorkflowRef: req.WorkflowRef,
		Variables:   req.Variables,
		Status:      StatusPending,
		Steps:       []StepResult{},
		WebhookURL:  req.WebhookURL,
		CreatedAt:   time.Now(),
	}

	if err := m.store.Create(ctx, t); err != nil {
		return nil, fmt.Errorf("persisting task: %w", err)
	}

	// Dispatch execution asynchronously.
	m.wg.Add(1)
	taskCtx, cancel := context.WithCancel(context.Background())

	m.mu.Lock()
	m.cancels[t.ID] = cancel
	m.mu.Unlock()

	go m.executeTask(taskCtx, cancel, t.ID, wfPath, req.Variables)

	return t, nil
}

// Get returns a task by ID.
func (m *Manager) Get(ctx context.Context, id string) (*Task, error) {
	return m.store.Get(ctx, id)
}

// List returns tasks matching the filter.
func (m *Manager) List(ctx context.Context, filter ListFilter) ([]*Task, error) {
	return m.store.List(ctx, filter)
}

// Cancel cancels a running or pending task.
func (m *Manager) Cancel(ctx context.Context, id string) (*Task, error) {
	m.mu.Lock()
	cancel, ok := m.cancels[id]
	m.mu.Unlock()

	if ok {
		cancel()
	}

	// Fetch current state — the execution goroutine will handle the status update,
	// but if the task is still pending (queued), update it now.
	t, err := m.store.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting task: %w", err)
	}
	if t == nil {
		return nil, nil
	}

	if t.Status == StatusPending {
		t.Status = StatusCanceled
		now := time.Now()
		t.CompletedAt = &now
		if err := m.store.Update(ctx, t); err != nil {
			return nil, fmt.Errorf("updating task: %w", err)
		}
	}

	return t, nil
}

// Shutdown waits for all in-flight tasks to complete within the context deadline.
func (m *Manager) Shutdown(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// executeTask runs a task through the workflow engine.
func (m *Manager) executeTask(ctx context.Context, cancel context.CancelFunc, taskID string, workflowPath string, variables map[string]string) {
	defer m.wg.Done()
	defer func() {
		cancel()
		m.mu.Lock()
		delete(m.cancels, taskID)
		m.mu.Unlock()
	}()

	// Acquire semaphore slot (blocks if at capacity).
	select {
	case m.sem <- struct{}{}:
		defer func() { <-m.sem }()
	case <-ctx.Done():
		// Canceled while queued.
		m.markCanceled(taskID)
		return
	}

	// Check context again after acquiring slot.
	if ctx.Err() != nil {
		m.markCanceled(taskID)
		return
	}

	// Update to running.
	t, err := m.store.Get(context.Background(), taskID)
	if err != nil || t == nil {
		slog.Error("failed to load task from store", "task_id", taskID, "error", err)
		return
	}

	// If already canceled (e.g., Cancel was called while pending), don't proceed.
	if t.Status == StatusCanceled {
		return
	}

	t.Status = StatusRunning
	now := time.Now()
	t.StartedAt = &now
	if err := m.store.Update(context.Background(), t); err != nil {
		slog.Error("failed to update task to running", "task_id", taskID, "error", err)
	}

	// Execute workflow via the executor interface.
	if m.executor == nil {
		t.Status = StatusFailed
		t.Error = "no executor configured"
		completedAt := time.Now()
		t.CompletedAt = &completedAt
		_ = m.store.Update(context.Background(), t)
		return
	}
	result, execErr := m.executor.RunWorkflow(ctx, taskID, workflowPath, variables)

	// Re-fetch to check if canceled during execution.
	t, err = m.store.Get(context.Background(), taskID)
	if err != nil || t == nil {
		slog.Error("failed to reload task from store", "task_id", taskID, "error", err)
		return
	}

	if t.Status == StatusCanceled {
		return
	}

	// Update task from execution result.
	if result != nil {
		t.Status = result.Status
		t.Steps = result.Steps
		t.CompletedAt = result.CompletedAt
		t.Error = result.Error
	} else if execErr != nil {
		t.Status = StatusFailed
		t.Error = execErr.Error()
		completedAt := time.Now()
		t.CompletedAt = &completedAt
	}

	// Handle context cancellation during execution.
	if ctx.Err() != nil && t.Status == StatusRunning {
		t.Status = StatusCanceled
		completedAt := time.Now()
		t.CompletedAt = &completedAt
	}

	if err := m.store.Update(context.Background(), t); err != nil {
		slog.Error("failed to persist task result", "task_id", taskID, "error", err)
	}

	// Fire webhook.
	if t.WebhookURL != "" {
		go SendWebhook(t.WebhookURL, t)
	}
}

// markCanceled sets a task's status to canceled in the store.
func (m *Manager) markCanceled(taskID string) {
	t, err := m.store.Get(context.Background(), taskID)
	if err != nil || t == nil {
		return
	}
	if t.Status == StatusCanceled {
		return
	}
	t.Status = StatusCanceled
	now := time.Now()
	t.CompletedAt = &now
	_ = m.store.Update(context.Background(), t)
}

// resolveWorkflowPath resolves a workflow reference to a file path,
// preventing path traversal attacks.
func (m *Manager) resolveWorkflowPath(ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("workflow reference is empty")
	}

	// Prevent path traversal.
	if strings.Contains(ref, "..") || strings.HasPrefix(ref, "/") {
		return "", fmt.Errorf("invalid workflow reference: %q", ref)
	}

	path := filepath.Join(m.cfg.WorkflowDir, ref+".yaml")

	// Verify the resolved path stays within the workflow directory.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}
	absDir, err := filepath.Abs(m.cfg.WorkflowDir)
	if err != nil {
		return "", fmt.Errorf("resolving workflow dir: %w", err)
	}
	if !strings.HasPrefix(absPath, absDir+string(filepath.Separator)) {
		return "", fmt.Errorf("workflow path escapes workflow directory")
	}

	return path, nil
}

// fileExists checks if a file exists at the given path.
func fileExists(path string) bool {
	info, err := filepath.Glob(path)
	return err == nil && len(info) > 0
}

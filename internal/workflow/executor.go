package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/austinroos/sidekick/internal/event"
	"github.com/austinroos/sidekick/internal/sandbox"
	"github.com/austinroos/sidekick/internal/task"
)

// Executor orchestrates workflow execution: creates a sandbox, runs steps
// in topological order, emits events, and applies failure policies.
type Executor struct {
	Provider sandbox.Provider
	Bus      *event.Bus
	Store    event.Store
}

// Execute runs a workflow for the given task, returning the populated task result.
func (e *Executor) Execute(ctx context.Context, taskID string, wf *Workflow, variables map[string]string) (*task.Task, error) {
	// Validate before executing.
	if err := Validate(wf); err != nil {
		return nil, err
	}

	// Interpolate variables into step commands/prompts.
	Interpolate(wf, variables)

	t := &task.Task{
		ID:          taskID,
		WorkflowRef: wf.Name,
		Variables:   variables,
		Status:      task.StatusRunning,
		CreatedAt:   time.Now(),
	}
	now := time.Now()
	t.StartedAt = &now

	// Create sandbox.
	sbCfg := sandbox.Config{
		Image:      wf.Sandbox.Image,
		Network:    sandbox.NetworkPolicy(wf.Sandbox.Network),
		AllowHosts: wf.Sandbox.AllowHosts,
		Timeout:    wf.Timeout.Duration,
	}
	sb, err := e.Provider.Create(ctx, sbCfg)
	if err != nil {
		t.Status = task.StatusFailed
		t.Error = fmt.Sprintf("creating sandbox: %v", err)
		return t, fmt.Errorf("creating sandbox: %w", err)
	}
	defer func() {
		_ = e.Provider.Destroy(context.Background(), sb.ID())
	}()

	// Apply workflow-level timeout if set.
	if wf.Timeout.Duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, wf.Timeout.Duration)
		defer cancel()
	}

	e.emit(ctx, taskID, "task.started", event.TaskStarted{})

	// Build step lookup for results and determine execution order.
	stepMap := make(map[string]*Step, len(wf.Steps))
	for i := range wf.Steps {
		stepMap[wf.Steps[i].Name] = &wf.Steps[i]
	}
	order := TopologicalOrder(wf.Steps)
	results := make(map[string]task.StepResult, len(wf.Steps))

	totalTokens := 0
	failed := false

	for _, stepName := range order {
		if ctx.Err() != nil {
			t.Status = task.StatusFailed
			t.Error = "workflow timeout exceeded"
			break
		}

		step := stepMap[stepName]

		// Evaluate when condition.
		if step.When != "" {
			ok, err := EvalCondition(step.When, results)
			if err != nil {
				t.Status = task.StatusFailed
				t.Error = fmt.Sprintf("evaluating when for step %q: %v", stepName, err)
				failed = true
				break
			}
			if !ok {
				sr := task.StepResult{
					Name:      stepName,
					Status:    task.StatusSkipped,
					StartedAt: time.Now(),
				}
				results[stepName] = sr
				t.Steps = append(t.Steps, sr)
				e.emit(ctx, taskID, "step.skipped", event.StepSkipped{
					Step:   stepName,
					Reason: fmt.Sprintf("when condition evaluated false: %s", step.When),
				})
				continue
			}
		}

		// Execute the step (with retry support).
		sr := e.executeStep(ctx, taskID, sb, step, wf.MaxRetries)
		results[stepName] = sr
		t.Steps = append(t.Steps, sr)
		totalTokens += sr.TokensUsed

		// Apply failure policy.
		if sr.Status == task.StatusFailed {
			switch step.OnFailure {
			case FailAbort, "":
				t.Status = task.StatusFailed
				t.Error = fmt.Sprintf("step %q failed with exit code %d", stepName, sr.ExitCode)
				failed = true
			case FailContinue:
				// Record the failure but keep going.
			case FailRetry:
				// Already retried in executeStep; if still failed, abort.
				t.Status = task.StatusFailed
				t.Error = fmt.Sprintf("step %q failed after retries", stepName)
				failed = true
			}

			if failed {
				break
			}
		}
	}

	if !failed && t.Status == task.StatusRunning {
		t.Status = task.StatusSucceeded
	}

	completedAt := time.Now()
	t.CompletedAt = &completedAt

	e.emit(ctx, taskID, "task.completed", event.TaskCompleted{
		Status:          string(t.Status),
		TotalTokensUsed: totalTokens,
	})

	return t, nil
}

// executeStep runs a single step, handling retries for FailRetry policy.
func (e *Executor) executeStep(ctx context.Context, taskID string, sb sandbox.Sandbox, step *Step, maxRetries int) task.StepResult {
	attempts := 1
	if step.OnFailure == FailRetry && maxRetries > 0 {
		attempts = maxRetries + 1
	}

	var sr task.StepResult
	for attempt := range attempts {
		sr = e.runStep(ctx, taskID, sb, step)
		if sr.Status == task.StatusSucceeded {
			return sr
		}
		if attempt < attempts-1 {
			// Will retry — don't break yet.
			continue
		}
	}
	return sr
}

// runStep executes a single step and returns its result.
func (e *Executor) runStep(ctx context.Context, taskID string, sb sandbox.Sandbox, step *Step) task.StepResult {
	start := time.Now()
	sr := task.StepResult{
		Name:      step.Name,
		StartedAt: start,
	}

	if step.Type == StepAgent {
		sr.Status = task.StatusFailed
		sr.Stderr = "agent steps not yet supported"
		sr.Duration = time.Since(start)
		e.emit(ctx, taskID, "step.started", event.StepStarted{Step: step.Name})
		e.emit(ctx, taskID, "step.completed", event.StepCompleted{
			Step:       step.Name,
			Status:     string(sr.Status),
			DurationMs: sr.Duration.Milliseconds(),
		})
		return sr
	}

	e.emit(ctx, taskID, "step.started", event.StepStarted{Step: step.Name})

	// Build the command.
	cmd := sandbox.Command{
		Args:    []string{"sh", "-c", step.Run},
		WorkDir: "/workspace",
		Timeout: step.Timeout.Duration,
	}

	// Use ExecStream for real-time output.
	stream, err := sb.ExecStream(ctx, cmd)
	if err != nil {
		sr.Status = task.StatusFailed
		sr.Stderr = fmt.Sprintf("exec error: %v", err)
		sr.Duration = time.Since(start)
		e.emit(ctx, taskID, "step.completed", event.StepCompleted{
			Step:       step.Name,
			Status:     string(sr.Status),
			DurationMs: sr.Duration.Milliseconds(),
		})
		return sr
	}

	// Stream output lines as events.
	for line := range stream.Output {
		e.emit(ctx, taskID, "step.output", event.StepOutput{
			Step:   step.Name,
			Stream: line.Stream,
			Line:   line.Line,
		})
	}

	// Get the final result.
	result := <-stream.Done
	sr.ExitCode = result.ExitCode
	sr.Stdout = result.Stdout
	sr.Stderr = result.Stderr
	sr.Duration = result.Duration

	if result.ExitCode == 0 {
		sr.Status = task.StatusSucceeded
	} else {
		sr.Status = task.StatusFailed
	}

	e.emit(ctx, taskID, "step.completed", event.StepCompleted{
		Step:       step.Name,
		Status:     string(sr.Status),
		DurationMs: sr.Duration.Milliseconds(),
	})

	return sr
}

// emit publishes an event to both the store (for persistence) and the bus (for real-time delivery).
func (e *Executor) emit(ctx context.Context, taskID, eventType string, data any) {
	payload, err := json.Marshal(data)
	if err != nil {
		return // Best effort.
	}

	evt := &event.Event{
		TaskID:    taskID,
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      json.RawMessage(payload),
	}

	if e.Store != nil {
		if id, err := e.Store.Append(ctx, taskID, evt); err == nil {
			evt.ID = id
		}
	}

	if e.Bus != nil {
		e.Bus.Publish(taskID, evt)
	}
}

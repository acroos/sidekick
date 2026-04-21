package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/austinroos/sidekick/internal/agent"
	"github.com/austinroos/sidekick/internal/event"
	"github.com/austinroos/sidekick/internal/sandbox"
	"github.com/austinroos/sidekick/internal/task"
)

// Executor orchestrates workflow execution: creates a sandbox, runs steps
// in topological order, emits events, and applies failure policies.
type Executor struct {
	Provider    sandbox.Provider
	Bus         *event.Bus
	Store       event.Store
	AgentRunner *agent.Runner // nil = agent steps fail gracefully
}

// RunWorkflow parses a workflow file and runs it for the given task.
// This method satisfies the task.Executor interface, decoupling the task
// manager from direct workflow package imports.
func (e *Executor) RunWorkflow(ctx context.Context, taskID, workflowPath string, variables map[string]string) (*task.Task, error) {
	wf, err := ParseFile(workflowPath)
	if err != nil {
		return nil, fmt.Errorf("loading workflow: %w", err)
	}
	return e.Execute(ctx, taskID, wf, variables)
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
	allowHosts := wf.Sandbox.AllowHosts
	if e.AgentRunner != nil && hasAgentSteps(wf) {
		// Ensure sandboxes can reach the LLM proxy through restricted networks.
		allowHosts = append(append([]string{}, allowHosts...), e.AgentRunner.ProxyHost())
	}
	sbCfg := sandbox.Config{
		Image:      wf.Sandbox.Image,
		Network:    sandbox.NetworkPolicy(wf.Sandbox.Network),
		AllowHosts: allowHosts,
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
		sr := e.executeStep(ctx, taskID, sb, step, wf.MaxRetries, variables, results)
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
func (e *Executor) executeStep(ctx context.Context, taskID string, sb sandbox.Sandbox, step *Step, maxRetries int, variables map[string]string, results map[string]task.StepResult) task.StepResult {
	attempts := 1
	if step.OnFailure == FailRetry && maxRetries > 0 {
		attempts = maxRetries + 1
	}

	var sr task.StepResult
	for attempt := range attempts {
		sr = e.runStep(ctx, taskID, sb, step, variables, results)
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
func (e *Executor) runStep(ctx context.Context, taskID string, sb sandbox.Sandbox, step *Step, variables map[string]string, results map[string]task.StepResult) task.StepResult {
	start := time.Now()
	sr := task.StepResult{
		Name:      step.Name,
		StartedAt: start,
	}

	if step.Type == StepAgent {
		return e.runAgentStep(ctx, taskID, sb, step, variables, results, sr, start)
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

// runAgentStep executes an agent step using the Claude Code agent runtime.
func (e *Executor) runAgentStep(ctx context.Context, taskID string, sb sandbox.Sandbox, step *Step, variables map[string]string, results map[string]task.StepResult, sr task.StepResult, start time.Time) task.StepResult {
	if e.AgentRunner == nil {
		sr.Status = task.StatusFailed
		sr.Stderr = "agent runner not configured"
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

	// Build readFile using sandbox CopyOut.
	readFile := func(path string) (string, error) {
		rc, err := sb.CopyOut(ctx, "/workspace/"+path)
		if err != nil {
			return "", err
		}
		defer rc.Close() //nolint:errcheck // best-effort cleanup
		data, err := io.ReadAll(rc)
		return string(data), err
	}

	// Assemble context from file, variable, and step output sources.
	contextStr, err := AssembleContext(step.Context, variables, results, readFile)
	if err != nil {
		sr.Status = task.StatusFailed
		sr.Stderr = fmt.Sprintf("assembling context: %v", err)
		sr.Duration = time.Since(start)
		e.emit(ctx, taskID, "step.completed", event.StepCompleted{
			Step:       step.Name,
			Status:     string(sr.Status),
			DurationMs: sr.Duration.Milliseconds(),
		})
		return sr
	}

	fullPrompt := contextStr + step.Prompt

	// Wrap the executor's emit as the agent's event callback.
	emitFn := func(eventType string, data any) {
		e.emit(ctx, taskID, eventType, data)
	}

	agentResult, err := e.AgentRunner.Run(ctx, sb, agent.RunConfig{
		TaskID:       taskID,
		StepName:     step.Name,
		Prompt:       fullPrompt,
		AllowedTools: step.AllowedTools,
		WorkDir:      "/workspace",
	}, emitFn)

	switch {
	case err != nil:
		sr.Status = task.StatusFailed
		sr.Stderr = fmt.Sprintf("agent error: %v", err)
	case agentResult.ExitCode != 0:
		sr.Status = task.StatusFailed
		sr.ExitCode = agentResult.ExitCode
	default:
		sr.Status = task.StatusSucceeded
		sr.Stdout = agentResult.Output
	}

	sr.ExitCode = agentResult.ExitCode
	sr.TokensUsed = agentResult.TokensUsed
	sr.Duration = time.Since(start)

	e.emit(ctx, taskID, "step.completed", event.StepCompleted{
		Step:       step.Name,
		Status:     string(sr.Status),
		DurationMs: sr.Duration.Milliseconds(),
		TokensUsed: sr.TokensUsed,
	})
	return sr
}

// hasAgentSteps returns true if the workflow contains any agent-type steps.
func hasAgentSteps(wf *Workflow) bool {
	for i := range wf.Steps {
		if wf.Steps[i].Type == StepAgent {
			return true
		}
	}
	return false
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

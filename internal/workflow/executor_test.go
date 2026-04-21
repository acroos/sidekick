package workflow

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/austinroos/sidekick/internal/event"
	"github.com/austinroos/sidekick/internal/sandbox"
	"github.com/austinroos/sidekick/internal/task"
)

// --- Mock sandbox provider and sandbox ---

type mockProvider struct {
	sb *mockSandbox
}

func (p *mockProvider) Create(_ context.Context, _ sandbox.Config) (sandbox.Sandbox, error) {
	return p.sb, nil
}

func (p *mockProvider) Destroy(_ context.Context, _ string) error {
	return nil
}

type mockSandbox struct {
	// execFunc is called for each Exec/ExecStream invocation.
	// The command args are passed so tests can return different results per command.
	execFunc func(args []string) *sandbox.ExecResult
}

func (s *mockSandbox) ID() string { return "mock-sandbox-1" }

func (s *mockSandbox) Exec(_ context.Context, cmd sandbox.Command) (*sandbox.ExecResult, error) {
	r := s.execFunc(cmd.Args)
	return r, nil
}

func (s *mockSandbox) ExecStream(_ context.Context, cmd sandbox.Command) (*sandbox.ExecStream, error) {
	r := s.execFunc(cmd.Args)
	output := make(chan sandbox.OutputLine, 10)
	done := make(chan sandbox.ExecResult, 1)

	// Emit stdout lines.
	if r.Stdout != "" {
		output <- sandbox.OutputLine{Stream: "stdout", Line: r.Stdout, Time: time.Now()}
	}
	if r.Stderr != "" {
		output <- sandbox.OutputLine{Stream: "stderr", Line: r.Stderr, Time: time.Now()}
	}
	close(output)
	done <- *r
	return &sandbox.ExecStream{Output: output, Done: done}, nil
}

func (s *mockSandbox) CopyIn(_ context.Context, _, _ string) error {
	return nil
}

func (s *mockSandbox) CopyOut(_ context.Context, _ string) (io.ReadCloser, error) {
	return nil, nil
}

func (s *mockSandbox) Status() sandbox.Status {
	return sandbox.StatusReady
}

func newMockExecutor(execFunc func(args []string) *sandbox.ExecResult) *Executor {
	bus := event.NewBus()
	store, _ := event.NewSQLiteStore(":memory:")
	return &Executor{
		Provider: &mockProvider{sb: &mockSandbox{execFunc: execFunc}},
		Bus:      bus,
		Store:    store,
	}
}

func simpleWorkflow(steps ...Step) *Workflow {
	return &Workflow{
		Name:    "test",
		Sandbox: SandboxConfig{Image: "test:latest"},
		Steps:   steps,
	}
}

// --- Tests ---

func TestExecuteSimpleSuccess(t *testing.T) {
	exec := newMockExecutor(func(_ []string) *sandbox.ExecResult {
		return &sandbox.ExecResult{ExitCode: 0, Stdout: "ok", Duration: time.Millisecond}
	})

	wf := simpleWorkflow(
		Step{Name: "step1", Type: StepDeterministic, Run: "echo ok"},
	)

	result, err := exec.Execute(context.Background(), "task-1", wf, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Status != task.StatusSucceeded {
		t.Fatalf("expected succeeded, got %s", result.Status)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step result, got %d", len(result.Steps))
	}
	if result.Steps[0].Status != task.StatusSucceeded {
		t.Fatalf("expected step succeeded, got %s", result.Steps[0].Status)
	}
}

func TestExecuteMultipleSteps(t *testing.T) {
	callCount := 0
	exec := newMockExecutor(func(_ []string) *sandbox.ExecResult {
		callCount++
		return &sandbox.ExecResult{ExitCode: 0, Stdout: "ok", Duration: time.Millisecond}
	})

	wf := simpleWorkflow(
		Step{Name: "a", Type: StepDeterministic, Run: "echo a"},
		Step{Name: "b", Type: StepDeterministic, Run: "echo b", DependsOn: []string{"a"}},
		Step{Name: "c", Type: StepDeterministic, Run: "echo c", DependsOn: []string{"b"}},
	)

	result, err := exec.Execute(context.Background(), "task-1", wf, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Status != task.StatusSucceeded {
		t.Fatalf("expected succeeded, got %s", result.Status)
	}
	if callCount != 3 {
		t.Fatalf("expected 3 exec calls, got %d", callCount)
	}
}

func TestExecuteOnFailureAbort(t *testing.T) {
	exec := newMockExecutor(func(args []string) *sandbox.ExecResult {
		// Second step fails.
		if len(args) > 2 && args[2] == "echo b" {
			return &sandbox.ExecResult{ExitCode: 1, Stderr: "fail", Duration: time.Millisecond}
		}
		return &sandbox.ExecResult{ExitCode: 0, Stdout: "ok", Duration: time.Millisecond}
	})

	wf := simpleWorkflow(
		Step{Name: "a", Type: StepDeterministic, Run: "echo a"},
		Step{Name: "b", Type: StepDeterministic, Run: "echo b", OnFailure: FailAbort},
		Step{Name: "c", Type: StepDeterministic, Run: "echo c"},
	)

	result, err := exec.Execute(context.Background(), "task-1", wf, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Status != task.StatusFailed {
		t.Fatalf("expected failed, got %s", result.Status)
	}
	// Step c should not have executed.
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 step results (a succeeded, b failed), got %d", len(result.Steps))
	}
}

func TestExecuteOnFailureContinue(t *testing.T) {
	exec := newMockExecutor(func(args []string) *sandbox.ExecResult {
		if len(args) > 2 && args[2] == "echo b" {
			return &sandbox.ExecResult{ExitCode: 1, Stderr: "fail", Duration: time.Millisecond}
		}
		return &sandbox.ExecResult{ExitCode: 0, Stdout: "ok", Duration: time.Millisecond}
	})

	wf := simpleWorkflow(
		Step{Name: "a", Type: StepDeterministic, Run: "echo a"},
		Step{Name: "b", Type: StepDeterministic, Run: "echo b", OnFailure: FailContinue},
		Step{Name: "c", Type: StepDeterministic, Run: "echo c"},
	)

	result, err := exec.Execute(context.Background(), "task-1", wf, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Status != task.StatusSucceeded {
		t.Fatalf("expected succeeded (continue past failure), got %s", result.Status)
	}
	if len(result.Steps) != 3 {
		t.Fatalf("expected 3 step results, got %d", len(result.Steps))
	}
	if result.Steps[1].Status != task.StatusFailed {
		t.Fatalf("expected step b to be failed, got %s", result.Steps[1].Status)
	}
}

func TestExecuteWhenConditionSkip(t *testing.T) {
	exec := newMockExecutor(func(args []string) *sandbox.ExecResult {
		if len(args) > 2 && args[2] == "echo test" {
			return &sandbox.ExecResult{ExitCode: 1, Duration: time.Millisecond}
		}
		return &sandbox.ExecResult{ExitCode: 0, Stdout: "ok", Duration: time.Millisecond}
	})

	wf := simpleWorkflow(
		Step{Name: "test", Type: StepDeterministic, Run: "echo test", OnFailure: FailContinue},
		Step{Name: "deploy", Type: StepDeterministic, Run: "echo deploy", When: "steps.test.status == 'succeeded'"},
	)

	result, err := exec.Execute(context.Background(), "task-1", wf, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	// Deploy should be skipped because test failed.
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 step results, got %d", len(result.Steps))
	}
	if result.Steps[1].Status != task.StatusSkipped {
		t.Fatalf("expected deploy to be skipped, got %s", result.Steps[1].Status)
	}
}

func TestExecuteVariableInterpolation(t *testing.T) {
	var capturedCmd string
	exec := newMockExecutor(func(args []string) *sandbox.ExecResult {
		if len(args) > 2 {
			capturedCmd = args[2]
		}
		return &sandbox.ExecResult{ExitCode: 0, Stdout: "ok", Duration: time.Millisecond}
	})

	wf := simpleWorkflow(
		Step{Name: "greet", Type: StepDeterministic, Run: "echo hello $NAME"},
	)

	_, err := exec.Execute(context.Background(), "task-1", wf, map[string]string{"NAME": "world"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if capturedCmd != "echo hello world" {
		t.Fatalf("expected interpolated command, got %q", capturedCmd)
	}
}

func TestExecuteEmitsEvents(t *testing.T) {
	exec := newMockExecutor(func(_ []string) *sandbox.ExecResult {
		return &sandbox.ExecResult{ExitCode: 0, Stdout: "ok", Duration: time.Millisecond}
	})

	wf := simpleWorkflow(
		Step{Name: "step1", Type: StepDeterministic, Run: "echo ok"},
	)

	// Subscribe to events before executing.
	ch, unsub := exec.Bus.Subscribe("task-1")
	defer unsub()

	_, err := exec.Execute(context.Background(), "task-1", wf, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	// Collect events.
	var types []string
	for {
		select {
		case evt := <-ch:
			types = append(types, evt.Type)
		default:
			goto done
		}
	}
done:

	// Should have: task.started, step.started, step.output, step.completed, task.completed
	expected := []string{"task.started", "step.started", "step.output", "step.completed", "task.completed"}
	if len(types) < len(expected) {
		t.Fatalf("expected at least %d events, got %d: %v", len(expected), len(types), types)
	}
	if types[0] != "task.started" {
		t.Fatalf("expected first event task.started, got %s", types[0])
	}
	if types[len(types)-1] != "task.completed" {
		t.Fatalf("expected last event task.completed, got %s", types[len(types)-1])
	}
}

func TestExecuteEventsPersistedToStore(t *testing.T) {
	exec := newMockExecutor(func(_ []string) *sandbox.ExecResult {
		return &sandbox.ExecResult{ExitCode: 0, Stdout: "ok", Duration: time.Millisecond}
	})

	wf := simpleWorkflow(
		Step{Name: "step1", Type: StepDeterministic, Run: "echo ok"},
	)

	_, err := exec.Execute(context.Background(), "task-1", wf, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	// Fetch events from store.
	events, err := exec.Store.Fetch(context.Background(), "task-1", 0, 100)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	if len(events) < 4 {
		t.Fatalf("expected at least 4 persisted events, got %d", len(events))
	}

	// Verify monotonic IDs.
	for i := 1; i < len(events); i++ {
		if events[i].ID <= events[i-1].ID {
			t.Fatalf("event IDs not monotonic: %d <= %d", events[i].ID, events[i-1].ID)
		}
	}
}

func TestExecuteRetry(t *testing.T) {
	callCount := 0
	exec := newMockExecutor(func(_ []string) *sandbox.ExecResult {
		callCount++
		if callCount <= 2 {
			return &sandbox.ExecResult{ExitCode: 1, Stderr: "fail", Duration: time.Millisecond}
		}
		return &sandbox.ExecResult{ExitCode: 0, Stdout: "ok", Duration: time.Millisecond}
	})

	wf := simpleWorkflow(
		Step{Name: "flaky", Type: StepDeterministic, Run: "echo flaky", OnFailure: FailRetry},
	)
	wf.MaxRetries = 2

	result, err := exec.Execute(context.Background(), "task-1", wf, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Status != task.StatusSucceeded {
		t.Fatalf("expected succeeded after retries, got %s", result.Status)
	}
	if callCount != 3 {
		t.Fatalf("expected 3 attempts, got %d", callCount)
	}
}

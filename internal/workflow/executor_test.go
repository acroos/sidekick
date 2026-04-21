package workflow

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/austinroos/sidekick/internal/agent"
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
	// streamFunc, if set, provides fine-grained control over ExecStream output lines.
	streamFunc func(args []string) ([]sandbox.OutputLine, sandbox.ExecResult)
	// files maps sandbox paths to contents for CopyOut.
	files map[string]string
}

func (s *mockSandbox) ID() string { return "mock-sandbox-1" }

func (s *mockSandbox) Exec(_ context.Context, cmd sandbox.Command) (*sandbox.ExecResult, error) {
	r := s.execFunc(cmd.Args)
	return r, nil
}

func (s *mockSandbox) ExecStream(_ context.Context, cmd sandbox.Command) (*sandbox.ExecStream, error) {
	// Use streamFunc if available for fine-grained control.
	if s.streamFunc != nil {
		lines, result := s.streamFunc(cmd.Args)
		output := make(chan sandbox.OutputLine, len(lines)+1)
		done := make(chan sandbox.ExecResult, 1)
		for _, l := range lines {
			output <- l
		}
		close(output)
		done <- result
		return &sandbox.ExecStream{Output: output, Done: done}, nil
	}

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

func (s *mockSandbox) CopyOut(_ context.Context, path string) (io.ReadCloser, error) {
	if s.files != nil {
		if content, ok := s.files[path]; ok {
			return io.NopCloser(strings.NewReader(content)), nil
		}
	}
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

// --- Agent step tests ---

// agentStreamLines returns canned stream-json output lines for agent tests.
func agentStreamLines() []sandbox.OutputLine {
	return []sandbox.OutputLine{
		{Stream: "stdout", Line: `{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"Analyzing the code..."}]}}`, Time: time.Now()},
		{Stream: "stdout", Line: `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","id":"c1","input":{"file_path":"src/app.ts"}}]}}`, Time: time.Now()},
		{Stream: "stdout", Line: `{"type":"tool_result","tool":"Read","output":"const x = 1;"}`, Time: time.Now()},
		{Stream: "stdout", Line: `{"type":"result","subtype":"success","result":"Fixed the issue.","is_error":false}`, Time: time.Now()},
	}
}

func newAgentMockExecutor(sb *mockSandbox) *Executor {
	bus := event.NewBus()
	store, _ := event.NewSQLiteStore(":memory:")
	return &Executor{
		Provider:    &mockProvider{sb: sb},
		Bus:         bus,
		Store:       store,
		AgentRunner: &agent.Runner{ProxyAddr: "localhost:8089"},
	}
}

func TestExecuteAgentStepSuccess(t *testing.T) {
	sb := &mockSandbox{
		streamFunc: func(args []string) ([]sandbox.OutputLine, sandbox.ExecResult) {
			// Return agent stream-json for claude commands, normal output otherwise.
			if len(args) > 0 && args[0] == "claude" {
				return agentStreamLines(), sandbox.ExecResult{ExitCode: 0, Duration: time.Second}
			}
			return nil, sandbox.ExecResult{ExitCode: 0, Stdout: "ok", Duration: time.Millisecond}
		},
	}
	exec := newAgentMockExecutor(sb)

	wf := simpleWorkflow(
		Step{
			Name:         "solve",
			Type:         StepAgent,
			Prompt:       "Fix the bug",
			AllowedTools: []string{"Read", "Edit"},
		},
	)

	result, err := exec.Execute(context.Background(), "task-1", wf, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Status != task.StatusSucceeded {
		t.Fatalf("expected succeeded, got %s (error: %s)", result.Status, result.Error)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}
	if result.Steps[0].Status != task.StatusSucceeded {
		t.Fatalf("expected step succeeded, got %s", result.Steps[0].Status)
	}
	if result.Steps[0].Stdout != "Fixed the issue." {
		t.Fatalf("expected agent output in stdout, got %q", result.Steps[0].Stdout)
	}
}

func TestExecuteAgentStepNoRunner(t *testing.T) {
	exec := newMockExecutor(func(_ []string) *sandbox.ExecResult {
		return &sandbox.ExecResult{ExitCode: 0, Duration: time.Millisecond}
	})
	// AgentRunner is nil by default in newMockExecutor.

	wf := simpleWorkflow(
		Step{Name: "solve", Type: StepAgent, Prompt: "Fix it"},
	)

	result, err := exec.Execute(context.Background(), "task-1", wf, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Status != task.StatusFailed {
		t.Fatalf("expected failed without agent runner, got %s", result.Status)
	}
}

func TestExecuteAgentStepEmitsEvents(t *testing.T) {
	sb := &mockSandbox{
		streamFunc: func(args []string) ([]sandbox.OutputLine, sandbox.ExecResult) {
			if len(args) > 0 && args[0] == "claude" {
				return agentStreamLines(), sandbox.ExecResult{ExitCode: 0, Duration: time.Second}
			}
			return nil, sandbox.ExecResult{ExitCode: 0, Duration: time.Millisecond}
		},
	}
	exec := newAgentMockExecutor(sb)

	wf := simpleWorkflow(
		Step{Name: "solve", Type: StepAgent, Prompt: "Fix it", AllowedTools: []string{"Read"}},
	)

	ch, unsub := exec.Bus.Subscribe("task-1")
	defer unsub()

	_, err := exec.Execute(context.Background(), "task-1", wf, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

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

	// Expect: task.started, step.started, agent.thinking, agent.action, agent.action_result,
	//         agent.output, step.completed, task.completed
	if len(types) < 8 {
		t.Fatalf("expected at least 8 events, got %d: %v", len(types), types)
	}

	// Verify agent events are present.
	agentTypes := map[string]bool{}
	for _, typ := range types {
		if strings.HasPrefix(typ, "agent.") {
			agentTypes[typ] = true
		}
	}
	for _, want := range []string{"agent.thinking", "agent.action", "agent.action_result", "agent.output"} {
		if !agentTypes[want] {
			t.Fatalf("missing event type %s in %v", want, types)
		}
	}
}

func TestExecuteAgentStepWithContext(t *testing.T) {
	var capturedPrompt string
	sb := &mockSandbox{
		streamFunc: func(args []string) ([]sandbox.OutputLine, sandbox.ExecResult) {
			if len(args) > 0 && args[0] == "claude" {
				capturedPrompt = args[2] // -p is at index 1, prompt at index 2
				return []sandbox.OutputLine{
					{Stream: "stdout", Line: `{"type":"result","result":"Done."}`, Time: time.Now()},
				}, sandbox.ExecResult{ExitCode: 0, Duration: time.Millisecond}
			}
			return nil, sandbox.ExecResult{ExitCode: 0, Duration: time.Millisecond}
		},
		files: map[string]string{
			"/workspace/.sidekick/standards.md": "Use TypeScript strict mode.",
		},
	}
	exec := newAgentMockExecutor(sb)

	wf := simpleWorkflow(
		Step{
			Name: "solve",
			Type: StepAgent,
			Context: []ContextItem{
				{Type: ContextFile, Path: ".sidekick/standards.md", Label: "Standards"},
				{Type: ContextVariable, Key: "TASK", Label: "Task"},
			},
			Prompt: "Fix the bug.",
		},
	)

	_, err := exec.Execute(context.Background(), "task-1", wf, map[string]string{"TASK": "Fix null pointer"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	// Verify context was assembled into the prompt.
	if !strings.Contains(capturedPrompt, "Use TypeScript strict mode.") {
		t.Fatalf("expected file context in prompt, got:\n%s", capturedPrompt)
	}
	if !strings.Contains(capturedPrompt, "Fix null pointer") {
		t.Fatalf("expected variable context in prompt, got:\n%s", capturedPrompt)
	}
	if !strings.Contains(capturedPrompt, "Fix the bug.") {
		t.Fatalf("expected step prompt at end, got:\n%s", capturedPrompt)
	}
}

func TestExecuteMixedWorkflow(t *testing.T) {
	sb := &mockSandbox{
		streamFunc: func(args []string) ([]sandbox.OutputLine, sandbox.ExecResult) {
			if len(args) > 0 && args[0] == "claude" {
				return []sandbox.OutputLine{
					{Stream: "stdout", Line: `{"type":"result","result":"Fixed."}`, Time: time.Now()},
				}, sandbox.ExecResult{ExitCode: 0, Duration: time.Millisecond}
			}
			// Deterministic steps.
			return []sandbox.OutputLine{
				{Stream: "stdout", Line: "ok", Time: time.Now()},
			}, sandbox.ExecResult{ExitCode: 0, Stdout: "ok", Duration: time.Millisecond}
		},
	}
	exec := newAgentMockExecutor(sb)

	wf := simpleWorkflow(
		Step{Name: "clone", Type: StepDeterministic, Run: "git clone repo"},
		Step{Name: "solve", Type: StepAgent, Prompt: "Fix bug", DependsOn: []string{"clone"}},
		Step{Name: "test", Type: StepDeterministic, Run: "npm test", DependsOn: []string{"solve"}},
	)

	result, err := exec.Execute(context.Background(), "task-1", wf, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Status != task.StatusSucceeded {
		t.Fatalf("expected succeeded, got %s (error: %s)", result.Status, result.Error)
	}
	if len(result.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(result.Steps))
	}
	for _, sr := range result.Steps {
		if sr.Status != task.StatusSucceeded {
			t.Fatalf("expected all steps succeeded, %s got %s", sr.Name, sr.Status)
		}
	}
}

func TestExecuteAgentStepFailure(t *testing.T) {
	sb := &mockSandbox{
		streamFunc: func(args []string) ([]sandbox.OutputLine, sandbox.ExecResult) {
			if len(args) > 0 && args[0] == "claude" {
				return nil, sandbox.ExecResult{ExitCode: 1, Stderr: "claude error", Duration: time.Millisecond}
			}
			return nil, sandbox.ExecResult{ExitCode: 0, Duration: time.Millisecond}
		},
	}
	exec := newAgentMockExecutor(sb)

	wf := simpleWorkflow(
		Step{Name: "solve", Type: StepAgent, Prompt: "Fix it"},
	)

	result, err := exec.Execute(context.Background(), "task-1", wf, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Status != task.StatusFailed {
		t.Fatalf("expected failed, got %s", result.Status)
	}
	if result.Steps[0].ExitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", result.Steps[0].ExitCode)
	}
}

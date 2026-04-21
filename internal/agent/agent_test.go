package agent

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/austinroos/sidekick/internal/sandbox"
)

// --- Mock sandbox ---

type mockSandbox struct {
	execFunc func(args []string) ([]sandbox.OutputLine, sandbox.ExecResult)
}

func (s *mockSandbox) ID() string { return "mock-agent-sandbox" }

func (s *mockSandbox) Exec(_ context.Context, cmd sandbox.Command) (*sandbox.ExecResult, error) {
	_, result := s.execFunc(cmd.Args)
	return &result, nil
}

func (s *mockSandbox) ExecStream(_ context.Context, cmd sandbox.Command) (*sandbox.ExecStream, error) {
	lines, result := s.execFunc(cmd.Args)
	output := make(chan sandbox.OutputLine, len(lines)+1)
	done := make(chan sandbox.ExecResult, 1)
	for _, l := range lines {
		output <- l
	}
	close(output)
	done <- result
	return &sandbox.ExecStream{Output: output, Done: done}, nil
}

func (s *mockSandbox) CopyIn(_ context.Context, _, _ string) error          { return nil }
func (s *mockSandbox) CopyOut(_ context.Context, _ string) (io.ReadCloser, error) { return nil, nil }
func (s *mockSandbox) Status() sandbox.Status                               { return sandbox.StatusReady }

// --- Tests ---

func TestRunEmitsEvents(t *testing.T) {
	sb := &mockSandbox{
		execFunc: func(_ []string) ([]sandbox.OutputLine, sandbox.ExecResult) {
			lines := []sandbox.OutputLine{
				{Stream: "stdout", Line: `{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"Analyzing the code..."}]}}`, Time: time.Now()},
				{Stream: "stdout", Line: `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","id":"c1","input":{"file_path":"src/app.ts"}}]}}`, Time: time.Now()},
				{Stream: "stdout", Line: `{"type":"tool_result","tool":"Read","output":"const x = 1;"}`, Time: time.Now()},
				{Stream: "stdout", Line: `{"type":"result","subtype":"success","result":"Fixed the issue.","is_error":false}`, Time: time.Now()},
			}
			return lines, sandbox.ExecResult{ExitCode: 0, Duration: time.Second}
		},
	}

	runner := &Runner{ProxyAddr: "localhost:8089"}
	var emitted []string
	emitFn := func(eventType string, _ any) {
		emitted = append(emitted, eventType)
	}

	result, err := runner.Run(context.Background(), sb, RunConfig{
		TaskID:       "task-1",
		StepName:     "solve",
		Prompt:       "Fix the bug",
		AllowedTools: []string{"Read", "Edit"},
	}, emitFn)

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Output != "Fixed the issue." {
		t.Fatalf("expected output 'Fixed the issue.', got %q", result.Output)
	}

	expected := []string{"agent.thinking", "agent.action", "agent.action_result", "agent.output"}
	if len(emitted) != len(expected) {
		t.Fatalf("expected %d events, got %d: %v", len(expected), len(emitted), emitted)
	}
	for i, want := range expected {
		if emitted[i] != want {
			t.Fatalf("event %d: expected %s, got %s", i, want, emitted[i])
		}
	}
}

func TestRunCommandArgs(t *testing.T) {
	var capturedArgs []string
	var capturedEnv map[string]string
	sb := &mockSandbox{
		execFunc: func(args []string) ([]sandbox.OutputLine, sandbox.ExecResult) {
			capturedArgs = args
			return nil, sandbox.ExecResult{ExitCode: 0, Duration: time.Millisecond}
		},
	}

	// Override ExecStream to also capture env.
	origExecStream := sb.ExecStream
	_ = origExecStream
	sb2 := &envCaptureSandbox{
		mockSandbox: sb,
		capturedEnv: &capturedEnv,
	}

	runner := &Runner{ProxyAddr: "myhost:9000"}
	result, err := runner.Run(context.Background(), sb2, RunConfig{
		TaskID:       "task-42",
		StepName:     "solve",
		Prompt:       "Fix it",
		AllowedTools: []string{"Read", "Bash"},
		WorkDir:      "/code",
	}, func(string, any) {})

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}

	// Verify command args.
	if len(capturedArgs) < 5 {
		t.Fatalf("expected at least 5 args, got %v", capturedArgs)
	}
	if capturedArgs[0] != "claude" {
		t.Fatalf("expected claude command, got %s", capturedArgs[0])
	}
	if capturedArgs[2] != "Fix it" {
		t.Fatalf("expected prompt in args, got %s", capturedArgs[2])
	}

	// Verify env.
	if capturedEnv == nil {
		t.Fatal("expected env to be captured")
	}
	expectedURL := "http://myhost:9000/proxy/task-42"
	if capturedEnv["ANTHROPIC_BASE_URL"] != expectedURL {
		t.Fatalf("expected ANTHROPIC_BASE_URL=%s, got %s", expectedURL, capturedEnv["ANTHROPIC_BASE_URL"])
	}
}

func TestRunExitCodePropagation(t *testing.T) {
	sb := &mockSandbox{
		execFunc: func(_ []string) ([]sandbox.OutputLine, sandbox.ExecResult) {
			return nil, sandbox.ExecResult{ExitCode: 1, Stderr: "error", Duration: time.Millisecond}
		},
	}

	runner := &Runner{ProxyAddr: "localhost:8089"}
	result, err := runner.Run(context.Background(), sb, RunConfig{
		TaskID:   "task-1",
		StepName: "solve",
		Prompt:   "Fix it",
	}, func(string, any) {})

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", result.ExitCode)
	}
}

func TestRunSkipsStderrLines(t *testing.T) {
	sb := &mockSandbox{
		execFunc: func(_ []string) ([]sandbox.OutputLine, sandbox.ExecResult) {
			lines := []sandbox.OutputLine{
				{Stream: "stderr", Line: "debug info", Time: time.Now()},
				{Stream: "stdout", Line: `{"type":"result","result":"Done."}`, Time: time.Now()},
			}
			return lines, sandbox.ExecResult{ExitCode: 0, Duration: time.Millisecond}
		},
	}

	runner := &Runner{ProxyAddr: "localhost:8089"}
	var emitted []string
	result, err := runner.Run(context.Background(), sb, RunConfig{
		TaskID:   "task-1",
		StepName: "solve",
		Prompt:   "Fix it",
	}, func(eventType string, _ any) {
		emitted = append(emitted, eventType)
	})

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Output != "Done." {
		t.Fatalf("expected output 'Done.', got %q", result.Output)
	}
	// Only the result event should have been emitted, not the stderr line.
	if len(emitted) != 1 {
		t.Fatalf("expected 1 event, got %d: %v", len(emitted), emitted)
	}
}

func TestRunWithoutProxy(t *testing.T) {
	sb := &mockSandbox{
		execFunc: func(_ []string) ([]sandbox.OutputLine, sandbox.ExecResult) {
			return nil, sandbox.ExecResult{ExitCode: 0, Duration: time.Millisecond}
		},
	}

	runner := &Runner{ProxyAddr: "localhost:8089", Proxy: nil}
	result, err := runner.Run(context.Background(), sb, RunConfig{
		TaskID:   "task-1",
		StepName: "solve",
		Prompt:   "Fix it",
	}, func(string, any) {})

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.TokensUsed != 0 {
		t.Fatalf("expected 0 tokens without proxy, got %d", result.TokensUsed)
	}
}

func TestProxyHost(t *testing.T) {
	runner := &Runner{ProxyAddr: "host.docker.internal:8089"}
	if got := runner.ProxyHost(); got != "host.docker.internal" {
		t.Fatalf("expected host.docker.internal, got %s", got)
	}
}

// envCaptureSandbox wraps mockSandbox to capture Command.Env.
type envCaptureSandbox struct {
	*mockSandbox
	capturedEnv *map[string]string
}

func (s *envCaptureSandbox) ExecStream(_ context.Context, cmd sandbox.Command) (*sandbox.ExecStream, error) {
	*s.capturedEnv = cmd.Env
	lines, result := s.mockSandbox.execFunc(cmd.Args)
	output := make(chan sandbox.OutputLine, len(lines)+1)
	done := make(chan sandbox.ExecResult, 1)
	for _, l := range lines {
		output <- l
	}
	close(output)
	done <- result
	return &sandbox.ExecStream{Output: output, Done: done}, nil
}

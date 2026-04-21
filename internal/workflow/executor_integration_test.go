package workflow

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/austinroos/sidekick/internal/event"
	"github.com/austinroos/sidekick/internal/sandbox"
	"github.com/austinroos/sidekick/internal/task"
)

func skipIfNoIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("SIDEKICK_INTEGRATION") == "" {
		t.Skip("set SIDEKICK_INTEGRATION=1 to run integration tests")
	}
}

func TestExecutorIntegration(t *testing.T) {
	skipIfNoIntegration(t)

	provider, err := sandbox.NewDockerProvider()
	if err != nil {
		t.Fatalf("creating docker provider: %v", err)
	}

	store, err := event.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("creating sqlite store: %v", err)
	}
	defer func() { _ = store.Close() }()

	bus := event.NewBus()

	exec := &Executor{
		Provider: provider,
		Bus:      bus,
		Store:    store,
	}

	wf := &Workflow{
		Name:    "integration-test",
		Timeout: Duration{Duration: 2 * time.Minute},
		Sandbox: SandboxConfig{
			Image:   "sidekick-sandbox-base:latest",
			Network: "none",
		},
		Steps: []Step{
			{
				Name:      "setup",
				Type:      StepDeterministic,
				Run:       "echo setting up",
				Timeout:   Duration{Duration: 30 * time.Second},
				OnFailure: FailAbort,
			},
			{
				Name:      "greet",
				Type:      StepDeterministic,
				Run:       "echo hello $NAME",
				Timeout:   Duration{Duration: 30 * time.Second},
				OnFailure: FailAbort,
				DependsOn: []string{"setup"},
			},
			{
				Name:    "deploy",
				Type:    StepDeterministic,
				Run:     "echo deploying",
				Timeout: Duration{Duration: 30 * time.Second},
				When:    "steps.greet.status == 'succeeded'",
			},
		},
	}

	result, err := exec.Execute(context.Background(), "integ-task-1", wf, map[string]string{
		"NAME": "world",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if result.Status != task.StatusSucceeded {
		t.Fatalf("expected succeeded, got %s (error: %s)", result.Status, result.Error)
	}
	if len(result.Steps) != 3 {
		t.Fatalf("expected 3 step results, got %d", len(result.Steps))
	}

	// Verify step outputs.
	for _, sr := range result.Steps {
		if sr.Status != task.StatusSucceeded {
			t.Fatalf("step %q: expected succeeded, got %s", sr.Name, sr.Status)
		}
	}

	// Verify events persisted to store.
	events, err := store.Fetch(context.Background(), "integ-task-1", 0, 100)
	if err != nil {
		t.Fatalf("fetching events: %v", err)
	}

	// Should have: task.started, 3x(step.started + step.output + step.completed), task.completed
	if len(events) < 8 {
		t.Fatalf("expected at least 8 events, got %d", len(events))
	}

	// Verify first and last event types.
	if events[0].Type != "task.started" {
		t.Fatalf("expected first event task.started, got %s", events[0].Type)
	}
	if events[len(events)-1].Type != "task.completed" {
		t.Fatalf("expected last event task.completed, got %s", events[len(events)-1].Type)
	}

	// Verify monotonic IDs.
	for i := 1; i < len(events); i++ {
		if events[i].ID <= events[i-1].ID {
			t.Fatalf("event IDs not monotonic: %d <= %d", events[i].ID, events[i-1].ID)
		}
	}
}

func TestExecutorIntegrationWhenSkip(t *testing.T) {
	skipIfNoIntegration(t)

	provider, err := sandbox.NewDockerProvider()
	if err != nil {
		t.Fatalf("creating docker provider: %v", err)
	}

	store, err := event.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("creating sqlite store: %v", err)
	}
	defer func() { _ = store.Close() }()

	exec := &Executor{
		Provider: provider,
		Bus:      event.NewBus(),
		Store:    store,
	}

	wf := &Workflow{
		Name:    "skip-test",
		Timeout: Duration{Duration: 2 * time.Minute},
		Sandbox: SandboxConfig{
			Image:   "sidekick-sandbox-base:latest",
			Network: "none",
		},
		Steps: []Step{
			{
				Name:      "fail-step",
				Type:      StepDeterministic,
				Run:       "exit 1",
				Timeout:   Duration{Duration: 30 * time.Second},
				OnFailure: FailContinue,
			},
			{
				Name:    "should-skip",
				Type:    StepDeterministic,
				Run:     "echo should not run",
				Timeout: Duration{Duration: 30 * time.Second},
				When:    "steps.fail-step.status == 'succeeded'",
			},
		},
	}

	result, err := exec.Execute(context.Background(), "integ-task-2", wf, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if result.Status != task.StatusSucceeded {
		t.Fatalf("expected succeeded (continue policy), got %s", result.Status)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 step results, got %d", len(result.Steps))
	}
	if result.Steps[0].Status != task.StatusFailed {
		t.Fatalf("expected fail-step to be failed, got %s", result.Steps[0].Status)
	}
	if result.Steps[1].Status != task.StatusSkipped {
		t.Fatalf("expected should-skip to be skipped, got %s", result.Steps[1].Status)
	}
}

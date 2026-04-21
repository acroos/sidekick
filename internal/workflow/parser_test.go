package workflow

import (
	"testing"
	"time"
)

const validWorkflowYAML = `
name: test-workflow
timeout: 10m
max_retries: 1

sandbox:
  image: sidekick-sandbox-base:latest
  network: none

steps:
  - name: setup
    type: deterministic
    run: echo "hello"
    timeout: 1m
    on_failure: abort

  - name: build
    type: deterministic
    run: echo "building $PROJECT_NAME"
    timeout: 5m
    on_failure: abort
    depends_on: [setup]

  - name: test
    type: deterministic
    run: echo "testing"
    timeout: 5m
    on_failure: continue

  - name: deploy
    type: deterministic
    when: steps.test.status == 'succeeded'
    run: echo "deploying"
    timeout: 2m
    depends_on: [test]
`

func TestParseValidWorkflow(t *testing.T) {
	wf, err := Parse([]byte(validWorkflowYAML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if wf.Name != "test-workflow" {
		t.Fatalf("expected name 'test-workflow', got %q", wf.Name)
	}
	if wf.Timeout.Duration != 10*time.Minute {
		t.Fatalf("expected timeout 10m, got %v", wf.Timeout.Duration)
	}
	if wf.MaxRetries != 1 {
		t.Fatalf("expected max_retries 1, got %d", wf.MaxRetries)
	}
	if wf.Sandbox.Image != "sidekick-sandbox-base:latest" {
		t.Fatalf("expected image sidekick-sandbox-base:latest, got %q", wf.Sandbox.Image)
	}
	if wf.Sandbox.Network != "none" {
		t.Fatalf("expected network 'none', got %q", wf.Sandbox.Network)
	}
	if len(wf.Steps) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(wf.Steps))
	}

	// Check first step.
	s := wf.Steps[0]
	if s.Name != "setup" {
		t.Fatalf("expected step name 'setup', got %q", s.Name)
	}
	if s.Type != StepDeterministic {
		t.Fatalf("expected step type 'deterministic', got %q", s.Type)
	}
	if s.Timeout.Duration != 1*time.Minute {
		t.Fatalf("expected step timeout 1m, got %v", s.Timeout.Duration)
	}
	if s.OnFailure != FailAbort {
		t.Fatalf("expected on_failure 'abort', got %q", s.OnFailure)
	}

	// Check dependencies.
	build := wf.Steps[1]
	if len(build.DependsOn) != 1 || build.DependsOn[0] != "setup" {
		t.Fatalf("expected build depends_on [setup], got %v", build.DependsOn)
	}

	// Check when condition.
	deploy := wf.Steps[3]
	if deploy.When != "steps.test.status == 'succeeded'" {
		t.Fatalf("expected when condition, got %q", deploy.When)
	}
}

func TestParseDurationFormats(t *testing.T) {
	yaml := `
name: dur-test
sandbox:
  image: test
steps:
  - name: s1
    type: deterministic
    run: echo hi
    timeout: 30s
  - name: s2
    type: deterministic
    run: echo hi
    timeout: 2h30m
`
	wf, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if wf.Steps[0].Timeout.Duration != 30*time.Second {
		t.Fatalf("expected 30s, got %v", wf.Steps[0].Timeout.Duration)
	}
	if wf.Steps[1].Timeout.Duration != 2*time.Hour+30*time.Minute {
		t.Fatalf("expected 2h30m, got %v", wf.Steps[1].Timeout.Duration)
	}
}

func TestParseMalformedYAML(t *testing.T) {
	_, err := Parse([]byte(`{invalid yaml`))
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestParseAgentStep(t *testing.T) {
	yaml := `
name: agent-test
sandbox:
  image: test
steps:
  - name: solve
    type: agent
    prompt: "Fix the bug"
    context:
      - type: file
        path: README.md
        label: "Readme"
      - type: variable
        key: TASK_DESC
        label: "Task"
      - type: step_output
        step: build
        output: stderr
        label: "Build errors"
        max_lines: 20
    allowed_tools:
      - Edit
      - Read
      - Bash
    timeout: 20m
`
	wf, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := wf.Steps[0]
	if s.Type != StepAgent {
		t.Fatalf("expected agent type, got %q", s.Type)
	}
	if s.Prompt != "Fix the bug" {
		t.Fatalf("expected prompt, got %q", s.Prompt)
	}
	if len(s.Context) != 3 {
		t.Fatalf("expected 3 context items, got %d", len(s.Context))
	}
	if s.Context[0].Type != ContextFile || s.Context[0].Path != "README.md" {
		t.Fatalf("unexpected first context item: %+v", s.Context[0])
	}
	if s.Context[2].MaxLines != 20 {
		t.Fatalf("expected max_lines 20, got %d", s.Context[2].MaxLines)
	}
	if len(s.AllowedTools) != 3 {
		t.Fatalf("expected 3 allowed tools, got %d", len(s.AllowedTools))
	}
}

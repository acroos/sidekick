package workflow

import (
	"strings"
	"testing"
)

func minimalWorkflow(steps ...Step) *Workflow {
	return &Workflow{
		Name:    "test",
		Sandbox: SandboxConfig{Image: "test:latest"},
		Steps:   steps,
	}
}

func TestValidateValid(t *testing.T) {
	wf := minimalWorkflow(
		Step{Name: "a", Type: StepDeterministic, Run: "echo a"},
		Step{Name: "b", Type: StepDeterministic, Run: "echo b", DependsOn: []string{"a"}},
	)
	if err := Validate(wf); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidateMissingName(t *testing.T) {
	wf := minimalWorkflow(Step{Name: "a", Type: StepDeterministic, Run: "echo a"})
	wf.Name = ""
	err := Validate(wf)
	if err == nil || !strings.Contains(err.Error(), "workflow name is required") {
		t.Fatalf("expected name error, got: %v", err)
	}
}

func TestValidateMissingImage(t *testing.T) {
	wf := minimalWorkflow(Step{Name: "a", Type: StepDeterministic, Run: "echo a"})
	wf.Sandbox.Image = ""
	err := Validate(wf)
	if err == nil || !strings.Contains(err.Error(), "sandbox image is required") {
		t.Fatalf("expected image error, got: %v", err)
	}
}

func TestValidateNoSteps(t *testing.T) {
	wf := minimalWorkflow()
	err := Validate(wf)
	if err == nil || !strings.Contains(err.Error(), "at least one step") {
		t.Fatalf("expected no-steps error, got: %v", err)
	}
}

func TestValidateDuplicateStepNames(t *testing.T) {
	wf := minimalWorkflow(
		Step{Name: "a", Type: StepDeterministic, Run: "echo a"},
		Step{Name: "a", Type: StepDeterministic, Run: "echo b"},
	)
	err := Validate(wf)
	if err == nil || !strings.Contains(err.Error(), "duplicate step name") {
		t.Fatalf("expected duplicate error, got: %v", err)
	}
}

func TestValidateInvalidStepType(t *testing.T) {
	wf := minimalWorkflow(
		Step{Name: "a", Type: "unknown", Run: "echo a"},
	)
	err := Validate(wf)
	if err == nil || !strings.Contains(err.Error(), "invalid type") {
		t.Fatalf("expected type error, got: %v", err)
	}
}

func TestValidateDeterministicMissingRun(t *testing.T) {
	wf := minimalWorkflow(Step{Name: "a", Type: StepDeterministic})
	err := Validate(wf)
	if err == nil || !strings.Contains(err.Error(), "run is required") {
		t.Fatalf("expected run error, got: %v", err)
	}
}

func TestValidateAgentMissingPrompt(t *testing.T) {
	wf := minimalWorkflow(Step{Name: "a", Type: StepAgent})
	err := Validate(wf)
	if err == nil || !strings.Contains(err.Error(), "prompt is required") {
		t.Fatalf("expected prompt error, got: %v", err)
	}
}

func TestValidateUnknownDependency(t *testing.T) {
	wf := minimalWorkflow(
		Step{Name: "a", Type: StepDeterministic, Run: "echo a", DependsOn: []string{"nonexistent"}},
	)
	err := Validate(wf)
	if err == nil || !strings.Contains(err.Error(), "unknown step nonexistent") {
		t.Fatalf("expected unknown dep error, got: %v", err)
	}
}

func TestValidateUnknownWhenRef(t *testing.T) {
	wf := minimalWorkflow(
		Step{Name: "a", Type: StepDeterministic, Run: "echo a", When: "steps.missing.status == 'succeeded'"},
	)
	err := Validate(wf)
	if err == nil || !strings.Contains(err.Error(), "when references unknown step") {
		t.Fatalf("expected when ref error, got: %v", err)
	}
}

func TestValidateCycleSelfLoop(t *testing.T) {
	wf := minimalWorkflow(
		Step{Name: "a", Type: StepDeterministic, Run: "echo a", DependsOn: []string{"a"}},
	)
	err := Validate(wf)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got: %v", err)
	}
}

func TestValidateCycleMultiNode(t *testing.T) {
	wf := minimalWorkflow(
		Step{Name: "a", Type: StepDeterministic, Run: "echo a", DependsOn: []string{"c"}},
		Step{Name: "b", Type: StepDeterministic, Run: "echo b", DependsOn: []string{"a"}},
		Step{Name: "c", Type: StepDeterministic, Run: "echo c", DependsOn: []string{"b"}},
	)
	err := Validate(wf)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got: %v", err)
	}
}

func TestValidateImplicitWhenDependency(t *testing.T) {
	// Step b has when referencing a but no explicit depends_on.
	// Validator should auto-infer and not error.
	wf := minimalWorkflow(
		Step{Name: "a", Type: StepDeterministic, Run: "echo a"},
		Step{Name: "b", Type: StepDeterministic, Run: "echo b", When: "steps.a.status == 'succeeded'"},
	)
	if err := Validate(wf); err != nil {
		t.Fatalf("expected valid with implicit when dep, got: %v", err)
	}
}

func TestTopologicalOrder(t *testing.T) {
	steps := []Step{
		{Name: "c", Type: StepDeterministic, Run: "echo c", DependsOn: []string{"b"}},
		{Name: "a", Type: StepDeterministic, Run: "echo a"},
		{Name: "b", Type: StepDeterministic, Run: "echo b", DependsOn: []string{"a"}},
	}
	order := TopologicalOrder(steps)

	indexOf := func(name string) int {
		for i, n := range order {
			if n == name {
				return i
			}
		}
		return -1
	}

	if indexOf("a") > indexOf("b") {
		t.Fatal("a should come before b")
	}
	if indexOf("b") > indexOf("c") {
		t.Fatal("b should come before c")
	}
}

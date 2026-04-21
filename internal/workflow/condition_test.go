package workflow

import (
	"testing"

	"github.com/austinroos/sidekick/internal/task"
)

func TestEvalConditionEquals(t *testing.T) {
	results := map[string]task.StepResult{
		"test": {Name: "test", Status: task.StatusSucceeded},
	}

	ok, err := EvalCondition("steps.test.status == 'succeeded'", results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected true")
	}
}

func TestEvalConditionEqualsFalse(t *testing.T) {
	results := map[string]task.StepResult{
		"test": {Name: "test", Status: task.StatusFailed},
	}

	ok, err := EvalCondition("steps.test.status == 'succeeded'", results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected false")
	}
}

func TestEvalConditionNotEquals(t *testing.T) {
	results := map[string]task.StepResult{
		"test": {Name: "test", Status: task.StatusFailed},
	}

	ok, err := EvalCondition("steps.test.status != 'succeeded'", results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected true")
	}
}

func TestEvalConditionEmptyExpression(t *testing.T) {
	ok, err := EvalCondition("", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("empty expression should evaluate to true")
	}
}

func TestEvalConditionUnknownStep(t *testing.T) {
	results := map[string]task.StepResult{}

	_, err := EvalCondition("steps.missing.status == 'succeeded'", results)
	if err == nil {
		t.Fatal("expected error for unknown step")
	}
}

func TestEvalConditionMalformed(t *testing.T) {
	results := map[string]task.StepResult{}

	cases := []string{
		"invalid expression",
		"steps.",
		"steps.test",
		"steps.test.status",
		"steps.test.status > 'succeeded'",
		"steps.test.name == 'test'",
	}
	for _, expr := range cases {
		_, err := EvalCondition(expr, results)
		if err == nil {
			t.Fatalf("expected error for %q", expr)
		}
	}
}

func TestEvalConditionDoubleQuotes(t *testing.T) {
	results := map[string]task.StepResult{
		"test": {Name: "test", Status: task.StatusSucceeded},
	}

	ok, err := EvalCondition(`steps.test.status == "succeeded"`, results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected true with double quotes")
	}
}

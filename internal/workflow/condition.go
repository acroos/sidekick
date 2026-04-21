package workflow

import (
	"fmt"
	"strings"

	"github.com/austinroos/sidekick/internal/task"
)

// EvalCondition evaluates a when expression against completed step results.
// Supported expressions:
//
//	steps.<name>.status == '<value>'
//	steps.<name>.status != '<value>'
//
// Returns (true, nil) if the condition is met, (false, nil) if not,
// and (false, error) if the expression is malformed or references an unknown step.
func EvalCondition(expr string, results map[string]task.StepResult) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true, nil
	}

	// Parse: steps.<name>.status <op> '<value>'
	if !strings.HasPrefix(expr, "steps.") {
		return false, fmt.Errorf("unsupported when expression: %q", expr)
	}

	rest := strings.TrimPrefix(expr, "steps.")

	// Extract step name (everything before the next dot).
	dotIdx := strings.Index(rest, ".")
	if dotIdx == -1 {
		return false, fmt.Errorf("malformed when expression: %q", expr)
	}
	stepName := rest[:dotIdx]
	rest = rest[dotIdx+1:]

	// Expect "status" field.
	if !strings.HasPrefix(rest, "status") {
		return false, fmt.Errorf("unsupported field in when expression: %q (only 'status' is supported)", expr)
	}
	rest = strings.TrimSpace(strings.TrimPrefix(rest, "status"))

	// Parse operator.
	var op string
	switch {
	case strings.HasPrefix(rest, "=="):
		op = "=="
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "=="))
	case strings.HasPrefix(rest, "!="):
		op = "!="
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "!="))
	default:
		return false, fmt.Errorf("unsupported operator in when expression: %q", expr)
	}

	// Parse quoted value.
	if len(rest) < 2 || (rest[0] != '\'' && rest[0] != '"') {
		return false, fmt.Errorf("expected quoted value in when expression: %q", expr)
	}
	quote := rest[0]
	endIdx := strings.IndexByte(rest[1:], quote)
	if endIdx == -1 {
		return false, fmt.Errorf("unterminated quote in when expression: %q", expr)
	}
	value := rest[1 : endIdx+1]

	// Look up the step result.
	result, ok := results[stepName]
	if !ok {
		return false, fmt.Errorf("when expression references unknown step %q", stepName)
	}

	// Compare.
	actual := string(result.Status)
	switch op {
	case "==":
		return actual == value, nil
	case "!=":
		return actual != value, nil
	default:
		return false, fmt.Errorf("unsupported operator %q", op)
	}
}

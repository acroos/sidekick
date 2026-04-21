package workflow

import (
	"fmt"
	"strings"
)

// Validate checks a workflow for structural errors: required fields,
// unique step names, valid references, and DAG cycle detection.
// Returns a multi-error listing all issues found, or nil if valid.
func Validate(wf *Workflow) error {
	var errs []string

	if wf.Name == "" {
		errs = append(errs, "workflow name is required")
	}
	if wf.Sandbox.Image == "" {
		errs = append(errs, "sandbox image is required")
	}
	if len(wf.Steps) == 0 {
		errs = append(errs, "workflow must have at least one step")
	}

	stepNames := make(map[string]bool, len(wf.Steps))
	for i, s := range wf.Steps {
		prefix := fmt.Sprintf("step[%d]", i)

		if s.Name == "" {
			errs = append(errs, prefix+": name is required")
			continue
		}
		prefix = fmt.Sprintf("step %q", s.Name)

		if stepNames[s.Name] {
			errs = append(errs, prefix+": duplicate step name")
		}
		stepNames[s.Name] = true

		if s.Type == "" {
			errs = append(errs, prefix+": type is required")
		} else if s.Type != StepDeterministic && s.Type != StepAgent {
			errs = append(errs, prefix+": invalid type "+string(s.Type))
		}

		if s.Type == StepDeterministic && s.Run == "" {
			errs = append(errs, prefix+": run is required for deterministic steps")
		}
		if s.Type == StepAgent && s.Prompt == "" {
			errs = append(errs, prefix+": prompt is required for agent steps")
		}

		if s.OnFailure != "" && s.OnFailure != FailAbort && s.OnFailure != FailContinue && s.OnFailure != FailRetry {
			errs = append(errs, prefix+": invalid on_failure policy "+string(s.OnFailure))
		}

		// depends_on references are validated in the second pass below,
		// since the referenced step may be defined later in the list.
	}

	// Second pass: validate references (depends_on, when, context step_output).
	for _, s := range wf.Steps {
		prefix := fmt.Sprintf("step %q", s.Name)

		for _, dep := range s.DependsOn {
			if !stepNames[dep] {
				errs = append(errs, prefix+": depends_on references unknown step "+dep)
			}
		}

		if s.When != "" {
			ref := parseWhenStepRef(s.When)
			if ref != "" && !stepNames[ref] {
				errs = append(errs, prefix+": when references unknown step "+ref)
			}
		}

		for _, ci := range s.Context {
			if ci.Type == ContextStepOutput && ci.Step != "" && !stepNames[ci.Step] {
				errs = append(errs, prefix+": context references unknown step "+ci.Step)
			}
		}
	}

	// DAG cycle detection.
	if cycleErr := validateDAG(wf.Steps); cycleErr != nil {
		errs = append(errs, cycleErr.Error())
	}

	if len(errs) > 0 {
		return fmt.Errorf("workflow validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// validateDAG checks for cycles in the step dependency graph using Kahn's algorithm.
// It considers both explicit depends_on and implicit dependencies from when expressions.
func validateDAG(steps []Step) error {
	nameToIdx := make(map[string]int, len(steps))
	for i, s := range steps {
		nameToIdx[s.Name] = i
	}

	// Build adjacency list and compute in-degrees.
	n := len(steps)
	adj := make([][]int, n)
	inDegree := make([]int, n)

	addEdge := func(from, to int) {
		adj[from] = append(adj[from], to)
		inDegree[to]++
	}

	for i, s := range steps {
		// Explicit dependencies.
		for _, dep := range s.DependsOn {
			if depIdx, ok := nameToIdx[dep]; ok {
				addEdge(depIdx, i)
			}
		}
		// Implicit dependency from when expression.
		if ref := parseWhenStepRef(s.When); ref != "" {
			if refIdx, ok := nameToIdx[ref]; ok {
				// Avoid duplicate edge if already in depends_on.
				alreadyAdded := false
				for _, dep := range s.DependsOn {
					if dep == ref {
						alreadyAdded = true
						break
					}
				}
				if !alreadyAdded {
					addEdge(refIdx, i)
				}
			}
		}
	}

	// Kahn's algorithm.
	var queue []int
	for i, d := range inDegree {
		if d == 0 {
			queue = append(queue, i)
		}
	}

	processed := 0
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		processed++
		for _, neighbor := range adj[node] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if processed < n {
		// Find the steps involved in the cycle.
		var cycleSteps []string
		for i, d := range inDegree {
			if d > 0 {
				cycleSteps = append(cycleSteps, steps[i].Name)
			}
		}
		return fmt.Errorf("dependency cycle detected among steps: %s", strings.Join(cycleSteps, ", "))
	}
	return nil
}

// TopologicalOrder returns step names in a valid execution order
// respecting depends_on and implicit when dependencies.
// Falls back to definition order for steps without dependencies.
func TopologicalOrder(steps []Step) []string {
	nameToIdx := make(map[string]int, len(steps))
	for i, s := range steps {
		nameToIdx[s.Name] = i
	}

	n := len(steps)
	adj := make([][]int, n)
	inDegree := make([]int, n)

	addEdge := func(from, to int) {
		adj[from] = append(adj[from], to)
		inDegree[to]++
	}

	for i, s := range steps {
		for _, dep := range s.DependsOn {
			if depIdx, ok := nameToIdx[dep]; ok {
				addEdge(depIdx, i)
			}
		}
		if ref := parseWhenStepRef(s.When); ref != "" {
			if refIdx, ok := nameToIdx[ref]; ok {
				alreadyAdded := false
				for _, dep := range s.DependsOn {
					if dep == ref {
						alreadyAdded = true
						break
					}
				}
				if !alreadyAdded {
					addEdge(refIdx, i)
				}
			}
		}
	}

	// Kahn's algorithm, preferring definition order for tie-breaking.
	var queue []int
	for i, d := range inDegree {
		if d == 0 {
			queue = append(queue, i)
		}
	}

	order := make([]string, 0, n)
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		order = append(order, steps[node].Name)
		for _, neighbor := range adj[node] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	return order
}

// parseWhenStepRef extracts the step name from a when expression like
// "steps.test.status == 'succeeded'". Returns "" if no reference is found.
func parseWhenStepRef(expr string) string {
	expr = strings.TrimSpace(expr)
	if !strings.HasPrefix(expr, "steps.") {
		return ""
	}
	// Extract: steps.<name>.status ...
	rest := strings.TrimPrefix(expr, "steps.")
	dotIdx := strings.Index(rest, ".")
	if dotIdx == -1 {
		return ""
	}
	return rest[:dotIdx]
}

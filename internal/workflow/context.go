package workflow

import (
	"fmt"
	"strings"

	"github.com/austinroos/sidekick/internal/task"
)

// AssembleContext builds the context string for an agent step by combining
// file contents, variables, and prior step output. Each item is rendered
// under its label as a markdown heading. The result is prepended to the prompt.
//
// readFile is a function that reads a file from the sandbox workspace.
// It is injected to decouple context assembly from the sandbox implementation.
func AssembleContext(
	items []ContextItem,
	variables map[string]string,
	stepResults map[string]task.StepResult,
	readFile func(path string) (string, error),
) (string, error) {
	var sections []string

	for _, item := range items {
		var content string
		var err error

		switch item.Type {
		case ContextFile:
			content, err = readFile(item.Path)
			if err != nil {
				return "", fmt.Errorf("reading context file %q: %w", item.Path, err)
			}

		case ContextVariable:
			val, ok := variables[item.Key]
			if !ok {
				return "", fmt.Errorf("missing variable %q for context item", item.Key)
			}
			content = val

		case ContextStepOutput:
			result, ok := stepResults[item.Step]
			if !ok {
				// Step was skipped or hasn't run — omit this context item.
				continue
			}
			switch item.Output {
			case "stdout":
				content = result.Stdout
			case "stderr":
				content = result.Stderr
			default:
				content = result.Stdout
			}
			if item.MaxLines > 0 {
				content = lastNLines(content, item.MaxLines)
			}

		default:
			return "", fmt.Errorf("unknown context type %q", item.Type)
		}

		section := fmt.Sprintf("## %s\n%s", item.Label, content)
		sections = append(sections, section)
	}

	if len(sections) == 0 {
		return "", nil
	}
	return strings.Join(sections, "\n\n") + "\n\n---\n\n", nil
}

// lastNLines returns the last n lines of s.
func lastNLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

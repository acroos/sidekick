package workflow

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Parse parses a workflow definition from YAML bytes.
func Parse(data []byte) (*Workflow, error) {
	var wf Workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("parsing workflow YAML: %w", err)
	}
	return &wf, nil
}

// ParseFile reads and parses a workflow definition from a YAML file.
func ParseFile(path string) (*Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading workflow file: %w", err)
	}
	return Parse(data)
}

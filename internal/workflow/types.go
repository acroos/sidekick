// Package workflow defines the workflow schema, parser, validator, and executor.
package workflow

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Workflow is the top-level workflow definition parsed from YAML.
type Workflow struct {
	Name       string        `yaml:"name"`
	Timeout    Duration      `yaml:"timeout"`
	MaxRetries int           `yaml:"max_retries"`
	Sandbox    SandboxConfig `yaml:"sandbox"`
	Steps      []Step        `yaml:"steps"`
}

// SandboxConfig describes the sandbox environment for the workflow.
type SandboxConfig struct {
	Image      string   `yaml:"image"`
	Network    string   `yaml:"network"`
	AllowHosts []string `yaml:"allow_hosts"`
}

// Step is a single executable step in a workflow.
type Step struct {
	Name         string        `yaml:"name"`
	Type         StepType      `yaml:"type"`
	Run          string        `yaml:"run"`
	Prompt       string        `yaml:"prompt"`
	Context      []ContextItem `yaml:"context"`
	AllowedTools []string      `yaml:"allowed_tools"`
	Timeout      Duration      `yaml:"timeout"`
	OnFailure    FailPolicy    `yaml:"on_failure"`
	When         string        `yaml:"when"`
	DependsOn    []string      `yaml:"depends_on"`
}

// StepType represents the execution mode of a step.
type StepType string

const (
	StepDeterministic StepType = "deterministic"
	StepAgent         StepType = "agent"
)

// FailPolicy defines how to handle step failures.
type FailPolicy string

const (
	FailAbort    FailPolicy = "abort"
	FailContinue FailPolicy = "continue"
	FailRetry    FailPolicy = "retry"
)

// ContextItem represents a single piece of context assembled into the agent prompt.
type ContextItem struct {
	Type     ContextType `yaml:"type"`
	Path     string      `yaml:"path"`
	Key      string      `yaml:"key"`
	Step     string      `yaml:"step"`
	Output   string      `yaml:"output"`
	Label    string      `yaml:"label"`
	MaxLines int         `yaml:"max_lines"`
}

// ContextType identifies the source of context.
type ContextType string

const (
	ContextFile       ContextType = "file"
	ContextVariable   ContextType = "variable"
	ContextStepOutput ContextType = "step_output"
)

// Duration wraps time.Duration with YAML unmarshaling support.
// YAML values like "30m" or "5m30s" are parsed via time.ParseDuration.
type Duration struct {
	time.Duration
}

// UnmarshalYAML parses a duration string (e.g., "30m", "2h30m") into a Duration.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = dur
	return nil
}

// MarshalYAML outputs the duration as a string.
func (d Duration) MarshalYAML() (any, error) {
	return d.Duration.String(), nil
}

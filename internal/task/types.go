// Package task defines the task model and execution status types.
package task

import "time"

// Task represents a submitted workflow execution.
type Task struct {
	ID          string
	WorkflowRef string
	Variables   map[string]string
	Status      Status
	Steps       []StepResult
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	Error       string // Set if task failed
	WebhookURL  string // Optional webhook for completion notification
}

// Status represents the lifecycle state of a task.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCanceled  Status = "canceled"
	StatusSkipped   Status = "skipped"
)

// StepResult captures the outcome of a single step execution.
type StepResult struct {
	Name       string
	Status     Status
	ExitCode   int
	Stdout     string
	Stderr     string
	StartedAt  time.Time
	Duration   time.Duration
	TokensUsed int // For agent steps only
}

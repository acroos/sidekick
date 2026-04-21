package task

import "context"

// Executor runs a workflow for a given task.
// This interface decouples the task manager from the workflow package,
// avoiding an import cycle.
type Executor interface {
	RunWorkflow(ctx context.Context, taskID string, workflowPath string, variables map[string]string) (*Task, error)
}

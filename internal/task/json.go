package task

import "time"

// TaskResponse is the API-facing JSON representation of a Task.
type TaskResponse struct {
	ID              string         `json:"id"`
	Status          string         `json:"status"`
	WorkflowRef     string         `json:"workflow"`
	Steps           []StepResponse `json:"steps,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	StartedAt       *time.Time     `json:"started_at,omitempty"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
	Error           string         `json:"error,omitempty"`
	TotalTokensUsed int            `json:"total_tokens_used,omitempty"`
}

// StepResponse is the API-facing JSON representation of a StepResult.
type StepResponse struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	TokensUsed int    `json:"tokens_used,omitempty"`
}

// TaskToResponse converts a Task to its API JSON representation.
func TaskToResponse(t *Task) *TaskResponse {
	resp := &TaskResponse{
		ID:          t.ID,
		Status:      string(t.Status),
		WorkflowRef: t.WorkflowRef,
		CreatedAt:   t.CreatedAt,
		StartedAt:   t.StartedAt,
		CompletedAt: t.CompletedAt,
		Error:       t.Error,
	}

	totalTokens := 0
	for _, s := range t.Steps {
		resp.Steps = append(resp.Steps, StepResponse{
			Name:       s.Name,
			Status:     string(s.Status),
			DurationMs: s.Duration.Milliseconds(),
			TokensUsed: s.TokensUsed,
		})
		totalTokens += s.TokensUsed
	}
	resp.TotalTokensUsed = totalTokens

	return resp
}

// Package event defines the event system for real-time task execution streaming.
package event

import (
	"encoding/json"
	"time"
)

// Event is the common envelope for all task execution events.
type Event struct {
	ID        int64           `json:"id"`
	TaskID    string          `json:"task_id"`
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// --- Task-level events ---

// TaskStarted is emitted when a task begins execution.
type TaskStarted struct{}

// TaskCompleted is emitted when a task finishes.
type TaskCompleted struct {
	Status          string `json:"status"`
	TotalTokensUsed int    `json:"total_tokens_used"`
}

// --- Step-level events ---

// StepStarted is emitted when a step begins execution.
type StepStarted struct {
	Step string `json:"step"`
}

// StepOutput is emitted for each line of stdout/stderr from a step.
type StepOutput struct {
	Step   string `json:"step"`
	Stream string `json:"stream"` // "stdout" or "stderr"
	Line   string `json:"line"`
}

// StepCompleted is emitted when a step finishes.
type StepCompleted struct {
	Step       string `json:"step"`
	Status     string `json:"status"`
	DurationMs int64  `json:"duration_ms"`
	TokensUsed int    `json:"tokens_used,omitempty"`
}

// StepSkipped is emitted when a step is skipped due to a when condition.
type StepSkipped struct {
	Step   string `json:"step"`
	Reason string `json:"reason"`
}

// --- Agent events (emitted in Phase 3) ---

// AgentThinking is emitted when the agent produces reasoning text.
type AgentThinking struct {
	Step string `json:"step"`
	Text string `json:"text"`
}

// AgentAction is emitted when the agent invokes a tool.
type AgentAction struct {
	Step   string `json:"step"`
	Tool   string `json:"tool"`
	Detail string `json:"detail"`
}

// AgentActionResult is emitted with the output of an agent tool invocation.
type AgentActionResult struct {
	Step   string `json:"step"`
	Tool   string `json:"tool"`
	Output string `json:"output"`
}

// AgentOutput is emitted for the agent's final text output.
type AgentOutput struct {
	Step string `json:"step"`
	Text string `json:"text"`
}

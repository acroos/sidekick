package agent

import "encoding/json"

// streamEvent represents a single line of Claude Code's stream-json output.
// Claude Code emits newline-delimited JSON with varying top-level types.
type streamEvent struct {
	Type    string         `json:"type"`              // "system", "assistant", "tool_use", "tool_result", "result"
	Subtype string         `json:"subtype,omitempty"` // e.g., "init", "success"
	Message *streamMessage `json:"message,omitempty"` // Present for "assistant" type
	// Top-level tool fields (alternative to message.content blocks).
	Tool   string          `json:"tool,omitempty"`
	Input  json.RawMessage `json:"input,omitempty"`
	Output string          `json:"output,omitempty"`
	// Result fields.
	Result  string `json:"result,omitempty"`
	IsError bool   `json:"is_error,omitempty"`
}

// streamMessage is the message payload within an "assistant" event.
type streamMessage struct {
	Content []contentBlock `json:"content"`
	Usage   *usage         `json:"usage,omitempty"`
}

// contentBlock represents one block within a message's content array.
type contentBlock struct {
	Type     string          `json:"type"` // "thinking", "text", "tool_use", "tool_result"
	Text     string          `json:"text,omitempty"`
	Thinking string          `json:"thinking,omitempty"`
	Name     string          `json:"name,omitempty"` // tool name for tool_use
	ID       string          `json:"id,omitempty"`
	Input    json.RawMessage `json:"input,omitempty"`
	Content  string          `json:"content,omitempty"` // tool_result content
}

// usage holds token counts from Claude API responses.
type usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// parsedEvent is the result of parsing a single stream-json line into a Sidekick event.
type parsedEvent struct {
	eventType string // e.g., "agent.thinking", "agent.action"
	data      any    // The typed event struct (AgentThinking, AgentAction, etc.)
}

// parseStreamLine parses a single line of Claude Code stream-json output.
// Returns nil if the line is not parseable or not relevant (e.g., system events).
func parseStreamLine(line, stepName string) []parsedEvent {
	if line == "" {
		return nil
	}

	var evt streamEvent
	if err := json.Unmarshal([]byte(line), &evt); err != nil {
		return nil
	}

	switch evt.Type {
	case "assistant":
		return parseAssistantEvent(evt, stepName)
	case "tool_use":
		return parseTopLevelToolUse(evt, stepName)
	case "tool_result":
		return parseTopLevelToolResult(evt, stepName)
	case "result":
		return parseResultEvent(evt, stepName)
	default:
		// system, init, etc. — skip.
		return nil
	}
}

// parseAssistantEvent extracts events from an assistant message's content blocks.
func parseAssistantEvent(evt streamEvent, stepName string) []parsedEvent {
	if evt.Message == nil {
		return nil
	}

	var events []parsedEvent
	for _, block := range evt.Message.Content {
		switch block.Type {
		case "thinking":
			if block.Thinking != "" {
				events = append(events, parsedEvent{
					eventType: "agent.thinking",
					data:      agentThinking{Step: stepName, Text: block.Thinking},
				})
			}
		case "text":
			if block.Text != "" {
				events = append(events, parsedEvent{
					eventType: "agent.output",
					data:      agentOutput{Step: stepName, Text: block.Text},
				})
			}
		case "tool_use":
			detail := formatToolInput(block.Input)
			events = append(events, parsedEvent{
				eventType: "agent.action",
				data:      agentAction{Step: stepName, Tool: block.Name, Detail: detail},
			})
		case "tool_result":
			events = append(events, parsedEvent{
				eventType: "agent.action_result",
				data:      agentActionResult{Step: stepName, Tool: "", Output: block.Content},
			})
		}
	}
	return events
}

// parseTopLevelToolUse handles a top-level tool_use event (alternative format).
func parseTopLevelToolUse(evt streamEvent, stepName string) []parsedEvent {
	detail := formatToolInput(evt.Input)
	return []parsedEvent{{
		eventType: "agent.action",
		data:      agentAction{Step: stepName, Tool: evt.Tool, Detail: detail},
	}}
}

// parseTopLevelToolResult handles a top-level tool_result event.
func parseTopLevelToolResult(evt streamEvent, stepName string) []parsedEvent {
	return []parsedEvent{{
		eventType: "agent.action_result",
		data:      agentActionResult{Step: stepName, Tool: evt.Tool, Output: evt.Output},
	}}
}

// parseResultEvent handles the final result event.
func parseResultEvent(evt streamEvent, stepName string) []parsedEvent {
	if evt.Result == "" {
		return nil
	}
	return []parsedEvent{{
		eventType: "agent.output",
		data:      agentOutput{Step: stepName, Text: evt.Result},
	}}
}

// formatToolInput converts a tool's JSON input into a human-readable detail string.
func formatToolInput(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	// Try to extract common fields for a concise summary.
	var m map[string]any
	if err := json.Unmarshal(input, &m); err != nil {
		return string(input)
	}
	// Prefer file_path, command, or pattern for a short detail.
	for _, key := range []string{"file_path", "path", "command", "pattern"} {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return string(input)
}

// Event structs matching the event package types.
// These are local copies to avoid a circular import with the event package.
// The workflow executor maps these to the canonical event types when emitting.

type agentThinking struct {
	Step string `json:"step"`
	Text string `json:"text"`
}

type agentAction struct {
	Step   string `json:"step"`
	Tool   string `json:"tool"`
	Detail string `json:"detail"`
}

type agentActionResult struct {
	Step   string `json:"step"`
	Tool   string `json:"tool"`
	Output string `json:"output"`
}

type agentOutput struct {
	Step string `json:"step"`
	Text string `json:"text"`
}

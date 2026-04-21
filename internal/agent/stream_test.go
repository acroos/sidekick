package agent

import (
	"testing"
)

func TestParseThinkingBlock(t *testing.T) {
	line := `{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"Let me examine the error..."}]}}`
	events := parseStreamLine(line, "solve")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].eventType != "agent.thinking" {
		t.Fatalf("expected agent.thinking, got %s", events[0].eventType)
	}
	at, ok := events[0].data.(agentThinking)
	if !ok {
		t.Fatal("expected agentThinking data")
	}
	if at.Step != "solve" {
		t.Fatalf("expected step solve, got %s", at.Step)
	}
	if at.Text != "Let me examine the error..." {
		t.Fatalf("unexpected text: %s", at.Text)
	}
}

func TestParseTextBlock(t *testing.T) {
	line := `{"type":"assistant","message":{"content":[{"type":"text","text":"I fixed the bug."}]}}`
	events := parseStreamLine(line, "solve")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].eventType != "agent.output" {
		t.Fatalf("expected agent.output, got %s", events[0].eventType)
	}
	ao := events[0].data.(agentOutput)
	if ao.Text != "I fixed the bug." {
		t.Fatalf("unexpected text: %s", ao.Text)
	}
}

func TestParseToolUseBlock(t *testing.T) {
	line := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","id":"call_1","input":{"file_path":"src/main.ts"}}]}}`
	events := parseStreamLine(line, "solve")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].eventType != "agent.action" {
		t.Fatalf("expected agent.action, got %s", events[0].eventType)
	}
	aa := events[0].data.(agentAction)
	if aa.Tool != "Read" {
		t.Fatalf("expected tool Read, got %s", aa.Tool)
	}
	if aa.Detail != "src/main.ts" {
		t.Fatalf("expected detail src/main.ts, got %s", aa.Detail)
	}
}

func TestParseMultipleContentBlocks(t *testing.T) {
	line := `{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"Analyzing..."},{"type":"tool_use","name":"Edit","id":"call_2","input":{"path":"src/app.ts"}}]}}`
	events := parseStreamLine(line, "solve")
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].eventType != "agent.thinking" {
		t.Fatalf("expected agent.thinking first, got %s", events[0].eventType)
	}
	if events[1].eventType != "agent.action" {
		t.Fatalf("expected agent.action second, got %s", events[1].eventType)
	}
}

func TestParseTopLevelToolUse(t *testing.T) {
	line := `{"type":"tool_use","tool":"Bash","input":{"command":"npm test"}}`
	events := parseStreamLine(line, "solve")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	aa := events[0].data.(agentAction)
	if aa.Tool != "Bash" {
		t.Fatalf("expected tool Bash, got %s", aa.Tool)
	}
	if aa.Detail != "npm test" {
		t.Fatalf("expected detail 'npm test', got %s", aa.Detail)
	}
}

func TestParseTopLevelToolResult(t *testing.T) {
	line := `{"type":"tool_result","tool":"Bash","output":"Tests: 10 passed"}`
	events := parseStreamLine(line, "solve")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].eventType != "agent.action_result" {
		t.Fatalf("expected agent.action_result, got %s", events[0].eventType)
	}
	ar := events[0].data.(agentActionResult)
	if ar.Tool != "Bash" {
		t.Fatalf("expected tool Bash, got %s", ar.Tool)
	}
	if ar.Output != "Tests: 10 passed" {
		t.Fatalf("unexpected output: %s", ar.Output)
	}
}

func TestParseResultEvent(t *testing.T) {
	line := `{"type":"result","subtype":"success","result":"I fixed the null pointer issue.","is_error":false}`
	events := parseStreamLine(line, "solve")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].eventType != "agent.output" {
		t.Fatalf("expected agent.output, got %s", events[0].eventType)
	}
	ao := events[0].data.(agentOutput)
	if ao.Text != "I fixed the null pointer issue." {
		t.Fatalf("unexpected text: %s", ao.Text)
	}
}

func TestParseEmptyResultEvent(t *testing.T) {
	line := `{"type":"result","subtype":"success","result":"","is_error":false}`
	events := parseStreamLine(line, "solve")
	if events != nil {
		t.Fatalf("expected nil for empty result, got %d events", len(events))
	}
}

func TestParseMalformedLine(t *testing.T) {
	events := parseStreamLine("this is not json", "solve")
	if events != nil {
		t.Fatalf("expected nil for malformed line, got %d events", len(events))
	}
}

func TestParseEmptyLine(t *testing.T) {
	events := parseStreamLine("", "solve")
	if events != nil {
		t.Fatal("expected nil for empty line")
	}
}

func TestParseSystemEvent(t *testing.T) {
	line := `{"type":"system","subtype":"init","session_id":"abc"}`
	events := parseStreamLine(line, "solve")
	if events != nil {
		t.Fatal("expected nil for system event")
	}
}

func TestFormatToolInputFilePath(t *testing.T) {
	input := []byte(`{"file_path":"src/main.ts","offset":10}`)
	result := formatToolInput(input)
	if result != "src/main.ts" {
		t.Fatalf("expected src/main.ts, got %s", result)
	}
}

func TestFormatToolInputCommand(t *testing.T) {
	input := []byte(`{"command":"npm test"}`)
	result := formatToolInput(input)
	if result != "npm test" {
		t.Fatalf("expected 'npm test', got %s", result)
	}
}

func TestFormatToolInputFallback(t *testing.T) {
	input := []byte(`{"regex":"foo.*bar","replacement":"baz"}`)
	result := formatToolInput(input)
	// Should fall back to full JSON string.
	if result != `{"regex":"foo.*bar","replacement":"baz"}` {
		t.Fatalf("unexpected fallback: %s", result)
	}
}

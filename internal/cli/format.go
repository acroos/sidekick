package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ANSI color codes for terminal output.
const (
	colorReset  = "\033[0m"
	colorDim    = "\033[2m"
	colorBold   = "\033[1m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
)

// FormatEvent renders an SSE event as a terminal-friendly string.
func FormatEvent(eventType string, data json.RawMessage) string {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return ""
	}

	switch eventType {
	case "task.started":
		return colorBlue + colorBold + "Task started" + colorReset

	case "task.completed":
		status, _ := m["status"].(string)
		color := colorGreen
		if status != "succeeded" {
			color = colorRed
		}
		tokens := ""
		if t, ok := m["total_tokens_used"].(float64); ok && t > 0 {
			tokens = fmt.Sprintf(" (%d tokens)", int(t))
		}
		return color + colorBold + "Task " + status + colorReset + tokens

	case "step.started":
		step, _ := m["step"].(string)
		return colorBlue + ">> Step: " + step + colorReset

	case "step.completed":
		step, _ := m["step"].(string)
		status, _ := m["status"].(string)
		dur, _ := m["duration_ms"].(float64)
		color := colorGreen
		if status != "succeeded" {
			color = colorRed
		}
		tokens := ""
		if t, ok := m["tokens_used"].(float64); ok && t > 0 {
			tokens = fmt.Sprintf(", %d tokens", int(t))
		}
		return fmt.Sprintf("%s   %s: %s%s (%.1fs%s)", color, step, status, colorReset, dur/1000, tokens)

	case "step.skipped":
		step, _ := m["step"].(string)
		reason, _ := m["reason"].(string)
		return fmt.Sprintf("%s   %s: skipped (%s)%s", colorDim, step, reason, colorReset)

	case "step.output":
		stream, _ := m["stream"].(string)
		line, _ := m["line"].(string)
		prefix := "   "
		if stream == "stderr" {
			prefix = colorDim + "   "
			return prefix + line + colorReset
		}
		return prefix + line

	case "agent.thinking":
		text, _ := m["text"].(string)
		return colorDim + "   [thinking] " + truncate(text, 120) + colorReset

	case "agent.action":
		tool, _ := m["tool"].(string)
		detail, _ := m["detail"].(string)
		return colorYellow + "   [" + tool + "] " + colorReset + truncate(detail, 100)

	case "agent.action_result":
		output, _ := m["output"].(string)
		lines := strings.Split(output, "\n")
		if len(lines) > 5 {
			lines = append(lines[:5], fmt.Sprintf("... (%d more lines)", len(lines)-5))
		}
		return colorDim + "   " + strings.Join(lines, "\n   ") + colorReset

	case "agent.output":
		text, _ := m["text"].(string)
		return colorCyan + "   " + text + colorReset

	default:
		return colorDim + "   [" + eventType + "]" + colorReset
	}
}

// FormatTask renders a task as a terminal-friendly string.
func FormatTask(t *TaskResponse) string {
	var b strings.Builder

	statusColor := colorYellow
	switch t.Status {
	case "succeeded":
		statusColor = colorGreen
	case "failed", "canceled":
		statusColor = colorRed
	case "pending":
		statusColor = colorDim
	}

	fmt.Fprintf(&b, "%sTask:%s  %s\n", colorBold, colorReset, t.ID)
	fmt.Fprintf(&b, "Status:   %s%s%s\n", statusColor, t.Status, colorReset)
	fmt.Fprintf(&b, "Workflow: %s\n", t.WorkflowRef)
	fmt.Fprintf(&b, "Created:  %s\n", t.CreatedAt.Format(time.RFC3339))

	if t.StartedAt != nil {
		fmt.Fprintf(&b, "Started:  %s\n", t.StartedAt.Format(time.RFC3339))
	}
	if t.CompletedAt != nil {
		fmt.Fprintf(&b, "Finished: %s\n", t.CompletedAt.Format(time.RFC3339))
	}
	if t.Error != "" {
		fmt.Fprintf(&b, "%sError:    %s%s\n", colorRed, t.Error, colorReset)
	}
	if t.TotalTokensUsed > 0 {
		fmt.Fprintf(&b, "Tokens:   %d\n", t.TotalTokensUsed)
	}

	if len(t.Steps) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "Steps:")
		for _, s := range t.Steps {
			stepColor := colorDim
			switch s.Status {
			case "succeeded":
				stepColor = colorGreen
			case "failed":
				stepColor = colorRed
			case "running":
				stepColor = colorYellow
			}
			dur := ""
			if s.DurationMs > 0 {
				dur = fmt.Sprintf(" (%.1fs)", float64(s.DurationMs)/1000)
			}
			tokens := ""
			if s.TokensUsed > 0 {
				tokens = fmt.Sprintf(", %d tokens", s.TokensUsed)
			}
			fmt.Fprintf(&b, "  %s%-12s %s%s%s%s\n", stepColor, s.Name, s.Status, colorReset, dur, tokens)
		}
	}

	return b.String()
}

// FormatTaskList renders a slice of tasks as a table.
func FormatTaskList(tasks []*TaskResponse) string {
	if len(tasks) == 0 {
		return "No tasks found."
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%-20s  %-12s  %-16s  %s\n", "ID", "STATUS", "WORKFLOW", "CREATED")
	fmt.Fprintln(&b, strings.Repeat("-", 72))

	for _, t := range tasks {
		statusColor := colorDim
		switch t.Status {
		case "succeeded":
			statusColor = colorGreen
		case "failed", "canceled":
			statusColor = colorRed
		case "running":
			statusColor = colorYellow
		}
		fmt.Fprintf(&b, "%-20s  %s%-12s%s  %-16s  %s\n",
			t.ID,
			statusColor, t.Status, colorReset,
			t.WorkflowRef,
			t.CreatedAt.Format(time.RFC3339),
		)
	}

	return b.String()
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

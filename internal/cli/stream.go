package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// StreamEvents connects to the SSE endpoint and prints formatted events to the writer.
// It blocks until the stream ends (task.completed) or an error occurs.
func StreamEvents(c *Client, taskID string, types string, w io.Writer) error {
	resp, err := c.StreamResponse(taskID, types, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)

	var eventType string
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
			continue
		}

		// Empty line = end of SSE event.
		if line == "" && eventType != "" {
			data := strings.Join(dataLines, "\n")
			formatted := FormatEvent(eventType, json.RawMessage(data))
			if formatted != "" {
				fmt.Fprintln(w, formatted)
			}

			if eventType == "task.completed" {
				return nil
			}

			eventType = ""
			dataLines = dataLines[:0]
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading stream: %w", err)
	}
	return nil
}

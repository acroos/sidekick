package task

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// WebhookPayload is the JSON body sent to webhook URLs on task completion.
type WebhookPayload struct {
	Event string        `json:"event"`
	Task  *TaskResponse `json:"task"`
}

// SendWebhook POSTs a task completion notification to the given URL.
// It is safe to call from a goroutine. Errors are logged but not returned.
func SendWebhook(webhookURL string, t *Task) {
	payload := WebhookPayload{
		Event: "task.completed",
		Task:  TaskToResponse(t),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("webhook marshal error", "error", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		slog.Error("webhook request error", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("webhook POST failed", "url", webhookURL, "error", err)
		return
	}
	_ = resp.Body.Close()

	if resp.StatusCode >= 400 {
		slog.Warn("webhook returned error status", "url", webhookURL, "status", resp.StatusCode)
	}
}

package task

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
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
		log.Printf("webhook: marshal error: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		log.Printf("webhook: request error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("webhook: POST to %s failed: %v", webhookURL, err)
		return
	}
	_ = resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("webhook: POST to %s returned status %d", webhookURL, resp.StatusCode)
	}
}

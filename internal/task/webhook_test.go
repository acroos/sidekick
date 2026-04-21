package task

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSendWebhook(t *testing.T) {
	var received WebhookPayload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	now := time.Now()
	completedAt := now.Add(30 * time.Second)
	task := &Task{
		ID:          "task_wh1",
		WorkflowRef: "fix-issue",
		Variables:   map[string]string{"REPO_URL": "https://github.com/test/repo"},
		Status:      StatusSucceeded,
		Steps: []StepResult{
			{Name: "clone", Status: StatusSucceeded, Duration: 3 * time.Second},
			{Name: "solve", Status: StatusSucceeded, Duration: 20 * time.Second, TokensUsed: 5000},
		},
		CreatedAt:   now,
		CompletedAt: &completedAt,
	}

	SendWebhook(server.URL, task)

	if received.Event != "task.completed" {
		t.Errorf("Event = %q, want %q", received.Event, "task.completed")
	}
	if received.Task == nil {
		t.Fatal("Task is nil")
	}
	if received.Task.ID != "task_wh1" {
		t.Errorf("Task.ID = %q, want %q", received.Task.ID, "task_wh1")
	}
	if received.Task.TotalTokensUsed != 5000 {
		t.Errorf("TotalTokensUsed = %d, want 5000", received.Task.TotalTokensUsed)
	}
}

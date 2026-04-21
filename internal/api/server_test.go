package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/austinroos/sidekick/internal/event"
	"github.com/austinroos/sidekick/internal/task"
)

const testAPIKey = "sk-test-key"

// testServer creates an API server backed by real in-memory stores.
func testServer(t *testing.T) (*httptest.Server, *task.Manager, *event.Bus, event.Store) {
	t.Helper()

	taskStore, err := task.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("creating task store: %v", err)
	}
	t.Cleanup(func() { _ = taskStore.Close() })

	eventStore, err := event.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("creating event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	eventBus := event.NewBus()

	wfDir := setupTestWorkflowDir(t)
	mgr := task.NewManager(taskStore, nil, task.ManagerConfig{
		MaxConcurrent: 4,
		WorkflowDir:   wfDir,
	})

	srv := NewServer(ServerConfig{
		APIKey:     testAPIKey,
		Manager:    mgr,
		EventBus:   eventBus,
		EventStore: eventStore,
	})

	ts := httptest.NewServer(srv.routes())
	t.Cleanup(func() {
		ts.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = mgr.Shutdown(ctx)
	})

	return ts, mgr, eventBus, eventStore
}

func setupTestWorkflowDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	content := []byte(`name: test-workflow
timeout: 1m
sandbox:
  image: alpine:latest
steps:
  - name: hello
    type: deterministic
    run: echo hello
`)
	if err := os.WriteFile(filepath.Join(dir, "test-workflow.yaml"), content, 0644); err != nil {
		t.Fatalf("writing test workflow: %v", err)
	}
	return dir
}

func doRequest(t *testing.T, ts *httptest.Server, method, path, body string) *http.Response {
	t.Helper()
	var req *http.Request
	var err error
	if body != "" {
		req, err = http.NewRequest(method, ts.URL+path, strings.NewReader(body))
	} else {
		req, err = http.NewRequest(method, ts.URL+path, nil)
	}
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}
	req.Header.Set("X-Sidekick-Key", testAPIKey)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func TestAuth_MissingKey(t *testing.T) {
	ts, _, _, _ := testServer(t)

	req, _ := http.NewRequest("GET", ts.URL+"/tasks", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestAuth_WrongKey(t *testing.T) {
	ts, _, _, _ := testServer(t)

	req, _ := http.NewRequest("GET", ts.URL+"/tasks", nil)
	req.Header.Set("X-Sidekick-Key", "wrong-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestAuth_BearerToken(t *testing.T) {
	ts, _, _, _ := testServer(t)

	req, _ := http.NewRequest("GET", ts.URL+"/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestCreateTask(t *testing.T) {
	ts, _, _, _ := testServer(t)

	resp := doRequest(t, ts, "POST", "/tasks",
		`{"workflow":"test-workflow","variables":{"FOO":"bar"}}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var result task.TaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.ID == "" {
		t.Error("expected task ID")
	}
	if result.Status != "pending" {
		t.Errorf("status = %q, want %q", result.Status, "pending")
	}
	if result.WorkflowRef != "test-workflow" {
		t.Errorf("workflow = %q, want %q", result.WorkflowRef, "test-workflow")
	}
}

func TestCreateTask_MissingWorkflow(t *testing.T) {
	ts, _, _, _ := testServer(t)

	resp := doRequest(t, ts, "POST", "/tasks", `{"variables":{}}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestCreateTask_InvalidWorkflow(t *testing.T) {
	ts, _, _, _ := testServer(t)

	resp := doRequest(t, ts, "POST", "/tasks", `{"workflow":"nonexistent"}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnprocessableEntity)
	}
}

func TestGetTask(t *testing.T) {
	ts, _, _, _ := testServer(t)

	// Create a task first.
	createResp := doRequest(t, ts, "POST", "/tasks",
		`{"workflow":"test-workflow","variables":{}}`)
	defer createResp.Body.Close()

	var created task.TaskResponse
	json.NewDecoder(createResp.Body).Decode(&created)

	// Get it.
	resp := doRequest(t, ts, "GET", "/tasks/"+created.ID, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got task.TaskResponse
	json.NewDecoder(resp.Body).Decode(&got)
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
}

func TestGetTask_NotFound(t *testing.T) {
	ts, _, _, _ := testServer(t)

	resp := doRequest(t, ts, "GET", "/tasks/nonexistent", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestListTasks(t *testing.T) {
	ts, _, _, _ := testServer(t)

	// Create two tasks.
	doRequest(t, ts, "POST", "/tasks", `{"workflow":"test-workflow","variables":{}}`).Body.Close()
	doRequest(t, ts, "POST", "/tasks", `{"workflow":"test-workflow","variables":{}}`).Body.Close()

	resp := doRequest(t, ts, "GET", "/tasks", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var tasks []task.TaskResponse
	json.NewDecoder(resp.Body).Decode(&tasks)
	if len(tasks) < 2 {
		t.Errorf("got %d tasks, want >= 2", len(tasks))
	}
}

func TestListTasks_Empty(t *testing.T) {
	ts, _, _, _ := testServer(t)

	resp := doRequest(t, ts, "GET", "/tasks", "")
	defer resp.Body.Close()

	var tasks []task.TaskResponse
	json.NewDecoder(resp.Body).Decode(&tasks)

	// Should be empty array, not null.
	if tasks == nil {
		t.Error("expected empty array, got null")
	}
}

func TestCancelTask(t *testing.T) {
	ts, _, _, _ := testServer(t)

	// Create a task.
	createResp := doRequest(t, ts, "POST", "/tasks",
		`{"workflow":"test-workflow","variables":{}}`)
	var created task.TaskResponse
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	// Cancel it.
	resp := doRequest(t, ts, "POST", "/tasks/"+created.ID+"/cancel", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestCancelTask_NotFound(t *testing.T) {
	ts, _, _, _ := testServer(t)

	resp := doRequest(t, ts, "POST", "/tasks/nonexistent/cancel", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestSSE_ReplayEvents(t *testing.T) {
	ts, _, _, eventStore := testServer(t)

	// Insert some events directly into the store.
	ctx := context.Background()
	taskID := "task_sse_test"

	// Create the task in the task store via the API isn't practical here,
	// so create it directly in the store used by the manager.
	// Instead, let's test with a task that exists.

	// First create a task via the API.
	createResp := doRequest(t, ts, "POST", "/tasks",
		`{"workflow":"test-workflow","variables":{}}`)
	var created task.TaskResponse
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()
	taskID = created.ID

	// Wait a moment for the goroutine to process.
	time.Sleep(50 * time.Millisecond)

	// Insert test events into the event store.
	events := []struct {
		eventType string
		data      string
	}{
		{"step.started", `{"step":"hello"}`},
		{"step.output", `{"step":"hello","stream":"stdout","line":"hello"}`},
		{"step.completed", `{"step":"hello","status":"succeeded","duration_ms":100}`},
		{"task.completed", `{"status":"succeeded","total_tokens_used":0}`},
	}

	for _, e := range events {
		evt := &event.Event{
			Type:      e.eventType,
			Timestamp: time.Now(),
			Data:      json.RawMessage(e.data),
		}
		if _, err := eventStore.Append(ctx, taskID, evt); err != nil {
			t.Fatalf("appending event: %v", err)
		}
	}

	// Connect to SSE endpoint.
	req, _ := http.NewRequest("GET", ts.URL+"/tasks/"+taskID+"/stream", nil)
	req.Header.Set("X-Sidekick-Key", testAPIKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	// Read SSE events.
	scanner := bufio.NewScanner(resp.Body)
	var receivedEvents []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			receivedEvents = append(receivedEvents, strings.TrimPrefix(line, "event: "))
		}
	}

	if len(receivedEvents) != 4 {
		t.Fatalf("got %d events, want 4: %v", len(receivedEvents), receivedEvents)
	}
	if receivedEvents[0] != "step.started" {
		t.Errorf("first event = %q, want step.started", receivedEvents[0])
	}
	if receivedEvents[3] != "task.completed" {
		t.Errorf("last event = %q, want task.completed", receivedEvents[3])
	}
}

func TestSSE_TypeFilter(t *testing.T) {
	ts, _, _, eventStore := testServer(t)
	ctx := context.Background()

	// Create task via API.
	createResp := doRequest(t, ts, "POST", "/tasks",
		`{"workflow":"test-workflow","variables":{}}`)
	var created task.TaskResponse
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	time.Sleep(50 * time.Millisecond)

	// Insert events.
	for _, e := range []struct {
		t    string
		data string
	}{
		{"step.started", `{"step":"hello"}`},
		{"step.output", `{"step":"hello","stream":"stdout","line":"hello"}`},
		{"step.completed", `{"step":"hello","status":"succeeded"}`},
		{"task.completed", `{"status":"succeeded"}`},
	} {
		evt := &event.Event{Type: e.t, Timestamp: time.Now(), Data: json.RawMessage(e.data)}
		eventStore.Append(ctx, created.ID, evt)
	}

	// Connect with type filter — only step.started and task.completed.
	req, _ := http.NewRequest("GET", ts.URL+"/tasks/"+created.ID+"/stream?types=step.started,task.completed", nil)
	req.Header.Set("X-Sidekick-Key", testAPIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var receivedEvents []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			receivedEvents = append(receivedEvents, strings.TrimPrefix(line, "event: "))
		}
	}

	if len(receivedEvents) != 2 {
		t.Fatalf("got %d events, want 2: %v", len(receivedEvents), receivedEvents)
	}
	if receivedEvents[0] != "step.started" {
		t.Errorf("first = %q, want step.started", receivedEvents[0])
	}
	if receivedEvents[1] != "task.completed" {
		t.Errorf("second = %q, want task.completed", receivedEvents[1])
	}
}

func TestSSE_LastEventID(t *testing.T) {
	ts, _, _, eventStore := testServer(t)
	ctx := context.Background()

	// Create task.
	createResp := doRequest(t, ts, "POST", "/tasks",
		`{"workflow":"test-workflow","variables":{}}`)
	var created task.TaskResponse
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	time.Sleep(50 * time.Millisecond)

	// Insert 4 events.
	for _, e := range []struct {
		t    string
		data string
	}{
		{"step.started", `{"step":"a"}`},
		{"step.completed", `{"step":"a","status":"succeeded"}`},
		{"step.started", `{"step":"b"}`},
		{"task.completed", `{"status":"succeeded"}`},
	} {
		evt := &event.Event{Type: e.t, Timestamp: time.Now(), Data: json.RawMessage(e.data)}
		eventStore.Append(ctx, created.ID, evt)
	}

	// Connect with Last-Event-ID: 2 — should skip first 2 events.
	req, _ := http.NewRequest("GET", ts.URL+"/tasks/"+created.ID+"/stream", nil)
	req.Header.Set("X-Sidekick-Key", testAPIKey)
	req.Header.Set("Last-Event-ID", "2")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var receivedEvents []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			receivedEvents = append(receivedEvents, strings.TrimPrefix(line, "event: "))
		}
	}

	if len(receivedEvents) != 2 {
		t.Fatalf("got %d events, want 2: %v", len(receivedEvents), receivedEvents)
	}
	if receivedEvents[0] != "step.started" {
		t.Errorf("first = %q, want step.started", receivedEvents[0])
	}
}

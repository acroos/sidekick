package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testAPIServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("POST /tasks", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Sidekick-Key") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(TaskResponse{
			ID:          "task_123",
			Status:      "pending",
			WorkflowRef: "fix-issue",
			CreatedAt:   time.Now(),
		})
	})

	mux.HandleFunc("GET /tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "nonexistent" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "task not found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TaskResponse{
			ID:          id,
			Status:      "running",
			WorkflowRef: "fix-issue",
			Steps: []StepResponse{
				{Name: "clone", Status: "succeeded", DurationMs: 3200},
			},
			CreatedAt: time.Now(),
		})
	})

	mux.HandleFunc("GET /tasks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]*TaskResponse{
			{ID: "task_1", Status: "succeeded", WorkflowRef: "fix-issue", CreatedAt: time.Now()},
			{ID: "task_2", Status: "running", WorkflowRef: "code-review", CreatedAt: time.Now()},
		})
	})

	mux.HandleFunc("POST /tasks/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TaskResponse{
			ID:     r.PathValue("id"),
			Status: "canceled",
		})
	})

	mux.HandleFunc("GET /tasks/{id}/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		flusher := w.(http.Flusher)
		w.Write([]byte("event: step.started\nid: 1\ndata: {\"step\":\"hello\"}\n\n"))
		flusher.Flush()
		w.Write([]byte("event: task.completed\nid: 2\ndata: {\"status\":\"succeeded\"}\n\n"))
		flusher.Flush()
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	return ts, "test-key"
}

func TestClient_Submit(t *testing.T) {
	ts, key := testAPIServer(t)
	c := NewClient(ts.URL, key)

	resp, err := c.Submit("fix-issue", map[string]string{"FOO": "bar"}, "")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if resp.ID != "task_123" {
		t.Errorf("ID = %q, want task_123", resp.ID)
	}
	if resp.Status != "pending" {
		t.Errorf("Status = %q, want pending", resp.Status)
	}
}

func TestClient_Get(t *testing.T) {
	ts, key := testAPIServer(t)
	c := NewClient(ts.URL, key)

	resp, err := c.Get("task_abc")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if resp.ID != "task_abc" {
		t.Errorf("ID = %q, want task_abc", resp.ID)
	}
	if resp.Status != "running" {
		t.Errorf("Status = %q, want running", resp.Status)
	}
	if len(resp.Steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(resp.Steps))
	}
}

func TestClient_GetNotFound(t *testing.T) {
	ts, key := testAPIServer(t)
	c := NewClient(ts.URL, key)

	_, err := c.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestClient_List(t *testing.T) {
	ts, key := testAPIServer(t)
	c := NewClient(ts.URL, key)

	tasks, err := c.List("", "", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(tasks))
	}
}

func TestClient_Cancel(t *testing.T) {
	ts, key := testAPIServer(t)
	c := NewClient(ts.URL, key)

	resp, err := c.Cancel("task_abc")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if resp.Status != "canceled" {
		t.Errorf("Status = %q, want canceled", resp.Status)
	}
}

func TestClient_Unauthorized(t *testing.T) {
	ts, _ := testAPIServer(t)
	c := NewClient(ts.URL, "wrong-key")

	_, err := c.Submit("fix-issue", nil, "")
	if err == nil {
		t.Fatal("expected error for wrong key")
	}
}

func TestStreamEvents(t *testing.T) {
	ts, key := testAPIServer(t)
	c := NewClient(ts.URL, key)

	var buf stringWriter
	err := StreamEvents(c, "task_stream", "", &buf)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	output := buf.String()
	if output == "" {
		t.Error("expected output from stream")
	}
}

// stringWriter implements io.Writer for testing.
type stringWriter struct {
	data []byte
}

func (w *stringWriter) Write(p []byte) (int, error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

func (w *stringWriter) String() string {
	return string(w.data)
}

func TestFormatTask(t *testing.T) {
	now := time.Now()
	resp := &TaskResponse{
		ID:          "task_fmt",
		Status:      "succeeded",
		WorkflowRef: "fix-issue",
		Steps: []StepResponse{
			{Name: "clone", Status: "succeeded", DurationMs: 3200},
			{Name: "solve", Status: "succeeded", DurationMs: 45000, TokensUsed: 5000},
		},
		CreatedAt:       now,
		TotalTokensUsed: 5000,
	}

	output := FormatTask(resp)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatTaskList(t *testing.T) {
	tasks := []*TaskResponse{
		{ID: "task_1", Status: "succeeded", WorkflowRef: "fix-issue", CreatedAt: time.Now()},
		{ID: "task_2", Status: "running", WorkflowRef: "code-review", CreatedAt: time.Now()},
	}

	output := FormatTaskList(tasks)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatTaskList_Empty(t *testing.T) {
	output := FormatTaskList(nil)
	if output != "No tasks found." {
		t.Errorf("expected 'No tasks found.', got %q", output)
	}
}

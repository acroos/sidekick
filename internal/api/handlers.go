package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/austinroos/sidekick/internal/task"
)

// createTaskRequest is the JSON body for POST /tasks.
type createTaskRequest struct {
	Workflow   string            `json:"workflow"`
	Variables  map[string]string `json:"variables"`
	WebhookURL string            `json:"webhook_url"`
}

// handleCreateTask handles POST /tasks.
func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit

	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Workflow == "" {
		writeError(w, http.StatusBadRequest, "workflow is required")
		return
	}

	t, err := s.manager.Submit(r.Context(), task.SubmitRequest{
		WorkflowRef: req.Workflow,
		Variables:   req.Variables,
		WebhookURL:  req.WebhookURL,
	})
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, task.TaskToResponse(t))
}

// handleGetTask handles GET /tasks/{id}.
func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	t, err := s.manager.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if t == nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	writeJSON(w, http.StatusOK, task.TaskToResponse(t))
}

// handleListTasks handles GET /tasks.
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := task.ListFilter{
		Status:      task.Status(q.Get("status")),
		WorkflowRef: q.Get("workflow"),
	}

	if limitStr := q.Get("limit"); limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil {
			filter.Limit = n
		}
	}
	if offsetStr := q.Get("offset"); offsetStr != "" {
		if n, err := strconv.Atoi(offsetStr); err == nil {
			filter.Offset = n
		}
	}

	tasks, err := s.manager.List(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var resp []*task.TaskResponse
	for _, t := range tasks {
		resp = append(resp, task.TaskToResponse(t))
	}

	// Return empty array instead of null.
	if resp == nil {
		resp = []*task.TaskResponse{}
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleCancelTask handles POST /tasks/{id}/cancel.
func (s *Server) handleCancelTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	t, err := s.manager.Cancel(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if t == nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	writeJSON(w, http.StatusOK, task.TaskToResponse(t))
}

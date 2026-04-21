package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/austinroos/sidekick/internal/event"
)

// handleStreamEvents handles GET /tasks/{id}/stream (SSE endpoint).
func (s *Server) handleStreamEvents(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")

	// Verify task exists.
	t, err := s.manager.Get(r.Context(), taskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if t == nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	// Parse type filter.
	typeFilter := parseTypeFilter(r.URL.Query().Get("types"))

	// Parse Last-Event-ID for replay.
	lastEventID := parseLastEventID(r.Header.Get("Last-Event-ID"))

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Subscribe to live events BEFORE replaying to prevent gaps.
	ch, unsub := s.eventBus.Subscribe(taskID)
	defer unsub()

	// Replay historical events from store.
	highestID := lastEventID
	taskCompleted := false
	for {
		events, fetchErr := s.eventStore.Fetch(r.Context(), taskID, highestID, 100)
		if fetchErr != nil || len(events) == 0 {
			break
		}
		for _, evt := range events {
			if matchesFilter(evt.Type, typeFilter) {
				writeSSEEvent(w, evt)
			}
			highestID = evt.ID
			if evt.Type == "task.completed" {
				taskCompleted = true
			}
		}
		flusher.Flush()
		if len(events) < 100 {
			break
		}
	}
	flusher.Flush()

	// If task already completed during replay, we're done.
	if taskCompleted {
		return
	}

	// Stream live events.
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			// Skip events already sent during replay.
			if evt.ID <= highestID {
				continue
			}
			if matchesFilter(evt.Type, typeFilter) {
				writeSSEEvent(w, evt)
				flusher.Flush()
			}
			if evt.Type == "task.completed" {
				return
			}
		}
	}
}

// writeSSEEvent writes a single SSE event to the response.
func writeSSEEvent(w http.ResponseWriter, evt *event.Event) {
	fmt.Fprintf(w, "event: %s\n", evt.Type)
	fmt.Fprintf(w, "id: %d\n", evt.ID)
	fmt.Fprintf(w, "data: %s\n\n", string(evt.Data))
}

// parseTypeFilter parses a comma-separated list of event types.
func parseTypeFilter(types string) map[string]bool {
	if types == "" {
		return nil
	}
	filter := make(map[string]bool)
	for _, t := range strings.Split(types, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			filter[t] = true
		}
	}
	return filter
}

// matchesFilter returns true if the event type passes the filter.
// A nil filter matches all types.
func matchesFilter(eventType string, filter map[string]bool) bool {
	if filter == nil {
		return true
	}
	return filter[eventType]
}

// parseLastEventID parses the Last-Event-ID header value.
func parseLastEventID(header string) int64 {
	if header == "" {
		return 0
	}
	id, err := strconv.ParseInt(header, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

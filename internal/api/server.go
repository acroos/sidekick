// Package api implements the HTTP API server, handlers, middleware, and SSE endpoints.
package api

import (
	"context"
	"net"
	"net/http"

	"github.com/austinroos/sidekick/internal/event"
	"github.com/austinroos/sidekick/internal/task"
	"github.com/austinroos/sidekick/internal/ui"
)

// ServerConfig configures the API server.
type ServerConfig struct {
	ListenAddr string
	APIKey     string
	Manager    *task.Manager
	EventBus   *event.Bus
	EventStore event.Store
}

// Server is the Sidekick HTTP API server.
type Server struct {
	manager    *task.Manager
	eventBus   *event.Bus
	eventStore event.Store
	apiKey     string
	httpServer *http.Server
	listener   net.Listener
}

// NewServer creates a new API server.
func NewServer(cfg ServerConfig) *Server {
	return &Server{
		manager:    cfg.Manager,
		eventBus:   cfg.EventBus,
		eventStore: cfg.EventStore,
		apiKey:     cfg.APIKey,
	}
}

// ListenAndServe sets up the server and begins serving. Call this from main.
func (s *Server) ListenAndServe(addr string) error {
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.routes(),
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.listener = ln

	go func() { _ = s.httpServer.Serve(ln) }()

	return nil
}

// Addr returns the server's listen address, or empty string if not started.
func (s *Server) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

// routes configures the HTTP mux with all API routes.
func (s *Server) routes() http.Handler {
	// API routes require authentication.
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("POST /tasks", s.handleCreateTask)
	apiMux.HandleFunc("GET /tasks", s.handleListTasks)
	apiMux.HandleFunc("GET /tasks/{id}", s.handleGetTask)
	apiMux.HandleFunc("POST /tasks/{id}/cancel", s.handleCancelTask)
	apiMux.HandleFunc("GET /tasks/{id}/stream", s.handleStreamEvents)

	// Top-level mux: UI is public, API is authenticated.
	mux := http.NewServeMux()
	mux.Handle("/tasks", s.authMiddleware(apiMux))
	mux.Handle("/tasks/", s.authMiddleware(apiMux))
	mux.Handle("/", ui.Handler())

	return requestLoggingMiddleware(mux)
}

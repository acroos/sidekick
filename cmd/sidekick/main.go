package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/austinroos/sidekick/internal/agent"
	"github.com/austinroos/sidekick/internal/api"
	"github.com/austinroos/sidekick/internal/cli"
	"github.com/austinroos/sidekick/internal/event"
	"github.com/austinroos/sidekick/internal/proxy"
	"github.com/austinroos/sidekick/internal/sandbox"
	"github.com/austinroos/sidekick/internal/task"
	"github.com/austinroos/sidekick/internal/workflow"
	"github.com/austinroos/sidekick/pkg/config"
)

var version = "dev"

func main() {
	// Load .env file if present; ignore error when file doesn't exist.
	_ = godotenv.Load()

	if err := dispatch(); err != nil {
		fmt.Fprintf(os.Stderr, "sidekick: %v\n", err)
		os.Exit(1)
	}
}

func dispatch() error {
	if len(os.Args) < 2 {
		printUsage()
		return nil
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "server":
		return runServer()
	case "submit":
		return cli.RunSubmit(args)
	case "status":
		return cli.RunStatus(args)
	case "logs":
		return cli.RunLogs(args)
	case "version":
		fmt.Printf("sidekick %s\n", version)
		return nil
	case "help", "--help", "-h":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Sidekick — autonomous coding agent platform

Usage: sidekick <command> [options]

Commands:
  server    Start the Sidekick API server
  submit    Submit a new task
  status    Show task status (or list tasks if no ID given)
  logs      Stream real-time events for a task
  version   Print version information
  help      Show this help message

Run 'sidekick <command> --help' for command-specific options.
`)
}

func setupLogger(format, level string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}
	var handler slog.Handler
	if strings.ToLower(format) == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(handler))
}

func runServer() error {
	// Load configuration.
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Initialize structured logger.
	setupLogger(cfg.LogFormat, cfg.LogLevel)

	// Open event store.
	eventStore, err := event.NewSQLiteStore(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("opening event store: %w", err)
	}
	defer eventStore.Close() //nolint:errcheck // best-effort cleanup on shutdown

	// Open task store (same database, separate table).
	taskStore, err := task.NewSQLiteStore(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("opening task store: %w", err)
	}
	defer taskStore.Close() //nolint:errcheck // best-effort cleanup on shutdown

	// Create event bus.
	eventBus := event.NewBus()

	// Start LLM proxy.
	llmProxy := proxy.New(proxy.Config{
		ListenAddr:    cfg.ProxyListenAddr,
		APIKey:        cfg.AnthropicAPIKey,
		BaseURL:       cfg.AnthropicBaseURL,
		DefaultBudget: cfg.DefaultTokenBudget,
	})
	proxyAddr, err := llmProxy.Start()
	if err != nil {
		return fmt.Errorf("starting proxy: %w", err)
	}
	slog.Info("LLM proxy started", "addr", proxyAddr)

	// Create sandbox provider.
	provider, err := sandbox.NewDockerProvider()
	if err != nil {
		return fmt.Errorf("creating sandbox provider: %w", err)
	}

	// Create agent runner.
	agentRunner := &agent.Runner{
		ProxyAddr: proxyAddr,
		Proxy:     llmProxy,
	}

	// Create workflow executor.
	executor := &workflow.Executor{
		Provider:    provider,
		Bus:         eventBus,
		Store:       eventStore,
		AgentRunner: agentRunner,
	}

	// Create task manager.
	manager := task.NewManager(taskStore, executor, task.ManagerConfig{
		MaxConcurrent: cfg.MaxConcurrentTasks,
		WorkflowDir:   cfg.WorkflowDir,
	})

	// Start API server.
	server := api.NewServer(api.ServerConfig{
		APIKey:     cfg.APIKey,
		Manager:    manager,
		EventBus:   eventBus,
		EventStore: eventStore,
	})
	if err := server.ListenAndServe(cfg.ListenAddr); err != nil {
		return fmt.Errorf("starting API server: %w", err)
	}
	slog.Info("Sidekick API server started", "addr", server.Addr(), "version", version)

	// Wait for shutdown signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	slog.Info("shutting down", "signal", sig.String())

	// Graceful shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("API server shutdown failed", "error", err)
	}
	if err := manager.Shutdown(ctx); err != nil {
		slog.Error("task manager shutdown failed", "error", err)
	}
	if err := llmProxy.Stop(ctx); err != nil {
		slog.Error("LLM proxy shutdown failed", "error", err)
	}

	slog.Info("shutdown complete")
	return nil
}

// Package agent implements the Claude Code agent runtime for executing agent steps in sandboxes.
package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/austinroos/sidekick/internal/proxy"
	"github.com/austinroos/sidekick/internal/sandbox"
)

// Runner executes agent steps inside sandboxes.
type Runner struct {
	// ProxyAddr is the address of the LLM proxy reachable from inside containers.
	// e.g., "host.docker.internal:8089" (Docker Desktop) or "172.17.0.1:8089" (Linux).
	ProxyAddr string

	// Proxy is a reference to the running proxy for token registration.
	// If nil, token tracking is disabled.
	Proxy *proxy.Proxy
}

// RunConfig holds parameters for a single agent step execution.
type RunConfig struct {
	TaskID       string
	StepName     string
	Prompt       string   // Full assembled prompt (context + user prompt)
	AllowedTools []string // e.g., ["Read", "Edit", "Bash"]
	WorkDir      string   // Working directory in sandbox (default "/workspace")
	TokenBudget  int      // Per-step token budget (0 = use proxy default)
}

// EmitFunc is called for each parsed event from the agent output stream.
type EmitFunc func(eventType string, data any)

// Result holds the outcome of an agent step execution.
type Result struct {
	Output     string        // Final text output from the agent
	ExitCode   int           // Process exit code
	TokensUsed int           // Total tokens (input + output)
	Duration   time.Duration // Wall-clock time
}

// Run executes Claude Code in the given sandbox with the provided config.
// It streams events via emitFn and returns the final result.
func (r *Runner) Run(ctx context.Context, sb sandbox.Sandbox, cfg RunConfig, emitFn EmitFunc) (Result, error) {
	start := time.Now()

	// Register task with proxy for token tracking.
	if r.Proxy != nil {
		r.Proxy.RegisterTask(cfg.TaskID, cfg.TokenBudget)
	}

	workDir := cfg.WorkDir
	if workDir == "" {
		workDir = "/workspace"
	}

	// Build the claude command.
	args := []string{"claude", "-p", cfg.Prompt, "--output-format", "stream-json"}
	if len(cfg.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(cfg.AllowedTools, ","))
	}

	cmd := sandbox.Command{
		Args:    args,
		WorkDir: workDir,
		Env: map[string]string{
			"ANTHROPIC_BASE_URL": fmt.Sprintf("http://%s/proxy/%s", r.ProxyAddr, cfg.TaskID),
		},
	}

	stream, err := sb.ExecStream(ctx, cmd)
	if err != nil {
		r.unregisterTask(cfg.TaskID)
		return Result{}, fmt.Errorf("starting agent: %w", err)
	}

	// Parse each output line as stream-json and emit events.
	var finalOutput string
	for line := range stream.Output {
		if line.Stream != "stdout" {
			continue
		}
		events := parseStreamLine(line.Line, cfg.StepName)
		for _, evt := range events {
			emitFn(evt.eventType, evt.data)
			// Capture the last agent output as the final result text.
			if evt.eventType == "agent.output" {
				if ao, ok := evt.data.(agentOutput); ok {
					finalOutput = ao.Text
				}
			}
		}
	}

	// Wait for the process to exit.
	execResult := <-stream.Done

	// Collect token usage from proxy.
	tokensUsed := r.unregisterTask(cfg.TaskID)

	return Result{
		Output:     finalOutput,
		ExitCode:   execResult.ExitCode,
		TokensUsed: tokensUsed,
		Duration:   time.Since(start),
	}, nil
}

// unregisterTask removes the task from proxy tracking and returns total tokens used.
func (r *Runner) unregisterTask(taskID string) int {
	if r.Proxy == nil {
		return 0
	}
	input, output := r.Proxy.UnregisterTask(taskID)
	return input + output
}

// ProxyHost returns the hostname portion of the proxy address (for AllowHosts).
func (r *Runner) ProxyHost() string {
	host := r.ProxyAddr
	if idx := strings.LastIndex(host, ":"); idx >= 0 {
		host = host[:idx]
	}
	return host
}

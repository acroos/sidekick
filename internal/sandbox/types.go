// Package sandbox defines the sandbox provider interface and implements the Docker provider.
package sandbox

import "time"

// Config describes the desired sandbox environment.
type Config struct {
	Image       string            // Container image to use
	Network     NetworkPolicy     // none, restricted, open
	AllowHosts  []string          // Egress allowlist (when restricted)
	CPULimit    float64           // CPU cores
	MemoryLimit int64             // Bytes
	Timeout     time.Duration     // Hard kill deadline
	Env         map[string]string // Environment variables
	Mounts      []Mount           // Volume mounts (e.g., repo checkout)
}

// Mount describes a bind mount from the host into the sandbox.
type Mount struct {
	Source   string // Host path
	Target   string // Container path
	ReadOnly bool
}

// NetworkPolicy controls sandbox network access.
type NetworkPolicy string

const (
	// NetworkNone disables all networking (default).
	NetworkNone NetworkPolicy = "none"
	// NetworkRestricted allows egress only to AllowHosts.
	NetworkRestricted NetworkPolicy = "restricted"
	// NetworkOpen allows unrestricted network access.
	NetworkOpen NetworkPolicy = "open"
)

// Command describes a command to execute inside a sandbox.
type Command struct {
	Args    []string          // e.g., ["npm", "test"]
	WorkDir string            // Working directory inside sandbox
	Env     map[string]string // Additional environment variables
	Timeout time.Duration     // Per-command timeout (0 = no limit)
}

// ExecResult holds the outcome of a completed command execution.
type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

// ExecStream provides real-time output from a running command.
// Used by the workflow engine to feed the event bus.
type ExecStream struct {
	Output <-chan OutputLine // Multiplexed stdout/stderr, line by line
	Done   <-chan ExecResult // Final result when process exits
}

// OutputLine is a single line of output from a running command.
type OutputLine struct {
	Stream string    // "stdout" or "stderr"
	Line   string    // The line content (without trailing newline)
	Time   time.Time // When the line was received
}

// Status represents the lifecycle state of a sandbox.
type Status string

const (
	StatusCreating Status = "creating"
	StatusReady    Status = "ready"
	StatusRunning  Status = "running"
	StatusStopped  Status = "stopped"
	StatusFailed   Status = "failed"
)

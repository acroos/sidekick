package sandbox

import (
	"context"
	"io"
)

// Provider manages the lifecycle of sandboxes.
type Provider interface {
	// Create provisions a new sandbox with the given configuration.
	// The sandbox is ready for command execution when Create returns.
	Create(ctx context.Context, cfg Config) (Sandbox, error)

	// Destroy tears down a sandbox and cleans up all resources.
	// It is idempotent — destroying an already-destroyed sandbox is not an error.
	Destroy(ctx context.Context, id string) error
}

// Sandbox is an isolated execution environment.
type Sandbox interface {
	// ID returns the unique identifier for this sandbox.
	ID() string

	// Exec runs a command and waits for it to complete, returning the full result.
	Exec(ctx context.Context, cmd Command) (*ExecResult, error)

	// ExecStream runs a command and returns channels for real-time output streaming.
	ExecStream(ctx context.Context, cmd Command) (*ExecStream, error)

	// CopyIn copies a file or directory from the host into the sandbox.
	CopyIn(ctx context.Context, src, dst string) error

	// CopyOut reads a file from the sandbox. The caller must read and close the returned reader.
	CopyOut(ctx context.Context, src string) (io.ReadCloser, error)

	// Status returns the current lifecycle state of the sandbox.
	Status() Status
}

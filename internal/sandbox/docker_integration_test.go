package sandbox

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testImage = "sidekick-sandbox-base:latest"

// skipIfNoIntegration skips the test unless SIDEKICK_INTEGRATION=1 and Docker is reachable.
func skipIfNoIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("SIDEKICK_INTEGRATION") == "" {
		t.Skip("set SIDEKICK_INTEGRATION=1 to run integration tests")
	}
}

func newTestProvider(t *testing.T) *DockerProvider {
	t.Helper()
	skipIfNoIntegration(t)
	p, err := NewDockerProvider()
	if err != nil {
		t.Fatalf("creating docker provider: %v", err)
	}
	return p
}

func newTestSandbox(t *testing.T, p *DockerProvider) Sandbox {
	t.Helper()
	cfg := Config{
		Image:       testImage,
		Network:     NetworkNone,
		CPULimit:    0.5,
		MemoryLimit: 256 * 1024 * 1024, // 256MB
		Timeout:     30 * time.Second,
	}
	sb, err := p.Create(context.Background(), cfg)
	if err != nil {
		t.Fatalf("creating sandbox: %v", err)
	}
	t.Cleanup(func() {
		if err := p.Destroy(context.Background(), sb.ID()); err != nil {
			t.Errorf("destroying sandbox: %v", err)
		}
	})
	return sb
}

func TestCreateAndDestroy(t *testing.T) {
	p := newTestProvider(t)
	sb := newTestSandbox(t, p)

	if sb.ID() == "" {
		t.Fatal("sandbox ID is empty")
	}
	if sb.Status() != StatusReady {
		t.Fatalf("expected status %q, got %q", StatusReady, sb.Status())
	}
}

func TestExecSimple(t *testing.T) {
	p := newTestProvider(t)
	sb := newTestSandbox(t, p)

	result, err := sb.Exec(context.Background(), Command{
		Args: []string{"echo", "hello"},
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
	if got := strings.TrimSpace(result.Stdout); got != "hello" {
		t.Fatalf("expected stdout %q, got %q", "hello", got)
	}
}

func TestExecExitCode(t *testing.T) {
	p := newTestProvider(t)
	sb := newTestSandbox(t, p)

	result, err := sb.Exec(context.Background(), Command{
		Args: []string{"sh", "-c", "exit 42"},
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if result.ExitCode != 42 {
		t.Fatalf("expected exit code 42, got %d", result.ExitCode)
	}
}

func TestExecStderr(t *testing.T) {
	p := newTestProvider(t)
	sb := newTestSandbox(t, p)

	result, err := sb.Exec(context.Background(), Command{
		Args: []string{"sh", "-c", "echo error >&2"},
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if got := strings.TrimSpace(result.Stderr); got != "error" {
		t.Fatalf("expected stderr %q, got %q", "error", got)
	}
}

func TestExecTimeout(t *testing.T) {
	p := newTestProvider(t)
	sb := newTestSandbox(t, p)

	_, err := sb.Exec(context.Background(), Command{
		Args:    []string{"sleep", "60"},
		Timeout: 2 * time.Second,
	})
	// The exec should fail due to context deadline.
	if err == nil {
		t.Fatal("expected error from timed-out exec, got nil")
	}
}

func TestExecStream(t *testing.T) {
	p := newTestProvider(t)
	sb := newTestSandbox(t, p)

	stream, err := sb.ExecStream(context.Background(), Command{
		Args: []string{"sh", "-c", "echo line1; echo line2; echo err >&2"},
	})
	if err != nil {
		t.Fatalf("exec stream: %v", err)
	}

	var lines []OutputLine
	for line := range stream.Output {
		lines = append(lines, line)
	}

	// Verify we got all three lines.
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 output lines, got %d: %+v", len(lines), lines)
	}

	// Check that stdout and stderr lines are present.
	var gotStdout, gotStderr bool
	for _, l := range lines {
		if l.Stream == "stdout" && (l.Line == "line1" || l.Line == "line2") {
			gotStdout = true
		}
		if l.Stream == "stderr" && l.Line == "err" {
			gotStderr = true
		}
	}
	if !gotStdout {
		t.Error("missing expected stdout lines")
	}
	if !gotStderr {
		t.Error("missing expected stderr line")
	}

	// Check the final result.
	result := <-stream.Done
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
}

func TestCopyInAndOut(t *testing.T) {
	p := newTestProvider(t)
	sb := newTestSandbox(t, p)

	// Create a temp file on the host.
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "test.txt")
	content := "hello from host"
	if err := os.WriteFile(srcPath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}

	// Copy it into the sandbox.
	if err := sb.CopyIn(context.Background(), srcPath, "/workspace/test.txt"); err != nil {
		t.Fatalf("copy in: %v", err)
	}

	// Verify the file is there via exec.
	result, err := sb.Exec(context.Background(), Command{
		Args: []string{"cat", "/workspace/test.txt"},
	})
	if err != nil {
		t.Fatalf("exec cat: %v", err)
	}
	if result.Stdout != content {
		t.Fatalf("expected content %q, got %q", content, result.Stdout)
	}

	// Copy it back out.
	reader, err := sb.CopyOut(context.Background(), "/workspace/test.txt")
	if err != nil {
		t.Fatalf("copy out: %v", err)
	}
	defer func() { _ = reader.Close() }()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading copy out: %v", err)
	}
	if string(got) != content {
		t.Fatalf("copy out content: expected %q, got %q", content, string(got))
	}
}

func TestReadOnlyRootfs(t *testing.T) {
	p := newTestProvider(t)
	sb := newTestSandbox(t, p)

	result, err := sb.Exec(context.Background(), Command{
		Args: []string{"touch", "/etc/test"},
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code writing to read-only rootfs")
	}
}

func TestNonRootUser(t *testing.T) {
	p := newTestProvider(t)
	sb := newTestSandbox(t, p)

	result, err := sb.Exec(context.Background(), Command{
		Args: []string{"id", "-u"},
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if got := strings.TrimSpace(result.Stdout); got != "1000" {
		t.Fatalf("expected uid 1000, got %q", got)
	}
}

func TestNetworkNone(t *testing.T) {
	p := newTestProvider(t)
	sb := newTestSandbox(t, p)

	result, err := sb.Exec(context.Background(), Command{
		Args:    []string{"curl", "-s", "--max-time", "2", "https://example.com"},
		Timeout: 5 * time.Second,
	})
	if err != nil {
		// Timeout or connection error is expected.
		return
	}
	if result.ExitCode == 0 {
		t.Fatal("expected network request to fail with NetworkNone")
	}
}

func TestDestroyIdempotent(t *testing.T) {
	p := newTestProvider(t)
	cfg := Config{
		Image:       testImage,
		Network:     NetworkNone,
		CPULimit:    0.5,
		MemoryLimit: 256 * 1024 * 1024,
		Timeout:     30 * time.Second,
	}
	sb, err := p.Create(context.Background(), cfg)
	if err != nil {
		t.Fatalf("creating sandbox: %v", err)
	}

	// Destroy twice — second call should not error.
	if err := p.Destroy(context.Background(), sb.ID()); err != nil {
		t.Fatalf("first destroy: %v", err)
	}
	if err := p.Destroy(context.Background(), sb.ID()); err != nil {
		t.Fatalf("second destroy should be idempotent: %v", err)
	}
}

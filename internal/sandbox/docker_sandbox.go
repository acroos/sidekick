package sandbox

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// Exec runs a command in the sandbox and waits for it to complete.
func (s *dockerSandbox) Exec(ctx context.Context, cmd Command) (*ExecResult, error) {
	if cmd.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cmd.Timeout)
		defer cancel()
	}

	start := time.Now()

	execCfg := container.ExecOptions{
		Cmd:          cmd.Args,
		WorkingDir:   cmd.WorkDir,
		Env:          envToSlice(cmd.Env),
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := s.client.ContainerExecCreate(ctx, s.containerID, execCfg)
	if err != nil {
		return nil, fmt.Errorf("creating exec: %w", err)
	}

	resp, err := s.client.ContainerExecAttach(ctx, execID.ID, container.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("attaching to exec: %w", err)
	}
	defer resp.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, resp.Reader); err != nil {
		return nil, fmt.Errorf("reading exec output: %w", err)
	}

	inspect, err := s.client.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return nil, fmt.Errorf("inspecting exec result: %w", err)
	}

	return &ExecResult{
		ExitCode: inspect.ExitCode,
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		Duration: time.Since(start),
	}, nil
}

// ExecStream runs a command and returns channels for real-time output streaming.
func (s *dockerSandbox) ExecStream(ctx context.Context, cmd Command) (*ExecStream, error) {
	if cmd.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cmd.Timeout)
		defer cancel()
	}

	start := time.Now()

	execCfg := container.ExecOptions{
		Cmd:          cmd.Args,
		WorkingDir:   cmd.WorkDir,
		Env:          envToSlice(cmd.Env),
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := s.client.ContainerExecCreate(ctx, s.containerID, execCfg)
	if err != nil {
		return nil, fmt.Errorf("creating exec: %w", err)
	}

	resp, err := s.client.ContainerExecAttach(ctx, execID.ID, container.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("attaching to exec: %w", err)
	}

	output := make(chan OutputLine, 100)
	done := make(chan ExecResult, 1)

	go s.streamOutput(resp, execID.ID, start, output, done)

	return &ExecStream{
		Output: output,
		Done:   done,
	}, nil
}

// streamOutput reads Docker's multiplexed stream and sends lines to channels.
// It handles demultiplexing, line buffering, and cleanup.
func (s *dockerSandbox) streamOutput(
	resp types.HijackedResponse,
	execID string,
	start time.Time,
	output chan<- OutputLine,
	done chan<- ExecResult,
) {
	defer resp.Close()
	defer close(output)
	defer close(done)

	// Per-stream line buffers for handling partial lines.
	var stdoutBuf, stderrBuf strings.Builder
	// Full output for the final ExecResult.
	var fullStdout, fullStderr strings.Builder

	// Read Docker's multiplexed stream frame by frame.
	// Format: [stream_type(1), 0, 0, 0, size(4)] + payload
	header := make([]byte, 8)
	for {
		_, err := io.ReadFull(resp.Reader, header)
		if err != nil {
			// EOF or context canceled — flush remaining partial lines.
			flushPartialLine(&stdoutBuf, "stdout", output)
			flushPartialLine(&stderrBuf, "stderr", output)
			break
		}

		streamType := header[0]
		frameSize := binary.BigEndian.Uint32(header[4:8])

		payload := make([]byte, frameSize)
		if _, err := io.ReadFull(resp.Reader, payload); err != nil {
			flushPartialLine(&stdoutBuf, "stdout", output)
			flushPartialLine(&stderrBuf, "stderr", output)
			break
		}

		var stream string
		var buf *strings.Builder
		var fullBuf *strings.Builder
		switch streamType {
		case 1: // stdout
			stream = "stdout"
			buf = &stdoutBuf
			fullBuf = &fullStdout
		case 2: // stderr
			stream = "stderr"
			buf = &stderrBuf
			fullBuf = &fullStderr
		default:
			continue // Ignore unknown stream types (e.g., stdin=0).
		}

		fullBuf.Write(payload)
		buf.Write(payload)

		// Extract complete lines from the buffer.
		emitCompleteLines(buf, stream, output)
	}

	// Get the exit code.
	exitCode := -1
	inspect, err := s.client.ContainerExecInspect(context.Background(), execID)
	if err == nil {
		exitCode = inspect.ExitCode
	}

	done <- ExecResult{
		ExitCode: exitCode,
		Stdout:   fullStdout.String(),
		Stderr:   fullStderr.String(),
		Duration: time.Since(start),
	}
}

// emitCompleteLines extracts and sends complete lines from the buffer,
// leaving any trailing partial line in the buffer.
func emitCompleteLines(buf *strings.Builder, stream string, output chan<- OutputLine) {
	content := buf.String()
	for {
		idx := strings.IndexByte(content, '\n')
		if idx == -1 {
			break
		}
		line := content[:idx]
		content = content[idx+1:]
		output <- OutputLine{
			Stream: stream,
			Line:   line,
			Time:   time.Now(),
		}
	}
	buf.Reset()
	buf.WriteString(content)
}

// flushPartialLine sends any remaining content in the buffer as a final line.
func flushPartialLine(buf *strings.Builder, stream string, output chan<- OutputLine) {
	if buf.Len() > 0 {
		output <- OutputLine{
			Stream: stream,
			Line:   buf.String(),
			Time:   time.Now(),
		}
		buf.Reset()
	}
}

// CopyIn copies a file or directory from the host into the sandbox.
// Because the rootfs is read-only, we pipe a tar archive through an exec
// command rather than using Docker's CopyToContainer API.
func (s *dockerSandbox) CopyIn(ctx context.Context, src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source %q: %w", src, err)
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	if info.IsDir() {
		if err := tarDirectory(tw, src, filepath.Base(dst)); err != nil {
			return fmt.Errorf("creating tar from directory: %w", err)
		}
	} else {
		if err := tarFile(tw, src, filepath.Base(dst), info); err != nil {
			return fmt.Errorf("creating tar from file: %w", err)
		}
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("closing tar writer: %w", err)
	}

	// Extract via exec to write into the tmpfs-mounted directory.
	dstDir := filepath.Dir(dst)
	execCfg := container.ExecOptions{
		Cmd:          []string{"tar", "xf", "-", "-C", dstDir},
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := s.client.ContainerExecCreate(ctx, s.containerID, execCfg)
	if err != nil {
		return fmt.Errorf("creating exec for copy-in: %w", err)
	}

	resp, err := s.client.ContainerExecAttach(ctx, execID.ID, container.ExecAttachOptions{})
	if err != nil {
		return fmt.Errorf("attaching to exec for copy-in: %w", err)
	}
	defer resp.Close()

	// Write the tar data to stdin.
	if _, err := io.Copy(resp.Conn, &buf); err != nil {
		return fmt.Errorf("writing tar to container: %w", err)
	}
	// Close the write side so tar sees EOF.
	if err := resp.CloseWrite(); err != nil {
		return fmt.Errorf("closing write for copy-in: %w", err)
	}

	// Drain output and wait for completion.
	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, resp.Reader); err != nil {
		return fmt.Errorf("reading copy-in output: %w", err)
	}

	inspect, err := s.client.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return fmt.Errorf("inspecting copy-in exec: %w", err)
	}
	if inspect.ExitCode != 0 {
		return fmt.Errorf("copy-in tar exited with code %d: %s", inspect.ExitCode, stderrBuf.String())
	}

	return nil
}

// CopyOut reads a file from the sandbox.
// Uses exec+tar rather than Docker's CopyFromContainer API for consistency
// with the read-only rootfs approach.
func (s *dockerSandbox) CopyOut(ctx context.Context, src string) (io.ReadCloser, error) {
	srcDir := filepath.Dir(src)
	srcBase := filepath.Base(src)

	execCfg := container.ExecOptions{
		Cmd:          []string{"tar", "cf", "-", "-C", srcDir, srcBase},
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := s.client.ContainerExecCreate(ctx, s.containerID, execCfg)
	if err != nil {
		return nil, fmt.Errorf("creating exec for copy-out: %w", err)
	}

	resp, err := s.client.ContainerExecAttach(ctx, execID.ID, container.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("attaching to exec for copy-out: %w", err)
	}

	// Demux the Docker multiplexed stream into separate stdout/stderr.
	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, resp.Reader); err != nil {
		resp.Close()
		return nil, fmt.Errorf("reading copy-out output: %w", err)
	}
	resp.Close()

	// Extract the file from the tar.
	tr := tar.NewReader(&stdoutBuf)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("file %q not found in tar stream", src)
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar stream: %w", err)
		}
		if hdr.Typeflag == tar.TypeReg {
			content, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("reading file from tar: %w", err)
			}
			return io.NopCloser(bytes.NewReader(content)), nil
		}
	}
}

// tarFile adds a single file to a tar writer.
func tarFile(tw *tar.Writer, src, name string, info os.FileInfo) error {
	hdr := &tar.Header{
		Name: name,
		Mode: int64(info.Mode()),
		Size: info.Size(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}

	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = io.Copy(tw, f)
	return err
}

// tarDirectory recursively adds a directory to a tar writer.
func tarDirectory(tw *tar.Writer, src, base string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		name := filepath.Join(base, rel)

		if info.IsDir() {
			hdr := &tar.Header{
				Name:     name + "/",
				Mode:     int64(info.Mode()),
				Typeflag: tar.TypeDir,
			}
			return tw.WriteHeader(hdr)
		}

		return tarFile(tw, path, name, info)
	})
}

package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
)

// DockerProvider implements Provider using the Docker Engine API.
type DockerProvider struct {
	client client.APIClient
}

// NewDockerProvider creates a DockerProvider that connects to the local Docker daemon.
func NewDockerProvider() (*DockerProvider, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("creating docker client: %w", err)
	}
	return &DockerProvider{client: cli}, nil
}

// Create provisions a hardened Docker container and returns a ready Sandbox.
func (p *DockerProvider) Create(ctx context.Context, cfg Config) (Sandbox, error) {
	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("generating sandbox id: %w", err)
	}
	containerName := "sidekick-" + id

	// Ensure the image exists locally; pull if missing.
	if err := p.ensureImage(ctx, cfg.Image); err != nil {
		return nil, err
	}

	// Resolve network policy.
	nc, err := setupNetwork(ctx, p.client, id, cfg.Network, cfg.AllowHosts)
	if err != nil {
		return nil, err
	}

	// Merge environment variables: config env + any network-injected env.
	env := mergeEnv(cfg.Env, nc.extraEnv)

	containerCfg := &container.Config{
		Image:      cfg.Image,
		User:       "1000:1000",
		WorkingDir: "/workspace",
		Env:        envToSlice(env),
		Cmd:        []string{"sleep", "infinity"},
		Labels: map[string]string{
			"managed-by":          "sidekick",
			"sidekick-sandbox-id": id,
		},
	}

	hostCfg := p.buildHostConfig(cfg, nc)

	resp, err := p.client.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, containerName)
	if err != nil {
		// Clean up network if container creation failed.
		_ = teardownNetwork(ctx, p.client, nc)
		return nil, fmt.Errorf("creating container: %w", err)
	}

	if err := p.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		// Clean up on start failure.
		_ = p.client.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		_ = teardownNetwork(ctx, p.client, nc)
		return nil, fmt.Errorf("starting container: %w", err)
	}

	sb := &dockerSandbox{
		id:            id,
		containerID:   resp.ID,
		containerName: containerName,
		client:        p.client,
		network:       nc,
		status:        StatusReady,
		done:          make(chan struct{}),
	}

	// Enforce container-level timeout.
	if cfg.Timeout > 0 {
		go sb.enforceTimeout(cfg.Timeout)
	}

	return sb, nil
}

// Destroy tears down a sandbox and cleans up all resources.
// It is idempotent — destroying an already-destroyed sandbox is not an error.
func (p *DockerProvider) Destroy(ctx context.Context, id string) error {
	containerName := "sidekick-" + id

	// Inspect to find the sandbox's network config before removal.
	info, err := p.client.ContainerInspect(ctx, containerName)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return nil // Already gone.
		}
		return fmt.Errorf("inspecting container for destroy: %w", err)
	}

	// Stop the container (5s grace period).
	timeout := 5
	_ = p.client.ContainerStop(ctx, info.ID, container.StopOptions{Timeout: &timeout})

	// Remove the container.
	if err := p.client.ContainerRemove(ctx, info.ID, container.RemoveOptions{Force: true, RemoveVolumes: true}); err != nil {
		if !cerrdefs.IsNotFound(err) {
			return fmt.Errorf("removing container: %w", err)
		}
	}

	// Clean up any custom network created for this sandbox.
	networkName := "sidekick-net-" + id
	_ = p.client.NetworkRemove(ctx, networkName)

	return nil
}

// buildHostConfig constructs the Docker HostConfig with all security hardening.
func (p *DockerProvider) buildHostConfig(cfg Config, nc *networkConfig) *container.HostConfig {
	hc := &container.HostConfig{
		CapDrop:        []string{"ALL"},
		SecurityOpt:    []string{"no-new-privileges", "seccomp=" + DefaultSeccompProfile},
		ReadonlyRootfs: true,
		Tmpfs: map[string]string{
			"/tmp":       "rw,noexec,nosuid,size=256m,uid=1000,gid=1000",
			"/workspace": "rw,noexec,nosuid,size=1g,uid=1000,gid=1000",
		},
		NetworkMode: container.NetworkMode(nc.mode),
		Resources:   p.buildResources(cfg),
	}

	// Convert sandbox mounts to Docker bind mounts.
	for _, m := range cfg.Mounts {
		hc.Mounts = append(hc.Mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		})
	}

	return hc
}

// buildResources constructs the Docker resource limits.
func (p *DockerProvider) buildResources(cfg Config) container.Resources {
	cpuLimit := cfg.CPULimit
	if cpuLimit == 0 {
		cpuLimit = 1.0
	}
	memLimit := cfg.MemoryLimit
	if memLimit == 0 {
		memLimit = 512 * 1024 * 1024 // 512MB
	}
	pidsLimit := int64(256)

	return container.Resources{
		NanoCPUs:  int64(cpuLimit * 1e9),
		Memory:    memLimit,
		PidsLimit: &pidsLimit,
	}
}

// ensureImage checks if the image exists locally and pulls it if missing.
func (p *DockerProvider) ensureImage(ctx context.Context, img string) error {
	_, err := p.client.ImageInspect(ctx, img)
	if err == nil {
		return nil // Image exists.
	}
	if !cerrdefs.IsNotFound(err) {
		return fmt.Errorf("inspecting image %q: %w", img, err)
	}

	// Pull the image.
	reader, err := p.client.ImagePull(ctx, img, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling image %q: %w", img, err)
	}
	// Drain the pull output to completion, then close.
	_, copyErr := io.Copy(io.Discard, reader)
	closeErr := reader.Close()
	if copyErr != nil {
		return fmt.Errorf("pulling image %q: %w", img, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("closing image pull reader for %q: %w", img, closeErr)
	}
	return nil
}

// dockerSandbox implements the Sandbox interface backed by a Docker container.
type dockerSandbox struct {
	id            string
	containerID   string
	containerName string
	client        client.APIClient
	network       *networkConfig
	status        Status
	mu            sync.RWMutex
	done          chan struct{} // Closed when Destroy is called; stops the timeout goroutine.
}

// ID returns the unique identifier for this sandbox.
func (s *dockerSandbox) ID() string {
	return s.id
}

// Status returns the current lifecycle state.
func (s *dockerSandbox) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *dockerSandbox) setStatus(st Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = st
}

// enforceTimeout kills the container after the deadline and marks it as failed.
func (s *dockerSandbox) enforceTimeout(timeout time.Duration) {
	select {
	case <-time.After(timeout):
		s.setStatus(StatusFailed)
		// Use a background context since the original may already be canceled.
		_ = s.client.ContainerKill(context.Background(), s.containerID, "KILL")
	case <-s.done:
		// Sandbox was destroyed before timeout fired.
	}
}

// generateID produces a random 12-character hex string.
func generateID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// envToSlice converts a map of env vars to Docker's KEY=VALUE slice format.
func envToSlice(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	s := make([]string, 0, len(env))
	for k, v := range env {
		s = append(s, k+"="+v)
	}
	return s
}

// mergeEnv combines two env maps. Values in b override values in a.
func mergeEnv(a, b map[string]string) map[string]string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	merged := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		merged[k] = v
	}
	for k, v := range b {
		merged[k] = v
	}
	return merged
}

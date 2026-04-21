package sandbox

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

// networkConfig holds the resolved network settings for a sandbox container.
type networkConfig struct {
	// mode is the Docker network mode (e.g., "none", "bridge", or a custom network name).
	mode string
	// networkID is the ID of a custom Docker network, if one was created.
	// Empty for "none" and "bridge" modes.
	networkID string
	// extraEnv holds environment variables to inject (e.g., HTTP_PROXY for restricted mode).
	extraEnv map[string]string
}

// setupNetwork resolves the network policy into Docker network settings.
// For NetworkRestricted, it creates a custom bridge network and returns proxy
// environment variables for the allowed hosts. This is an MVP approach — proper
// iptables-based enforcement is a future improvement.
func setupNetwork(ctx context.Context, cli client.APIClient, sandboxID string, policy NetworkPolicy, allowHosts []string) (*networkConfig, error) {
	switch policy {
	case NetworkNone, "":
		return &networkConfig{mode: "none"}, nil

	case NetworkOpen:
		return &networkConfig{mode: "bridge"}, nil

	case NetworkRestricted:
		return setupRestrictedNetwork(ctx, cli, sandboxID, allowHosts)

	default:
		return nil, fmt.Errorf("unknown network policy: %q", policy)
	}
}

// setupRestrictedNetwork creates a custom bridge network and returns proxy
// environment variables that restrict egress to the allowed hosts.
func setupRestrictedNetwork(ctx context.Context, cli client.APIClient, sandboxID string, allowHosts []string) (*networkConfig, error) {
	networkName := fmt.Sprintf("sidekick-net-%s", sandboxID)

	resp, err := cli.NetworkCreate(ctx, networkName, network.CreateOptions{
		Driver: "bridge",
		Labels: map[string]string{
			"managed-by":          "sidekick",
			"sidekick-sandbox-id": sandboxID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating restricted network: %w", err)
	}

	// Build a no_proxy value from the allowed hosts so that only those hosts
	// are reachable. Traffic to any other host goes through the (nonexistent)
	// proxy and fails — achieving a basic allowlist effect.
	//
	// TODO: Replace with iptables-based enforcement for non-HTTP traffic.
	noProxy := ""
	for i, h := range allowHosts {
		if i > 0 {
			noProxy += ","
		}
		noProxy += h
	}

	env := map[string]string{
		// Point proxy at a non-routable address so non-allowed traffic fails.
		"HTTP_PROXY":  "http://0.0.0.0:0",
		"HTTPS_PROXY": "http://0.0.0.0:0",
		"NO_PROXY":    noProxy,
		"http_proxy":  "http://0.0.0.0:0",
		"https_proxy": "http://0.0.0.0:0",
		"no_proxy":    noProxy,
	}

	return &networkConfig{
		mode:      networkName,
		networkID: resp.ID,
		extraEnv:  env,
	}, nil
}

// teardownNetwork removes a custom network if one was created.
func teardownNetwork(ctx context.Context, cli client.APIClient, nc *networkConfig) error {
	if nc == nil || nc.networkID == "" {
		return nil
	}
	return cli.NetworkRemove(ctx, nc.networkID)
}

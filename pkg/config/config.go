// Package config handles Sidekick server configuration.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all server configuration.
type Config struct {
	// API server
	ListenAddr string // HTTP listen address
	APIKey     string // X-Sidekick-Key value for authentication

	// Task management
	MaxConcurrentTasks int    // Worker pool size
	WorkflowDir        string // Directory containing workflow YAML files

	// Database
	DatabasePath string // SQLite database file path

	// LLM Proxy
	ProxyListenAddr    string // Proxy listen address
	AnthropicAPIKey    string // Anthropic API key
	AnthropicBaseURL   string // Anthropic API base URL
	DefaultTokenBudget int    // Default per-task token budget (0 = unlimited)

	// Logging
	LogFormat string // "text" or "json" (default "text")
	LogLevel  string // "debug", "info", "warn", "error" (default "info")

	// Timeouts
	ShutdownTimeout time.Duration // Graceful shutdown deadline
}

// Load reads configuration from environment variables with sensible defaults.
// Returns an error if required fields are missing.
func Load() (*Config, error) {
	cfg := &Config{
		ListenAddr:         envOrDefault("SIDEKICK_LISTEN_ADDR", ":8080"),
		APIKey:             os.Getenv("SIDEKICK_API_KEY"),
		MaxConcurrentTasks: envIntOrDefault("SIDEKICK_MAX_CONCURRENT_TASKS", 4),
		WorkflowDir:        envOrDefault("SIDEKICK_WORKFLOW_DIR", "workflows/templates"),
		DatabasePath:       envOrDefault("SIDEKICK_DB_PATH", "sidekick.db"),
		ProxyListenAddr:    envOrDefault("SIDEKICK_PROXY_ADDR", ":8089"),
		AnthropicAPIKey:    os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicBaseURL:   envOrDefault("ANTHROPIC_BASE_URL", "https://api.anthropic.com"),
		DefaultTokenBudget: envIntOrDefault("SIDEKICK_DEFAULT_TOKEN_BUDGET", 0),
		LogFormat:          envOrDefault("SIDEKICK_LOG_FORMAT", "text"),
		LogLevel:           envOrDefault("SIDEKICK_LOG_LEVEL", "info"),
		ShutdownTimeout:    envDurationOrDefault("SIDEKICK_SHUTDOWN_TIMEOUT", 30*time.Second),
	}

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("SIDEKICK_API_KEY is required")
	}
	if cfg.AnthropicAPIKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is required")
	}

	return cfg, nil
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envIntOrDefault(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

// ClientConfig holds configuration for the CLI client.
type ClientConfig struct {
	ServerURL string // Sidekick server URL
	APIKey    string // X-Sidekick-Key value
}

// LoadClient reads client configuration from environment variables.
func LoadClient() (*ClientConfig, error) {
	cfg := &ClientConfig{
		ServerURL: envOrDefault("SIDEKICK_SERVER_URL", "http://localhost:8080"),
		APIKey:    os.Getenv("SIDEKICK_API_KEY"),
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("SIDEKICK_API_KEY is required")
	}
	return cfg, nil
}

func envDurationOrDefault(key string, defaultVal time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return defaultVal
	}
	return d
}

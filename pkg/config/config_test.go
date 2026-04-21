package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("SIDEKICK_API_KEY", "sk-test")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, ":8080")
	}
	if cfg.MaxConcurrentTasks != 4 {
		t.Errorf("MaxConcurrentTasks = %d, want 4", cfg.MaxConcurrentTasks)
	}
	if cfg.WorkflowDir != "workflows/templates" {
		t.Errorf("WorkflowDir = %q, want %q", cfg.WorkflowDir, "workflows/templates")
	}
	if cfg.DatabasePath != "sidekick.db" {
		t.Errorf("DatabasePath = %q, want %q", cfg.DatabasePath, "sidekick.db")
	}
	if cfg.ProxyListenAddr != ":8089" {
		t.Errorf("ProxyListenAddr = %q, want %q", cfg.ProxyListenAddr, ":8089")
	}
	if cfg.AnthropicBaseURL != "https://api.anthropic.com" {
		t.Errorf("AnthropicBaseURL = %q, want %q", cfg.AnthropicBaseURL, "https://api.anthropic.com")
	}
	if cfg.DefaultTokenBudget != 0 {
		t.Errorf("DefaultTokenBudget = %d, want 0", cfg.DefaultTokenBudget)
	}
	if cfg.ShutdownTimeout != 30*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 30s", cfg.ShutdownTimeout)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	t.Setenv("SIDEKICK_API_KEY", "sk-test")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	t.Setenv("SIDEKICK_LISTEN_ADDR", ":9090")
	t.Setenv("SIDEKICK_MAX_CONCURRENT_TASKS", "8")
	t.Setenv("SIDEKICK_SHUTDOWN_TIMEOUT", "1m")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ListenAddr != ":9090" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, ":9090")
	}
	if cfg.MaxConcurrentTasks != 8 {
		t.Errorf("MaxConcurrentTasks = %d, want 8", cfg.MaxConcurrentTasks)
	}
	if cfg.ShutdownTimeout != time.Minute {
		t.Errorf("ShutdownTimeout = %v, want 1m", cfg.ShutdownTimeout)
	}
}

func TestLoad_MissingSidekickAPIKey(t *testing.T) {
	os.Unsetenv("SIDEKICK_API_KEY")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing SIDEKICK_API_KEY")
	}
}

func TestLoad_MissingAnthropicAPIKey(t *testing.T) {
	t.Setenv("SIDEKICK_API_KEY", "sk-test")
	os.Unsetenv("ANTHROPIC_API_KEY")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing ANTHROPIC_API_KEY")
	}
}

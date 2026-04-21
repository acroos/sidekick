package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func startProxy(t *testing.T, apiKey, baseURL string) *Proxy {
	t.Helper()
	p := New(Config{
		ListenAddr: "127.0.0.1:0",
		APIKey:     apiKey,
		BaseURL:    baseURL,
	})
	addr, err := p.Start()
	if err != nil {
		t.Fatalf("start proxy: %v", err)
	}
	t.Cleanup(func() { _ = p.Stop(context.Background()) })
	// Store the actual address back for constructing request URLs.
	p.cfg.ListenAddr = addr
	return p
}

func TestProxyInjectsAuth(t *testing.T) {
	var capturedReq *http.Request
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"usage":{"input_tokens":10,"output_tokens":5}}`))
	}))
	t.Cleanup(upstream.Close)

	p := startProxy(t, "sk-test-key-123", upstream.URL)
	p.RegisterTask("task-1", 0)

	resp, err := http.Post("http://"+p.cfg.ListenAddr+"/proxy/task-1/v1/messages",
		"application/json", strings.NewReader(`{"model":"claude-sonnet-4-20250514"}`))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck // test cleanup

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if capturedReq == nil {
		t.Fatal("upstream did not receive request")
	}
	if got := capturedReq.Header.Get("x-api-key"); got != "sk-test-key-123" {
		t.Fatalf("expected x-api-key header, got %q", got)
	}
	if got := capturedReq.Header.Get("anthropic-version"); got == "" {
		t.Fatal("expected anthropic-version header to be set")
	}
}

func TestProxyStripsTaskPrefix(t *testing.T) {
	var capturedPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	p := startProxy(t, "key", upstream.URL)
	p.RegisterTask("task-abc", 0)

	resp, err := http.Get("http://" + p.cfg.ListenAddr + "/proxy/task-abc/v1/messages")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck // test cleanup

	if capturedPath != "/v1/messages" {
		t.Fatalf("expected /v1/messages upstream, got %q", capturedPath)
	}
}

func TestProxyTracksTokensNonStreaming(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"msg_1","usage":{"input_tokens":100,"output_tokens":50}}`))
	}))
	t.Cleanup(upstream.Close)

	p := startProxy(t, "key", upstream.URL)
	p.RegisterTask("task-1", 0)

	// Make two requests.
	for range 2 {
		resp, err := http.Post("http://"+p.cfg.ListenAddr+"/proxy/task-1/v1/messages",
			"application/json", strings.NewReader(`{}`))
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		_ = resp.Body.Close()
	}

	input, output := p.TokensUsed("task-1")
	if input != 200 {
		t.Fatalf("expected 200 input tokens, got %d", input)
	}
	if output != 100 {
		t.Fatalf("expected 100 output tokens, got %d", output)
	}
}

func TestProxyBudgetExceeded(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"usage":{"input_tokens":80,"output_tokens":30}}`))
	}))
	t.Cleanup(upstream.Close)

	p := startProxy(t, "key", upstream.URL)
	p.RegisterTask("task-1", 100) // Budget of 100 tokens total

	// First request uses 110 tokens (80+30), which will be tracked.
	resp1, err := http.Post("http://"+p.cfg.ListenAddr+"/proxy/task-1/v1/messages",
		"application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("request 1: %v", err)
	}
	_ = resp1.Body.Close()

	// Second request should be rejected — budget exceeded (110 >= 100).
	resp2, err := http.Post("http://"+p.cfg.ListenAddr+"/proxy/task-1/v1/messages",
		"application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("request 2: %v", err)
	}
	defer resp2.Body.Close() //nolint:errcheck // test cleanup

	if resp2.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", resp2.StatusCode)
	}
}

func TestProxyRegisterUnregister(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"usage":{"input_tokens":42,"output_tokens":18}}`))
	}))
	t.Cleanup(upstream.Close)

	p := startProxy(t, "key", upstream.URL)
	p.RegisterTask("task-1", 0)

	resp, err := http.Post("http://"+p.cfg.ListenAddr+"/proxy/task-1/v1/messages",
		"application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	_ = resp.Body.Close()

	input, output := p.UnregisterTask("task-1")
	if input != 42 || output != 18 {
		t.Fatalf("expected 42/18, got %d/%d", input, output)
	}

	// After unregister, TokensUsed returns 0.
	input, output = p.TokensUsed("task-1")
	if input != 0 || output != 0 {
		t.Fatalf("expected 0/0 after unregister, got %d/%d", input, output)
	}
}

func TestProxyUnknownTask(t *testing.T) {
	p := startProxy(t, "key", "http://localhost:1")
	// Do NOT register any task.

	resp, err := http.Post("http://"+p.cfg.ListenAddr+"/proxy/unknown-task/v1/messages",
		"application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck // test cleanup

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for unknown task, got %d", resp.StatusCode)
	}
}

func TestProxyMissingTaskID(t *testing.T) {
	p := startProxy(t, "key", "http://localhost:1")

	resp, err := http.Get("http://" + p.cfg.ListenAddr + "/proxy/")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck // test cleanup

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

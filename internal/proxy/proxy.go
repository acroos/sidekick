// Package proxy implements the LLM reverse proxy for auth injection and token budgeting.
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Config configures the LLM proxy server.
type Config struct {
	ListenAddr     string        // e.g., ":8089"
	APIKey         string        // Anthropic API key
	BaseURL        string        // "https://api.anthropic.com"
	DefaultBudget  int           // Default per-task token budget (0 = unlimited)
	RequestTimeout time.Duration // Per-request timeout to upstream
}

// Proxy is the LLM reverse proxy server.
type Proxy struct {
	cfg    Config
	server *http.Server

	mu      sync.RWMutex
	budgets map[string]*taskBudget
}

// taskBudget tracks token usage for a single task.
type taskBudget struct {
	limit      int
	inputUsed  int
	outputUsed int
}

// New creates a new Proxy. Call Start to begin listening.
func New(cfg Config) *Proxy {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com"
	}
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = 5 * time.Minute
	}
	return &Proxy{
		cfg:     cfg,
		budgets: make(map[string]*taskBudget),
	}
}

// Start begins listening and returns the actual address.
func (p *Proxy) Start() (string, error) {
	ln, err := net.Listen("tcp", p.cfg.ListenAddr)
	if err != nil {
		return "", fmt.Errorf("proxy listen: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/proxy/", p.handleProxy)

	p.server = &http.Server{Handler: mux}
	go func() { _ = p.server.Serve(ln) }()

	return ln.Addr().String(), nil
}

// Stop gracefully shuts down the proxy.
func (p *Proxy) Stop(ctx context.Context) error {
	if p.server == nil {
		return nil
	}
	return p.server.Shutdown(ctx)
}

// RegisterTask sets up token tracking for a task. Budget of 0 means unlimited.
func (p *Proxy) RegisterTask(taskID string, budget int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.budgets[taskID] = &taskBudget{limit: budget}
}

// UnregisterTask removes token tracking for a task and returns final usage.
func (p *Proxy) UnregisterTask(taskID string) (inputTokens, outputTokens int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	b, ok := p.budgets[taskID]
	if !ok {
		return 0, 0
	}
	delete(p.budgets, taskID)
	return b.inputUsed, b.outputUsed
}

// TokensUsed returns current token count for a task.
func (p *Proxy) TokensUsed(taskID string) (inputTokens, outputTokens int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	b, ok := p.budgets[taskID]
	if !ok {
		return 0, 0
	}
	return b.inputUsed, b.outputUsed
}

// handleProxy routes /proxy/{taskID}/... requests to the upstream API.
func (p *Proxy) handleProxy(w http.ResponseWriter, r *http.Request) {
	// Extract task ID from path: /proxy/{taskID}/v1/messages -> taskID, /v1/messages
	path := strings.TrimPrefix(r.URL.Path, "/proxy/")
	slashIdx := strings.Index(path, "/")
	if slashIdx < 0 {
		http.Error(w, "invalid proxy path: missing API path after task ID", http.StatusBadRequest)
		return
	}
	taskID := path[:slashIdx]
	apiPath := path[slashIdx:]

	if taskID == "" {
		http.Error(w, "missing task ID in proxy path", http.StatusBadRequest)
		return
	}

	// Check that the task is registered and within budget.
	if err := p.checkBudget(taskID); err != nil {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}

	// Build upstream request.
	upstreamURL := strings.TrimRight(p.cfg.BaseURL, "/") + apiPath
	if r.URL.RawQuery != "" {
		upstreamURL += "?" + r.URL.RawQuery
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "reading request body: "+err.Error(), http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	if p.cfg.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.cfg.RequestTimeout)
		defer cancel()
	}

	upReq, err := http.NewRequestWithContext(ctx, r.Method, upstreamURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "creating upstream request: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Copy headers from original request.
	for k, vals := range r.Header {
		for _, v := range vals {
			upReq.Header.Add(k, v)
		}
	}

	// Inject auth.
	upReq.Header.Set("x-api-key", p.cfg.APIKey)
	if upReq.Header.Get("anthropic-version") == "" {
		upReq.Header.Set("anthropic-version", "2023-06-01")
	}

	// Forward request.
	resp, err := http.DefaultClient.Do(upReq)
	if err != nil {
		http.Error(w, "upstream request failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort cleanup

	// Read the response body to extract token usage.
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "reading upstream response: "+err.Error(), http.StatusBadGateway)
		return
	}

	// Extract and accumulate token usage.
	p.extractAndTrackTokens(taskID, respBody)

	// Write response back to caller.
	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

// checkBudget returns an error if the task's token budget is exceeded.
func (p *Proxy) checkBudget(taskID string) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	b, ok := p.budgets[taskID]
	if !ok {
		return fmt.Errorf("task %q not registered with proxy", taskID)
	}

	if b.limit > 0 && (b.inputUsed+b.outputUsed) >= b.limit {
		return fmt.Errorf("token budget exceeded for task %q: used %d of %d",
			taskID, b.inputUsed+b.outputUsed, b.limit)
	}
	return nil
}

// usageResponse is the minimal structure to extract token counts from Anthropic responses.
type usageResponse struct {
	Usage *usageData `json:"usage,omitempty"`
}

type usageData struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// extractAndTrackTokens parses token usage from the response body and accumulates it.
func (p *Proxy) extractAndTrackTokens(taskID string, body []byte) {
	// Try parsing as a single JSON response (non-streaming).
	var resp usageResponse
	if err := json.Unmarshal(body, &resp); err == nil && resp.Usage != nil {
		p.addTokens(taskID, resp.Usage.InputTokens, resp.Usage.OutputTokens)
		return
	}

	// For streaming responses (SSE), scan for lines containing usage data.
	// Anthropic streaming puts usage in message_delta events near the end.
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimPrefix(line, "data: ")
		if !strings.Contains(line, "usage") {
			continue
		}
		var evt usageResponse
		if err := json.Unmarshal([]byte(line), &evt); err == nil && evt.Usage != nil {
			p.addTokens(taskID, evt.Usage.InputTokens, evt.Usage.OutputTokens)
		}
	}
}

// addTokens accumulates token usage for a task.
func (p *Proxy) addTokens(taskID string, input, output int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	b, ok := p.budgets[taskID]
	if !ok {
		return
	}
	b.inputUsed += input
	b.outputUsed += output
}

// Package cli implements the Sidekick CLI client for interacting with the API server.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client is an HTTP client for the Sidekick API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new API client.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// TaskResponse mirrors the API JSON representation of a task.
type TaskResponse struct {
	ID              string         `json:"id"`
	Status          string         `json:"status"`
	WorkflowRef     string         `json:"workflow"`
	Steps           []StepResponse `json:"steps,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	StartedAt       *time.Time     `json:"started_at,omitempty"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
	Error           string         `json:"error,omitempty"`
	TotalTokensUsed int            `json:"total_tokens_used,omitempty"`
}

// StepResponse mirrors the API JSON representation of a step result.
type StepResponse struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	TokensUsed int    `json:"tokens_used,omitempty"`
}

// Submit creates a new task.
func (c *Client) Submit(workflow string, variables map[string]string, webhookURL string) (*TaskResponse, error) {
	body := map[string]any{
		"workflow":  workflow,
		"variables": variables,
	}
	if webhookURL != "" {
		body["webhook_url"] = webhookURL
	}

	var resp TaskResponse
	if err := c.do("POST", "/tasks", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Get returns a task by ID.
func (c *Client) Get(taskID string) (*TaskResponse, error) {
	var resp TaskResponse
	if err := c.do("GET", "/tasks/"+taskID, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// List returns tasks matching the given filters.
func (c *Client) List(status, workflow string, limit int) ([]*TaskResponse, error) {
	params := url.Values{}
	if status != "" {
		params.Set("status", status)
	}
	if workflow != "" {
		params.Set("workflow", workflow)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}

	path := "/tasks"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp []*TaskResponse
	if err := c.do("GET", path, nil, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// Cancel cancels a running task.
func (c *Client) Cancel(taskID string) (*TaskResponse, error) {
	var resp TaskResponse
	if err := c.do("POST", "/tasks/"+taskID+"/cancel", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// StreamResponse returns a raw HTTP response for SSE streaming.
// The caller must close the response body.
func (c *Client) StreamResponse(taskID string, types string, lastEventID string) (*http.Response, error) {
	path := "/tasks/" + taskID + "/stream"
	if types != "" {
		path += "?types=" + url.QueryEscape(types)
	}

	req, err := http.NewRequest("GET", c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("X-Sidekick-Key", c.apiKey)
	req.Header.Set("Accept", "text/event-stream")
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}

	// Use a client without timeout for streaming.
	streamClient := &http.Client{}
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connecting to stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, parseErrorResponse(resp)
	}

	return resp, nil
}

func (c *Client) do(method, path string, body any, result any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("X-Sidekick-Key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return parseErrorResponse(resp)
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}

func parseErrorResponse(resp *http.Response) error {
	var errResp struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, errResp.Error)
}

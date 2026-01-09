package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	defaultBaseURL = "https://openrouter.ai/api/v1"
	contentType    = "application/json"
)

// Client is the OpenRouter API client.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client

	// OpenRouter specific headers for app rankings
	httpReferer string // Optional: URL of your site
	xTitle      string // Optional: Name of your site
}

// Option defines a functional option for configuring the Client.
type Option func(*Client)

// WithBaseURL overrides the default OpenRouter API base URL.
func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(url, "/")
	}
}

// WithHTTPClient allows providing a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.httpClient = client
	}
}

// WithReferer sets the HTTP-Referer header (recommended by OpenRouter for rankings).
func WithReferer(referer string) Option {
	return func(c *Client) {
		c.httpReferer = referer
	}
}

// WithTitle sets the X-Title header (recommended by OpenRouter for rankings).
func WithTitle(title string) Option {
	return func(c *Client) {
		c.xTitle = title
	}
}

// NewClient creates a new OpenRouter client.
func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:     apiKey,
		baseURL:    defaultBaseURL,
		httpClient: http.DefaultClient,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// -----------------------------------------------------------------------------
// Models
// -----------------------------------------------------------------------------

// ListModels retrieves the list of available models from OpenRouter.
func (c *Client) ListModels(ctx context.Context) (*ListModelsResponse, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return nil, err
	}

	var resp ListModelsResponse
	if err := c.sendRequest(req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// -----------------------------------------------------------------------------
// Internal Helpers
// -----------------------------------------------------------------------------

func (c *Client) newRequest(ctx context.Context, method, path string, payload interface{}) (*http.Request, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		body = bytes.NewReader(b)
	}

	url := fmt.Sprintf("%s%s", c.baseURL, path)
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	if c.httpReferer != "" {
		req.Header.Set("HTTP-Referer", c.httpReferer)
	}
	if c.xTitle != "" {
		req.Header.Set("X-Title", c.xTitle)
	}

	return req, nil
}

func (c *Client) sendRequest(req *http.Request, v interface{}) error {
	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer res.Body.Close()

	// Check for non-2xx status codes
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var errResp ErrorResponse
		// Try to decode the error response
		if decodeErr := json.NewDecoder(res.Body).Decode(&errResp); decodeErr == nil && errResp.Error.Message != "" {
			return fmt.Errorf("api error (status %d): %s - %s", res.StatusCode, errResp.Error.Type, errResp.Error.Message)
		}
		// Fallback if JSON decoding fails
		return fmt.Errorf("api error (status %d)", res.StatusCode)
	}

	if v == nil {
		return nil
	}

	if err := json.NewDecoder(res.Body).Decode(v); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

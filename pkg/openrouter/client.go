package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/errors"
)

const (
	defaultBaseURL = "https://openrouter.ai/api/v1"
	contentType    = "application/json"
	defaultTimeout = 30 * time.Second
)

type Client struct {
	apiKey      string
	baseURL     string
	httpClient  *http.Client
	httpReferer string
	xTitle      string
}

type Option func(*Client)

func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(url, "/")
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.httpClient = client
	}
}

func WithReferer(referer string) Option {
	return func(c *Client) {
		c.httpReferer = referer
	}
}

func WithTitle(title string) Option {
	return func(c *Client) {
		c.xTitle = title
	}
}

func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// GetModelInfo retrieves details for a specific model ID
func (c *Client) GetModelInfo(ctx context.Context, modelID string) (*Model, error) {
	list, err := c.ListModels(ctx)
	if err != nil {
		return nil, err
	}

	for _, m := range list.Data {
		if m.ID == modelID {
			return &m, nil
		}
	}
	return nil, fmt.Errorf("model %s not found", modelID)
}

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

// Helper to create a request with a reusable body reader
func (c *Client) newRequestWithBytes(ctx context.Context, method, path string, payload []byte) (*http.Request, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
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

// Kept for backward compatibility if needed, but we prefer newRequestWithBytes internally
func (c *Client) newRequest(ctx context.Context, method, path string, payload interface{}) (*http.Request, error) {
	var b []byte
	var err error
	if payload != nil {
		b, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
	}
	return c.newRequestWithBytes(ctx, method, path, b)
}

func (c *Client) sendRequest(req *http.Request, v any) error {
	res, err := c.httpClient.Do(req)
	if err != nil {
		if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return errors.NewAPIError(0, err.Error(), "", req.URL.Path)
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// FIX: Increased buffer to 4KB to catch full error descriptions
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return errors.NewAPIError(res.StatusCode, string(body), "", req.URL.Path)
	}

	if v == nil {
		return nil
	}
	if err := json.NewDecoder(res.Body).Decode(v); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	return nil
}

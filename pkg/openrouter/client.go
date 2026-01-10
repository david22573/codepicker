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
	defaultTimeout = 30 * time.Second // Phase 1.1: Enforce sane default timeout
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
		// Phase 1.1: Initialize with a timeout to prevent hanging forever
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
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

func (c *Client) sendRequest(req *http.Request, v any) error {
	res, err := c.httpClient.Do(req)
	if err != nil {
		// Go's http client returns an error for context cancellation
		if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return errors.NewAPIError(0, err.Error(), "", req.URL.Path)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
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


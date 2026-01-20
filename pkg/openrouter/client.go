package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/constants"
	"github.com/david22573/codepicker/internal/errors"
)

const (
	defaultBaseURL = "https://openrouter.ai/api/v1"
	contentType    = "application/json"
	// Increased timeout to handle large context processing
	defaultTimeout = 5 * time.Minute
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

func (c *Client) newRequestWithBytes(
	ctx context.Context,
	method, path string,
	payload []byte,
) (*http.Request, error) {

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

func (c *Client) newRequest(
	ctx context.Context,
	method, path string,
	payload interface{},
) (*http.Request, error) {

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

// sendRequest handles HTTP execution with Exponential Backoff for 429s
func (c *Client) sendRequest(req *http.Request, v any) error {
	var lastErr error
	// Use more retries than default for rate limits
	maxAttempts := constants.MaxRetries + 2

	for attempt := 0; attempt <= maxAttempts; attempt++ {
		// 1. Handle Backoff
		if attempt > 0 {
			// Calculate exponential backoff: 2s, 4s, 8s, 16s...
			delay := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			// Add jitter
			jitter := time.Duration(rand.Int63n(int64(1000 * time.Millisecond)))
			sleepTime := delay + jitter

			// Check if context is already canceled before sleeping
			if ctxErr := req.Context().Err(); ctxErr != nil {
				return ctxErr
			}

			time.Sleep(sleepTime)
		}

		// 2. Rewind Body (Critical for retries)
		if attempt > 0 && req.GetBody != nil {
			bodyCopy, err := req.GetBody()
			if err != nil {
				return fmt.Errorf("failed to rewind request body: %w", err)
			}
			req.Body = bodyCopy
		}

		// 3. Execute
		res, err := c.httpClient.Do(req)
		if err != nil {
			if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) {
				return err
			}
			lastErr = errors.NewInternalError("openrouter.http", err)
			continue // Network error, retry
		}

		// 4. Handle Rate Limits & Server Errors
		if res.StatusCode == 429 || (res.StatusCode >= 500 && res.StatusCode != 501) {
			res.Body.Close() // Close immediately to reuse connection

			// Check for specific Retry-After header
			if retryAfter := res.Header.Get("Retry-After"); retryAfter != "" {
				if seconds, err := strconv.Atoi(retryAfter); err == nil {
					// Use server's suggestion if valid
					time.Sleep(time.Duration(seconds) * time.Second)
					continue
				}
			}

			lastErr = fmt.Errorf("server error %d", res.StatusCode)
			continue // Retry with standard backoff
		}

		// 5. Handle Other Failures (Non-retryable)
		defer res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
			return errors.NewInternalError(
				"openrouter.http",
				fmt.Errorf("status %d on %s: %s", res.StatusCode, req.URL.Path, string(body)),
			)
		}

		// 6. Success - Decode
		if v == nil {
			return nil
		}

		if err := json.NewDecoder(res.Body).Decode(v); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}

		return nil
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

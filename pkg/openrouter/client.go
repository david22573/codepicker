package openrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
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
	// Keep timeout high for reasoning models like DeepSeek
	defaultTimeout = 30 * time.Minute
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

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	c := &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout:   defaultTimeout,
			Transport: transport,
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

func (c *Client) CreateChatCompletion(
	ctx context.Context,
	req ChatCompletionRequest,
) (*ChatCompletionResponse, error) {

	// PREFILL INJECTION:
	// If a prefill is defined, we append it as the last message with role "assistant".
	// This forces the model to continue from this point.
	if req.Prefill != "" {
		req.Messages = append(req.Messages, ChatMessage{
			Role:    "assistant",
			Content: req.Prefill,
		})
	}

	req.Stream = true

	stream, err := c.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	// Initialize the response with the prefill content already present.
	// The API will only return the *continuation*, so we must prepend
	// the prefill to ensure the caller gets the full valid text/JSON.
	fullResp := &ChatCompletionResponse{
		Choices: []Choice{{
			Message: &ChatMessage{
				Role:    "assistant",
				Content: req.Prefill,
			},
		}},
	}

	var toolCalls []ToolCall
	var contentBuilder strings.Builder

	// Start builder with prefill so we append subsequent chunks correctly
	contentBuilder.WriteString(req.Prefill)

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("stream interrupted: %w", err)
		}

		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta

			if delta.Content != nil {
				contentBuilder.WriteString(fmt.Sprintf("%v", delta.Content))
			}

			if len(delta.ToolCalls) > 0 {
				for _, tc := range delta.ToolCalls {

					if tc.Index >= len(toolCalls) {
						newCalls := make([]ToolCall, tc.Index+1)
						copy(newCalls, toolCalls)
						toolCalls = newCalls
					}

					current := &toolCalls[tc.Index]
					if tc.ID != "" {
						current.ID = tc.ID
						current.Type = tc.Type
						current.Function.Name = tc.Function.Name
					}
					current.Function.Arguments += tc.Function.Arguments
				}
			}

			if chunk.Usage != nil {
				fullResp.Usage = chunk.Usage
			}

			if fullResp.ID == "" {
				fullResp.ID = chunk.ID
				fullResp.Model = chunk.Model
			}
		}
	}

	fullResp.Choices[0].Message.Content = contentBuilder.String()
	fullResp.Choices[0].Message.ToolCalls = toolCalls

	return fullResp, nil
}

func (c *Client) CreateChatCompletionStream(
	ctx context.Context,
	req ChatCompletionRequest,
) (*ChatCompletionStream, error) {

	req.Stream = true

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var lastErr error

	for attempt := 0; attempt < constants.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := constants.RetryDelay * time.Duration(1<<attempt)
			jitter := time.Duration(float64(delay) * (rand.Float64() * 0.2))
			time.Sleep(delay + jitter)
		}

		httpReq, err := c.newRequestWithBytes(ctx, http.MethodPost, "/chat/completions", reqBytes)
		if err != nil {
			return nil, err
		}

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			defer resp.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			lastErr = errors.NewInternalError(
				"openrouter.stream",
				fmt.Errorf("status %d: %s", resp.StatusCode, string(body)),
			)
			continue
		}

		return &ChatCompletionStream{
			reader: bufio.NewReader(resp.Body),
			body:   resp.Body,
		}, nil
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
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

func (c *Client) sendRequest(req *http.Request, v any) error {
	var lastErr error

	maxAttempts := constants.MaxRetries + 2

	for attempt := 0; attempt <= maxAttempts; attempt++ {

		if attempt > 0 {

			delay := time.Duration(math.Pow(2, float64(attempt))) * time.Second

			jitter := time.Duration(rand.Int63n(int64(1000 * time.Millisecond)))
			sleepTime := delay + jitter

			if ctxErr := req.Context().Err(); ctxErr != nil {
				return ctxErr
			}

			time.Sleep(sleepTime)
		}

		if attempt > 0 && req.GetBody != nil {
			bodyCopy, err := req.GetBody()
			if err != nil {
				return fmt.Errorf("failed to rewind request body: %w", err)
			}
			req.Body = bodyCopy
		}

		res, err := c.httpClient.Do(req)
		if err != nil {
			if strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "deadline exceeded") {
				return err
			}
			lastErr = errors.NewInternalError("openrouter.http", err)
			continue
		}

		if res.StatusCode == 429 || (res.StatusCode >= 500 && res.StatusCode != 501) {
			res.Body.Close()

			if retryAfter := res.Header.Get("Retry-After"); retryAfter != "" {
				if seconds, err := strconv.Atoi(retryAfter); err == nil {
					time.Sleep(time.Duration(seconds) * time.Second)
					continue
				}
			}

			lastErr = fmt.Errorf("server error %d", res.StatusCode)
			continue
		}

		defer res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
			return errors.NewInternalError(
				"openrouter.http",
				fmt.Errorf("status %d on %s: %s", res.StatusCode, req.URL.Path, string(body)),
			)
		}

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

type ChatCompletionStream struct {
	reader *bufio.Reader
	body   io.Closer
}

func (s *ChatCompletionStream) Recv() (*ChatCompletionResponse, error) {
	for {
		line, err := s.reader.ReadBytes('\n')
		if err != nil {
			return nil, err
		}

		line = bytes.TrimSpace(line)
		if len(line) == 0 || !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}

		data := bytes.TrimPrefix(line, []byte("data: "))
		if string(data) == "[DONE]" {
			return nil, io.EOF
		}

		var response ChatCompletionResponse
		if err := json.Unmarshal(data, &response); err != nil {
			return nil, fmt.Errorf("stream unmarshal error: %w", err)
		}
		return &response, nil
	}
}

func (s *ChatCompletionStream) Close() error {
	return s.body.Close()
}

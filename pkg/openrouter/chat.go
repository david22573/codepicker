package openrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"

	"github.com/david22573/codepicker/internal/constants"
	errs "github.com/david22573/codepicker/internal/errors"
)

func (c *Client) CreateChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	req.Stream = false
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/chat/completions", req)
	if err != nil {
		return nil, err
	}

	var resp ChatCompletionResponse
	if err := c.sendRequest(httpReq, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
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
		if len(line) == 0 {
			continue
		}

		if !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}

		data := bytes.TrimPrefix(line, []byte("data: "))
		if string(data) == "[DONE]" {
			return nil, io.EOF
		}

		var response ChatCompletionResponse
		if err := json.Unmarshal(data, &response); err != nil {
			return nil, fmt.Errorf("failed to unmarshal stream data: %w", err)
		}
		return &response, nil
	}
}

func (s *ChatCompletionStream) Close() error {
	return s.body.Close()
}

func (c *Client) CreateChatCompletionStream(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionStream, error) {
	req.Stream = true

	var lastErr error
	for attempt := 0; attempt < constants.MaxRetries; attempt++ {
		// Phase 1.3: Harden retry logic with exponential backoff + jitter
		if attempt > 0 {
			// 1. Calculate base delay using integer math (time.Duration is int64)
			// Explicitly casting 1 to time.Duration prevents the "invalid operation: shift of type float64" error
			baseDelay := constants.RetryDelay * time.Duration(1<<attempt)

			// 2. Add 0-20% jitter (requires conversion to float for percentage calc)
			jitter := time.Duration(float64(baseDelay) * (rand.Float64() * 0.2))

			sleepDuration := baseDelay + jitter

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(sleepDuration):
				// Continue after sleep
			}
		}

		httpReq, err := c.newRequest(ctx, http.MethodPost, "/chat/completions", req)
		if err != nil {
			return nil, err
		}

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = err
			// Check for context cancellation immediately (using standard lib errors)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}

			// Check if we should retry (using internal errors package)
			if errs.IsRetryable(err) {
				continue
			}
			return nil, fmt.Errorf("failed to execute stream request: %w", err)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			defer resp.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			apiErr := errs.NewAPIError(resp.StatusCode, string(body), req.Model, "/chat/completions")
			lastErr = apiErr

			var errResp ErrorResponse
			if decodeErr := json.Unmarshal(body, &errResp); decodeErr == nil && errResp.Error.Message != "" {
				lastErr = fmt.Errorf("api error (status %d): %s - %s", resp.StatusCode, errResp.Error.Type, errResp.Error.Message)
			}

			if errs.IsRetryable(apiErr) {
				continue
			}
			return nil, lastErr
		}

		return &ChatCompletionStream{
			reader: bufio.NewReader(resp.Body),
			body:   resp.Body,
		}, nil
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}


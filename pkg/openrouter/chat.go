package openrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
		// ReadLine is lower level but safer for scanning tokens than ReadBytes
		line, err := s.reader.ReadBytes('\n')
		if err != nil {
			return nil, err
		}

		line = bytes.TrimSpace(line)

		// Skip empty keep-alive lines
		if len(line) == 0 {
			continue
		}

		// Check for SSE prefix
		if !bytes.HasPrefix(line, []byte("data: ")) {
			// Optional: Handle "event: error" here if needed in future
			continue
		}

		// Strip the prefix safely
		data := bytes.TrimPrefix(line, []byte("data: "))

		// Check for stream terminator
		if string(data) == "[DONE]" {
			return nil, io.EOF
		}

		var response ChatCompletionResponse
		if err := json.Unmarshal(data, &response); err != nil {
			// Don't kill the stream on a single bad frame, just log/skip
			// or return error if critical. For now, we return error to be safe.
			return nil, fmt.Errorf("unmarshal error on stream frame: %w | data: %s", err, string(data))
		}
		return &response, nil
	}
}

func (s *ChatCompletionStream) Close() error {
	return s.body.Close()
}

func (c *Client) CreateChatCompletionStream(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionStream, error) {
	req.Stream = true

	// FIX: Marshal ONCE before the loop to save CPU on retries
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var lastErr error

	for attempt := 0; attempt < constants.MaxRetries; attempt++ {
		if attempt > 0 {
			baseDelay := constants.RetryDelay * time.Duration(1<<attempt)
			jitter := time.Duration(float64(baseDelay) * (rand.Float64() * 0.2))

			t := time.NewTimer(baseDelay + jitter)
			select {
			case <-ctx.Done():
				t.Stop()
				return nil, ctx.Err()
			case <-t.C:
				// Retry allowed
			}
		}

		// Use the optimized byte-reader helper
		httpReq, err := c.newRequestWithBytes(ctx, http.MethodPost, "/chat/completions", reqBytes)
		if err != nil {
			return nil, err
		}

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = err
			// Check if context died (user pressed Ctrl+C)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// Only retry network errors or timeouts
			if errs.IsRetryable(err) {
				continue
			}
			return nil, fmt.Errorf("stream request failed: %w", err)
		}

		// Handle API Errors (Non-200)
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			defer resp.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096)) // Increased limit

			// Try to parse the specific error message from JSON
			var errMsg string
			var errResp ErrorResponse
			if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Error.Message != "" {
				errMsg = fmt.Sprintf("%s: %s", errResp.Error.Type, errResp.Error.Message)
			} else {
				errMsg = string(body)
			}

			apiErr := errs.NewAPIError(resp.StatusCode, errMsg, req.Model, "/chat/completions")
			lastErr = apiErr

			if errs.IsRetryable(apiErr) {
				continue
			}
			return nil, lastErr
		}

		// Success!
		return &ChatCompletionStream{
			reader: bufio.NewReader(resp.Body),
			body:   resp.Body,
		}, nil
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

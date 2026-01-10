package openrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/david22573/codepicker/internal/constants"
	"github.com/david22573/codepicker/internal/errors"
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
		if attempt > 0 {
			time.Sleep(constants.RetryDelay * time.Duration(attempt))
		}

		httpReq, err := c.newRequest(ctx, http.MethodPost, "/chat/completions", req)
		if err != nil {
			return nil, err
		}

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = err
			if errors.IsRetryable(err) {
				continue
			}
			return nil, fmt.Errorf("failed to execute stream request: %w", err)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			defer resp.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			apiErr := errors.NewAPIError(resp.StatusCode, string(body), req.Model, "/chat/completions")
			lastErr = apiErr

			var errResp ErrorResponse
			if decodeErr := json.Unmarshal(body, &errResp); decodeErr == nil && errResp.Error.Message != "" {
				lastErr = fmt.Errorf("api error (status %d): %s - %s", resp.StatusCode, errResp.Error.Type, errResp.Error.Message)
			}

			if errors.IsRetryable(apiErr) {
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

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

func (c *Client) CreateChatCompletion(
	ctx context.Context,
	req ChatCompletionRequest,
) (*ChatCompletionResponse, error) {

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
			lastErr = errs.NewInternalError(
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

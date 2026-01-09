package openrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// CreateChatCompletion sends a request to the chat completions endpoint.
// This is for non-streaming requests.
func (c *Client) CreateChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	req.Stream = false // Force stream to false for this method

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

// -----------------------------------------------------------------------------
// Streaming Support
// -----------------------------------------------------------------------------

// ChatCompletionStream manages the stream of responses.
type ChatCompletionStream struct {
	reader *bufio.Reader
	body   io.Closer
}

// Recv returns the next response from the stream.
// Returns io.EOF when the stream is finished.
func (s *ChatCompletionStream) Recv() (*ChatCompletionResponse, error) {
	for {
		// Read line by line (SSE format)
		line, err := s.reader.ReadBytes('\n')
		if err != nil {
			return nil, err
		}

		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		// SSE lines start with "data: "
		if !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}

		// Remove the prefix
		data := bytes.TrimPrefix(line, []byte("data: "))

		// Check for the [DONE] signal
		if string(data) == "[DONE]" {
			return nil, io.EOF
		}

		var response ChatCompletionResponse
		if err := json.Unmarshal(data, &response); err != nil {
			return nil, fmt.Errorf("failed to unmarshal stream data: %w", err)
		}

		// OpenRouter (and OpenAI) sometimes send keep-alive comments or empty updates
		// We return valid structs, but empty ones might be filtered by the caller if desired.
		return &response, nil
	}
}

// Close closes the underlying response body.
func (s *ChatCompletionStream) Close() error {
	return s.body.Close()
}

// CreateChatCompletionStream sends a request to the chat completions endpoint with streaming enabled.
func (c *Client) CreateChatCompletionStream(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionStream, error) {
	req.Stream = true // Force stream to true

	httpReq, err := c.newRequest(ctx, http.MethodPost, "/chat/completions", req)
	if err != nil {
		return nil, err
	}

	// We use c.httpClient.Do directly here because we need to keep the body open
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute stream request: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		var errResp ErrorResponse
		if decodeErr := json.NewDecoder(resp.Body).Decode(&errResp); decodeErr == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("api error (status %d): %s - %s", resp.StatusCode, errResp.Error.Type, errResp.Error.Message)
		}
		return nil, fmt.Errorf("api error (status %d)", resp.StatusCode)
	}

	return &ChatCompletionStream{
		reader: bufio.NewReader(resp.Body),
		body:   resp.Body,
	}, nil
}

// infra/llm/openrouter.go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	domainContext "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/domain/errors"
)

const openRouterURL = "https://openrouter.ai/api/v1/chat/completions"

// --- Request/Response Structures ---

type chatRequest struct {
	Model    string           `json:"model"`
	Messages []Message        `json:"messages"`
	Tools    []ToolDefinition `json:"tools,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type OpenRouterAdapter struct {
	apiKey         string
	model          string
	client         *http.Client
	circuitBreaker *CircuitBreaker
}

func NewOpenRouterAdapter(apiKey, model string, timeout time.Duration) *OpenRouterAdapter {
	return &OpenRouterAdapter{
		apiKey:         apiKey,
		model:          model,
		client:         &http.Client{Timeout: timeout},
		circuitBreaker: NewCircuitBreaker(5, 60*time.Second),
	}
}

// ChatNative implements structured tool calling with automatic retries for transient API errors.
func (o *OpenRouterAdapter) ChatNative(ctx context.Context, messages []Message, tools []ToolDefinition) (Message, domainContext.TokenUsage, error) {
	reqBody := chatRequest{
		Model:    o.model,
		Messages: messages,
		Tools:    tools,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return Message{}, domainContext.TokenUsage{}, errors.NewSystem("llm.ChatNative", "failed to marshal request", err)
	}

	var responseMessage Message
	var usage domainContext.TokenUsage

	opErr := o.circuitBreaker.Execute(func() error {
		const maxRetries = 5
		var lastErr error

		for attempt := 0; attempt <= maxRetries; attempt++ {
			if attempt > 0 {
				// Exponential backoff: 500ms, 1s, 2s, 4s, 8s...
				backoff := time.Duration(math.Pow(2, float64(attempt-1))) * 500 * time.Millisecond
				fmt.Printf("   ⚠️ [LLM Client] Provider unstable. Retrying %d/%d in %v (Error: %v)\n", attempt, maxRetries, backoff, lastErr)
				
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return ctx.Err()
				}
			}

			req, err := http.NewRequestWithContext(ctx, "POST", openRouterURL, bytes.NewBuffer(jsonBody))
			if err != nil {
				return err
			}

			req.Header.Set("Authorization", "Bearer "+o.apiKey)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("HTTP-Referer", "https://github.com/david22573/codepicker")

			resp, err := o.client.Do(req)
			if err != nil {
				lastErr = errors.NewLLM("llm.ChatNative", err)
				continue // Network level error (e.g. DNS, connection reset)
			}

			body, err := io.ReadAll(resp.Body)
			resp.Body.Close() // Explicitly close immediately to avoid leaks during retry iterations

			if err != nil {
				lastErr = fmt.Errorf("failed to read response body: %w", err)
				continue
			}

			// Transient Server Errors & Rate Limits (502, 503, 504, 429)
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
				continue
			}

			// Hard Client Errors (400, 401, 403, 404) should fail immediately without retrying
			if resp.StatusCode != http.StatusOK {
				return errors.NewLLM("llm.ChatNative", fmt.Errorf("API client error (%d): %s", resp.StatusCode, string(body)))
			}

			var chatResp chatResponse
			if err := json.Unmarshal(body, &chatResp); err != nil {
				return errors.NewSystem("llm.ChatNative", "failed to decode API response JSON", err)
			}

			if chatResp.Error != nil {
				return errors.NewLLM("llm.ChatNative", fmt.Errorf("upstream API error: %s", chatResp.Error.Message))
			}

			if len(chatResp.Choices) > 0 {
				responseMessage = chatResp.Choices[0].Message
				usage = domainContext.TokenUsage{
					PromptTokens:     chatResp.Usage.PromptTokens,
					CompletionTokens: chatResp.Usage.CompletionTokens,
					TotalTokens:      chatResp.Usage.TotalTokens,
				}
			}

			// Success - break the retry loop
			return nil
		}

		return fmt.Errorf("exhausted all %d retries. Last error: %w", maxRetries, lastErr)
	})

	return responseMessage, usage, opErr
}

// Chat maintains backward compatibility for simple system/user prompts.
func (o *OpenRouterAdapter) Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	messages := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	msg, _, err := o.ChatNative(ctx, messages, nil)
	return msg.Content, err
}
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	domainContext "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/domain/errors"
)

const openRouterURL = "https://openrouter.ai/api/v1/chat/completions"

// OpenRouterAdapter implements agent.LLMClient using OpenRouter API.
type OpenRouterAdapter struct {
	apiKey         string
	model          string
	client         *http.Client
	circuitBreaker *CircuitBreaker
}

// NewOpenRouterAdapter initializes the client with config-defined timeout and a standard circuit breaker.
func NewOpenRouterAdapter(apiKey, model string, timeout time.Duration) *OpenRouterAdapter {
	return &OpenRouterAdapter{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{
			Timeout: timeout,
		},
		// Initialize Circuit Breaker: 5 failures allowed, 1 minute reset timeout
		circuitBreaker: NewCircuitBreaker(5, 60*time.Second),
	}
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message message `json:"message"`
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

// Chat implements the basic LLMClient interface.
func (o *OpenRouterAdapter) Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	resp, _, err := o.ChatWithUsage(ctx, systemPrompt, userPrompt)
	return resp, err
}

// ChatWithUsage implements the extended interface for cost tracking.
func (o *OpenRouterAdapter) ChatWithUsage(ctx context.Context, systemPrompt, userPrompt string) (string, domainContext.TokenUsage, error) {
	reqBody := chatRequest{
		Model: o.model,
		Messages: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", domainContext.TokenUsage{}, errors.NewSystem("llm.Chat", "failed to marshal request", err)
	}

	var responseContent string
	var usage domainContext.TokenUsage

	// Wrap the network call in the Circuit Breaker
	opErr := o.circuitBreaker.Execute(func() error {
		req, err := http.NewRequestWithContext(ctx, "POST", openRouterURL, bytes.NewBuffer(jsonBody))
		if err != nil {
			return errors.NewSystem("llm.Chat", "failed to create request", err)
		}

		req.Header.Set("Authorization", "Bearer "+o.apiKey)
		req.Header.Set("Content-Type", "application/json")
		// Optional: Site URL and Title for OpenRouter rankings
		req.Header.Set("HTTP-Referer", "https://github.com/david22573/codepicker")
		req.Header.Set("X-Title", "CodePicker")

		resp, err := o.client.Do(req)
		if err != nil {
			return errors.NewLLM("llm.Chat", fmt.Errorf("network request failed: %w", err))
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return errors.NewSystem("llm.Chat", "failed to read response body", err)
		}

		if resp.StatusCode != http.StatusOK {
			return errors.NewLLM("llm.Chat", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body)))
		}

		var chatResp chatResponse
		if err := json.Unmarshal(body, &chatResp); err != nil {
			return errors.NewSystem("llm.Chat", "failed to parse response json", err)
		}

		if chatResp.Error != nil {
			return errors.NewLLM("llm.Chat", fmt.Errorf("api returned error: %s", chatResp.Error.Message))
		}

		if len(chatResp.Choices) == 0 {
			return errors.NewLLM("llm.Chat", fmt.Errorf("received empty choices from API"))
		}

		responseContent = chatResp.Choices[0].Message.Content

		// Capture usage stats
		usage = domainContext.TokenUsage{
			PromptTokens:     chatResp.Usage.PromptTokens,
			CompletionTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:      chatResp.Usage.TotalTokens,
		}

		return nil
	})

	if opErr != nil {
		return "", domainContext.TokenUsage{}, opErr
	}

	return responseContent, usage, nil
}

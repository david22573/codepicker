package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/david22573/codepicker/domain/errors"
)

const openRouterURL = "https://openrouter.ai/api/v1/chat/completions"

// OpenRouterAdapter implements domain.agent.LLMClient
type OpenRouterAdapter struct {
	apiKey string
	model  string
	client *http.Client
}

// NewOpenRouterAdapter creates a new client instance
func NewOpenRouterAdapter(apiKey, model string) *OpenRouterAdapter {
	return &OpenRouterAdapter{
		apiKey: apiKey,
		model:  model,
		// Increased timeout to 120s because "thinking" models (like Nemotron/Llama-3)
		// take longer to generate the first token.
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

// Chat fulfills the domain.agent.LLMClient interface with retries
func (c *OpenRouterAdapter) Chat(ctx context.Context, systemPrompt, userMsg string) (string, error) {
	// Simple retry logic (3 attempts)
	maxRetries := 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		response, err := c.doChat(ctx, systemPrompt, userMsg)
		if err == nil {
			return response, nil
		}

		// If it's a context cancellation (user pressed Ctrl+C), stop immediately
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		lastErr = err
		// Exponential backoff: 1s, 2s, 4s
		time.Sleep(time.Duration(1<<i) * time.Second)
	}

	return "", lastErr
}

func (c *OpenRouterAdapter) doChat(ctx context.Context, systemPrompt, userMsg string) (string, error) {
	// Prepare the request payload
	reqBody := map[string]interface{}{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userMsg},
		},
		"temperature": 0.1, // Slight temp often helps avoid "stuck" loops better than 0.0
		// Optional: Add OpenRouter specific provider preferences if needed
		// "provider": map[string]string{"order": "Liquid,DeepInfra"},
	}

	// Specific tweak for models that support/require "reasoning" parameter
	if strings.Contains(c.model, "nemotron") || strings.Contains(c.model, "thinking") {
		reqBody["include_reasoning"] = true
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", errors.NewSystem("llm.Chat", "failed to marshal request", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", openRouterURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", errors.NewSystem("llm.Chat", "failed to create request", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://github.com/david22573/codepicker")
	req.Header.Set("X-Title", "CodePicker")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", errors.NewLLM("llm.Chat", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// IMPROVEMENT: Handle non-200 status codes gracefully
	if resp.StatusCode != http.StatusOK {
		return "", errors.NewLLM("llm.Chat", fmt.Errorf("API error %d: %s", resp.StatusCode, string(body)))
	}

	// IMPROVEMENT: Handle empty bodies which cause "unexpected end of JSON"
	if len(body) == 0 {
		return "", errors.NewLLM("llm.Chat", fmt.Errorf("received empty response body from API"))
	}

	// Parse the response
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"` // Useful for debugging
		} `json:"choices"`
		Error *struct { // Capture API-level errors that return 200 OK (rare but possible)
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		// Log the raw body for debugging if parsing fails
		return "", errors.NewSystem("llm.Chat", fmt.Sprintf("failed to parse response: %s", string(body)), err)
	}

	if result.Error != nil {
		return "", errors.NewLLM("llm.Chat", fmt.Errorf("provider error: %s", result.Error.Message))
	}

	if len(result.Choices) == 0 {
		return "", errors.NewLLM("llm.Chat", fmt.Errorf("empty response choices from API"))
	}

	return result.Choices[0].Message.Content, nil
}

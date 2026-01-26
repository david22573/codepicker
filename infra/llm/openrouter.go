package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// Chat fulfills the domain.agent.LLMClient interface
func (c *OpenRouterAdapter) Chat(ctx context.Context, systemPrompt, userMsg string) (string, error) {
	// Prepare the request payload
	reqBody := map[string]interface{}{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userMsg},
		},
		"temperature": 0.0, // Deterministic for coding tasks
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", errors.NewSystem("llm.Chat", "failed to marshal request", err)
	}

	// Create the HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", openRouterURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", errors.NewSystem("llm.Chat", "failed to create request", err)
	}

	// Set necessary headers
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://github.com/david22573/codepicker")
	req.Header.Set("X-Title", "CodePicker")

	// Execute the request
	resp, err := c.client.Do(req)
	if err != nil {
		return "", errors.NewLLM("llm.Chat", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", errors.NewLLM("llm.Chat", fmt.Errorf("API error %d: %s", resp.StatusCode, string(body)))
	}

	// Parse the response
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", errors.NewSystem("llm.Chat", "failed to parse response", err)
	}

	if len(result.Choices) == 0 {
		return "", errors.NewLLM("llm.Chat", fmt.Errorf("empty response choices from API"))
	}

	return result.Choices[0].Message.Content, nil
}

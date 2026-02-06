package llm

import (
	"bytes"
	"context" // Standard library context (REQUIRED)
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	domainContext "github.com/david22573/codepicker/domain/context" // Aliased to prevent collision
	"github.com/david22573/codepicker/domain/errors"
)

const openRouterURL = "https://openrouter.ai/api/v1/chat/completions"

// OpenRouterAdapter implements domain.agent.LLMClient with reliability features.
type OpenRouterAdapter struct {
	apiKey         string
	model          string
	client         *http.Client
	circuitBreaker *CircuitBreaker
}

// NewOpenRouterAdapter creates a client with a circuit breaker configuration.
func NewOpenRouterAdapter(apiKey, model string) *OpenRouterAdapter {
	return &OpenRouterAdapter{
		apiKey: apiKey,
		model:  model,
		// Increased timeout to 120s for "reasoning" models that take longer to start
		client: &http.Client{Timeout: 120 * time.Second},
		// Circuit breaker trips after 5 failures, resets after 1 minute of probation
		circuitBreaker: NewCircuitBreaker(5, 1*time.Minute),
	}
}

// ChatWithUsage returns the response text along with token metrics for cost tracking.
func (c *OpenRouterAdapter) ChatWithUsage(ctx context.Context, systemPrompt, userMsg string) (string, domainContext.TokenUsage, error) {
	var result string
	var usage domainContext.TokenUsage

	// Execute the request inside the circuit breaker
	err := c.circuitBreaker.Execute(func() error {
		// Use a closure to capture the return values
		resp, use, err := c.doChat(ctx, systemPrompt, userMsg)
		if err != nil {
			return err
		}
		result = resp
		usage = use
		return nil
	})

	return result, usage, err
}

// Chat fulfills the standard interface, discarding usage metrics.
func (c *OpenRouterAdapter) Chat(ctx context.Context, systemPrompt, userMsg string) (string, error) {
	resp, _, err := c.ChatWithUsage(ctx, systemPrompt, userMsg)
	return resp, err
}

// doChat performs the actual HTTP request logic.
func (c *OpenRouterAdapter) doChat(ctx context.Context, systemPrompt, userMsg string) (string, domainContext.TokenUsage, error) {
	reqBody := map[string]interface{}{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userMsg},
		},
		"temperature": 0.1, // Low temp for deterministic code generation
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", domainContext.TokenUsage{}, errors.NewSystem("llm.Chat", "failed to marshal request", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", openRouterURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", domainContext.TokenUsage{}, errors.NewSystem("llm.Chat", "failed to create request", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://github.com/david22573/codepicker")
	req.Header.Set("X-Title", "CodePicker")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", domainContext.TokenUsage{}, errors.NewLLM("llm.Chat", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Handle non-200 status codes
	if resp.StatusCode != http.StatusOK {
		return "", domainContext.TokenUsage{}, errors.NewLLM("llm.Chat", fmt.Errorf("API error %d: %s", resp.StatusCode, string(body)))
	}

	// Parse the response
	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage domainContext.TokenUsage `json:"usage"` // Capture token usage
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", domainContext.TokenUsage{}, errors.NewSystem("llm.Chat", fmt.Sprintf("failed to parse response: %s", string(body)), err)
	}

	if apiResp.Error != nil {
		return "", domainContext.TokenUsage{}, errors.NewLLM("llm.Chat", fmt.Errorf("provider error: %s", apiResp.Error.Message))
	}

	if len(apiResp.Choices) == 0 {
		return "", domainContext.TokenUsage{}, errors.NewLLM("llm.Chat", fmt.Errorf("received empty choices from API"))
	}

	return apiResp.Choices[0].Message.Content, apiResp.Usage, nil
}

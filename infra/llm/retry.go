package llm

import (
	"context"
	"fmt"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/errors"
)

// ValidatorFunc defines a logic check for the LLM response.
type ValidatorFunc func(response string) error

// RetryClient provides an auto-correcting wrapper for LLM interactions.
type RetryClient struct {
	client     agent.LLMClient
	maxRetries int
}

func NewRetryClient(client agent.LLMClient, retries int) *RetryClient {
	return &RetryClient{
		client:     client,
		maxRetries: retries,
	}
}

// ChatWithRetry executes a prompt and validates the output, retrying on failure.
func (c *RetryClient) ChatWithRetry(ctx context.Context, system, user string, validate ValidatorFunc) (string, error) {
	var lastResponse string
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		// 1. Initial Call or Repair Call
		resp, err := c.client.Chat(ctx, system, user)
		if err != nil {
			return "", err
		}

		lastResponse = resp

		// 2. Run Validation Logic
		if lastErr = validate(resp); lastErr == nil {
			return resp, nil // Success!
		}

		// 3. Construct Repair Prompt
		// Feed the exact error back to the model for correction.
		user = fmt.Sprintf("%s\n\n⚠️ PREVIOUS ATTEMPT FAILED PARSING:\n%s\n\nERROR: %v\n\nPlease correct the output and try again.",
			user, lastResponse, lastErr)

		fmt.Printf("🔄 [RETRY] Attempt %d/%d: Correcting LLM output...\n", attempt+1, c.maxRetries)
	}

	return lastResponse, errors.NewSystem("llm.Retry", "max retries exceeded", lastErr)
}

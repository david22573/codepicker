package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/errors"
)

// StructuredLLM defines the interface for type-safe LLM interactions.
type StructuredLLM interface {
	ChatJSON(ctx context.Context, system string, user string, out interface{}) error
}

// StructuredAdapter wraps a raw LLMClient to add JSON enforcement and repair capabilities.
type StructuredAdapter struct {
	client     agent.LLMClient
	maxRetries int
}

// NewStructuredAdapter creates a wrapper that enforces JSON output schemas.
func NewStructuredAdapter(client agent.LLMClient) *StructuredAdapter {
	return &StructuredAdapter{
		client:     client,
		maxRetries: 2, // Allow 2 repair attempts before failing
	}
}

// ChatJSON sends a prompt and unmarshals the result into the target struct.
// It includes logic to strip Markdown formatting and retry if JSON is malformed.
func (s *StructuredAdapter) ChatJSON(ctx context.Context, system string, user string, out interface{}) error {
	// 1. Append instructions to force JSON mode (redundancy helps weaker models)
	jsonSystem := fmt.Sprintf("%s\n\nIMPORTANT: Return ONLY valid raw JSON. Do not use Markdown code blocks.", system)

	var lastErr error
	currentPrompt := user

	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		// 2. Call the raw LLM
		rawResponse, err := s.client.Chat(ctx, jsonSystem, currentPrompt)
		if err != nil {
			return err
		}

		// 3. Clean and Parse
		cleaned := s.cleanJSON(rawResponse)
		if err := json.Unmarshal([]byte(cleaned), out); err == nil {
			return nil // Success
		} else {
			lastErr = err
		}

		// 4. Repair Strategy: Feed the error back to the model
		// This is the "Self-Healing" loop
		currentPrompt = fmt.Sprintf("Your previous response was invalid JSON.\nError: %v\n\nOriginal Request: %s\n\nPlease fix the JSON syntax and return ONLY the JSON.", lastErr, user)
	}

	return errors.NewSystem("llm.ChatJSON", fmt.Sprintf("failed to parse JSON after %d attempts", s.maxRetries+1), lastErr)
}

// cleanJSON attempts to extract the JSON payload from potential Markdown wrappers.
func (s *StructuredAdapter) cleanJSON(raw string) string {
	raw = strings.TrimSpace(raw)

	// Strip standard Markdown code blocks
	if strings.HasPrefix(raw, "```json") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimSuffix(raw, "```")
	} else if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
	}

	return strings.TrimSpace(raw)
}

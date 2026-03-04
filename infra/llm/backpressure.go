package llm

import (
	"context"
	"fmt"
	"time"

	domainContext "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/domain/errors"
)

// BackpressureAdapter wraps an LLM Provider to limit maximum concurrent requests.
type BackpressureAdapter struct {
	underlying Provider
	sem        chan struct{}
	timeout    time.Duration
}

// NewBackpressureAdapter initializes a semaphore-bounded wrapper for LLM calls.
func NewBackpressureAdapter(underlying Provider, maxConcurrent int, timeout time.Duration) *BackpressureAdapter {
	return &BackpressureAdapter{
		underlying: underlying,
		sem:        make(chan struct{}, maxConcurrent),
		timeout:    timeout,
	}
}

func (b *BackpressureAdapter) ChatNative(ctx context.Context, messages []Message, tools []ToolDefinition) (Message, domainContext.TokenUsage, error) {
	select {
	case b.sem <- struct{}{}:
		defer func() { <-b.sem }()
		return b.underlying.ChatNative(ctx, messages, tools)
	case <-time.After(b.timeout):
		return Message{}, domainContext.TokenUsage{}, errors.NewExecutionError(
			errors.CodeSystem,
			"BackpressureAdapter.ChatNative",
			"LLM request rejected: too many concurrent requests (backpressure threshold reached)",
			nil,
		)
	case <-ctx.Done():
		return Message{}, domainContext.TokenUsage{}, ctx.Err()
	}
}

func (b *BackpressureAdapter) Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	select {
	case b.sem <- struct{}{}:
		defer func() { <-b.sem }()
		return b.underlying.Chat(ctx, systemPrompt, userPrompt)
	case <-time.After(b.timeout):
		return "", fmt.Errorf("LLM request rejected: backpressure threshold reached")
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

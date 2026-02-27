package llm

import (
	"context"

	domainContext "github.com/david22573/codepicker/domain/context"
)

type LLMRecorder interface {
	RecordLLM(msgs []Message, resp Message, usage domainContext.TokenUsage, err error)
}

// TraceAdapter wraps a Provider to intercept and record its interactions.
type TraceAdapter struct {
	underlying Provider
	recorder   LLMRecorder
}

func NewTraceAdapter(underlying Provider, recorder LLMRecorder) *TraceAdapter {
	return &TraceAdapter{underlying: underlying, recorder: recorder}
}

func (t *TraceAdapter) ChatNative(ctx context.Context, messages []Message, tools []ToolDefinition) (Message, domainContext.TokenUsage, error) {
	resp, usage, err := t.underlying.ChatNative(ctx, messages, tools)
	t.recorder.RecordLLM(messages, resp, usage, err)
	return resp, usage, err
}

func (t *TraceAdapter) Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	resp, err := t.underlying.Chat(ctx, systemPrompt, userPrompt)
	
	// Synthesize a structured payload for the legacy text interface
	msgs := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	t.recorder.RecordLLM(msgs, Message{Role: "assistant", Content: resp}, domainContext.TokenUsage{}, err)
	
	return resp, err
}
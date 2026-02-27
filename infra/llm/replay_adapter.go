package llm

import (
	"context"

	domainContext "github.com/david22573/codepicker/domain/context"
)

type LLMReplayer interface {
	NextLLM() (Message, domainContext.TokenUsage, error)
}

// ReplayAdapter implements Provider by strictly popping responses from a transcript.
type ReplayAdapter struct {
	replayer LLMReplayer
}

func NewReplayAdapter(replayer LLMReplayer) *ReplayAdapter {
	return &ReplayAdapter{replayer: replayer}
}

func (r *ReplayAdapter) ChatNative(ctx context.Context, messages []Message, tools []ToolDefinition) (Message, domainContext.TokenUsage, error) {
	return r.replayer.NextLLM()
}

func (r *ReplayAdapter) Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	msg, _, err := r.replayer.NextLLM()
	return msg.Content, err
}
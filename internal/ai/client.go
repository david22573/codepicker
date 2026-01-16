package ai

import (
	"context"

	"github.com/david22573/codepicker/pkg/openrouter"
)

// AIClient abstracts the OpenRouter client for testing and flexibility
type AIClient interface {
	CreateChatCompletion(ctx context.Context, req openrouter.ChatCompletionRequest) (*openrouter.ChatCompletionResponse, error)
	CreateChatCompletionStream(ctx context.Context, req openrouter.ChatCompletionRequest) (*openrouter.ChatCompletionStream, error)
	GetModelInfo(ctx context.Context, modelID string) (*openrouter.Model, error)
}

// Verify openrouter.Client implements AIClient
var _ AIClient = (*openrouter.Client)(nil)

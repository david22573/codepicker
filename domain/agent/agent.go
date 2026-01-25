package agent

import (
	"context"
)

// Agent represents an autonomous entity capable of performing a task
type Agent interface {
	Name() string
	Run(ctx context.Context, input string) (string, error)
}

// Tool represents a capability the agent can use
// Note: We return interface{} or specific value objects to avoid coupling to external JSON/HTTP libs
type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, args string) (string, error)
}

// Policy defines the rules of engagement
type Policy interface {
	CanExecute(toolName string) bool
	CanWrite(path string) bool
	MaxSteps() int
}

// LLMClient is a port for the AI provider (implemented by infra/llm)
type LLMClient interface {
	Chat(ctx context.Context, systemPrompt string, userMessage string) (string, error)
}

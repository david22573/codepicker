package agent

import (
	"context"
)

// Agent represents the high-level autonomous entity
type Agent interface {
	Name() string
	// Run executes a task and returns the final summary/result
	Run(ctx context.Context, input string) (string, error)
}

// Tool represents a capability the agent can use
type Tool interface {
	Name() string
	Description() string
	// Execute runs the tool logic.
	// We use string for input/output to keep the domain generic (JSON agnostic)
	Execute(ctx context.Context, args string) (string, error)
}

// Policy defines the security rules
type Policy interface {
	// CanExecute checks if a specific tool execution is allowed
	CanExecute(toolName string, args string) (bool, string)
	// Mode returns the current strictness level (e.g. "interactive", "batch")
	Mode() string
}

// LLMClient abstracts the AI provider (OpenRouter, OpenAI, etc.)
// It simplifies the interaction to just "System + User -> Text Response"
// Complex history management is handled by the application layer, not the domain interface.
type LLMClient interface {
	Chat(ctx context.Context, systemPrompt string, userMessage string) (string, error)
}

// Repository defines how we save/load executions (Port)
type Repository interface {
	SaveExecution(ctx context.Context, exec *Execution) error
	GetExecution(ctx context.Context, id string) (*Execution, error)
}

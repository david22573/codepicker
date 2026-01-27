package agent

import (
	"context"
	"time"

	"github.com/david22573/codepicker/domain/task"
)

// Agent represents the high-level autonomous entity
type Agent interface {
	Name() string
	Run(ctx context.Context, input string) (string, error)
}

// Tool represents a capability the agent can use
type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, args string) (string, error)
}

// Policy defines the security rules
type Policy interface {
	CanExecute(toolName string, args string) (bool, string)
	Mode() string
}

// LLMClient abstracts the AI provider
type LLMClient interface {
	Chat(ctx context.Context, systemPrompt string, userMessage string) (string, error)
}

// ExecutionSummary is a lightweight view for listing
type ExecutionSummary struct {
	ID        string
	PlanID    string
	Status    task.Status
	StartTime time.Time
}

// Repository defines how we save/load executions
type Repository interface {
	SaveExecution(ctx context.Context, exec *Execution) error
	GetExecution(ctx context.Context, id string) (*Execution, error)
	// New method for Phase 7
	ListExecutions(ctx context.Context, limit int) ([]ExecutionSummary, error)
}

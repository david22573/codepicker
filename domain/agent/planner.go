package agent

import (
	"context"

	contextDomain "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/domain/task"
)

// Planner defines the interface for generating and optimizing execution plans.
type Planner interface {
	// CreatePlan generates a new plan based on the structured LLM context.
	CreatePlan(ctx context.Context, taskInput string, llmCtx *contextDomain.LLMContext) (*task.Plan, error)

	// OptimizePlan uses AI to refine an existing plan based on feedback.
	OptimizePlan(ctx context.Context, plan *task.Plan, feedback string) (*task.Plan, error)
}

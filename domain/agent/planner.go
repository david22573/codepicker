package agent

import (
	"context"

	"github.com/david22573/codepicker/domain/task"
)

// Planner defines the interface for generating and optimizing execution plans. [cite: 509]
type Planner interface {
	// CreatePlan generates a new plan based on the task, project map, and file context.
	CreatePlan(ctx context.Context, taskInput, fileContext, primer string) (*task.Plan, error)

	// OptimizePlan uses AI to refine an existing plan based on feedback. [cite: 511]
	OptimizePlan(ctx context.Context, plan *task.Plan, feedback string) (*task.Plan, error)
}

package agent

import (
	"context"
	"fmt"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/task"
)

type PlanExecutor struct {
	worker agent.Agent
	repo   agent.Repository
}

func NewPlanExecutor(worker agent.Agent, repo agent.Repository) *PlanExecutor {
	return &PlanExecutor{
		worker: worker,
		repo:   repo,
	}
}

// Execute iterates through the plan steps and runs the worker for each
func (e *PlanExecutor) Execute(ctx context.Context, plan *task.Plan) error {
	plan.Status = task.StatusRunning
	// In a real app, we'd save the plan state to DB here

	fmt.Printf("\n📋 Plan Generated: %s\n", plan.Reasoning)
	fmt.Printf("steps: %d\n", len(plan.Steps))

	for _, step := range plan.Steps {
		fmt.Printf("\n▶️  Step %d: %s\n", step.ID, step.Description)

		// 1. Prepare Worker Context
		// We augment the instruction with the file context to ensure the worker focuses
		workerInput := fmt.Sprintf("%s\n\nFocus on these files: %v", step.Instruction, step.Files)

		// 2. Execute Worker
		// The worker (ReActAgent) runs its own loop here
		result, err := e.worker.Run(ctx, workerInput)

		if err != nil {
			fmt.Printf("❌ Step %d Failed: %v\n", step.ID, err)
			plan.MarkStepFailed(step.ID, err)
			plan.Status = task.StatusFailed
			return err
		}

		// 3. Record Success
		fmt.Printf("✅ Step %d Complete.\n", step.ID)
		plan.MarkStepComplete(step.ID, result)
	}

	plan.Status = task.StatusCompleted
	return nil
}

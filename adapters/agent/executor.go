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

func (e *PlanExecutor) Execute(ctx context.Context, plan *task.Plan) error {
	plan.Status = task.StatusRunning
	_ = e.repo.SavePlan(ctx, plan)

	fmt.Println("\n---------------------------------------------------")
	fmt.Printf("📋 [PLANNER] Plan ID: %s (%d steps)\n", plan.ID, len(plan.Steps))
	fmt.Printf("🎯 [PLANNER] Goal: %s\n", plan.OriginalTask)
	fmt.Println("---------------------------------------------------")

	for _, step := range plan.Steps {
		if step.Status == task.StatusCompleted {
			fmt.Printf("⏭️  [PLANNER] Skipping completed step %d\n", step.ID)
			continue
		}

		fmt.Printf("\n🔹 [PLANNER] STEP %d/%d: %s\n", step.ID, len(plan.Steps), step.Description)

		workerInput := fmt.Sprintf("%s\n\nFocus on these files: %v", step.Instruction, step.Files)

		// The worker now handles its own verbose logging (Agent/System output)
		result, err := e.worker.Run(ctx, workerInput)

		if err != nil {
			fmt.Printf("\n❌ [PLANNER] Step %d Failed.\n", step.ID)
			plan.MarkStepFailed(step.ID, err)
			plan.Status = task.StatusFailed
			_ = e.repo.SavePlan(ctx, plan)
			return err
		}

		fmt.Printf("\n✨ [PLANNER] Step %d Complete.\n", step.ID)
		plan.MarkStepComplete(step.ID, result)
		_ = e.repo.SavePlan(ctx, plan)
	}

	plan.Status = task.StatusCompleted
	return e.repo.SavePlan(ctx, plan)
}

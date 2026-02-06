package agent

import (
	"context"
	"fmt"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/task"
	"github.com/david22573/codepicker/infra/fs"
)

type PlanExecutor struct {
	worker       agent.Agent
	repo         agent.Repository
	workspaceMgr *fs.WorkspaceManager
}

// NewPlanExecutor initializes the executor with a workspace manager for transactions.
func NewPlanExecutor(worker agent.Agent, repo agent.Repository, ws *fs.WorkspaceManager) *PlanExecutor {
	return &PlanExecutor{
		worker:       worker,
		repo:         repo,
		workspaceMgr: ws,
	}
}

func (e *PlanExecutor) Execute(ctx context.Context, plan *task.Plan) error {
	txn, err := e.workspaceMgr.BeginTransaction()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	defer func() {
		// Updated to access the exported 'Committed' field
		if !txn.Committed {
			fmt.Println("⚠️  [SYSTEM] Error detected. Rolling back changes...")
			txn.Rollback()
		}
	}()

	plan.Status = task.StatusRunning
	_ = e.repo.SavePlan(ctx, plan)

	for _, step := range plan.Steps {
		if step.Status == task.StatusCompleted {
			continue
		}

		fmt.Printf("\n🔹 [PLANNER] STEP %d/%d: %s\n", step.ID, len(plan.Steps), step.Description)

		// Before execution, backup files
		for _, file := range step.Files {
			txn.BackupFile(file)
		}

		workerInput := fmt.Sprintf("%s\n\nFocus on these files: %v", step.Instruction, step.Files)
		result, err := e.worker.Run(ctx, workerInput)

		if err != nil {
			plan.MarkStepFailed(step.ID, err)
			plan.Status = task.StatusFailed
			_ = e.repo.SavePlan(ctx, plan)
			return err // Triggers rollback
		}

		plan.MarkStepComplete(step.ID, result)
		_ = e.repo.SavePlan(ctx, plan)
	}

	txn.Commit()
	plan.Status = task.StatusCompleted
	return e.repo.SavePlan(ctx, plan)
}

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
	// Start Transaction
	txn, err := e.workspaceMgr.BeginTransaction()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	// FIX: Robust rollback defer
	// If Execute returns before txn.Commit(), this will automatically rollback changes.
	defer func() {
		if !txn.Committed {
			fmt.Println("⚠️  [SYSTEM] Plan execution failed or interrupted. Rolling back filesystem changes...")
			if err := txn.Rollback(); err != nil {
				fmt.Printf("❌ [CRITICAL] Rollback failed: %v\n", err)
			} else {
				fmt.Println("✅ [SYSTEM] Rollback complete. Filesystem restored.")
			}
		}
	}()

	plan.Status = task.StatusRunning
	_ = e.repo.SavePlan(ctx, plan)

	for _, step := range plan.Steps {
		if step.Status == task.StatusCompleted {
			continue
		}

		fmt.Printf("\n🔹 [PLANNER] STEP %d/%d: %s\n", step.ID, len(plan.Steps), step.Description)

		// Backup relevant files before the agent touches them
		// FIX: We now check errors here. If backup fails, we abort immediately to be safe.
		for _, file := range step.Files {
			if err := txn.BackupFile(file); err != nil {
				plan.Status = task.StatusFailed
				_ = e.repo.SavePlan(ctx, plan)
				return fmt.Errorf("backup failed for %s: %w", file, err)
			}
		}

		workerInput := fmt.Sprintf("%s\n\nFocus on these files: %v", step.Instruction, step.Files)
		result, err := e.worker.Run(ctx, workerInput)

		if err != nil {
			plan.MarkStepFailed(step.ID, err)
			plan.Status = task.StatusFailed
			_ = e.repo.SavePlan(ctx, plan)
			return err // Triggers rollback via defer
		}

		plan.MarkStepComplete(step.ID, result)
		_ = e.repo.SavePlan(ctx, plan)
	}

	// Success! Commit the transaction.
	txn.Commit()
	plan.Status = task.StatusCompleted
	return e.repo.SavePlan(ctx, plan)
}

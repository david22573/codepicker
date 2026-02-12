package agent

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/task"
	infraCtx "github.com/david22573/codepicker/infra/context"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/fatih/color"
)

type PlanExecutor struct {
	worker       agent.Agent
	repo         agent.Repository
	workspaceMgr *fs.WorkspaceManager
	autoConfirm  bool
}

// NewPlanExecutor initializes the executor with a workspace manager for transactions.
func NewPlanExecutor(worker agent.Agent, repo agent.Repository, ws *fs.WorkspaceManager) *PlanExecutor {
	return &PlanExecutor{
		worker:       worker,
		repo:         repo,
		workspaceMgr: ws,
		autoConfirm:  false, // Default to safe mode (require confirmation)
	}
}

// SetAutoConfirm allows bypassing the interactive prompt (e.g. for CI or -y flag)
func (e *PlanExecutor) SetAutoConfirm(auto bool) {
	e.autoConfirm = auto
}

func (e *PlanExecutor) Execute(ctx context.Context, plan *task.Plan) error {
	// Start Transaction
	txn, err := e.workspaceMgr.BeginTransaction()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	// 1. CRITICAL: Register robust rollback defer
	// If Execute returns before txn.Commit() (due to error or panic), this ensures rollback.
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

	// 2. CRITICAL: Active Context Watcher
	// This monitors for Ctrl+C (cancellation) in the background.
	// If detected, it forces a rollback immediately, even if the main thread is busy.
	infraCtx.WatchContext(ctx, func() {
		if !txn.Committed {
			fmt.Println("\n🛑 [INTERRUPT] Cancellation detected during execution.")
			_ = txn.Rollback()
		}
	})

	plan.Status = task.StatusRunning
	_ = e.repo.SavePlan(ctx, plan)

	// Interactive Input Reader
	reader := bufio.NewReader(os.Stdin)

	for _, step := range plan.Steps {
		if step.Status == task.StatusCompleted {
			continue
		}

		// 3. CRITICAL: Check for cancellation before starting new work
		if infraCtx.IsCancelled(ctx) {
			return fmt.Errorf("execution cancelled by user")
		}

		fmt.Printf("\n🔹 [PLANNER] STEP %d/%d: %s\n", step.ID, len(plan.Steps), step.Description)

		// 4. Interactive Approval Logic (Phase 3 Feature)
		if !e.autoConfirm {
			fmt.Println(color.HiBlackString("   Files: %v", step.Files))
			fmt.Println(color.HiBlackString("   Instruction: %s", step.Instruction))
			fmt.Print(color.YellowString("   👉 Execute this step? [y]es / [s]kip / [q]uit: "))

			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(strings.ToLower(input))

			switch input {
			case "q", "quit":
				return fmt.Errorf("execution aborted by user")
			case "s", "skip":
				fmt.Println("   ⏭️  Skipping step...")
				plan.MarkStepComplete(step.ID, "Skipped by user request")
				_ = e.repo.SavePlan(ctx, plan)
				continue
			case "y", "yes", "":
				// Proceed
			default:
				// Assume yes if they type random stuff, or strictly enforce?
				// For safety, let's treat unknown as "proceed" only if strict mode isn't desired,
				// but explicitly asking requires explicit 'y' usually.
				// For now, loop if unclear? No, let's default to run for UX speed unless 's' or 'q'.
			}
		}

		// Backup relevant files before the agent touches them
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

	// Success!
	// Commit the transaction.
	txn.Commit()
	plan.Status = task.StatusCompleted
	return e.repo.SavePlan(ctx, plan)
}

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
	shadowMgr    *fs.ShadowManager
	autoConfirm  bool
}

func NewPlanExecutor(worker agent.Agent, repo agent.Repository, ws *fs.WorkspaceManager, shadow *fs.ShadowManager) *PlanExecutor {
	return &PlanExecutor{
		worker:       worker,
		repo:         repo,
		workspaceMgr: ws,
		shadowMgr:    shadow,
		autoConfirm:  false,
	}
}

func (e *PlanExecutor) SetAutoConfirm(auto bool) {
	e.autoConfirm = auto
}

func (e *PlanExecutor) Execute(ctx context.Context, plan *task.Plan) error {
	// 1. Begin Transaction
	txn, err := e.workspaceMgr.BeginTransaction()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	// 2. Link Shadow to Transaction (The Architectural Fix)
	txn.AttachShadow(e.shadowMgr)

	// Ensure we start with a clean slate
	_ = e.shadowMgr.Clear()

	defer func() {
		if !txn.Committed {
			fmt.Println("⚠️  Rolling back changes (restoring files + clearing shadow)...")
			_ = txn.Rollback()
		}
	}()

	infraCtx.WatchContext(ctx, func() {
		if !txn.Committed {
			_ = txn.Rollback()
		}
	})

	plan.Status = task.StatusRunning
	_ = e.repo.SavePlan(ctx, plan)
	reader := bufio.NewReader(os.Stdin)

	for _, step := range plan.Steps {
		if step.Status == task.StatusCompleted {
			continue
		}

		if infraCtx.IsCancelled(ctx) {
			return fmt.Errorf("cancelled")
		}

		fmt.Printf("\n🔹 STEP %d: %s\n", step.ID, step.Description)

		if !e.autoConfirm {
			fmt.Println(color.HiBlackString("   Instruction: %s", step.Instruction))
			fmt.Print(color.YellowString("   👉 Execute? [y/s/q]: "))
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(strings.ToLower(input))
			if input == "q" {
				return fmt.Errorf("aborted")
			}
			if input == "s" {
				continue
			}
		}

		// Pre-Backup original files listed in the plan (safety net)
		// Pre-Backup original files listed in the plan (safety net)
		for _, file := range step.Files {
			_ = txn.BackupFile(file)
		}

		// 🔥 IMPROVED: Wrap the instruction with execution-focused context
		// This reinforces that the agent must USE TOOLS, not just describe changes
		workerInput := fmt.Sprintf(`EXECUTION MODE - You MUST use tools to complete this task.

INSTRUCTION: %s

TARGET FILES: %v

MANDATORY EXECUTION REQUIREMENTS:
1. You MUST call read_file on each target file to see the current state
2. You MUST call write_file with the COMPLETE modified file content
3. You MUST NOT just describe what should be changed
4. Providing code snippets without calling write_file = FAILURE
5. Only respond "Final Answer:" after you have actually used write_file

EXECUTION PATTERN:
→ Call read_file for each target file
→ Analyze what needs to change based on the instruction
→ Call write_file with the complete new file content
→ Respond: "Final Answer: [description of what you actually did]"

Execute the instruction NOW using your tools.`, step.Instruction, step.Files)

		// Run Agent
		result, err := e.worker.Run(ctx, workerInput)
		if err != nil {
			plan.MarkStepFailed(step.ID, err)
			_ = e.repo.SavePlan(ctx, plan)
			return err
		}

		plan.MarkStepComplete(step.ID, result)
		_ = e.repo.SavePlan(ctx, plan)

		// 3. Apply Changes Incrementally (The "See Your Work" Requirement)
		changes, _ := e.shadowMgr.ListShadowFiles()

		// 🔥 ADD: Warning if no changes were made
		if len(changes) == 0 {
			fmt.Printf("⚠️  WARNING: Step completed but no files were modified. Agent may not have used write_file.\n")
			fmt.Printf("   Agent response: %s\n", result)
		}

		for _, file := range changes {
			// CRITICAL: Backup file immediately before overwriting with shadow content
			if err := txn.BackupFile(file); err != nil {
				return fmt.Errorf("backup failed for %s: %w", file, err)
			}

			if err := e.shadowMgr.Commit(file); err != nil {
				fmt.Printf("❌ Failed to apply %s: %v\n", file, err)
				return err
			}
			fmt.Printf("✅ Applied: %s\n", file)
		}
	}

	// 4. Finalize Transaction
	// This marks the transaction as safe, and clears the shadow dir via Commit() logic
	if err := txn.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	plan.Status = task.StatusCompleted
	return e.repo.SavePlan(ctx, plan)
}

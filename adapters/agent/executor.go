package agent

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/task"
	infraCtx "github.com/david22573/codepicker/infra/context"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/logging"
	"github.com/fatih/color"
	"go.uber.org/zap"
)

type PlanExecutor struct {
	worker       agent.Agent
	repo         agent.Repository
	workspaceMgr *fs.WorkspaceManager
	shadowMgr    *fs.ShadowManager
	logger       *logging.Logger
	autoConfirm  bool
}

func NewPlanExecutor(worker agent.Agent, repo agent.Repository, ws *fs.WorkspaceManager, shadow *fs.ShadowManager, logger *logging.Logger) *PlanExecutor {
	return &PlanExecutor{
		worker:       worker,
		repo:         repo,
		workspaceMgr: ws,
		shadowMgr:    shadow,
		logger:       logger,
		autoConfirm:  false,
	}
}

func (e *PlanExecutor) SetAutoConfirm(auto bool) {
	e.autoConfirm = auto
}

func (e *PlanExecutor) UpdateSystemPrompt(msg string) {
	if agent, ok := e.worker.(interface{ UpdateSystemPrompt(string) }); ok {
		agent.UpdateSystemPrompt(msg)
	}
}

func (e *PlanExecutor) GetSystemPrompt() string {
	if agent, ok := e.worker.(interface{ GetSystemPrompt() string }); ok {
		return agent.GetSystemPrompt()
	}
	return ""
}

func (e *PlanExecutor) Execute(ctx context.Context, plan *task.Plan) error {
	txn, err := e.workspaceMgr.BeginTransaction()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	txn.AttachShadow(e.shadowMgr)

	if err := e.shadowMgr.Clear(); err != nil {
		e.logger.Error("failed to clear shadow manager on start", zap.Error(err))
	}

	var rollbackOnce sync.Once
	doRollback := func() {
		rollbackOnce.Do(func() {
			if !txn.Committed {
				fmt.Println("⚠️  Rolling back changes (restoring files + clearing shadow)...")
				if err := txn.Rollback(); err != nil {
					e.logger.Error("failed to rollback transaction", zap.Error(err))
				}
			}
		})
	}

	defer doRollback()

	stopWatch := infraCtx.WatchContext(ctx, doRollback)
	defer stopWatch()

	plan.Status = task.StatusRunning
	if err := e.repo.SavePlan(ctx, plan); err != nil {
		e.logger.Error("failed to persist plan state", zap.Error(err))
	}

	reader := bufio.NewReader(os.Stdin)

	for _, step := range plan.Steps {
		if step.Status == task.StatusCompleted {
			continue
		}

		if infraCtx.IsCancelled(ctx) {
			// FIX: Mark step as failed due to cancellation, and use context.Background() to ensure it saves
			plan.MarkStepFailed(step.ID, fmt.Errorf("aborted by user"))
			if err := e.repo.SavePlan(context.Background(), plan); err != nil {
				e.logger.Error("failed to persist plan state on cancel", zap.Error(err))
			}
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

		for _, file := range step.Files {
			if err := txn.BackupFile(file); err != nil {
				e.logger.Error("failed to backup file", zap.String("file", file), zap.Error(err))
			}
		}

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

		result, err := e.worker.Run(ctx, workerInput)
		if err != nil {
			plan.MarkStepFailed(step.ID, err)

			// FIX: Use context.Background() here as well to safely write the failure to SQLite
			if err := e.repo.SavePlan(context.Background(), plan); err != nil {
				e.logger.Error("failed to persist plan state on error", zap.Error(err))
			}
			return err
		}

		plan.MarkStepComplete(step.ID, result)
		// It's safe to use context.Background() here too to ensure DB state updates reliably
		if err := e.repo.SavePlan(context.Background(), plan); err != nil {
			e.logger.Error("failed to persist plan state", zap.Error(err))
		}

		changes, _ := e.shadowMgr.ListShadowFiles()

		if len(changes) == 0 {
			fmt.Printf("⚠️  WARNING: Step completed but no files were modified.\nAgent may not have used write_file.\n")
			fmt.Printf("   Agent response: %s\n", result)
		}

		for _, file := range changes {
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

	if err := txn.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	plan.Status = task.StatusCompleted
	// Ensure the final completed state is strictly saved
	if err := e.repo.SavePlan(context.Background(), plan); err != nil {
		e.logger.Error("failed to persist plan state", zap.Error(err))
	}

	return nil
}


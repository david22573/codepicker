package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/app"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/spf13/cobra"
)

var resumeCmd = &cobra.Command{
	Use:   "resume <checkpoint-id>",
	Short: "Resume execution from a checkpoint",
	Long:  `Restores the agent state from a checkpoint and continues execution where it left off.`,
	Example: `  codepicker agent resume abc123def456
  codepicker agent resume abc123def456 --ci`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		checkpointID := args[0]

		// Phase 4: Force strict policy in CI mode
		currentPolicy := policy.Interactive
		if ciMode {
			currentPolicy = policy.Batch
		}

		agentCtx, err := app.NewAgentContext(cmd.Context(), app.ContextOptions{
			SrcDir:   srcDir,
			LogLevel: 1,
			Mode:     app.ModeInteractive,
			Policy:   currentPolicy,
			Task:     fmt.Sprintf("Resume from checkpoint %s", checkpointID),
		})
		if err != nil {
			return fmt.Errorf("failed to initialize context: %w", err)
		}
		defer agentCtx.Close()

		// Cost summary on exit
		defer func() {
			cost, count := agentCtx.Engine.CostTracker.GetStats()
			fmt.Println("\n" + strings.Repeat("-", 40))
			fmt.Printf("💰 Session Cost Summary:\n")
			fmt.Printf("   Requests: %d\n", count)
			fmt.Printf("   Total:    $%.4f\n", cost)
			fmt.Println(strings.Repeat("-", 40))
		}()

		// Load the checkpoint
		agentCtx.Logger.Info(fmt.Sprintf("📸 Loading checkpoint: %s", checkpointID))

		checkpoint, err := agentCtx.Store.LoadCheckpoint(checkpointID)
		if err != nil {
			return fmt.Errorf("failed to load checkpoint: %w", err)
		}

		// Display checkpoint info
		fmt.Printf("\n📍 Checkpoint Information:\n")
		fmt.Printf("   Session: %s\n", checkpoint.SessionID)
		fmt.Printf("   Task: %s\n", checkpoint.Task)
		fmt.Printf("   Progress: %.1f%% (Step %d)\n", checkpoint.Progress*100, checkpoint.CurrentStep)
		fmt.Printf("   Previous Cost: $%.4f (%d requests)\n", checkpoint.TotalCost, checkpoint.RequestCount)
		fmt.Printf("   Created: %s\n", checkpoint.Timestamp.Format("2006-01-02 15:04:05"))
		fmt.Printf("   Status: %s\n", checkpoint.Status)
		fmt.Println()

		// Check if already completed
		if checkpoint.Status == "completed" {
			fmt.Println("⚠️  This checkpoint is marked as completed.")
			fmt.Print("Resume anyway? [y/N]: ")
			var confirm string
			fmt.Scanln(&confirm)
			if confirm != "y" && confirm != "Y" {
				fmt.Println("Cancelled.")
				return nil
			}
		}

		// Restore the plan if available
		var plan *agent.Plan
		if checkpoint.PlanID != "" {
			planRecord, err := agentCtx.Store.GetPlan(checkpoint.PlanID)
			if err != nil {
				return fmt.Errorf("failed to load plan %s: %w", checkpoint.PlanID, err)
			}

			var steps []agent.Step
			if err := json.Unmarshal([]byte(planRecord.StepsJSON), &steps); err != nil {
				return fmt.Errorf("failed to parse plan steps: %w", err)
			}

			// Restore step statuses from checkpoint
			for i := range steps {
				stepID := steps[i].ID
				if status, ok := checkpoint.StepsStatus[stepID]; ok {
					steps[i].Status = status
				}
				if result, ok := checkpoint.StepResults[stepID]; ok {
					steps[i].Result = result
				}
			}

			plan = &agent.Plan{
				ID:            checkpoint.PlanID,
				OriginalTask:  checkpoint.Task,
				Steps:         steps,
				EstimatedCost: planRecord.EstimatedCost,
			}
		} else {
			return fmt.Errorf("checkpoint does not have an associated plan")
		}

		// Create plan executor with the restored session ID
		executor := agent.NewPlanExecutorWithSession(agentCtx.Engine, plan, checkpoint.SessionID)

		// Disable auto-checkpointing during resume to avoid conflicts
		// (we'll create a new checkpoint after successful completion)
		executor.AutoCheckpoint = true
		executor.CheckpointInterval = 1

		agentCtx.Logger.Info(fmt.Sprintf("🚀 Resuming execution from step %d/%d", checkpoint.CurrentStep+1, len(plan.Steps)))

		// Resume execution using the checkpoint
		if err := executor.Resume(cmd.Context(), checkpointID); err != nil {
			agentCtx.Logger.Error(fmt.Sprintf("Execution failed: %v", err))
			return err
		}

		fmt.Println("\n✨ Execution resumed and completed successfully!")
		return nil
	},
}

func init() {
	agentCmd.AddCommand(resumeCmd)
}

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/david22573/codepicker/app"
	"github.com/david22573/codepicker/domain/task"
	"github.com/david22573/codepicker/infra/ui"
	"github.com/spf13/cobra"
)

// Local flags to ensure self-contained compilation
var (
	runDryRun  bool
	runCI      bool
	runPlanID  string
	runAutoYes bool
)

var runCmd = &cobra.Command{
	Use:   "run [task]",
	Short: "Execute a coding task (via plan)",
	Run: func(cmd *cobra.Command, args []string) {
		// Validation
		if runPlanID == "" && len(args) < 1 {
			ui.PrintError("You must provide a task string OR a --plan <id>")
			_ = cmd.Usage()
			os.Exit(1)
		}

		taskInput := ""
		if len(args) > 0 {
			taskInput = args[0]
		}

		// Execute
		if err := executeRun(taskInput, runPlanID); err != nil {
			if runCI {
				// JSON Output for CI
				res := task.CIResult{
					Status: "failure",
					Task:   taskInput,
					Error:  err.Error(),
				}
				json.NewEncoder(os.Stdout).Encode(res)
				os.Exit(1)
			}
			ui.PrintError(fmt.Sprintf("EXECUTION FAILED: %v", err))
			os.Exit(1)
		}
	},
}

func executeRun(taskInput, planID string) error {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENROUTER_API_KEY required")
	}

	cwd, _ := os.Getwd()

	// Initialize Container with local flags
	container, err := app.NewContainer(apiKey, cwd, "", runDryRun, runCI)
	if err != nil {
		return err
	}
	defer container.Close()

	if !runCI {
		printSafetyBanner(runDryRun)
	}

	ctx := context.Background()
	var plan *task.Plan

	// --- Step 1: Planning ---
	if planID != "" {
		// Load existing plan
		if !runCI {
			ui.PrintInfo(fmt.Sprintf("Loading Plan %s...", planID))
		}
		p, err := container.Repository.GetPlan(ctx, planID)
		if err != nil {
			return fmt.Errorf("failed to load plan: %w", err)
		}
		plan = p
		if plan.Status == task.StatusCompleted && !runCI {
			ui.PrintWarning("This plan is already marked as completed.")
		}
	} else {
		// Create new plan
		if !runCI {
			ui.PrintHeader("Planning Phase")

			var fileContext string
			var primer string // New variable for the map

			// Single spinner for context gathering
			err := ui.RunSpinner("Analyzing project context...", func() error {
				var innerErr error
				// FIX 1: Generate Primer
				primer = container.ProjectPrimer.Generate()
				// Build semantic context
				fileContext, innerErr = container.ContextBuilder.BuildForTask(taskInput)
				return innerErr
			})
			if err != nil {
				ui.PrintWarning(fmt.Sprintf("Context generation partial: %v", err))
			}

			// Spinner for plan generation
			err = ui.RunSpinner("Generating implementation plan...", func() error {
				var innerErr error
				// FIX 2: Pass 'primer' to CreatePlan (matches new signature)
				plan, innerErr = container.Planner.CreatePlan(ctx, taskInput, fileContext, primer)
				return innerErr
			})

			if err != nil {
				return err
			}

			ui.PrintSuccess(fmt.Sprintf("Plan Generated (ID: %s)", plan.ID))

			// Display Plan Summary
			fmt.Printf("\n%s\n", ui.InfoStyle.Render("Strategy: "+plan.Reasoning))
			for i, step := range plan.Steps {
				fmt.Printf("   %d. %s\n", i+1, step.Description)
			}
			fmt.Println()

		} else {
			// CI Mode - Silent execution
			primer := container.ProjectPrimer.Generate()
			fileContext, _ := container.ContextBuilder.BuildForTask(taskInput)
			// Pass primer here too
			p, err := container.Planner.CreatePlan(ctx, taskInput, fileContext, primer)
			if err != nil {
				return err
			}
			plan = p
		}
	}

	// --- Step 2: Execution ---
	if !runCI {
		ui.PrintHeader("Execution Phase")
	}

	// Execute Plan
	execErr := container.PlanExecutor.Execute(ctx, plan)

	// --- Step 3: Reporting ---
	if runCI {
		return handleCIOutput(plan, execErr)
	}

	// Print Cost Summary
	if container.CostTracker != nil {
		container.CostTracker.PrintSummary()
	}

	if execErr != nil {
		return execErr
	}

	ui.PrintSuccess("Task Completed Successfully.")
	return nil
}

// Helper functions

func printSafetyBanner(isDryRun bool) {
	if isDryRun {
		fmt.Println(ui.BoxStyle.Render(
			ui.InfoStyle.Render("🔒 MODE: DRY-RUN (Read-Only)\n") +
				"• File system writes are DISABLED\n" +
				"• Shell commands are DISABLED",
		))
	} else {
		fmt.Println(ui.BoxStyle.Render(
			ui.WarningStyle.Render("⚡ MODE: LIVE EXECUTION (Write-Enabled)\n") +
				"• The agent has permission to modify files.\n" +
				"• Shadow filesystem is ACTIVE for safety rollback.",
		))
	}
	fmt.Println()
}

func handleCIOutput(plan *task.Plan, execErr error) error {
	failedCount := 0
	for _, s := range plan.Steps {
		if s.Status == task.StatusFailed {
			failedCount++
		}
	}

	status := "success"
	errMsg := ""
	if execErr != nil || failedCount > 0 {
		status = "failure"
		if execErr != nil {
			errMsg = execErr.Error()
		} else {
			errMsg = "One or more steps failed"
		}
	}

	result := task.CIResult{
		Status:      status,
		Task:        plan.OriginalTask,
		PlanSummary: plan.Reasoning,
		StepsTotal:  len(plan.Steps),
		StepsFailed: failedCount,
		Error:       errMsg,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func init() {
	runCmd.Flags().BoolVar(&runDryRun, "dry-run", false, "Enable read-only mode")
	runCmd.Flags().BoolVar(&runCI, "ci", false, "Enable CI mode (JSON output, no prompts, strict safety)")
	runCmd.Flags().StringVar(&runPlanID, "plan", "", "Execute a specific pre-generated plan ID")
	runCmd.Flags().BoolVarP(&runAutoYes, "yes", "y", false, "Skip confirmation prompts")
	rootCmd.AddCommand(runCmd)
}

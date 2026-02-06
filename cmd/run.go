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

var (
	dryRunFlag bool
	ciFlag     bool
	planIDFlag string
)

var runCmd = &cobra.Command{
	Use:   "run [task]",
	Short: "Execute a coding task (via plan)",
	Run: func(cmd *cobra.Command, args []string) {
		if planIDFlag == "" && len(args) < 1 {
			ui.PrintError("You must provide a task string OR a --plan <id>")
			_ = cmd.Usage()
			os.Exit(1)
		}

		taskInput := ""
		if len(args) > 0 {
			taskInput = args[0]
		}

		if err := executeRun(taskInput, planIDFlag); err != nil {
			if ciFlag {
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

	// Initialize Container
	container, err := app.NewContainer(apiKey, cwd, "", dryRunFlag, ciFlag)
	if err != nil {
		return err
	}
	defer container.Close()

	if !ciFlag {
		printSafetyBanner(dryRunFlag)
	}

	ctx := context.Background()
	var plan *task.Plan

	// --- Step 1: Planning ---
	if planID != "" {
		if !ciFlag {
			ui.PrintInfo(fmt.Sprintf("Loading Plan %s...", planID))
		}
		p, err := container.Repository.GetPlan(ctx, planID)
		if err != nil {
			return fmt.Errorf("failed to load plan: %w", err)
		}
		plan = p
		if plan.Status == task.StatusCompleted && !ciFlag {
			ui.PrintWarning("This plan is already marked as completed.")
		}
	} else {
		if !ciFlag {
			ui.PrintHeader("Planning Phase")

			// Use Bubble Tea Spinner for the heavy lifting
			var fileContext string

			err := ui.RunSpinner("Analyzing project context...", func() error {
				var innerErr error
				fileContext, innerErr = container.ContextBuilder.BuildForTask(taskInput)
				return innerErr
			})
			if err != nil {
				ui.PrintWarning(fmt.Sprintf("Context generation partial: %v", err))
			}

			err = ui.RunSpinner("Generating implementation plan...", func() error {
				var innerErr error
				plan, innerErr = container.Planner.CreatePlan(ctx, taskInput, fileContext)
				return innerErr
			})

			if err != nil {
				return err
			}

			ui.PrintSuccess(fmt.Sprintf("Plan Generated (ID: %s)", plan.ID))

			// Display Plan Summary using Lipgloss style
			fmt.Printf("\n%s\n", ui.InfoStyle.Render("Strategy: "+plan.Reasoning))
			for i, step := range plan.Steps {
				fmt.Printf("   %d. %s\n", i+1, step.Description)
			}
			fmt.Println()
		} else {
			// CI Mode - Silent
			fileContext, _ := container.ContextBuilder.BuildForTask(taskInput)
			p, err := container.Planner.CreatePlan(ctx, taskInput, fileContext)
			if err != nil {
				return err
			}
			plan = p
		}
	}

	// --- Step 2: Execution ---
	if !ciFlag {
		ui.PrintHeader("Execution Phase")
	}

	// We don't use a spinner here because the agent logs real-time steps to stdout
	execErr := container.PlanExecutor.Execute(ctx, plan)

	// --- Step 3: Reporting ---
	if ciFlag {
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
	runCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Enable read-only mode")
	runCmd.Flags().BoolVar(&ciFlag, "ci", false, "Enable CI mode (JSON output, no prompts, strict safety)")
	runCmd.Flags().StringVar(&planIDFlag, "plan", "", "Execute a specific pre-generated plan ID")
	rootCmd.AddCommand(runCmd)
}

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
	runDryRun  bool
	runCI      bool
	runPlanID  string
	runAutoYes bool
)

var runCmd = &cobra.Command{
	Use:   "run [task]",
	Short: "Execute a coding task (via plan)",
	Run: func(cmd *cobra.Command, args []string) {
		if runPlanID == "" && len(args) < 1 {
			ui.PrintError("You must provide a task string OR a --plan <id>")
			_ = cmd.Usage()
			os.Exit(1)
		}

		taskInput := ""
		if len(args) > 0 {
			taskInput = args[0]
		}

		if err := executeRun(taskInput, runPlanID); err != nil {
			if runCI {
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
	container, err := app.NewContainer(apiKey, cwd, "", runDryRun, runCI)
	if err != nil {
		return err
	}
	defer container.Close()

	container.PlanExecutor.SetAutoConfirm(runAutoYes)

	if !runCI {
		printSafetyBanner(runDryRun)
	}

	ctx := context.Background()
	var plan *task.Plan

	// --- Step 1: Planning ---
	if planID != "" {
		if !runCI {
			ui.PrintInfo(fmt.Sprintf("Loading Plan %s...", planID))
		}
		plan, err = container.Repository.GetPlan(ctx, planID)
		if err != nil {
			return fmt.Errorf("failed to load plan: %w", err)
		}
	} else {
		if !runCI {
			ui.PrintHeader("Planning Phase")
			var fileContext, primer string

			err := ui.RunSpinner("Analyzing project context...", func() error {
				var innerErr error
				primer = container.ProjectPrimer.Generate()
				fileContext, innerErr = container.ContextBuilder.BuildForTask(taskInput)
				return innerErr
			})
			if err != nil {
				ui.PrintWarning(fmt.Sprintf("Context generation partial: %v", err))
			}

			err = ui.RunSpinner("Generating implementation plan...", func() error {
				var innerErr error
				plan, innerErr = container.Planner.CreatePlan(ctx, taskInput, fileContext, primer)
				return innerErr
			})
			if err != nil {
				return err
			}

			ui.PrintSuccess(fmt.Sprintf("Plan Generated (ID: %s)", plan.ID))
			fmt.Printf("\n%s\n", ui.InfoStyle.Render("Strategy: "+plan.Reasoning))
			for i, step := range plan.Steps {
				fmt.Printf("   %d. %s\n", i+1, step.Description)
			}
			fmt.Println()
		} else {
			primer := container.ProjectPrimer.Generate()
			fileContext, _ := container.ContextBuilder.BuildForTask(taskInput)
			plan, err = container.Planner.CreatePlan(ctx, taskInput, fileContext, primer)
			if err != nil {
				return err
			}
		}
	}

	// --- Step 2: Execution ---
	if !runCI {
		ui.PrintHeader("Execution Phase")
	}

	execErr := container.PlanExecutor.Execute(ctx, plan)

	// --- Step 3: Application (Feature 5) ---
	if execErr == nil {
		files, err := container.ShadowManager.ListChanges()
		if err == nil && len(files) > 0 {
			if runDryRun {
				fmt.Println("\n📝 [DRY-RUN] The following changes would be applied:")
				for _, f := range files {
					// We can improve this by using ShadowManager.Diff(f)
					summary, _ := container.ShadowManager.Diff(f)
					if summary != nil {
						fmt.Printf("   %s\n", summary.String())
					} else {
						fmt.Printf("   • %s\n", f)
					}
				}
				fmt.Println("   (No files were modified on disk)")
			} else {
				fmt.Println("\n💾 Applying changes to filesystem...")
				for _, f := range files {
					if err := container.ShadowManager.Commit(f); err != nil {
						ui.PrintError(fmt.Sprintf("Failed to commit %s: %v", f, err))
					} else {
						fmt.Printf("   ✔ Applied: %s\n", f)
					}
				}
			}
		}
	}

	// --- Step 4: Reporting ---
	if runCI {
		return handleCIOutput(plan, execErr)
	}

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
				"• Shell commands are DISABLED\n" +
				"• Changes are simulated in shadow FS",
		))
	} else {
		fmt.Println(ui.BoxStyle.Render(
			ui.WarningStyle.Render("⚡ MODE: LIVE EXECUTION (Write-Enabled)\n") +
				"• The agent has permission to modify files.\n" +
				"• Changes will be applied automatically upon success.",
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
	runCmd.Flags().BoolVar(&runCI, "ci", false, "Enable CI mode")
	runCmd.Flags().StringVar(&runPlanID, "plan", "", "Execute a specific plan ID")
	runCmd.Flags().BoolVarP(&runAutoYes, "yes", "y", false, "Skip confirmation prompts")
	rootCmd.AddCommand(runCmd)
}

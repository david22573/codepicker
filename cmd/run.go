package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	contextAdapters "github.com/david22573/codepicker/adapters/context"
	"github.com/david22573/codepicker/app"
	"github.com/david22573/codepicker/domain/task"
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
			fmt.Println("Error: You must provide a task string OR a --plan <id>")
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
			fmt.Printf("\n❌ EXECUTION FAILED: %v\n", err)
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

	container, err := app.NewContainer(apiKey, cwd, "", dryRunFlag, ciFlag)
	if err != nil {
		return err
	}

	// --- UX HARDENING: Startup Barriers ---
	if !ciFlag {
		printSafetyBanner(dryRunFlag)
	}

	ctx := context.Background()
	var plan *task.Plan

	// Scenario A: Resume Plan
	if planID != "" {
		if !ciFlag {
			fmt.Printf("📂 [SYSTEM] Loading Plan %s...\n", planID)
		}
		p, err := container.Repository.GetPlan(ctx, planID)
		if err != nil {
			return fmt.Errorf("failed to load plan: %w", err)
		}
		plan = p
		if plan.Status == task.StatusCompleted && !ciFlag {
			fmt.Println("⚠️  [SYSTEM] Warning: This plan is already marked as completed.")
		}
	} else {
		// Scenario B: Generate Plan
		if !ciFlag {
			fmt.Printf("🚀 [SYSTEM] Initializing task: %s\n", taskInput)
			fmt.Println("🧠 [AGENT] Generating plan...")
		}

		// FIX: Generate File Context so the Planner can see the files
		fileContext, err := container.ContextBuilder.Build(contextAdapters.Config{
			ProjectRoot: cwd,
			MaxTokens:   3000, // Sufficient for tree + critical file headers
		})
		if err != nil {
			// Non-fatal, but warn
			fmt.Printf("⚠️  Warning: Context generation incomplete: %v\n", err)
		}

		p, err := container.Planner.CreatePlan(ctx, taskInput, fileContext)
		if err != nil {
			return err
		}
		plan = p
		if !ciFlag {
			fmt.Printf("✅ [SYSTEM] Plan Generated (ID: %s)\n", plan.ID)
		}
	}

	// Execute
	if !ciFlag {
		fmt.Println("▶️  [SYSTEM] Starting Execution Phase...")
	}

	execErr := container.PlanExecutor.Execute(ctx, plan)

	// Output
	if ciFlag {
		return handleCIOutput(plan, execErr)
	}

	if execErr != nil {
		return execErr
	}
	fmt.Println("\n✅ [SYSTEM] Task Completed Successfully.")
	return nil
}

func printSafetyBanner(isDryRun bool) {
	fmt.Println("\n===================================================")
	fmt.Println("🛡️  CodePicker Safety Guardrails Active")
	fmt.Println("===================================================")

	if isDryRun {
		fmt.Println("🔒 MODE: DRY-RUN (Read-Only)")
		fmt.Println("   • File system writes are DISABLED")
		fmt.Println("   • Shell commands are DISABLED")
	} else {
		fmt.Println("⚡ MODE: LIVE EXECUTION (Write-Enabled)")
		fmt.Println("   ⚠️  The agent has permission to modify files.")
		fmt.Println("   ⚠️  Shadow filesystem is ACTIVE for safety rollback.")
		fmt.Println("   • Monitor the '🤖 [AGENT]' logs below closely.")
	}
	fmt.Println("===================================================\n")
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

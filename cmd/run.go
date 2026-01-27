package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/david22573/codepicker/app"
	"github.com/david22573/codepicker/domain/task"
	"github.com/spf13/cobra"
)

var (
	dryRunFlag bool
	ciFlag     bool
)

var runCmd = &cobra.Command{
	Use:   "run [task]",
	Short: "Execute a coding task",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Use Run instead of RunE to control exit code and output strictly for CI
		if err := executeRun(args[0]); err != nil {
			if ciFlag {
				// Ensure JSON error output
				res := task.CIResult{
					Status: "failure",
					Task:   args[0],
					Error:  err.Error(),
				}
				json.NewEncoder(os.Stdout).Encode(res)
				os.Exit(1)
			}
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func executeRun(taskInput string) error {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENROUTER_API_KEY required")
	}

	cwd, _ := os.Getwd()

	// Pass flags to container
	container, err := app.NewContainer(apiKey, cwd, dryRunFlag, ciFlag)
	if err != nil {
		return err
	}

	// In CI mode, we stay silent on stdout until the end
	if !ciFlag {
		fmt.Printf("🚀 Initializing CodePicker (CI=%v, DryRun=%v)...\n", ciFlag, dryRunFlag)
		if ciFlag {
			fmt.Println("⚠️  CI Mode Active: Prompts disabled, output will be JSON.")
		}
	}

	ctx := context.Background()

	// 1. Plan
	if !ciFlag {
		fmt.Println("🧠 Generating plan...")
	}
	plan, err := container.Planner.CreatePlan(ctx, taskInput)
	if err != nil {
		return err
	}

	// 2. Execute
	if !ciFlag {
		fmt.Println("🚀 Executing Plan...")
	}

	// Execute can return error, but we also want to inspect partial plan state
	execErr := container.PlanExecutor.Execute(ctx, plan)

	// 3. Output
	if ciFlag {
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
			Task:        taskInput,
			PlanSummary: plan.Reasoning,
			StepsTotal:  len(plan.Steps),
			StepsFailed: failedCount,
			Error:       errMsg,
		}

		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}

	// Normal Human Output
	if execErr != nil {
		return execErr
	}
	fmt.Println("✅ Task Completed.")
	return nil
}

func init() {
	runCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Enable read-only mode")
	runCmd.Flags().BoolVar(&ciFlag, "ci", false, "Enable CI mode (JSON output, no prompts, strict safety)")
	rootCmd.AddCommand(runCmd)
}

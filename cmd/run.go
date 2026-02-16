package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/david22573/codepicker/app"
	"github.com/david22573/codepicker/domain/task"
	"github.com/spf13/cobra"
)

var runDryRun bool
var runCiMode bool
var runLlmModel string
var runVerbose bool

var runCmd = &cobra.Command{
	Use:   "run [task description]",
	Short: "Run a single task using the agent",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			fmt.Println("❌ Error: OPENROUTER_API_KEY is not set.")
			os.Exit(1)
		}

		taskDescription := args[0]
		cwd, _ := os.Getwd()

		// Initialize Container
		container, err := app.NewContainer(apiKey, cwd, runLlmModel, runDryRun, runCiMode, GetVerbose())
		if err != nil {
			fmt.Printf("❌ Container Init Failed: %v\n", err)
			os.Exit(1)
		}
		defer container.Close()

		ctx := context.Background()
		fmt.Printf("🚀 Running task: %s\n", taskDescription)

		// 1. Load Context (The Primer)
		// FIX: Use a Smart Primer strategy.
		// If the user has manually packed context, use it.
		// Otherwise, generate a SHALLOW (Depth 2) primer to save tokens during the planning phase.
		var primer string
		manualContextPath := filepath.Join(cwd, "codepicker_context.txt")

		if content, err := os.ReadFile(manualContextPath); err == nil {
			fmt.Println("🗺️  Using manual context file (codepicker_context.txt)...")
			primer = string(content)
		} else {
			fmt.Println("🗺️  Generating shallow project map (Depth 2) for planning...")
			primer = container.ProjectPrimer.GenerateShallow()
		}

		// 2. Generate a Plan
		// We pass the primer as context so the planner knows the high-level file structure
		fmt.Println("🧠 Generating execution plan...")
		plan, err := container.Planner.CreatePlan(ctx, taskDescription, "", primer)
		if err != nil {
			fmt.Printf("❌ Planning failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("📝 Plan generated: %s (%d steps)\n", plan.ID, len(plan.Steps))

		// 3. Configure Execution Mode
		// If CI mode is on, we skip the interactive confirmation prompts
		if runCiMode {
			container.PlanExecutor.SetAutoConfirm(true)
		}

		// 4. Execute the Plan
		err = container.PlanExecutor.Execute(ctx, plan)
		if err != nil {
			fmt.Printf("❌ Execution failed: %v\n", err)
			os.Exit(1)
		}

		// 5. Final Report
		fmt.Println("\n✅ Task Execution Completed.")
		for _, step := range plan.Steps {
			icon := "✅"
			if step.Status == task.StatusFailed {
				icon = "❌"
			}
			fmt.Printf("   %s Step %d: %s\n", icon, step.ID, step.Description)
		}
	},
}

func init() {
	runCmd.Flags().BoolVar(&runDryRun, "dry-run", false, "Enable read-only mode")
	runCmd.Flags().BoolVar(&runCiMode, "ci", false, "Enable CI mode (skip confirmations)")
	runCmd.Flags().StringVar(&runLlmModel, "model", "", "LLM model to use")
	runCmd.Flags().BoolVarP(&runVerbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.AddCommand(runCmd)
}

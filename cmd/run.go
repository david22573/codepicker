package cmd

import (
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
	RunE: func(cmd *cobra.Command, args []string) error {
		apiKey := getAPIKeyOrExit()
		taskDescription := args[0]
		cwd, _ := os.Getwd()

		container, err := app.NewContainer(apiKey, cwd, runLlmModel, runDryRun, runCiMode, GetVerbose())
		if err != nil {
			return fmt.Errorf("container init failed: %w", err)
		}
		defer container.Close()

		ctx := cmd.Context()

		fmt.Printf("🚀 Running task: %s\n", taskDescription)

		var primer string
		manualContextPath := filepath.Join(cwd, "codepicker_context.txt")

		if content, err := os.ReadFile(manualContextPath); err == nil {
			fmt.Println("🗺️  Using manual context file (codepicker_context.txt)...")
			primer = string(content)
		} else {
			fmt.Println("🗺️  Generating shallow project map (Depth 2) for planning...")
			primer = container.ProjectPrimer.GenerateShallow()
		}

		fmt.Println("🧠 Generating execution plan...")
		plan, err := container.Planner.CreatePlan(ctx, taskDescription, "", primer)
		if err != nil {
			return fmt.Errorf("planning failed: %w", err)
		}

		fmt.Printf("📝 Plan generated: %s (%d steps)\n", plan.ID, len(plan.Steps))

		if runCiMode {
			container.PlanExecutor.SetAutoConfirm(true)
		}

		err = container.PlanExecutor.Execute(ctx, plan)
		if err != nil {
			return fmt.Errorf("execution failed: %w", err)
		}

		fmt.Println("\n✅ Task Execution Completed.")
		for _, step := range plan.Steps {
			icon := "✅"
			if step.Status == task.StatusFailed {
				icon = "❌"
			}
			fmt.Printf("   %s Step %d: %s\n", icon, step.ID, step.Description)
		}

		return nil
	},
}

func init() {
	runCmd.Flags().BoolVar(&runDryRun, "dry-run", false, "Enable read-only mode")
	runCmd.Flags().BoolVar(&runCiMode, "ci", false, "Enable CI mode (skip confirmations)")
	runCmd.Flags().StringVar(&runLlmModel, "model", "", "LLM model to use")
	runCmd.Flags().BoolVarP(&runVerbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.AddCommand(runCmd)
}

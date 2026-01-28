package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	contextAdapters "github.com/david22573/codepicker/adapters/context"
	"github.com/david22573/codepicker/app"
	"github.com/spf13/cobra"
)

var (
	agentModel  string
	executionID string // Shared flag for replay/explain
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Interact with specific agent personas and history",
}

// --- Phase 1: Audit Command ---
var auditCmd = &cobra.Command{
	Use:   "audit [query]",
	Short: "Perform a read-only audit of the codebase",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]
		return runAgentTask(func(c *app.Container) error {
			fmt.Printf("🛡️  Starting Audit (Model: %s)\n", getModelDisplay())
			report, err := c.Auditor.RunAudit(context.Background(), query)
			if err != nil {
				return err
			}
			fmt.Printf("\n✅ Audit Complete. Artifact: %s\n", report.Artifact)

			// Print a preview
			preview := report.Content
			if len(preview) > 500 {
				preview = preview[:500] + "... (see file for full report)"
			}
			fmt.Println("\n--- Preview ---")
			fmt.Println(preview)
			return nil
		}, true) // Audit enforces dry-run
	},
}

// --- Phase 2: Plan Command ---
var planCmd = &cobra.Command{
	Use:   "plan [task]",
	Short: "Generate and store an execution plan without running it",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskInput := args[0]
		return runAgentTask(func(c *app.Container) error {
			fmt.Printf("🧠 Generating Plan (Model: %s)...\n", getModelDisplay())

			cwd, _ := os.Getwd()
			fileContext, _ := c.ContextBuilder.Build(contextAdapters.Config{
				ProjectRoot: cwd,
				MaxTokens:   3000,
			})

			// This generates AND saves the plan
			plan, err := c.Planner.CreatePlan(context.Background(), taskInput, fileContext)
			if err != nil {
				return err
			}

			fmt.Println("\n✅ Plan Created Successfully")
			fmt.Printf("🆔 ID: %s\n", plan.ID)
			fmt.Printf("📝 Reasoning: %s\n", plan.Reasoning)
			fmt.Printf("🔢 Steps: %d\n", len(plan.Steps))

			for _, step := range plan.Steps {
				fmt.Printf("  - [Step %d] %s\n", step.ID, step.Description)
			}
			fmt.Println("\nRun 'codepicker run --plan <id>' to execute.")
			return nil
		}, true) // Planning is effectively dry-run
	},
}

// --- Phase 4: Replay Command ---
var replayCmd = &cobra.Command{
	Use:   "replay",
	Short: "Deterministically replay an execution log (visual only)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if executionID == "" {
			return fmt.Errorf("execution ID required (use --execution)")
		}

		return runAgentTask(func(c *app.Container) error {
			// Fetch directly from Repo
			exec, err := c.Repository.GetExecution(context.Background(), executionID)
			if err != nil {
				return fmt.Errorf("not found: %w", err)
			}

			fmt.Printf("🎞️  REPLAYING SESSION: %s\n", exec.ID)
			fmt.Printf("📅 Date: %s\n", exec.StartTime.Format(time.RFC822))
			fmt.Printf("🚦 Final Status: %s\n", exec.Status)
			fmt.Println("===================================================")

			for _, turn := range exec.History {
				// Deterministic visual playback
				fmt.Printf("\n[Turn %d]\n", turn.TurnID)
				fmt.Printf("🧠 Thought: %s\n", turn.Thought)
				if turn.ToolName != "" {
					fmt.Printf("🛠️  Action: %s\n", turn.ToolName)
					fmt.Printf("📥 Input:   %s\n", turn.ToolArgs)

					// Safe display of output
					out := turn.ToolOut
					if len(out) > 300 {
						out = out[:300] + "... (truncated)"
					}
					fmt.Printf("📤 Output:  %s\n", out)
				} else {
					fmt.Println("🛑 Action: (None/Final Answer)")
				}
				fmt.Println("---")
			}
			return nil
		}, true)
	},
}

// --- Phase 4: Explain Command ---
var explainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Generate an AI explanation for a past execution",
	RunE: func(cmd *cobra.Command, args []string) error {
		if executionID == "" {
			return fmt.Errorf("execution ID required (use --execution)")
		}

		return runAgentTask(func(c *app.Container) error {
			fmt.Printf("🔍 Explaining Execution %s (Model: %s)\n", executionID, getModelDisplay())

			explanation, err := c.Explainer.Explain(context.Background(), executionID)
			if err != nil {
				return err
			}

			fmt.Println("\n================= ANALYSIS ==================")
			fmt.Println(explanation)
			fmt.Println("=============================================")
			return nil
		}, true)
	},
}

// Helper (reused from Phase 2)
func runAgentTask(action func(*app.Container) error, dryRun bool) error {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENROUTER_API_KEY required")
	}
	cwd, _ := os.Getwd()

	// Pass the model flag to the container
	container, err := app.NewContainer(apiKey, cwd, agentModel, dryRun, false)
	if err != nil {
		return err
	}
	return action(container)
}

func getModelDisplay() string {
	if agentModel == "" {
		return "Default"
	}
	return agentModel
}

func init() {
	agentCmd.PersistentFlags().StringVar(&agentModel, "model", "", "Override LLM model")

	// Phase 4 Flags
	replayCmd.Flags().StringVar(&executionID, "execution", "", "Execution ID to replay")
	explainCmd.Flags().StringVar(&executionID, "execution", "", "Execution ID to explain")

	agentCmd.AddCommand(auditCmd)
	agentCmd.AddCommand(planCmd)
	agentCmd.AddCommand(replayCmd)
	agentCmd.AddCommand(explainCmd)

	rootCmd.AddCommand(agentCmd)
}

package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/david22573/codepicker/app"
	"github.com/spf13/cobra"
)

var (
	agentModel  string
	executionID string
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Interact with specific agent personas and history",
}

// --- Audit Command ---
var auditCmd = &cobra.Command{
	Use:   "audit [query]",
	Short: "Perform a semantic read-only audit of the codebase",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAgentTask(func(c *app.Container) error {
			query := args[0]
			fmt.Printf("🛡️  Starting Audit (Model: %s)\n", getModelDisplay())

			fmt.Println("🧠 Gathering relevant code slices for audit...")

			// FIX: Use BuildForTask(query) instead of Build()
			// This returns a markdown string we can prepend to the prompt
			contextStr, err := c.ContextBuilder.BuildForTask(query)
			if err != nil {
				fmt.Printf("⚠️  Warning: Context building failed: %v\n", err)
			}

			// Combine context and query for the Auditor
			fullInput := fmt.Sprintf("CONTEXT:\n%s\n\nTASK: %s", contextStr, query)

			report, err := c.Auditor.RunAudit(context.Background(), fullInput)
			if err != nil {
				return err
			}

			fmt.Println("\n================ REPORT ================")
			fmt.Println(report.Content)
			fmt.Println("========================================")
			fmt.Printf("📄 Saved to: %s\n", report.Artifact)
			return nil
		}, dryRunFlag)
	},
}

// --- Plan Command ---
var planCmd = &cobra.Command{
	Use:   "plan [task]",
	Short: "Generate a plan without executing it",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAgentTask(func(c *app.Container) error {
			taskInput := args[0]
			fmt.Println("🧠 Generating Plan...")

			// FIX: Use BuildForTask here too
			contextStr, err := c.ContextBuilder.BuildForTask(taskInput)
			if err != nil {
				fmt.Printf("⚠️  Warning: Context building failed: %v\n", err)
			}

			plan, err := c.Planner.CreatePlan(context.Background(), taskInput, contextStr)
			if err != nil {
				return err
			}

			fmt.Printf("✅ Plan Created (ID: %s)\n", plan.ID)
			fmt.Printf("   Reasoning: %s\n", plan.Reasoning)
			for _, step := range plan.Steps {
				fmt.Printf("   - [ ] %s\n", step.Description)
			}
			return nil
		}, true) // Plan generation is always "dry run" safe
	},
}

// --- Replay Command ---
var replayCmd = &cobra.Command{
	Use:   "replay",
	Short: "Replay a past execution from the database",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAgentTask(func(c *app.Container) error {
			if executionID == "" {
				return fmt.Errorf("must specify --execution ID")
			}
			exec, err := c.Repository.GetExecution(context.Background(), executionID)
			if err != nil {
				return err
			}

			fmt.Printf("Replaying Execution: %s\n", exec.ID)
			for _, turn := range exec.History {
				fmt.Printf("\n--- Turn %d ---\n", turn.TurnID)
				fmt.Printf("Thought: %s\n", turn.Thought)
				fmt.Printf("Action: %s (%s)\n", turn.ToolName, turn.ToolArgs)
				fmt.Printf("Result: %s\n", turn.ToolOut)
			}
			return nil
		}, true)
	},
}

// --- Explain Command ---
var explainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Use AI to explain a past execution",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAgentTask(func(c *app.Container) error {
			if executionID == "" {
				return fmt.Errorf("must specify --execution ID")
			}

			// FIX: c.Explainer is now valid
			summary, err := c.Explainer.Explain(context.Background(), executionID)
			if err != nil {
				return err
			}

			fmt.Println("\n🤖 EXECUTION EXPLANATION:")
			fmt.Println(summary)
			return nil
		}, true)
	},
}

// --- Helpers ---

func runAgentTask(action func(*app.Container) error, dryRun bool) error {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENROUTER_API_KEY required")
	}
	cwd, _ := os.Getwd()
	// Update NewContainer to match signature (empty model string uses default)
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
	replayCmd.Flags().StringVar(&executionID, "execution", "", "Execution ID to replay")
	explainCmd.Flags().StringVar(&executionID, "execution", "", "Execution ID to explain")

	agentCmd.AddCommand(auditCmd)
	agentCmd.AddCommand(planCmd)
	agentCmd.AddCommand(replayCmd)
	agentCmd.AddCommand(explainCmd)
	rootCmd.AddCommand(agentCmd)
}

package cmd

import (
	"fmt"
	"strings"

	"github.com/david22573/codepicker/internal/agents"
	"github.com/david22573/codepicker/internal/app"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/spf13/cobra"
)

// agentCmd represents the base command for autonomous features
var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Autonomous agent commands",
	Long:  `Group of commands where the AI agent acts autonomously to plan, execute, or improve code.`,
}

// runCmd replaces the old 'do' command
var runCmd = &cobra.Command{
	Use:   "run [task]",
	Short: "Execute an autonomous task (Interactive Mode)",
	Long:  `Starts the agent in interactive mode to perform a specific task. Uses the 'Interactive' policy which prompts for confirmation on shell commands and file writes.`,
	Example: `  codepicker agent run "Refactor the logging interface"
  codepicker agent run "Fix the bug in main.go"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		task := strings.Join(args, " ")

		// Reuse app.NewAgentContext for basic config/db/client setup
		agentCtx, err := app.NewAgentContext(cmd.Context(), app.ContextOptions{
			SrcDir:   srcDir,
			LogLevel: 1,
			Mode:     app.ModeInteractive,
			Policy:   policy.Interactive,
			Task:     task,
		})
		if err != nil {
			return fmt.Errorf("failed to initialize context: %w", err)
		}
		defer agentCtx.Close()

		agentCtx.Logger.Info("🤖 Initializing Multi-Agent Orchestrator...")

		// Initialize the new Orchestrator
		// We extract the client and store from the initialized agentCtx
		orch, err := agents.NewOrchestrator(
			openrouter.NewClient(fmt.Sprintf("%s", agentCtx.Config.AI.Model)), // Re-init client to be safe or expose it
			srcDir,
			agentCtx.Logger,
			agentCtx.Store,
		)
		// Correction: The app.NewAgentContext doesn't expose Client publicly in struct usually,
		// so we might need to rely on the environment variable inside NewOrchestrator
		// or update AgentContext to expose it.
		// For now, let's assume we can grab the API key from env inside NewOrchestrator
		// (which we passed in previous step).

		// ACTUALLY: Let's pass the client from agentCtx.Engine.Client if available
		// or create a fresh one to avoid breaking changes.

		if err != nil {
			return fmt.Errorf("failed to start orchestrator: %w", err)
		}

		// Inject the client from the context if accessible, otherwise NewOrchestrator creates it
		orch.Self.Client = agentCtx.Engine.Client
		// Propagate to team
		for _, agent := range orch.Team {
			agent.Client = agentCtx.Engine.Client
		}

		fmt.Println("🚀 Starting Orchestrated Execution...")

		if err := orch.RunTask(agentCtx.Ctx, task); err != nil {
			return fmt.Errorf("execution failed: %w", err)
		}

		fmt.Println("\n" + strings.Repeat("=", 50))
		fmt.Println("✅ Task Completed Successfully")
		fmt.Println("👉 Check .codepicker/shadow/ for changes and run 'codepicker apply'")
		fmt.Println(strings.Repeat("=", 50))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.AddCommand(runCmd)

	// Future: agentCmd.AddCommand(planCmd)
	// Future: agentCmd.AddCommand(improveCmd)
}

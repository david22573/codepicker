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

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Autonomous agent commands",
	Long:  `Group of commands where the AI agent acts autonomously to plan, execute, or improve code.`,
}

var runCmd = &cobra.Command{
	Use:   "run [task]",
	Short: "Execute an autonomous task (Interactive Mode)",
	Long:  `Starts the agent in interactive mode to perform a specific task. Uses the 'Interactive' policy which prompts for confirmation on shell commands and file writes.`,
	Example: `  codepicker agent run "Refactor the logging interface"
  codepicker agent run "Fix the bug in main.go"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		task := strings.Join(args, " ")

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

		orch, err := agents.NewOrchestrator(
			openrouter.NewClient(fmt.Sprintf("%s", agentCtx.Config.GetModel())),
			srcDir,
			agentCtx.Logger,
			agentCtx.Store,
			agentCtx.Config, // Pass Explicit Config
		)

		if err != nil {
			return fmt.Errorf("failed to start orchestrator: %w", err)
		}

		orch.Self.Client = agentCtx.Engine.Client

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

}

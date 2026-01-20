package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/internal/agent" // Import core agent package
	"github.com/david22573/codepicker/internal/agents"
	"github.com/david22573/codepicker/internal/app"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/internal/ui" // Use UI package
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/spf13/cobra"
)

var (
	planID string
	dryRun bool
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Autonomous agent commands",
	Long:  `Group of commands where the AI agent acts autonomously to plan, execute, or improve code.`,
}

var runCmd = &cobra.Command{
	Use:   "run [task]",
	Short: "Execute an autonomous task or a saved plan",
	Long:  `Starts the agent to perform a specific task. Can either generate a new dynamic plan (default) or execute a saved plan using --plan.`,
	Example: `  codepicker agent run "Refactor the logging interface"
  codepicker agent run --plan 123e4567-e89b-12d3-a456-426614174000`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Initialize UI
		if ui.Standard == nil {
			ui.Standard = ui.NewConsoleUI()
		}

		// Validation: Need either a task string OR a plan ID
		if len(args) == 0 && planID == "" {
			return fmt.Errorf("requires either a task description or a --plan ID")
		}

		task := strings.Join(args, " ")
		if planID != "" {
			task = fmt.Sprintf("Execute Plan %s", planID)
		}

		// Initialize Context
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

		// --- PATH A: Execute Saved Plan ---
		if planID != "" {
			return runSavedPlan(agentCtx, planID)
		}

		// --- PATH B: Dynamic Orchestration (Default) ---
		return runOrchestrator(agentCtx, task)
	},
}

// runSavedPlan loads a plan from DB and executes it
func runSavedPlan(ctx *app.AgentContext, id string) error {
	ui.Standard.Info("📂 Loading plan %s from database...", id)

	record, err := ctx.Store.GetPlan(id)
	if err != nil {
		return fmt.Errorf("plan not found: %w", err)
	}

	var steps []agent.Step
	if err := json.Unmarshal([]byte(record.StepsJSON), &steps); err != nil {
		return fmt.Errorf("corrupt plan data: %w", err)
	}

	// Reconstruct the Plan object
	plan := &agent.Plan{
		ID:            record.ID,
		OriginalTask:  record.Task,
		Steps:         steps,
		EstimatedCost: record.EstimatedCost,
	}

	ui.Standard.Info("🚀 Resuming execution of plan: %s", plan.OriginalTask)

	// Create Executor
	executor := agent.NewPlanExecutor(ctx.Engine, plan)

	// Execute (this automatically skips 'completed' steps)
	if err := executor.Execute(ctx.Ctx); err != nil {
		return err
	}

	ui.Standard.Success("Plan completed successfully.")
	return nil
}

// runOrchestrator spins up the multi-agent team
func runOrchestrator(ctx *app.AgentContext, task string) error {
	ctx.Logger.Info("🤖 Initializing Multi-Agent Orchestrator...")

	orch, err := agents.NewOrchestrator(
		openrouter.NewClient(fmt.Sprintf("%s", ctx.Config.GetModel())),
		srcDir,
		ctx.Logger,
		ctx.Store,
		ctx.Config,
	)

	if err != nil {
		return fmt.Errorf("failed to start orchestrator: %w", err)
	}

	// Share the engine's HTTP client to reuse connections/config
	orch.Self.Client = ctx.Engine.Client
	for _, a := range orch.Team {
		a.Client = ctx.Engine.Client
	}

	ctx.Engine.Executor.DryRun = dryRun

	fmt.Println("🚀 Starting Orchestrated Execution...")

	if err := orch.RunTask(ctx.Ctx, task); err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	ui.Standard.Success("Task Completed Successfully")
	ui.Standard.Info("👉 Check .codepicker/shadow/ for changes and run 'codepicker apply'")

	return nil
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.AddCommand(runCmd)
	runCmd.Flags().BoolVarP(&dryRun, "dry-run", "D", false, "Simulate execution without making changes")
	runCmd.Flags().StringVarP(&planID, "plan", "p", "", "Resume/Execute a specific plan ID")
}

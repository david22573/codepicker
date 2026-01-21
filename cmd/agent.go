package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/app"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/internal/ui"
	"github.com/olekukonko/tablewriter"
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

		if ui.Standard == nil {
			ui.Standard = ui.NewConsoleUI()
		}

		if len(args) == 0 && planID == "" {
			return fmt.Errorf("requires either a task description or a --plan ID")
		}

		task := strings.Join(args, " ")
		if planID != "" {
			task = fmt.Sprintf("Execute Plan %s", planID)
		}

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

		if planID != "" {
			return runSavedPlan(agentCtx, planID)
		}

		return runOrchestrator(agentCtx, task)
	},
}

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

	plan := &agent.Plan{
		ID:            record.ID,
		OriginalTask:  record.Task,
		Steps:         steps,
		EstimatedCost: record.EstimatedCost,
	}

	ui.Standard.Info("🚀 Resuming execution of plan: %s", plan.OriginalTask)

	executor := agent.NewPlanExecutor(ctx.Engine, plan)

	if err := executor.Execute(ctx.Ctx); err != nil {
		return err
	}

	ui.Standard.Success("Plan completed successfully.")
	return nil
}

func runOrchestrator(ctx *app.AgentContext, task string) error {
	ctx.Logger.Info("🤖 Initializing Multi-Agent Orchestrator...")

	orch, err := agent.NewOrchestrator(
		ctx.Engine.Client,
		srcDir,
		ctx.Logger,
		ctx.Store,
		ctx.Config,
	)

	if err != nil {
		return fmt.Errorf("failed to start orchestrator: %w", err)
	}

	orch.PlanReviewHandler = func(plan *agent.ExecutionPlan) bool {
		fmt.Println("\n" + strings.Repeat("=", 60))
		fmt.Println("📋 PROPOSED EXECUTION PLAN")
		fmt.Println(strings.Repeat("=", 60))

		table := tablewriter.NewWriter(os.Stdout)
		table.Header([]string{"Step", "Agent", "Task"})

		for i, step := range plan.Steps {
			displayTask := step.Task
			if len(displayTask) > 60 {
				displayTask = displayTask[:57] + "..."
			}
			table.Append([]string{
				fmt.Sprintf("%d", i+1),
				string(step.Agent),
				displayTask,
			})
		}
		table.Render()
		fmt.Println(strings.Repeat("=", 60))

		return ui.Standard.Confirm("Execute this plan?", true)
	}

	// Phase 3.2: Error Recovery Hook with Analysis
	orch.StepErrorHandler = func(step agent.PlanStep, err error, analysis string) string {
		ui.Standard.Error("\n🛑 Step failed: %v", err)

		fmt.Println(strings.Repeat("-", 60))
		fmt.Println("🤖 ORCHESTRATOR ADVICE:")
		fmt.Println(analysis)
		fmt.Println(strings.Repeat("-", 60))

		fmt.Println("\nRecover Strategy:")
		choices := []string{
			"Retry (Run step again - use if close)",
			"Skip (Mark done - use if stuck)",
			"Abort (Exit)"}

		choice, _, _ := ui.Standard.Select("How to proceed?", choices)

		switch choice {
		case 0:
			return "retry"
		case 1:
			return "skip"
		default:
			return "fail"
		}
	}

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

package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/app"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/internal/tui"
	"github.com/david22573/codepicker/internal/ui"
	"github.com/spf13/cobra"
)

var (
	planID string
	dryRun bool
	ciMode bool // Phase 4: CI Mode flag
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
  codepicker agent run --plan 123e4567-e89b-12d3-a456-426614174000 --ci`,
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

		// Phase 4: Force strict policy in CI mode
		currentPolicy := policy.Interactive
		if ciMode {
			currentPolicy = policy.Batch // No shell prompts in CI
		}

		agentCtx, err := app.NewAgentContext(cmd.Context(), app.ContextOptions{
			SrcDir:   srcDir,
			LogLevel: 1,
			Mode:     app.ModeInteractive,
			Policy:   currentPolicy,
			Task:     task,
		})
		if err != nil {
			return fmt.Errorf("failed to initialize context: %w", err)
		}
		defer agentCtx.Close()

		// Phase 4: Cost Visibility on Exit
		defer func() {
			cost, count := agentCtx.Engine.CostTracker.GetStats()
			fmt.Println("\n" + strings.Repeat("-", 40))
			fmt.Printf("💰 Session Cost Summary:\n")
			fmt.Printf("   Requests: %d\n", count)
			fmt.Printf("   Total:    $%.4f\n", cost)
			fmt.Println(strings.Repeat("-", 40))
		}()

		if planID != "" {
			return runSavedPlan(agentCtx, planID)
		}

		if ciMode {
			return runHeadlessOrchestrator(agentCtx, task)
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

// Phase 4: Headless Orchestrator for CI/Logs
func runHeadlessOrchestrator(ctx *app.AgentContext, task string) error {
	ui.Standard.Info("🤖 Starting Headless Agent (CI Mode)")

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

	// Simple stdout observer
	orch.Observer = func(eventType, content string) {
		switch eventType {
		case "thought":
			fmt.Printf("[🧠] %s\n", content)
		case "tool_start":
			fmt.Printf("[⚙️] Executing: %s\n", content)
		case "step":
			fmt.Printf("\n▶ STEP: %s\n", content)
		case "error":
			fmt.Printf("[❌] ERROR: %s\n", content)
		}
	}

	// Auto-approve plans in CI
	orch.PlanReviewHandler = func(plan *agent.ExecutionPlan) bool {
		fmt.Println("📋 Plan generated. Auto-approving (CI Mode).")
		return true
	}

	orch.StepErrorHandler = func(step agent.PlanStep, err error, analysis string) string {
		fmt.Printf("❌ Step failed: %v. \nAnalysis: %s\n", err, analysis)
		return "fail" // In CI, we fail fast usually, or could set to 'retry'
	}

	if err := orch.RunTask(ctx.Ctx, task); err != nil {
		return err
	}

	ui.Standard.Success("Headless execution finished.")
	return nil
}

func runOrchestrator(ctx *app.AgentContext, task string) error {

	feedModel := tui.NewFeedModel()

	p := tea.NewProgram(feedModel, tea.WithAltScreen())

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

	orch.Observer = func(eventType, content string) {
		feedModel.EventChan <- tui.AgentEvent{Type: eventType, Content: content}
	}

	orch.PlanReviewHandler = func(plan *agent.ExecutionPlan) bool {
		// TUI Review
		return tui.ReviewPlan(plan)
	}

	orch.StepErrorHandler = func(step agent.PlanStep, err error, analysis string) string {
		feedModel.EventChan <- tui.AgentEvent{Type: "error", Content: err.Error()}
		ui.RenderMarkdown(fmt.Sprintf("\n### 🤖 Orchestrator Advice\n%s\n", analysis))
		return "fail"
	}

	go func() {
		defer close(feedModel.DoneChan)

		if err := orch.RunTask(ctx.Ctx, task); err != nil {
			feedModel.EventChan <- tui.AgentEvent{Type: "error", Content: err.Error()}
		} else {
			feedModel.EventChan <- tui.AgentEvent{Type: "step", Content: "Task Completed Successfully"}
		}
	}()

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("UI error: %w", err)
	}

	ui.Standard.Success("Execution finished.")
	ui.Standard.Info("👉 Check .codepicker/shadow/ for changes and run 'codepicker apply'")

	return nil
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.AddCommand(runCmd)
	runCmd.Flags().BoolVarP(&dryRun, "dry-run", "D", false, "Simulate execution without making changes")
	runCmd.Flags().StringVarP(&planID, "plan", "p", "", "Resume/Execute a specific plan ID")
	// Phase 4: CI Flag
	runCmd.Flags().BoolVar(&ciMode, "ci", false, "Run in CI mode (headless, auto-approve, strict policy)")
}

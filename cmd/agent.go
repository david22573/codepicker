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

		// Initialize context
		// Note: We use LogLevel 1 (Info), but the TUI will mostly hide stderr logs
		// unless they are critical errors.
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

// runSavedPlan executes a plan directly from the database
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

// runOrchestrator handles the full lifecycle: Planning -> Review -> Execution -> TUI
func runOrchestrator(ctx *app.AgentContext, task string) error {

	// 1. Initialize the Activity Feed Model
	feedModel := tui.NewFeedModel()

	// 2. Start the Bubble Tea program for the feed
	// We use WithAltScreen to take over the terminal window nicely
	p := tea.NewProgram(feedModel, tea.WithAltScreen())

	// 3. Initialize the Orchestrator
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

	// 4. Wire up the Observer to pipe events to the TUI
	orch.Observer = func(eventType, content string) {
		feedModel.EventChan <- tui.AgentEvent{Type: eventType, Content: content}
	}

	// 5. Handle Plan Review
	// NOTE: This runs inside the goroutine below. The TUI (p) is already running.
	// For a cleaner UX in the future, we might pause the Feed and show the Plan.
	// For now, we print to stdout which might interleave, or use the TUI's logic if expanded.
	// To avoid graphical glitches, we'll log it to the feed for now or rely on the console.
	orch.PlanReviewHandler = func(plan *agent.ExecutionPlan) bool {
		// Ideally: p.ReleaseTerminal() -> ReviewPlan() -> p.RestoreTerminal()
		// But ReviewPlan starts its own tea.Program.

		// For this specific iteration, we will rely on tui.ReviewPlan handling the display.
		// If Bubble Tea conflicts occur, we might need to stop 'p' and restart it.
		// Let's try the direct approach:
		return tui.ReviewPlan(plan)
	}

	// 6. Handle Step Errors
	orch.StepErrorHandler = func(step agent.PlanStep, err error, analysis string) string {
		// Send error to the visual feed
		feedModel.EventChan <- tui.AgentEvent{Type: "error", Content: err.Error()}

		ui.RenderMarkdown(fmt.Sprintf("\n### 🤖 Orchestrator Advice\n%s\n", analysis))

		// In a headless run, we might just fail.
		// In interactive, we might prompt. For now, auto-retry once or fail.
		return "fail"
	}

	// 7. Run the Agent Logic in a separate goroutine so the TUI can render
	go func() {
		// Signal TUI to stop when function exits
		defer close(feedModel.DoneChan)

		if err := orch.RunTask(ctx.Ctx, task); err != nil {
			feedModel.EventChan <- tui.AgentEvent{Type: "error", Content: err.Error()}
		} else {
			feedModel.EventChan <- tui.AgentEvent{Type: "step", Content: "Task Completed Successfully"}
		}
	}()

	// 8. Block until TUI finishes
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("UI error: %w", err)
	}

	// 9. Post-Run Summary
	ui.Standard.Success("Execution finished.")
	ui.Standard.Info("👉 Check .codepicker/shadow/ for changes and run 'codepicker apply'")

	return nil
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.AddCommand(runCmd)
	runCmd.Flags().BoolVarP(&dryRun, "dry-run", "D", false, "Simulate execution without making changes")
	runCmd.Flags().StringVarP(&planID, "plan", "p", "", "Resume/Execute a specific plan ID")
}

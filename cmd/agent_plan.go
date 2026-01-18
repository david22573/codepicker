package cmd

import (
	"fmt"
	"strings"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/app"
	"github.com/david22573/codepicker/internal/contextgen"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var (
	isArchitect  bool
	executeAfter bool
)

var planCmd = &cobra.Command{
	Use:   "plan [task]",
	Short: "Generate an execution plan or audit the codebase",
	Long:  `Analyzes the codebase to create a step-by-step plan for a specific task, or performs a high-level architectural audit if --architect is used.`,
	Example: `  codepicker agent plan "Refactor the database layer"
  codepicker agent plan --architect`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Handle Architect Mode (Audit)
		if isArchitect {
			return runArchitectAudit(cmd)
		}

		// 2. Handle Standard Planning
		if len(args) < 1 {
			return fmt.Errorf("task description required (or use --architect)")
		}
		task := strings.Join(args, " ")
		return runStandardPlan(cmd, task)
	},
}

func runArchitectAudit(cmd *cobra.Command) error {
	// Architect Policy is Read-Only and Strict
	agentCtx, err := app.NewAgentContext(cmd.Context(), app.ContextOptions{
		SrcDir:   srcDir,
		LogLevel: 1,
		Mode:     app.ModeInteractive,
		Policy:   policy.Architect,
		Task:     "Architectural Audit",
	})
	if err != nil {
		return err
	}
	defer agentCtx.Close()

	agentCtx.Logger.Info("🏗️  Starting Architecture Audit...")

	// Inject the file tree into the prompt manually since we aren't using the standard runner loop
	tree, err := contextgen.GenerateTree(agentCtx.SrcDir)
	if err != nil {
		return err
	}

	// Override the system prompt for the audit
	// Note: We access the internal engine here.
	// In the future, "Audit" could be its own Agent Type, but this works for now.
	agentCtx.Engine.SystemPrompt = agent.ArchitectPrompt + "\n\n" + tree

	// Output handler
	printUpdate := func(msg openrouter.ChatMessage) {
		if msg.Role == "assistant" && msg.Content != nil {
			content := fmt.Sprintf("%v", msg.Content)
			if content != "" && !strings.Contains(content, "tool_calls") {
				fmt.Printf("🤖 Thought: %s\n", content)
			}
		}
	}

	task := "Perform a deep audit of this codebase. Output your findings to ARCHITECTURE_GOALS.md."
	result, err := agentCtx.Engine.Run(agentCtx.Ctx, task, printUpdate)
	if err != nil {
		return err
	}

	fmt.Printf("\n✅ Audit Complete.\n%s\n", result)
	fmt.Println("\n👉 Run 'codepicker apply' to save the ARCHITECTURE_GOALS.md file.")
	return nil
}

func runStandardPlan(cmd *cobra.Command, task string) error {
	// Standard Planner uses a specialized AgentContext just for reading
	agentCtx, err := app.NewAgentContext(cmd.Context(), app.ContextOptions{
		SrcDir:   srcDir,
		LogLevel: 1,
		Mode:     app.ModeInteractive,
		Policy:   policy.Batch, // Planning is safe/automated
		Task:     task,
	})
	if err != nil {
		return err
	}
	defer agentCtx.Close()

	agentCtx.Logger.Info("🗺️  Mapping codebase for planning...")
	projectTree, err := contextgen.GenerateTree(agentCtx.SrcDir)
	if err != nil {
		return err
	}

	planner := agent.NewPlanner(agentCtx.Engine.Client, agentCtx.Engine.Model, agentCtx.Logger)

	fmt.Println("🤔 Analyzing task and generating plan...")
	plan, err := planner.CreatePlan(cmd.Context(), task, projectTree)
	if err != nil {
		return err
	}

	// Save to DB
	if err := agentCtx.Store.SavePlan(plan.ID, task, plan.Steps, plan.EstimatedCost); err != nil {
		agentCtx.Logger.Warn("Failed to save plan to database: " + err.Error())
	}

	// Render Output
	printPlanTable(plan)

	// Optional: Immediate Execution
	if executeAfter {
		fmt.Println("\n🚀 Executing plan immediately...")

		// Re-initialize engine limits for execution
		// In a perfect world, we'd reuse the context, but the Planner isn't full engine yet.
		executor := agent.NewPlanExecutor(agentCtx.Engine, plan)
		if err := executor.Execute(cmd.Context()); err != nil {
			return err
		}
		fmt.Println("\n✅ Plan completed successfully.")
	} else {
		fmt.Printf("\n👉 To execute this plan, run: codepicker agent run --plan %s\n", plan.ID)
		// NOTE: You will need to add --plan support to agent run in a future step!
	}

	return nil
}

func printPlanTable(plan *agent.Plan) {
	fmt.Printf("\n📋 Plan ID: %s\n", plan.ID)
	fmt.Printf("💡 Reasoning: %s\n", plan.Reasoning)
	fmt.Printf("💰 Est. Cost: $%.4f\n\n", plan.EstimatedCost)

	table := tablewriter.NewWriter(agentCmd.OutOrStdout())
	table.Header([]string{"ID", "Description", "Files", "Critical"})

	for _, step := range plan.Steps {
		crit := ""
		if step.Critical {
			crit = "Yes"
		}
		files := strings.Join(step.Files, ", ")
		if len(files) > 30 {
			files = files[:27] + "..."
		}
		table.Append([]string{
			fmt.Sprintf("%d", step.ID),
			step.Description,
			files,
			crit,
		})
	}
	table.Render()
}

func init() {
	agentCmd.AddCommand(planCmd)
	planCmd.Flags().BoolVar(&isArchitect, "architect", false, "Run in Architect Mode to audit the codebase")
	planCmd.Flags().BoolVarP(&executeAfter, "execute", "x", false, "Immediately execute the generated plan")
}

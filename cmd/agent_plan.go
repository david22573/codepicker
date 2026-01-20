package cmd

import (
	"fmt"
	"strings"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/app"
	"github.com/david22573/codepicker/internal/contextgen"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/internal/prompts"
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
	agentCtx, err := app.NewAgentContext(cmd.Context(), app.ContextOptions{
		SrcDir:   srcDir,
		LogLevel: 1,
		Mode:     app.ModeInteractive,
		Policy:   policy.Batch, // ⚠️ CHANGED from policy.Architect
		Task:     "Architectural Audit",
	})
	if err != nil {
		return err
	}
	defer agentCtx.Close()

	agentCtx.Logger.Info("🏗️  Starting Architecture Audit...")

	// Generate project tree for context
	tree, err := contextgen.GenerateTree(agentCtx.SrcDir)
	if err != nil {
		return err
	}

	// Set the improved prompt
	agentCtx.Engine.SystemPrompt = prompts.ArchitectV2 + "\n\n" + tree

	// Track completion and goals file
	auditComplete := false
	goalsFileWritten := false
	turnCount := 0
	maxTurns := 12 // Lower limit for focused audit

	printUpdate := func(msg openrouter.ChatMessage) {
		// Check for completion signal
		if msg.Role == "assistant" && msg.Content != nil {
			content := fmt.Sprintf("%v", msg.Content)

			if strings.Contains(content, "AUDIT_COMPLETE") {
				auditComplete = true
			}

			// Show thoughts (but not tool_calls JSON)
			if content != "" && !strings.Contains(content, "tool_calls") {
				fmt.Printf("\n🤖 Thought: %s\n", content)
			}
		}

		// Track when goals file is written
		if msg.Role == "tool" {
			toolContent := fmt.Sprintf("%v", msg.Content)
			if strings.Contains(toolContent, "ARCHITECTURE_GOALS.md") {
				goalsFileWritten = true
				fmt.Println("📝 Goals file written to shadow workspace")
			}
			// Show tool results (truncated)
			if len(toolContent) > 200 {
				toolContent = toolContent[:200] + "..."
			}
			fmt.Printf("   🔧 %s\n", toolContent)
		}
	}

	// Main audit loop
	task := "Begin the architectural audit following the workflow in your instructions."

	for turnCount < maxTurns {
		turnCount++

		fmt.Printf("\n[Turn %d/%d] ", turnCount, maxTurns)

		result, err := agentCtx.Engine.RunSingleTurn(agentCtx.Ctx, task, printUpdate)
		if err != nil {
			return fmt.Errorf("turn %d failed: %w", turnCount, err)
		}

		// Check for explicit completion
		if auditComplete {
			fmt.Println("\n✅ Agent signaled completion")
			break
		}

		// Early exit if goals written and no more activity
		if goalsFileWritten && strings.TrimSpace(result) == "" {
			fmt.Println("\n✅ Goals file written, agent idle")
			auditComplete = true
			break
		}

		// Update task for next turn
		if goalsFileWritten {
			task = "You have written the goals file. Respond with 'AUDIT_COMPLETE'."
		} else {
			task = "Continue the audit. If you're done with discovery, write the ARCHITECTURE_GOALS.md file now."
		}
	}

	// Validate completion
	if !goalsFileWritten {
		return fmt.Errorf("audit failed: ARCHITECTURE_GOALS.md was never created (used %d turns)", turnCount)
	}

	if !auditComplete && turnCount >= maxTurns {
		agentCtx.Logger.Warn(fmt.Sprintf("Audit hit turn limit (%d) but goals file was written", maxTurns))
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✅ Audit Complete")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Turns used: %d/%d\n", turnCount, maxTurns)

	// Show cost
	cost, count := agentCtx.Engine.CostTracker.GetStats()
	fmt.Printf("API calls: %d | Cost: $%.4f\n", count, cost)

	fmt.Println("\n👉 Next steps:")
	fmt.Println("   1. Run 'codepicker apply' to review the goals")
	fmt.Println("   2. Run 'codepicker agent improve' to execute the first task")

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

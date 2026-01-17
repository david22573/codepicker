package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/constants"
	"github.com/david22573/codepicker/internal/contextgen"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var (
	executePlan bool
)

var planCmd = &cobra.Command{
	Use:   "plan [task]",
	Short: "Generate an AI execution plan for a task",
	Long:  `Analyzes your codebase and creates a step-by-step plan to accomplish the task. You can review the plan before execution.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		task := strings.Join(args, " ")

		apiKey, err := validateAPIKey()
		if err != nil {
			return err
		}

		absSrc, err := filepath.Abs(srcDir)
		if err != nil {
			return err
		}

		// 1. Initialize Infrastructure
		store, err := database.New(".codepicker")
		if err != nil {
			return fmt.Errorf("failed to init database: %w", err)
		}
		defer store.Close()

		client := openrouter.NewClient(apiKey)
		appLogger.Info("🗺️  Mapping codebase for planning...")

		// Generate lightweight tree context for the planner
		projectTree, err := contextgen.GenerateTree(absSrc)
		if err != nil {
			return err
		}

		// 2. Generate Plan
		planner := agent.NewPlanner(client, constants.DefaultModel, appLogger)

		fmt.Println("🤔 Analyzing task and generating plan...")
		plan, err := planner.CreatePlan(cmd.Context(), task, projectTree)
		if err != nil {
			return err
		}

		// 3. Save Plan
		if err := store.SavePlan(plan.ID, task, plan.Steps, plan.EstimatedCost); err != nil {
			appLogger.Warn("Failed to save plan to database: " + err.Error())
		}

		// 4. Display Plan
		fmt.Printf("\n📋 Plan ID: %s\n", plan.ID)
		fmt.Printf("💡 Reasoning: %s\n", plan.Reasoning)
		fmt.Printf("💰 Est. Cost: $%.4f\n\n", plan.EstimatedCost)

		table := tablewriter.NewWriter(os.Stdout)
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

		// 5. Interactive Approval
		if !executePlan {
			fmt.Print("\n🚀 Execute this plan now? [y/N]: ")
			scanner := bufio.NewScanner(os.Stdin)
			if scanner.Scan() {
				resp := strings.ToLower(strings.TrimSpace(scanner.Text()))
				if resp != "y" && resp != "yes" {
					fmt.Println("👋 Plan saved but not executed.")
					return nil
				}
				executePlan = true
			}
		}

		// 6. Execute Plan
		if executePlan {
			limits := config.DefaultLimits()
			// Init the heavy engine for execution
			eng, err := agent.NewEngine(client, constants.DefaultModel, absSrc, appLogger, limits, store)
			if err != nil {
				return err
			}

			// Define approval callback (auto-approve tools during plan execution for smoother flow,
			// or ask user. For Phase 1, we'll ask user to be safe).
			eng.ApprovalCallback = func(c, r string) bool {
				fmt.Printf("\n⚠️  Agent wants to run: %s\n   Reason: %s\n   Allow? [Y/n]: ", c, r)
				var resp string
				fmt.Scanln(&resp)
				return resp == "" || resp == "y" || resp == "Y"
			}

			executor := agent.NewPlanExecutor(eng, plan)
			if err := executor.Execute(cmd.Context()); err != nil {
				return err
			}
			fmt.Println("\n✅ Plan completed successfully. Check .codepicker/shadow for changes.")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(planCmd)
	planCmd.Flags().BoolVarP(&executePlan, "execute", "x", false, "Immediately execute the generated plan without prompting")
}

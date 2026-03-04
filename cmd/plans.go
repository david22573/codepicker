package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/david22573/codepicker/app"
	"github.com/david22573/codepicker/domain/task"
	"github.com/david22573/codepicker/infra/ui"
	"github.com/spf13/cobra"
)

var plansCmd = &cobra.Command{
	Use:   "plans [plan_id]",
	Short: "List plans or preview a specific plan",
	RunE: func(cmd *cobra.Command, args []string) error {
		apiKey := getAPIKeyOrExit()
		cwd, _ := os.Getwd()

		container, err := app.NewContainer(apiKey, cwd, "", false, false, GetVerbose())
		if err != nil {
			return fmt.Errorf("failed to initialize: %w", err)
		}
		defer container.Close()

		ctx := cmd.Context()

		if len(args) > 0 {
			previewPlan(ctx, container, args[0])
		} else {
			printDashboard(ctx, container)
		}

		return nil
	},
}

func previewPlan(ctx context.Context, c *app.Container, planID string) {
	plan, err := c.Repository.GetPlan(ctx, planID)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Plan not found: %s", planID))
		return
	}

	ui.PrintHeader("PLAN PREVIEW: " + plan.ID)
	fmt.Printf("🎯 Task: %s\n", plan.OriginalTask)
	fmt.Printf("🧠 Strategy: %s\n", plan.Reasoning)
	fmt.Printf("🚦 Status: %s\n", plan.Status)
	fmt.Println("\nSteps:")

	for _, step := range plan.Steps {
		icon := "⬜"
		if step.Status == task.StatusCompleted {
			icon = "✅"
		} else if step.Status == task.StatusFailed {
			icon = "❌"
		}

		fmt.Printf(" %s Step %d: %s\n", icon, step.ID, step.Description)
		if len(step.Files) > 0 {
			fmt.Printf("    📂 Targets: %v\n", step.Files)
		}
	}
	fmt.Println("\nTo execute this plan, run:")
	fmt.Printf("  codepicker run --plan %s\n", plan.ID)
}

func printDashboard(ctx context.Context, c *app.Container) {
	fmt.Print("\033[H\033[2J")

	summaries, err := c.Repository.ListPlans(ctx, 10)
	if err != nil {
		fmt.Printf("Error loading plans: %v\n", err)
		return
	}

	fmt.Println("===============================================================================")
	fmt.Println("                              CODEPICKER PLANS                                 ")
	fmt.Println("===============================================================================")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tSTEPS\tCREATED\tTASK")
	fmt.Fprintln(w, "--\t------\t-----\t-------\t----")

	for _, s := range summaries {
		statusIcon := "⚪"
		switch s.Status {
		case task.StatusCompleted:
			statusIcon = "🟢"
		case task.StatusRunning:
			statusIcon = "🟠"
		case task.StatusFailed:
			statusIcon = "🔴"
		}

		taskDisplay := s.OriginalTask
		if len(taskDisplay) > 50 {
			taskDisplay = taskDisplay[:47] + "..."
		}

		fmt.Fprintf(w, "%s\t%s %s\t%d\t%s\t%s\n",
			s.ID,
			statusIcon,
			s.Status,
			s.StepCount,
			s.CreatedAt.Format("01-02 15:04"),
			taskDisplay,
		)
	}
	w.Flush()
	fmt.Println("===============================================================================")
	fmt.Println("Use 'codepicker plans <ID>' to preview details.")
}

func init() {
	rootCmd.AddCommand(plansCmd)
}

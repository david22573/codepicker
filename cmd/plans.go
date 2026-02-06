package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/david22573/codepicker/app"
	"github.com/david22573/codepicker/domain/task"
	"github.com/spf13/cobra"
)

var plansCmd = &cobra.Command{
	Use:   "plans",
	Short: "List and manage coding plans",
	Run: func(cmd *cobra.Command, args []string) {
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		cwd, _ := os.Getwd()

		// Initialize container to get access to the repository
		container, err := app.NewContainer(apiKey, cwd, "", false, false)
		if err != nil {
			fmt.Printf("❌ Failed to initialize: %v\n", err)
			return
		}

		printDashboard(container)
	},
}

func printDashboard(c *app.Container) {
	// Clear screen for a clean dashboard view
	fmt.Print("\033[H\033[2J")

	// Repository now returns agent.PlanSummary which includes StepCount
	summaries, err := c.Repository.ListPlans(context.Background(), 10)
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

		// Using the fields defined in domain/agent/agent.go: PlanSummary struct
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
	fmt.Println("Use 'codepicker run --plan <ID>' to execute or resume a plan.")
}

func init() {
	rootCmd.AddCommand(plansCmd)
}

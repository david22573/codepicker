package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	contextAdapters "github.com/david22573/codepicker/adapters/context"
	"github.com/david22573/codepicker/app"
	"github.com/david22573/codepicker/domain/task"
	"github.com/spf13/cobra"
)

var plansCmd = &cobra.Command{
	Use:   "plans",
	Short: "Interactive dashboard for plan management",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Initialize container
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("OPENROUTER_API_KEY required")
		}
		cwd, _ := os.Getwd()
		container, err := app.NewContainer(apiKey, cwd, "", false, false)
		if err != nil {
			return err
		}

		// Enter Interactive Loop
		reader := bufio.NewReader(os.Stdin)
		for {
			printDashboard(container)
			fmt.Print("\n(R)un <id> | (O)ptimize <id> | (D)elete <id> | (N)ew | (Q)uit\n> ")

			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			parts := strings.Fields(input)

			if len(parts) == 0 {
				continue
			}

			action := strings.ToLower(parts[0])

			switch action {
			case "q", "quit":
				fmt.Println("Bye!")
				return nil

			case "n", "new":
				fmt.Print("Enter task description: ")
				taskStr, _ := reader.ReadString('\n')

				fmt.Println("🧠 Generating plan...")
				// Contextual Plan Generation
				fileContext, _ := container.ContextBuilder.Build(contextAdapters.Config{
					ProjectRoot: cwd,
					MaxTokens:   2000,
				})

				_, err := container.Planner.CreatePlan(context.Background(), strings.TrimSpace(taskStr), fileContext)
				if err != nil {
					fmt.Printf("Error: %v\n", err)
				}
				time.Sleep(1 * time.Second)

			case "r", "run":
				if len(parts) < 2 {
					fmt.Println("Usage: r <plan_id>")
					continue
				}
				// Execute existing logic from run.go
				// Note: executeRun must be available in package cmd
				if err := executeRun("", parts[1]); err != nil {
					fmt.Printf("Execution failed: %v\n", err)
				}
				fmt.Println("\nPress Enter to return to dashboard...")
				reader.ReadString('\n')

			case "d", "delete":
				if len(parts) < 2 {
					fmt.Println("Usage: d <plan_id>")
					continue
				}
				if err := container.Repository.DeletePlan(context.Background(), parts[1]); err != nil {
					fmt.Printf("Failed to delete: %v\n", err)
				} else {
					fmt.Printf("🗑️  Plan %s deleted.\n", parts[1])
					time.Sleep(1 * time.Second)
				}

			case "o", "optimize":
				if len(parts) < 2 {
					fmt.Println("Usage: o <plan_id>")
					continue
				}
				planID := parts[1]
				plan, err := container.Repository.GetPlan(context.Background(), planID)
				if err != nil {
					fmt.Printf("Plan not found: %v\n", err)
					continue
				}

				fmt.Printf("Current Goal: %s\n", plan.OriginalTask)
				fmt.Print("Enter your feedback/instruction for optimization: ")
				feedback, _ := reader.ReadString('\n')

				fmt.Println("✨ Optimizing plan with AI...")
				newPlan, err := container.Planner.OptimizePlan(context.Background(), plan, strings.TrimSpace(feedback))
				if err != nil {
					fmt.Printf("Optimization failed: %v\n", err)
					continue
				}
				fmt.Printf("✅ Plan updated! New step count: %d\n", len(newPlan.Steps))
				time.Sleep(1 * time.Second)

			default:
				fmt.Println("Unknown command.")
			}
		}
	},
}

func printDashboard(c *app.Container) {
	// Clear screen (ANSI)
	fmt.Print("\033[H\033[2J")

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
		// Shorten ID for display
		shortID := s.ID
		if len(s.ID) > 12 {
			shortID = s.ID // Keep full ID for copy-paste utility, or truncate if preferred
		}

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
			shortID, statusIcon, s.Status, s.StepCount,
			s.CreatedAt.Format("01-02 15:04"), taskDisplay)
	}
	w.Flush()
	fmt.Println("===============================================================================")
}

func init() {
	rootCmd.AddCommand(plansCmd)
}

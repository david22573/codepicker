package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/david22573/codepicker/infra/pathutil"
	"github.com/david22573/codepicker/infra/storage"
	"github.com/spf13/cobra"
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "List past agent execution sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		repo, err := getRepo()
		if err != nil {
			return err
		}

		list, err := repo.ListExecutions(context.Background(), 20)
		if err != nil {
			return fmt.Errorf("failed to fetch history: %w", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "EXEC ID\tSTATUS\tTIME\tPLAN ID")
		fmt.Fprintln(w, "-------\t------\t----\t-------")

		for _, item := range list {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				item.ID,
				item.Status,
				item.StartTime.Format(time.RFC3339),
				item.PlanID,
			)
		}
		w.Flush()
		return nil
	},
}

var inspectCmd = &cobra.Command{
	Use:   "inspect [exec_id]",
	Short: "Replay the timeline of a specific execution",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		repo, err := getRepo()
		if err != nil {
			return err
		}

		exec, err := repo.GetExecution(context.Background(), id)
		if err != nil {
			return fmt.Errorf("failed to load execution: %w", err)
		}

		fmt.Printf("🔍 INSPECTING SESSION: %s\n", exec.ID)
		fmt.Printf("📅 Date: %s\n", exec.StartTime.Format(time.RFC822))
		fmt.Printf("🚦 Final Status: %s\n", exec.Status)
		fmt.Println("===================================================")

		for _, turn := range exec.History {
			fmt.Printf("\n[Turn %d]\n", turn.TurnID)
			fmt.Printf("🧠 Thought: %s\n", turn.Thought)
			if turn.ToolName != "" {
				fmt.Printf("🛠️  Tool: %s\n", turn.ToolName)
				fmt.Printf("📥 Input: %s\n", turn.ToolArgs)
				// Truncate long outputs for readability
				out := turn.ToolOut
				if len(out) > 300 {
					out = out[:300] + "... (truncated)"
				}
				fmt.Printf("📤 Output: %s\n", out)
			} else {
				fmt.Println("🛑 Action: (None/Final Answer)")
			}
			fmt.Println("---")
		}

		return nil
	},
}

// Helper to quickly grab the repo without the full container overhead
func getRepo() (*storage.SQLiteRepository, error) {
	cwd, _ := os.Getwd()
	dbPath := pathutil.GetStateDBPath(cwd)
	return storage.NewSQLiteRepository(dbPath)
}

func init() {
	rootCmd.AddCommand(historyCmd)
	rootCmd.AddCommand(inspectCmd)
}

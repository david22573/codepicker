package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/david22573/codepicker/infra/pathutil"
	"github.com/david22573/codepicker/infra/storage"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var costCmd = &cobra.Command{
	Use:   "cost",
	Short: "Show accumulated LLM usage and costs",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		dbPath := pathutil.GetStateDBPath(cwd)
		repo, err := storage.NewSQLiteRepository(dbPath)
		if err != nil {
			return fmt.Errorf("failed to connect to database: %w", err)
		}
		defer repo.Close()

		cost, tokens, err := repo.GetTotalCost(cmd.Context())
		if err != nil {
			return fmt.Errorf("failed to fetch cost metrics: %w", err)
		}

		if GetJSON() {
			avgCost := 0.0
			if tokens > 0 {
				avgCost = (cost / float64(tokens)) * 1000
			}
			costJSON := map[string]interface{}{
				"total_spend":            cost,
				"total_tokens":           tokens,
				"avg_cost_per_1k_tokens": avgCost,
			}
			jsonData, _ := json.MarshalIndent(costJSON, "", "  ")
			fmt.Println(string(jsonData))
			return nil
		}

		fmt.Println("\n===================================================")
		fmt.Println(color.GreenString(" 💰 CODEPICKER COST DASHBOARD "))
		fmt.Println("===================================================")

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "Total Spend:\t$%.4f\n", cost)
		fmt.Fprintf(w, "Total Tokens:\t%d\n", tokens)

		avgCost := 0.0
		if tokens > 0 {
			avgCost = (cost / float64(tokens)) * 1000
		}
		fmt.Fprintf(w, "Avg Cost/1k Tokens:\t$%.4f\n", avgCost)

		w.Flush()
		fmt.Println("===================================================")
		fmt.Println(color.HiBlackString(" * Costs are estimates based on configured rates."))
		fmt.Println("")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(costCmd)
}

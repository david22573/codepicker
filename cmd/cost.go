package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/david22573/codepicker/infra/storage"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var costCmd = &cobra.Command{
	Use:   "cost",
	Short: "Show accumulated LLM usage and costs",
	Run: func(cmd *cobra.Command, args []string) {
		cwd, _ := os.Getwd()
		dbPath := fmt.Sprintf("%s/.codepicker/state.db", cwd)
		repo, err := storage.NewSQLiteRepository(dbPath)
		if err != nil {
			fmt.Printf("❌ Failed to connect to database: %v\n", err)
			return
		}
		defer repo.Close()

		cost, tokens, err := repo.GetTotalCost(context.Background())
		if err != nil {
			fmt.Printf("❌ Failed to fetch cost metrics: %v\n", err)
			return
		}

		fmt.Println("\n===================================================")
		fmt.Println(color.GreenString(" 💰 CODEPICKER COST DASHBOARD "))
		fmt.Println("===================================================")

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "Total Spend:\t$%.4f\n", cost)
		fmt.Fprintf(w, "Total Tokens:\t%d\n", tokens)

		avgCost := 0.0
		if tokens > 0 {
			// Very rough estimate of cost per 1k tokens
			avgCost = (cost / float64(tokens)) * 1000
		}
		fmt.Fprintf(w, "Avg Cost/1k Tokens:\t$%.4f\n", avgCost)

		w.Flush()
		fmt.Println("===================================================")
		fmt.Println(color.HiBlackString(" * Costs are estimates based on configured rates."))
		fmt.Println("")
	},
}

func init() {
	rootCmd.AddCommand(costCmd)
}

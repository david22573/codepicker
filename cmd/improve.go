package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/david22573/codepicker/app"
	"github.com/spf13/cobra"
)

var improveCmd = &cobra.Command{
	Use:   "improve",
	Short: "Automatically suggest and apply codebase improvements",
	Run: func(cmd *cobra.Command, args []string) {
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			fmt.Println("❌ Error: OPENROUTER_API_KEY is not set.")
			os.Exit(1)
		}

		cwd, _ := os.Getwd()

		// Initialize the container with current configuration
		container, err := app.NewContainer(apiKey, cwd, "", dryRunFlag, ciFlag)
		if err != nil {
			fmt.Printf("❌ Container Init Failed: %v\n", err)
			os.Exit(1)
		}

		ctx := context.Background()

		fmt.Println("📡 [SCOUT] Searching for potential improvements...")

		// The Auditor uses the LLM to scan for safe refactors
		tasks, err := container.Auditor.SuggestImprovements(ctx)
		if err != nil {
			fmt.Printf("❌ Audit Failed: %v\n", err)
			os.Exit(1)
		}

		if len(tasks) == 0 {
			fmt.Println("✅ No immediate improvements suggested. Your code is looking sharp!")
			return
		}

		fmt.Printf("\n✨ Found %d suggested improvements:\n", len(tasks))
		for i, t := range tasks {
			fmt.Printf("%d. %s\n", i+1, t)
		}

		fmt.Println("\n💡 To apply one, run: codepicker run \"<task_description>\"")
	},
}

func init() {
	rootCmd.AddCommand(improveCmd)
}

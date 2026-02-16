package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/david22573/codepicker/app"
	"github.com/spf13/cobra"
)

// Local flag to ensure this file compiles independently
var improveDryRun bool
var improveVerbose bool

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

		// FIX 1: Use local 'improveDryRun' and hardcode CI to false, add verboseFlag
		container, err := app.NewContainer(apiKey, cwd, "", improveDryRun, false, verboseFlag)
		if err != nil {
			fmt.Printf("❌ Container Init Failed: %v\n", err)
			os.Exit(1)
		}
		// Defer close to ensure logs flush
		defer container.Close()

		ctx := context.Background()

		// FIX 2: Generate the Project Primer (Map)
		fmt.Println("🗺️  Building project map...")
		primer := container.ProjectPrimer.Generate()

		fmt.Println("📡 [SCOUT] Searching for potential improvements...")

		// FIX 3: Pass 'primer' to SuggestImprovements (matches new signature)
		tasks, err := container.Auditor.SuggestImprovements(ctx, primer)
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
	improveCmd.Flags().BoolVar(&improveDryRun, "dry-run", false, "Enable read-only mode")
	improveCmd.Flags().BoolVarP(&improveVerbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.AddCommand(improveCmd)
}

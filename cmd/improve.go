package cmd

import (
	"fmt"
	"os"

	"github.com/david22573/codepicker/app"
	"github.com/spf13/cobra"
)

var improveDryRun bool
var improveVerbose bool

var improveCmd = &cobra.Command{
	Use:   "improve",
	Short: "Automatically suggest and apply codebase improvements",
	RunE: func(cmd *cobra.Command, args []string) error {
		apiKey := getAPIKeyOrExit()
		cwd, _ := os.Getwd()

		container, err := app.NewContainer(apiKey, cwd, "", improveDryRun, false, GetVerbose())
		if err != nil {
			return fmt.Errorf("container init failed: %w", err)
		}
		defer container.Close()

		ctx := cmd.Context()

		fmt.Println("🗺️  Building project map...")
		primer := container.ProjectPrimer.Generate()

		fmt.Println("📡 [SCOUT] Searching for potential improvements...")

		tasks, err := container.Auditor.SuggestImprovements(ctx, primer)
		if err != nil {
			return fmt.Errorf("audit failed: %w", err)
		}

		if len(tasks) == 0 {
			fmt.Println("✅ No immediate improvements suggested. Your code is looking sharp!")
			return nil
		}

		fmt.Printf("\n✨ Found %d suggested improvements:\n", len(tasks))
		for i, t := range tasks {
			fmt.Printf("%d. %s\n", i+1, t)
		}

		fmt.Println("\n💡 To apply one, run: codepicker run \"<task_description>\"")
		return nil
	},
}

func init() {
	improveCmd.Flags().BoolVar(&improveDryRun, "dry-run", false, "Enable read-only mode")
	improveCmd.Flags().BoolVarP(&improveVerbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.AddCommand(improveCmd)
}

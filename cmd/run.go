package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/david22573/codepicker/app"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run [task]",
	Short: "Execute a coding task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		task := args[0]

		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("OPENROUTER_API_KEY environment variable is required")
		}

		cwd, _ := os.Getwd()
		fmt.Printf("🚀 Initializing CodePicker in %s...\n", cwd)

		// This uses the NEW app/container.go
		container, err := app.NewContainer(apiKey, cwd)
		if err != nil {
			return fmt.Errorf("initialization failed: %w", err)
		}

		fmt.Printf("🤖 Agent working on: \"%s\"\n", task)
		fmt.Println("---------------------------------------------------")

		result, err := container.Agent.Run(context.Background(), task)
		if err != nil {
			return fmt.Errorf("agent failed: %w", err)
		}

		fmt.Println("---------------------------------------------------")
		fmt.Println("✅ Final Answer:")
		fmt.Println(result)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}

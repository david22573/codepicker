package cmd

import (
	"fmt"
	"os"

	"github.com/david22573/codepicker/adapters/context"
	"github.com/spf13/cobra"
)

var (
	ctxMaxTokens int
	ctxInclude   []string
	ctxExclude   []string
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Manage execution context",
}

var contextBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Generate a deterministic context file for the agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()

		// We don't need the full container for this, just the builder,
		// but using the builder pattern keeps things consistent.
		builder := context.NewBuilder()

		config := context.Config{
			ProjectRoot:     cwd,
			MaxTokens:       ctxMaxTokens,
			IncludePatterns: ctxInclude,
			ExcludePatterns: ctxExclude,
		}

		fmt.Printf("📦 Building context (Limit: %d tokens)...\n", ctxMaxTokens)

		output, err := builder.Build(config)
		if err != nil {
			return fmt.Errorf("failed to build context: %w", err)
		}

		// Write to disk
		outFile := "codepicker_context.md"
		if err := os.WriteFile(outFile, []byte(output), 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}

		fmt.Printf("✅ Context written to %s (%d bytes)\n", outFile, len(output))
		return nil
	},
}

func init() {
	// Register flags
	contextBuildCmd.Flags().IntVar(&ctxMaxTokens, "max-tokens", 8000, "Maximum estimated tokens to include")
	contextBuildCmd.Flags().StringSliceVar(&ctxInclude, "include", []string{}, "Glob patterns to include (e.g. '*.go')")
	contextBuildCmd.Flags().StringSliceVar(&ctxExclude, "exclude", []string{"*_test.go"}, "Glob patterns to exclude")

	contextCmd.AddCommand(contextBuildCmd)
	rootCmd.AddCommand(contextCmd)
}

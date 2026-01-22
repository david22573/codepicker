package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var docOutDir string

var docCmd = &cobra.Command{
	Use:   "doc",
	Short: "Generate CLI documentation",
	Long:  `Generates standard Markdown documentation for the Codepicker CLI. Useful for keeping project READMEs and Wikis up to date with the actual code behavior.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if docOutDir == "" {
			return fmt.Errorf("output directory is required (use --dir)")
		}

		if err := os.MkdirAll(docOutDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}

		appLogger.Info(fmt.Sprintf("📖 Generating documentation in: %s", docOutDir))

		// Standard Cobra doc generation
		err := doc.GenMarkdownTree(rootCmd, docOutDir)
		if err != nil {
			return fmt.Errorf("failed to generate docs: %w", err)
		}

		// Create a simple index file if it doesn't exist
		indexFile := filepath.Join(docOutDir, "README.md")
		if _, err := os.Stat(indexFile); os.IsNotExist(err) {
			indexContent := fmt.Sprintf("# Codepicker CLI Reference\n\nGenerated on %s\n\nSee [codepicker.md](codepicker.md) for the root command.", strings.ToUpper(Version))
			os.WriteFile(indexFile, []byte(indexContent), 0644)
		}

		appLogger.Info("✅ Documentation generated successfully.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(docCmd)
	docCmd.Flags().StringVar(&docOutDir, "dir", "docs/cli", "Directory to write Markdown files")
}

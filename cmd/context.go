package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/david22573/codepicker/app"
	ctxDomain "github.com/david22573/codepicker/domain/context"
	"github.com/spf13/cobra"
)

var (
	ctxInclude []string
	ctxExclude []string
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Manage the semantic code index and execution context",
}

var contextIndexCmd = &cobra.Command{
	Use:   "index [directory]",
	Short: "Scan, slice, and embed the codebase for RAG",
	Long:  `Indexes the codebase by breaking files into semantic slices and generating vector embeddings.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir := "."
		if len(args) > 0 {
			targetDir = args[0]
		}

		apiKey := os.Getenv("OPENROUTER_API_KEY")
		cwd, _ := os.Getwd()

		// Initialize container to get the configured IndexManager
		container, err := app.NewContainer(apiKey, cwd, "", false, false, verboseFlag)
		if err != nil {
			return fmt.Errorf("failed to initialize container: %w", err)
		}
		defer container.Close()

		fmt.Printf("🔍 Indexing codebase at: %s\n", targetDir)
		fmt.Println("   (This involves generating embeddings via API, please wait...)")

		// UPDATED: Use the container's manager which has the Embedder wired up
		if err := container.IndexManager.IndexDirectory(targetDir); err != nil {
			return fmt.Errorf("indexing failed: %w", err)
		}

		stats, err := container.SliceStore.GetStats()
		if err != nil {
			return err
		}
		fmt.Printf("\n✅ Indexing complete! Total Slices: %d across %d files.\n",
			stats.TotalSlices, stats.TotalFiles)

		return nil
	},
}

var contextExportCmd = &cobra.Command{
	Use:   "export [output_file]",
	Short: "Export the entire semantic index as a single Markdown file",
	RunE: func(cmd *cobra.Command, args []string) error {
		outFile := "codepicker_context.md"
		if len(args) > 0 {
			outFile = args[0]
		}

		cwd, _ := os.Getwd()
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		container, err := app.NewContainer(apiKey, cwd, "", false, false, verboseFlag)
		if err != nil {
			return err
		}
		defer container.Close()

		fmt.Printf("📄 Exporting full index to %s...\n", outFile)

		allSlices, err := container.Repository.GetAllSlices()
		if err != nil {
			return err
		}

		var sb strings.Builder
		sb.WriteString("# CODEPICKER FULL PROJECT CONTEXT\n\n")

		byFile := make(map[string][]ctxDomain.CodeSlice)
		for _, s := range allSlices {
			byFile[s.FilePath] = append(byFile[s.FilePath], s)
		}

		for path, slices := range byFile {
			sb.WriteString(fmt.Sprintf("## File: %s\n", path))
			for _, s := range slices {
				sb.WriteString(fmt.Sprintf("### %s (Lines %d-%d)\n```go\n%s\n```\n\n",
					s.SliceType, s.StartLine, s.EndLine, s.Content))
			}
			sb.WriteString("---\n")
		}

		return os.WriteFile(outFile, []byte(sb.String()), 0644)
	},
}

func init() {
	contextIndexCmd.Flags().StringSliceVar(&ctxInclude, "include", []string{}, "Include patterns")
	contextIndexCmd.Flags().StringSliceVar(&ctxExclude, "exclude", []string{}, "Exclude patterns")

	contextCmd.AddCommand(contextIndexCmd)
	contextCmd.AddCommand(contextExportCmd)
	rootCmd.AddCommand(contextCmd)
}

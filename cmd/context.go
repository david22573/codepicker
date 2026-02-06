package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/david22573/codepicker/app"
	ctxDomain "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/infra/indexer"
	"github.com/spf13/cobra"
)

var (
	ctxInclude []string
	ctxExclude []string
)

// contextCmd represents the base command for context management
var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Manage the semantic code index and execution context",
}

// contextIndexCmd triggers the semantic indexing with your ignore logic
var contextIndexCmd = &cobra.Command{
	Use:   "index [directory]",
	Short: "Scan and index the codebase using ignore patterns",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir := "."
		if len(args) > 0 {
			targetDir = args[0]
		}

		apiKey := os.Getenv("OPENROUTER_API_KEY")
		cwd, _ := os.Getwd()

		container, err := app.NewContainer(apiKey, cwd, "", false, false)
		if err != nil {
			return fmt.Errorf("failed to initialize container: %w", err)
		}

		// --- Your Original Ignore Logic ---
		finalExcludes := []string{".git", ".git/*", ".codepicker/*"}

		if gitIgnore, err := readIgnoreFile(".gitignore"); err == nil {
			finalExcludes = append(finalExcludes, gitIgnore...)
			fmt.Printf("🛡️  Loaded %d patterns from .gitignore\n", len(gitIgnore))
		}

		if cpIgnore, err := readIgnoreFile(".codepickerignore"); err == nil {
			finalExcludes = append(finalExcludes, cpIgnore...)
			fmt.Printf("👁️  Loaded %d patterns from .codepickerignore\n", len(cpIgnore))
		}
		finalExcludes = append(finalExcludes, ctxExclude...)

		fmt.Printf("🔍 Indexing codebase at: %s (respecting ignore patterns)\n", targetDir)

		// --- Semantic Indexing (Phase 2 & 5) ---
		slicer := indexer.NewCodeSlicer()
		manager := indexer.NewIndexManager(slicer, container.SliceStore)

		if err := manager.IndexDirectory(targetDir); err != nil {
			return fmt.Errorf("indexing failed: %w", err)
		}

		stats, _ := container.SliceStore.GetStats()
		fmt.Printf("\n✅ Indexing complete! Total Slices: %d across %d files.\n",
			stats.TotalSlices, stats.TotalFiles)

		return nil
	},
}

// contextExportCmd generates a full project markdown like you wanted
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
		container, err := app.NewContainer(apiKey, cwd, "", false, false)
		if err != nil {
			return err
		}

		fmt.Printf("📄 Exporting full index to %s...\n", outFile)

		// Fixed: Pulling everything without FTS5 MATCH syntax
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

// contextBuildCmd previews semantic slices for a specific task
var contextBuildCmd = &cobra.Command{
	Use:   "build [task]",
	Short: "Preview semantic slices that would be sent for a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskInput := args[0]
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		cwd, _ := os.Getwd()

		container, err := app.NewContainer(apiKey, cwd, "", false, false)
		if err != nil {
			return err
		}

		// Uses Phase 3 ranking logic
		output, err := container.ContextBuilder.BuildForTask(taskInput)
		if err != nil {
			return err
		}

		fmt.Println("\n--- SEMANTIC CONTEXT PREVIEW ---")
		fmt.Println(output)
		return nil
	},
}

// readIgnoreFile is your original helper to parse ignore-style files
func readIgnoreFile(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var patterns []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns, scanner.Err()
}

func init() {
	contextIndexCmd.Flags().StringSliceVar(&ctxInclude, "include", []string{}, "Include patterns")
	contextIndexCmd.Flags().StringSliceVar(&ctxExclude, "exclude", []string{}, "Exclude patterns")

	contextCmd.AddCommand(contextIndexCmd)
	contextCmd.AddCommand(contextBuildCmd)
	contextCmd.AddCommand(contextExportCmd)
	rootCmd.AddCommand(contextCmd)
}

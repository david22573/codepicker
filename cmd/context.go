package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/adapters/context" // Ensure this matches your folder structure
	"github.com/spf13/cobra"
)

var (
	ctxMaxTokens int
	ctxInclude   []string
	ctxExclude   []string
	skeletonMode bool // Defined for future use, but currently unused in Config
)

// contextCmd represents the context command namespace
var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Manage execution context",
}

var contextBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Generate a deterministic context file for the agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("could not determine current working directory: %w", err)
		}

		// 1. Initialize Excludes with reasonable defaults
		// We always want to ignore .git and the context file itself
		finalExcludes := []string{".git", ".git/*", "codepicker_context.md"}

		// 2. Load .gitignore (Safety)
		if gitIgnore, err := readIgnoreFile(".gitignore"); err == nil {
			finalExcludes = append(finalExcludes, gitIgnore...)
			fmt.Printf("🛡️  Loaded %d patterns from .gitignore\n", len(gitIgnore))
		}

		// 3. Load .codepickerignore (Noise Reduction)
		// This allows you to exclude things like go.sum, *.svg, etc.
		if cpIgnore, err := readIgnoreFile(".codepickerignore"); err == nil {
			finalExcludes = append(finalExcludes, cpIgnore...)
			fmt.Printf("👁️  Loaded %d patterns from .codepickerignore\n", len(cpIgnore))
		}

		// 4. Append CLI flags (Manual overrides)
		finalExcludes = append(finalExcludes, ctxExclude...)

		builder := context.NewBuilder()

		// Construct the config
		// Note: We are passing ctxMaxTokens directly.
		// If it is 0, the builder must handle it as "Unlimited".
		config := context.Config{
			ProjectRoot:     cwd,
			MaxTokens:       ctxMaxTokens,
			IncludePatterns: ctxInclude,
			ExcludePatterns: finalExcludes,
			// Skeleton: skeletonMode, // TODO: Add this field to context.Config if you implement skeleton logic later
		}

		// UX: nice output message
		limitMsg := fmt.Sprintf("%d tokens", ctxMaxTokens)
		if ctxMaxTokens == 0 {
			limitMsg = "Unlimited"
		}
		fmt.Printf("📦 Building context (Limit: %s)...\n", limitMsg)

		// 5. Build
		output, err := builder.Build(config)
		if err != nil {
			return fmt.Errorf("failed to build context: %w", err)
		}

		// 6. Write Output
		outFile := "codepicker_context.md"
		// Ensure we write to the root we scanned, or use a flag for output path if desired
		outPath := filepath.Join(cwd, outFile)

		if err := os.WriteFile(outPath, []byte(output), 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}

		fmt.Printf("✅ Context written to %s (%d bytes)\n", outFile, len(output))
		return nil
	},
}

// readIgnoreFile is a helper to parse ignore-style files
func readIgnoreFile(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		// It's okay if the file doesn't exist
		return nil, err
	}
	defer file.Close()

	var patterns []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		patterns = append(patterns, line)
	}

	return patterns, scanner.Err()
}

func init() {
	// Flags
	contextBuildCmd.Flags().BoolVar(&skeletonMode, "skeleton", false, "Generate skeleton (signatures only) context")

	// Default is 8000. To use unlimited, user must run: --max-tokens 0
	contextBuildCmd.Flags().IntVar(&ctxMaxTokens, "max-tokens", 8000, "Maximum estimated tokens to include (set 0 for unlimited)")

	contextBuildCmd.Flags().StringSliceVar(&ctxInclude, "include", []string{}, "Glob patterns to include (e.g. '*.go')")
	contextBuildCmd.Flags().StringSliceVar(&ctxExclude, "exclude", []string{}, "Additional glob patterns to exclude")

	// Register commands
	contextCmd.AddCommand(contextBuildCmd)
	rootCmd.AddCommand(contextCmd)
}

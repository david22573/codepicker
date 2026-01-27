package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/david22573/codepicker/adapters/context"
	"github.com/spf13/cobra"
)

var (
	ctxMaxTokens int
	ctxInclude   []string
	ctxExclude   []string
)

var skeletonMode bool

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Manage execution context",
}

var contextBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Generate a deterministic context file for the agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()

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

		config := context.Config{
			ProjectRoot:     cwd,
			MaxTokens:       ctxMaxTokens,
			IncludePatterns: ctxInclude,
			ExcludePatterns: finalExcludes, // Pass the merged list
		}

		fmt.Printf("📦 Building context (Limit: %d tokens)...\n", ctxMaxTokens)

		output, err := builder.Build(config)
		if err != nil {
			return fmt.Errorf("failed to build context: %w", err)
		}

		outFile := "codepicker_context.md"
		if err := os.WriteFile(outFile, []byte(output), 0644); err != nil {
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
	contextBuildCmd.Flags().BoolVar(&skeletonMode, "skeleton", false, "Generate skeleton (signatures only) context")
	contextBuildCmd.Flags().IntVar(&ctxMaxTokens, "max-tokens", 8000, "Maximum estimated tokens to include")
	contextBuildCmd.Flags().StringSliceVar(&ctxInclude, "include", []string{}, "Glob patterns to include (e.g. '*.go')")

	// We default to empty here because we load defaults in the function now
	contextBuildCmd.Flags().StringSliceVar(&ctxExclude, "exclude", []string{}, "Additional glob patterns to exclude")

	contextCmd.AddCommand(contextBuildCmd)
	rootCmd.AddCommand(contextCmd)
}

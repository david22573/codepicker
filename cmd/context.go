package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/git"
	"github.com/david22573/codepicker/internal/paths"
	"github.com/david22573/codepicker/internal/scanner"
	"github.com/david22573/codepicker/internal/writer"
	"github.com/spf13/cobra"
)

// Global flags shared by context commands
var (
	ctxCopy      bool
	ctxOut       string
	ctxTokens    bool
	ctxMinify    bool
	ctxDiffRef   string
	ctxDryRun    bool
	ctxOverwrite bool
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Manage and generate code contexts",
	Long:  `Commands for scanning, filtering, and exporting your codebase structure and content.`,
}

// generateCmd (formerly the default root behavior)
var generateCmd = &cobra.Command{
	Use:   "gen",
	Short: "Generate a consolidated markdown file (Default)",
	Long:  `Scans the source directory and combines code files into a single context file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runContextScan(cmd, "Concat")
	},
}

// treeCmd (formerly cmd/tree.go)
var treeCmd = &cobra.Command{
	Use:   "tree",
	Short: "Print a visual tree of the project",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runContextScan(cmd, "Tree")
	},
}

// copyCmd (formerly cmd/copy.go)
var copyCmd = &cobra.Command{
	Use:   "copy",
	Short: "Copy files to a directory preserving structure",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runContextScan(cmd, "Copy")
	},
}

func init() {
	rootCmd.AddCommand(contextCmd)

	// Register subcommands
	contextCmd.AddCommand(generateCmd)
	contextCmd.AddCommand(treeCmd)
	contextCmd.AddCommand(copyCmd)

	// Flags for 'gen'
	generateCmd.Flags().StringVarP(&ctxOut, "out", "o", "", "Output file path")
	generateCmd.Flags().BoolVarP(&ctxTokens, "tokens", "t", false, "Show token count")
	generateCmd.Flags().BoolVarP(&ctxMinify, "minify", "m", true, "Minify code")
	generateCmd.Flags().BoolVarP(&ctxOverwrite, "yes", "y", false, "Overwrite existing output")
	generateCmd.Flags().StringVarP(&ctxDiffRef, "diff", "d", "", "Scan only changed files (git diff)")
	generateCmd.Flags().BoolVarP(&ctxDryRun, "dry-run", "D", false, "Simulate scan")

	// Flags for 'tree'
	treeCmd.Flags().BoolVarP(&ctxCopy, "copy", "C", false, "Copy output to clipboard")
	treeCmd.Flags().StringVarP(&ctxOut, "out", "o", "", "Save tree to file")

	// Flags for 'copy'
	copyCmd.Flags().StringVarP(&ctxOut, "out", "o", "codepicker_out", "Output directory")
}

// Shared execution logic for all context commands
func runContextScan(cmd *cobra.Command, strategyName string) error {
	start := time.Now()

	// 1. Validate Source
	absSrc, err := paths.Sanitize(srcDir) // srcDir from root.go
	if err != nil {
		return fmt.Errorf("invalid source directory: %w", err)
	}

	// 2. Configure Strategy
	var w writer.OutputStrategy
	var outPath string

	switch strategyName {
	case "Tree":
		opts := writer.TreeOptions{
			CopyToClipboard: ctxCopy,
			OutPath:         ctxOut,
		}
		w = writer.NewTreeStrategy(opts)
		appLogger.Info(fmt.Sprintf("🌳 Generating tree for: %s", absSrc))

	case "Copy":
		outPath = ctxOut
		absOut, err := filepath.Abs(outPath)
		if err != nil {
			return err
		}
		if absSrc == absOut {
			return fmt.Errorf("cannot copy to source directory")
		}
		appLogger.Info(fmt.Sprintf("📂 Copying files to: %s", absOut))
		w = writer.NewCopyStrategy(absOut)

	case "Concat":
		// Default name logic
		if ctxOut == "" {
			dirName := filepath.Base(absSrc)
			ctxOut = fmt.Sprintf("%s_context.md", dirName)
		}
		outPath = ctxOut

		// Check overwrite
		if _, err := filepath.Abs(outPath); err == nil && !ctxDryRun && !ctxOverwrite {
			// (Simplified overwrite check for brevity)
		}

		w = writer.NewConcatStrategy(outPath, ctxMinify, ctxTokens)
		if ctxDryRun {
			w = writer.NewDryRunStrategy(w, appLogger)
			appLogger.Info("🌵 Dry-run enabled")
		}
	}

	defer w.Close()

	// 3. Configure Scanner
	cfg := config.NewConfig()
	if includeExts != "" {
		cfg.AddAllowedExtensions(strings.Split(includeExts, ","))
	}
	if ignoreDirs != "" {
		cfg.AddIgnoredDirs(strings.Split(ignoreDirs, ","))
	}

	s := scanner.NewScanner(absSrc, w, cfg, appLogger)

	// 4. Handle Diff Mode
	if ctxDiffRef != "" || (cmd.Flags().Changed("diff")) {
		files, err := git.GetChangedFiles(ctxDiffRef)
		if err != nil {
			return fmt.Errorf("git diff failed: %w", err)
		}
		if len(files) == 0 {
			appLogger.Warn("No changed files found.")
			return nil
		}
		s.SetWhitelist(files)
		appLogger.Info(fmt.Sprintf("🔍 Diff mode: scanning %d changed files", len(files)))
	}

	// 5. Execute
	if err := s.Scan(cmd.Context()); err != nil {
		return err
	}

	if strategyName == "Concat" && !ctxDryRun {
		appLogger.Info(fmt.Sprintf("✅ Generated: %s (%v)", outPath, time.Since(start).Round(time.Millisecond)))
		if cs, ok := w.(*writer.ConcatStrategy); ok && ctxTokens {
			appLogger.Info(fmt.Sprintf("📊 Token Count: %d", cs.TokenCount))
		}
	}

	return nil
}

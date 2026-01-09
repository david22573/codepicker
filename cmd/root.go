package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/scanner"
	"github.com/david22573/codepicker/internal/writer"
	"github.com/spf13/cobra"
)

var (
	srcDir      string
	outPath     string
	showTokens  bool
	minify      bool
	includeExts string
	ignoreDirs  string
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "codepicker",
	Short: "Harvest code for AI consumption",
	Long:  `Scans a directory and combines code files into a single context file.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default Mode: Concat
		absOut, _ := filepath.Abs(outPath)
		if filepath.Ext(absOut) == "" {
			absOut += ".md" // Changed default to .md
		}

		// Pass minify flag to strategy
		w := writer.NewConcatStrategy(absOut, minify)
		runScan(w)

		fmt.Printf("📦 Output: %s\n", absOut)
		if showTokens {
			fmt.Printf("🔢 Estimated Tokens: ~%d\n", w.TokenEstimate)
		}
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&srcDir, "src", "s", ".", "Source directory to scan")

	// Updated default to .md
	rootCmd.Flags().StringVarP(&outPath, "out", "o", "codepicker_context.md", "Output file path")

	// New Flags
	rootCmd.Flags().BoolVarP(&showTokens, "tokens", "t", false, "Show estimated token count")
	rootCmd.Flags().BoolVarP(&minify, "minify", "m", true, "Remove comments and extra whitespace to save tokens")
	rootCmd.Flags().StringVarP(&includeExts, "include", "i", "", "Comma-separated extensions to include (e.g. .vue,.svelte)")
	rootCmd.Flags().StringVarP(&ignoreDirs, "exclude", "e", "", "Comma-separated directories to exclude")
}

// Shared helper to run the scanner with any strategy
func runScan(w writer.OutputStrategy) {
	start := time.Now()
	absSrc, _ := filepath.Abs(srcDir)

	// Setup Configuration
	cfg := config.NewConfig()
	if includeExts != "" {
		cfg.AddAllowedExtensions(strings.Split(includeExts, ","))
	}
	if ignoreDirs != "" {
		cfg.AddIgnoredDirs(strings.Split(ignoreDirs, ","))
	}

	// UI Feedback
	if w.Name() != "Tree" {
		fmt.Printf("🍇 Scanning: %s\n", absSrc)
		if includeExts != "" {
			fmt.Printf("➕ Including: %s\n", includeExts)
		}
		if minify {
			fmt.Println("✂️  Minification enabled (removing comments & whitespace)")
		}
	}

	s := scanner.NewScanner(absSrc, w, cfg)
	if err := s.Scan(); err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}

	if w.Name() != "Tree" {
		fmt.Printf("✅ Done in %v\n", time.Since(start))
	}
}

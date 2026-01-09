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

var rootCmd = &cobra.Command{
	Use:   "codepicker",
	Short: "Harvest code for AI consumption",
	Long:  `Scans a directory and combines code files into a single context file.`,
	Run: func(cmd *cobra.Command, args []string) {
		// 1. Resolve Source Directory
		absSrc, _ := filepath.Abs(srcDir)

		// 2. Dynamic Output Naming Logic
		if outPath == "" {
			// Get the base name of the directory (e.g., "codepicker" from "/home/user/codepicker")
			dirName := filepath.Base(absSrc)

			// Handle edge cases (like scanning root or current dir dots)
			if dirName == "." || dirName == string(filepath.Separator) {
				wd, _ := os.Getwd()
				dirName = filepath.Base(wd)
			}

			outPath = fmt.Sprintf("%s_context.md", dirName)
		}

		absOut, _ := filepath.Abs(outPath)

		// Ensure extension exists if user provided a custom name without one
		if filepath.Ext(absOut) == "" {
			absOut += ".md"
		}

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
	// Reordered flags for better help output
	rootCmd.PersistentFlags().StringVarP(&srcDir, "src", "s", ".", "Source directory to scan")

	// Changed default to empty string "" to trigger dynamic logic in Run
	rootCmd.Flags().StringVarP(&outPath, "out", "o", "", "Output file path (default: [dir_name]_context.md)")

	rootCmd.Flags().BoolVarP(&showTokens, "tokens", "t", false, "Show estimated token count")
	rootCmd.Flags().BoolVarP(&minify, "minify", "m", true, "Remove comments and extra whitespace to save tokens")
	rootCmd.Flags().StringVarP(&includeExts, "include", "i", "", "Comma-separated extensions to include (e.g. .vue,.svelte)")
	rootCmd.Flags().StringVarP(&ignoreDirs, "exclude", "e", "", "Comma-separated directories to exclude")
}

func runScan(w writer.OutputStrategy) {
	start := time.Now()
	absSrc, _ := filepath.Abs(srcDir)

	cfg := config.NewConfig()
	if includeExts != "" {
		cfg.AddAllowedExtensions(strings.Split(includeExts, ","))
	}
	if ignoreDirs != "" {
		cfg.AddIgnoredDirs(strings.Split(ignoreDirs, ","))
	}

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


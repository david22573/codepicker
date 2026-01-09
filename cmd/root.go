package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/david22573/codepicker/internal/scanner"
	"github.com/david22573/codepicker/internal/writer"
	"github.com/spf13/cobra"
)

var (
	srcDir  string
	outPath string
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
			absOut += ".txt"
		}

		w := writer.NewConcatStrategy(absOut)
		runScan(w)
		fmt.Printf("📦 Output: %s\n", absOut)
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
	rootCmd.Flags().StringVarP(&outPath, "out", "o", "codepicker_context.txt", "Output file path")
}

// Shared helper to run the scanner with any strategy
func runScan(w writer.OutputStrategy) {
	start := time.Now()
	absSrc, _ := filepath.Abs(srcDir)

	// Only print scanning msg if not tree (tree prints its own header)
	if _, isTree := w.(*writer.TreeStrategy); !isTree {
		fmt.Printf("🍇 Scanning: %s\n", absSrc)
	}

	s := scanner.NewScanner(absSrc, w)
	if err := s.Scan(); err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}

	if _, isTree := w.(*writer.TreeStrategy); !isTree {
		fmt.Printf("✅ Done in %v\n", time.Since(start))
	}
}


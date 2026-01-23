package cmd

import (
	"fmt"
	"os"

	"github.com/david22573/codepicker/internal/code"
	"github.com/spf13/cobra"
)

var (
	keepDocs  bool
	skipTests bool
)

var skeletonCmd = &cobra.Command{
	Use:   "skeleton [path]",
	Short: "Generate a code skeleton (signatures only) for files or directories",
	Long: `Parses Go source files and generates a "skeleton" view containing only 
package declarations, imports, types, interfaces, and function signatures. 
Function bodies are stripped to save tokens while preserving context structure.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		targetPath := "."
		if len(args) > 0 {
			targetPath = args[0]
		}

		opts := code.SkeletonOptions{
			KeepDocComments: keepDocs,
			SkipTests:       skipTests,
		}

		result, err := code.GenerateSkeleton(targetPath, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating skeleton: %v\n", err)
			os.Exit(1)
		}

		fmt.Println(result)
	},
}

func init() {
	rootCmd.AddCommand(skeletonCmd)

	skeletonCmd.Flags().BoolVar(&keepDocs, "docs", false, "Keep documentation comments (docstrings)")
	skeletonCmd.Flags().BoolVar(&skipTests, "no-tests", true, "Skip _test.go files (default true)")
}

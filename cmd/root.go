package cmd

import (
	"fmt"
	"os"

	"github.com/david22573/codepicker/internal/logger"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	verbose     bool
	srcDir      string
	minify      bool
	includeExts string
	ignoreDirs  string

	// Global Logger (Must use the interface type to be compatible with internal packages)
	appLogger logger.Logger
)

var rootCmd = &cobra.Command{
	Use:   "codepicker",
	Short: "AI-powered code harvester and modifier",
	Long: `CodePicker is a CLI tool that helps AI agents understand and modify codebases.
It maintains a 'shadow' copy of your project to safely stage changes before applying them.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		setupLogger()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")

	// Global flags used by subcommands
	rootCmd.PersistentFlags().StringVarP(&srcDir, "src", "s", ".", "Source directory root")
	rootCmd.PersistentFlags().BoolVarP(&minify, "minify", "m", false, "Minify output")
	rootCmd.PersistentFlags().StringVar(&includeExts, "include", "", "Comma-separated extensions to include")
	rootCmd.PersistentFlags().StringVar(&ignoreDirs, "ignore", "", "Comma-separated directories to ignore")
}

func setupLogger() {
	level := 1 // Info
	if verbose {
		level = 2 // Debug
	}

	// Use the internal logger factory to ensure interface compatibility
	appLogger = logger.NewStandardLogger(level)
}

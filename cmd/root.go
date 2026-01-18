package cmd

import (
	"fmt"
	"os"

	"github.com/david22573/codepicker/internal/logger"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

// Global flags (Shared across multiple commands)
var (
	srcDir      string
	includeExts string
	ignoreDirs  string
	configFile  string
	verbose     bool
	minify      bool // Kept global because 'interact ask' also uses it
)

var appLogger logger.Logger

var rootCmd = &cobra.Command{
	Use:   "codepicker",
	Short: "AI Codebase Agent & Context Generator",
	Long:  `Codepicker is a developer tool that turns your codebase into AI-ready context and provides autonomous agents to work on it.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		level := 1
		if verbose {
			level = 2
		}
		appLogger = logger.NewStandardLogger(level)
	},
	// Default behavior: if no subcommand is given, run 'context gen'
	RunE: func(cmd *cobra.Command, args []string) error {
		return generateCmd.RunE(cmd, args)
	},
}

func Execute() {
	appLogger = logger.NewStandardLogger(1)
	if err := rootCmd.Execute(); err != nil {
		appLogger.Error(fmt.Sprintf("Fatal error: %v", err))
		os.Exit(1)
	}
}

func init() {
	_ = godotenv.Load()

	// Persistent flags available to all subcommands
	rootCmd.PersistentFlags().StringVarP(&srcDir, "src", "s", ".", "Source directory")
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Config file path")

	// Filtering flags are persistent because 'ask', 'agent', and 'context' all need them
	rootCmd.PersistentFlags().StringVarP(&includeExts, "include", "i", "", "Extensions to include (comma-separated)")
	rootCmd.PersistentFlags().StringVarP(&ignoreDirs, "exclude", "e", "", "Directories to exclude (comma-separated)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")

	// Minify is used by 'ask' and 'context', so keep it global
	rootCmd.PersistentFlags().BoolVarP(&minify, "minify", "m", true, "Minify output (global default)")
}

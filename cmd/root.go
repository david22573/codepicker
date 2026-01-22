package cmd

import (
	"fmt"
	"os"

	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/ui"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var (
	srcDir      string
	includeExts string
	ignoreDirs  string
	configFile  string
	verbose     bool
	minify      bool

	// Debug Flags
	debugPolicy bool
	traceTools  bool
	traceMemory bool
)

var appLogger logger.Logger
var userUI ui.UI

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
	// Phase 0 Fix: Default to help
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
		fmt.Println("\n💡 Hint: Run 'codepicker context gen' to generate context, or 'codepicker agent' to start the AI.")
	},
}

func Execute() {
	appLogger = logger.NewStandardLogger(1)
	userUI = ui.NewConsoleUI()
	if err := rootCmd.Execute(); err != nil {
		appLogger.Error(fmt.Sprintf("Fatal error: %v", err))
		os.Exit(1)
	}
}

func init() {
	_ = godotenv.Load()

	rootCmd.PersistentFlags().StringVarP(&srcDir, "src", "s", ".", "Source directory")
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Config file path")

	rootCmd.PersistentFlags().StringVarP(&includeExts, "include", "i", "", "Extensions to include (comma-separated)")
	rootCmd.PersistentFlags().StringVarP(&ignoreDirs, "exclude", "e", "", "Directories to exclude (comma-separated)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")

	rootCmd.PersistentFlags().BoolVarP(&minify, "minify", "m", true, "Minify output (global default)")

	rootCmd.PersistentFlags().BoolVar(&debugPolicy, "debug-policy", false, "Log detailed policy decisions")
	rootCmd.PersistentFlags().BoolVar(&traceTools, "trace-tools", false, "Log full tool arguments and outputs")
	rootCmd.PersistentFlags().BoolVar(&traceMemory, "trace-memory", false, "Log memory snapshot/restore operations")
}

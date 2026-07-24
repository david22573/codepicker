package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

var verboseFlag bool

var rootCmd = &cobra.Command{
	Use:           "codepicker",
	Short:         "CodePicker: The Autonomous Coding Agent",
	Long:          `CodePicker is a ReAct-based agent that safely refactors code using a shadow filesystem.`,
	SilenceUsage:  true, // Prevents printing help text on normal execution errors
	SilenceErrors: true, // We handle our own error printing in Execute()
}

// GetVerbose returns the value of the verbose flag for use by commands
func GetVerbose() bool {
	return verboseFlag
}

// getAPIKeyOrExit ensures the API key is present before launching LLM-dependent commands
func getAPIKeyOrExit(cmdName string) string {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		fmt.Printf("error: OPENROUTER_API_KEY is required for `codepicker %s`\n", cmdName)
		fmt.Println("hint: export OPENROUTER_API_KEY=...")
		os.Exit(1)
	}
	return apiKey
}

var jsonFlag bool

// GetJSON returns the value of the json flag for use by commands
func GetJSON() bool {
	return jsonFlag
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&jsonFlag, "json", false, "Output in JSON format")
}

func Execute() {
	// Create a global context that listens for interrupt signals
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}
}

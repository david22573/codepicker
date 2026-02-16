package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var setVerbose bool

var verboseFlag bool

var rootCmd = &cobra.Command{
	Use:   "codepicker",
	Short: "CodePicker: The Autonomous Coding Agent",
	Long:  `CodePicker is a ReAct-based agent that safely refactors code using a shadow filesystem.`,
}

// GetVerbose returns the value of the verbose flag for use by commands
func GetVerbose() bool {
	return setVerbose
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "v", false, "Enable verbose output")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

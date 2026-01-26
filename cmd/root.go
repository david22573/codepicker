package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "codepicker",
	Short: "CodePicker: The Autonomous Coding Agent",
	Long:  `CodePicker is a ReAct-based agent that safely refactors code using a shadow filesystem.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

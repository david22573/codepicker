package main

import (
	"fmt"
	"github.com/david22573/codepicker/cmd"
)

func main() {
	// Execute the root command and all subcommands
	cmd.Execute()

	// After execution, we can access the verbose flag value
	// This will be used by the container in command files
	isVerbose := cmd.GetVerbose()
	if isVerbose {
		fmt.Println("Verbose mode is enabled")
	}
}

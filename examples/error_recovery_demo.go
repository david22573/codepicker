package main

import (
	"fmt"

	"github.com/david22573/codepicker/internal/agent"
)

func main() {
	fmt.Println("🚑 Agent Self-Healing / Error Recovery Demo")
	fmt.Println("-------------------------------------------")

	// A list of simulated errors that might occur during execution
	simulatedErrors := []string{
		"go: go.mod file not found in current directory or any parent directory",
		"npm: command not found",
		"ModuleNotFoundError: No module named 'requests'",
		"dial tcp: i/o timeout",
		"unknown error: something exploded",
	}

	strategies := agent.GetRecoveryStrategies()

	for _, errStr := range simulatedErrors {
		fmt.Printf("\n❌ Simulated Error: %q\n", errStr)

		matched := false
		for _, strategy := range strategies {
			if strategy.Pattern.MatchString(errStr) {
				fmt.Printf("   ✅ MATCHED STRATEGY: %s\n", strategy.Name)
				fmt.Printf("   🔍 Diagnosis: %s\n", strategy.Diagnosis)

				if len(strategy.FixCommands) > 0 {
					fmt.Println("   🛠️  Proposed Fixes:")
					for _, cmd := range strategy.FixCommands {
						fmt.Printf("      $ %s %v\n", cmd.Binary, cmd.Args)
					}
				} else {
					fmt.Println("   ⚠️  No auto-fix available (Manual intervention required)")
				}
				matched = true
				break
			}
		}

		if !matched {
			fmt.Println("   🤷 No known recovery strategy found.")
		}
	}
}

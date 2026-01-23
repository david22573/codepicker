package main

import (
	"fmt"
	"os"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/pkg/openrouter"
)

func main() {
	log := logger.NewSimpleLogger(true)
	limits := config.DefaultLimits()

	// FIX: Use database.New() which matches the current internal/database API
	store, err := database.New(":memory:")
	if err != nil {
		log.Error(fmt.Sprintf("Failed to create store: %v", err))
		return
	}
	defer store.Close()

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Warn("OPENROUTER_API_KEY not set, using placeholder")
		apiKey = "placeholder"
	}
	client := openrouter.NewClient(apiKey)

	cfg := &config.ConfigFile{}

	// Create the engine with the standard configuration
	engine, err := agent.NewEngine(
		client,
		"deepseek/deepseek-chat",
		".",
		log,
		limits,
		store,
		cfg,
		agent.DebugConfig{Tools: true},
	)
	if err != nil {
		log.Error(fmt.Sprintf("Failed to create engine: %v", err))
		return
	}

	fmt.Println("=== Example 1: Python Module Missing ===")
	demoModuleMissing(engine)

	fmt.Println("\n=== Example 2: Go Module Missing ===")
	demoGoModMissing(engine)

	fmt.Println("\n=== Example 3: Pattern Testing ===")
	demoPatternTesting()

	fmt.Println("\n=== Example 4: Available Recovery Strategies ===")
	demoListStrategies()
}

func demoModuleMissing(engine *agent.Engine) {
	// Execute a command we expect to fail to demonstrate recovery
	result := engine.ExecuteWithRecovery("python3", []string{"-c", "import requests"}, 3)

	if result.Success {
		fmt.Println("✅ Command succeeded!")
	} else if result.Attempted {
		fmt.Printf("🚑 Recovery attempted using strategy: %s\n", result.StrategyUsed)
		fmt.Printf("📋 Actions taken: %v\n", result.ActionsTaken)
		if result.FinalError != nil {
			fmt.Printf("❌ Final error: %v\n", result.FinalError)
		}
	} else {
		fmt.Println("❌ No recovery strategy matched or recovery failed.")
	}
}

func demoGoModMissing(engine *agent.Engine) {
	// Execute a Go command that might trigger a missing module error
	result := engine.ExecuteWithRecovery("go", []string{"list", "./..."}, 3)

	if result.Success {
		fmt.Println("✅ Command succeeded!")
	} else if result.Attempted {
		fmt.Printf("🚑 Recovery attempted using strategy: %s\n", result.StrategyUsed)
		fmt.Printf("📋 Actions taken: %v\n", result.ActionsTaken)
	}
}

func demoPatternTesting() {
	testCases := []string{
		"ModuleNotFoundError: No module named 'numpy'",
		"Cannot find module 'express'",
		"permission denied: ./script.sh",
		"CONFLICT (content): Merge conflict in main.go",
		"database is locked",
	}

	for _, errorText := range testCases {
		matches := agent.TestPattern(errorText)
		if len(matches) > 0 {
			fmt.Printf("Error: %s\n", errorText)
			fmt.Printf("  Matches: %v\n", matches)
		}
	}
}

func demoListStrategies() {
	strategies := agent.GetRecoveryStrategies()
	fmt.Printf("Found %d strategies\n", len(strategies))
}

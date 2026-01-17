package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/constants"
	"github.com/david22573/codepicker/internal/contextgen" // Import contextgen
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/spf13/cobra"
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Analyze the codebase and generate an improvement plan",
	Long:  `Runs the agent in Architect Mode to scan the project for weaknesses and generate a prioritized 'ARCHITECTURE_GOALS.md' file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiKey, err := validateAPIKey()
		if err != nil {
			return err
		}

		absSrc, err := filepath.Abs(srcDir)
		if err != nil {
			return err
		}

		// 1. GENERATE THE MAP (The Fix)
		// We use the existing tree generator to create a lightweight map of the repo.
		appLogger.Info("🗺️  Generating project map...")
		projectTree, err := contextgen.GenerateTree(absSrc)
		if err != nil {
			return fmt.Errorf("failed to generate project tree: %w", err)
		}

		store, err := database.New(".codepicker")
		if err != nil {
			return fmt.Errorf("failed to init database: %w", err)
		}
		defer store.Close()

		client := openrouter.NewClient(apiKey)
		limits := config.DefaultLimits()

		// Increase turns to prevent early timeout
		limits.AgentMaxTurns = 50

		eng, err := agent.NewEngine(client, constants.DefaultModel, absSrc, appLogger, limits, store)
		if err != nil {
			return err
		}

		// 2. INJECT THE MAP INTO THE PROMPT
		// We append the tree to the Architect prompt so the agent sees it immediately.
		eng.SystemPrompt = agent.ArchitectPrompt + "\n\n" + projectTree

		appLogger.Info("🏗️  Starting Architecture Audit...")

		printUpdate := func(msg openrouter.ChatMessage) {
			if msg.Role == "assistant" && msg.Content != nil {
				content := fmt.Sprintf("%v", msg.Content)
				// Filter out tool calls to keep output clean
				if content != "" && !strings.Contains(content, "tool_calls") {
					fmt.Printf("🤖 Thought: %s\n", content)
				}
			}
		}

		task := "Perform a deep audit of this codebase. Output your findings to ARCHITECTURE_GOALS.md."

		result, err := eng.Run(cmd.Context(), task, printUpdate)
		if err != nil {
			return err
		}

		fmt.Printf("\n✅ Audit Complete.\n%s\n", result)
		fmt.Println("\n👉 Run 'codepicker apply' to save the ARCHITECTURE_GOALS.md file.")
		fmt.Println("👉 Then run 'codepicker improve' to start executing the plan.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(auditCmd)
}

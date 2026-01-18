package cmd

import (
	"fmt"
	"strings"

	"github.com/david22573/codepicker/internal/app"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/spf13/cobra"
)

// agentCmd represents the base command for autonomous features
var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Autonomous agent commands",
	Long:  `Group of commands where the AI agent acts autonomously to plan, execute, or improve code.`,
}

// runCmd replaces the old 'do' command
var runCmd = &cobra.Command{
	Use:   "run [task]",
	Short: "Execute an autonomous task (Interactive Mode)",
	Long:  `Starts the agent in interactive mode to perform a specific task. Uses the 'Interactive' policy which prompts for confirmation on shell commands and file writes.`,
	Example: `  codepicker agent run "Refactor the logging interface"
  codepicker agent run "Fix the bug in main.go"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		task := strings.Join(args, " ")

		// 1. Initialize the Agent Context
		// This single call handles DB connection, Config loading, API Key validation,
		// and Engine initialization with the correct Policy.
		agentCtx, err := app.NewAgentContext(cmd.Context(), app.ContextOptions{
			SrcDir:   srcDir, // Global flag from root.go
			LogLevel: 1,
			Mode:     app.ModeInteractive,
			Policy:   policy.Interactive,
			Task:     task,
		})
		if err != nil {
			return fmt.Errorf("failed to initialize agent: %w", err)
		}
		defer agentCtx.Close()

		agentCtx.Logger.Info("🤖 Agent initialized in Interactive Mode")
		agentCtx.Logger.Info(fmt.Sprintf("📋 Task: %s", task))

		// 2. Define Output Handler (The "Thought Loop")
		// This filters the raw event stream for user-facing consumption
		printUpdate := func(msg openrouter.ChatMessage) {
			// Only show assistant thoughts that aren't tool calls
			if msg.Role == "assistant" && msg.Content != nil {
				content := fmt.Sprintf("%v", msg.Content)
				// Clean up empty lines or brief flickers
				if strings.TrimSpace(content) != "" && !strings.Contains(content, "tool_calls") {
					fmt.Printf("\n🧠 \033[1mAgent Thought:\033[0m %s\n", content)
				}
			}

			// Optional: You could log tool inputs here if verbose
			if msg.Role == "tool" {
				agentCtx.Logger.Debug(fmt.Sprintf("Tool Output: %s", msg.Content))
			}
		}

		// 3. Execute
		fmt.Println("🚀 Starting execution...")
		result, err := agentCtx.Engine.Run(agentCtx.Ctx, task, printUpdate)
		if err != nil {
			return fmt.Errorf("agent execution failed: %w", err)
		}

		// 4. Final Report
		fmt.Println("\n" + strings.Repeat("=", 50))
		fmt.Println("✅ Task Completed Successfully")
		fmt.Println(strings.Repeat("=", 50))
		fmt.Printf("\nResult Summary:\n%s\n", result)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.AddCommand(runCmd)

	// Future: agentCmd.AddCommand(planCmd)
	// Future: agentCmd.AddCommand(improveCmd)
}

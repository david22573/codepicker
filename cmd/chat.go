package cmd

import (
	"fmt"
	"strings"

	"github.com/david22573/codepicker/internal/app"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/internal/ui"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/spf13/cobra"
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start an interactive chat session with the codebase",
	Long:  `Starts a conversational session where you can ask questions about the codebase. The agent retains context and can use tools to investigate answers.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Initialize UI if needed
		if ui.Standard == nil {
			ui.Standard = ui.NewConsoleUI()
		}

		// 2. Initialize Agent Context
		// We use Interactive policy so the user can approve tools if configured
		agentCtx, err := app.NewAgentContext(cmd.Context(), app.ContextOptions{
			SrcDir:   srcDir,
			LogLevel: 1, // Info level
			Mode:     app.ModeInteractive,
			Policy:   policy.Interactive,
			Task:     "Interactive Chat Session",
		})
		if err != nil {
			return fmt.Errorf("failed to initialize context: %w", err)
		}
		defer agentCtx.Close()

		ui.Standard.Success("💬 Chat Session Started")
		ui.Standard.Info("Type 'exit' or 'quit' to end the session.")
		fmt.Println("----------------------------------------------------------------")

		// 3. Setup Chat State
		var history []openrouter.ChatMessage
		client := agentCtx.Engine.Client
		engine := agentCtx.Engine

		// Prepare tools for the LLM
		var activeTools []openrouter.Tool
		for _, t := range engine.Executor.Tools {
			activeTools = append(activeTools, t.Definition())
		}

		// 4. Main Chat Loop
		for {
			// --- User Input ---
			userInput := ui.Standard.Input("You", "")
			userInput = strings.TrimSpace(userInput)

			if userInput == "" {
				continue
			}
			if userInput == "exit" || userInput == "quit" {
				break
			}

			// Add user message to history
			history = append(history, openrouter.ChatMessage{
				Role:    "user",
				Content: userInput,
			})

			// --- Agent Turn Loop (Thought/Tool Cycle) ---
			// We loop until the agent produces a text response (Role: assistant with content)
			// instead of a tool call.
			for {
				// Refresh System Prompt with latest file context (in case files changed)
				currentContext := engine.Memory.FormatContext()
				systemPrompt := fmt.Sprintf("%s\n\n%s", engine.SystemPrompt, currentContext)

				// Construct full message chain: [System, ...History]
				messages := append([]openrouter.ChatMessage{
					{Role: "system", Content: systemPrompt},
				}, history...)

				// Call LLM
				req := openrouter.ChatCompletionRequest{
					Model:     engine.Model,
					Messages:  messages,
					Tools:     activeTools,
					MaxTokens: engine.Limits.MaxStepTokens,
				}

				// Show a spinner while "Thinking"
				fmt.Print("🤖 Thinking...")
				resp, err := client.CreateChatCompletion(cmd.Context(), req)
				fmt.Print("\r\033[K") // Clear line

				if err != nil {
					ui.Standard.Error("LLM Error: %v", err)
					break
				}

				// Track Costs
				if resp.Usage != nil {
					engine.CostTracker.RecordRequest(
						resp.Usage.PromptTokens,
						resp.Usage.CompletionTokens,
						engine.Model,
					)
				}

				msg := resp.Choices[0].Message

				// --- Case A: Tool Call ---
				if len(msg.ToolCalls) > 0 {
					// Append the "assistant" message with tool calls to history
					history = append(history, *msg)

					// Execute Tools
					for _, toolCall := range msg.ToolCalls {
						ui.Standard.Info("⚙️  Executing: %s", toolCall.Function.Name)

						// Use the engine's executor (handles security/policy)
						result := engine.Executor.Execute(cmd.Context(), toolCall)

						// Add "tool" result message to history
						history = append(history, openrouter.ChatMessage{
							Role:       "tool",
							ToolCallID: toolCall.ID,
							Content:    result,
						})
					}
					// Loop back to send tool results to LLM
					continue
				}

				// --- Case B: Text Response (Final Answer) ---
				content := fmt.Sprintf("%v", msg.Content)

				// Print the response
				fmt.Printf("\n🤖 \033[1mAgent:\033[0m %s\n\n", content)

				// Add to history
				history = append(history, *msg)

				// Break inner loop to wait for next user input
				break
			}
		}

		// Exit Summary
		cost, _ := engine.CostTracker.GetStats()
		ui.Standard.Info("Session ended. Total Cost: $%.4f", cost)
		return nil
	},
}

func init() {
	// Register the command with the root
	rootCmd.AddCommand(chatCmd)
}

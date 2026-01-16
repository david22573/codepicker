package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/constants"
	"github.com/david22573/codepicker/internal/contextgen"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/spf13/cobra"
)

var chatModel string
var clearHistory bool

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start a smart interactive chat session with your codebase",
	RunE: func(cmd *cobra.Command, args []string) error {
		apiKey, err := validateAPIKey()
		if err != nil {
			return err
		}

		// 1. Initialize SQLite Store
		store, err := database.New(".codepicker")
		if err != nil {
			return fmt.Errorf("failed to init database: %w", err)
		}
		defer store.Close()

		if clearHistory {
			store.ClearHistory()
			appLogger.Info("Chat history cleared.")
		}

		// 2. Generate Shallow Context (Tree)
		appLogger.Info("Scanning project structure...")
		treeContext, err := contextgen.GenerateTree(srcDir)
		if err != nil {
			return err
		}

		// 3. Define System Prompt with Shallow Context
		systemPrompt := fmt.Sprintf(
			"You are an expert coding assistant. Date: %s.\n"+
				"MODE: Interactive Chat.\n"+
				"%s\n"+
				"NOTE: You see the file tree above. If you need to read specific code to answer a question, "+
				"ask the user or use tools (if available) to read files. Do not guess code content.",
			time.Now().Format("2006-01-02"),
			treeContext,
		)

		fmt.Println("\n💬 Smart Chat Mode (Type 'exit' to quit)")
		fmt.Println(strings.Repeat("─", 60))

		client := openrouter.NewClient(apiKey)
		scanner := bufio.NewScanner(os.Stdin)

		for {
			fmt.Print("\n👉 You: ")
			if !scanner.Scan() {
				break
			}
			input := strings.TrimSpace(scanner.Text())

			if input == "exit" || input == "quit" {
				break
			}
			if input == "" {
				continue
			}
			if input == "/clear" {
				store.ClearHistory()
				fmt.Println("🧹 History cleared.")
				continue
			}

			// Save user message to DB
			if err := store.AddMessage("user", input); err != nil {
				appLogger.Error("Failed to save message: " + err.Error())
			}

			// 4. Smart Context Loading (Token Budgeting)
			// Limit: 128k total - ~4k for system prompt - ~2k safety buffer = ~122k available
			history, err := store.GetContextAwareHistory(122000)
			if err != nil {
				appLogger.Error("Failed to load history: " + err.Error())
				continue
			}

			// Construct full request: System + History
			messages := append([]openrouter.ChatMessage{{Role: "system", Content: systemPrompt}}, history...)

			req := openrouter.ChatCompletionRequest{
				Model:    chatModel,
				Messages: messages,
				Stream:   true,
			}

			fmt.Print("🤖 AI: ")
			stream, err := client.CreateChatCompletionStream(cmd.Context(), req)
			if err != nil {
				appLogger.Error(fmt.Sprintf("Stream error: %v", err))
				continue
			}

			var responseBuf strings.Builder
			for {
				resp, err := stream.Recv()
				if err != nil {
					break
				}
				if len(resp.Choices) > 0 {
					if content, ok := resp.Choices[0].Delta.Content.(string); ok {
						fmt.Print(content)
						responseBuf.WriteString(content)
					}
				}
			}
			stream.Close()
			fmt.Println()

			// Save AI response to DB
			store.AddMessage("assistant", responseBuf.String())
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(chatCmd)
	chatCmd.Flags().StringVarP(&chatModel, "model", "m", constants.DefaultModel, "Model ID")
	chatCmd.Flags().BoolVarP(&clearHistory, "clear", "c", false, "Clear previous chat history")
}

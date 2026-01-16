package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/constants"
	"github.com/david22573/codepicker/internal/contextgen"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/spf13/cobra"
)

var chatModel string
var clearHistory bool

const historyFile = ".codepicker/chat_history.json"

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start an interactive chat session with your codebase",
	RunE: func(cmd *cobra.Command, args []string) error {

		apiKey, err := validateAPIKey()
		if err != nil {
			return err
		}

		if clearHistory {
			_ = os.Remove(historyFile)
			appLogger.Info("Chat history cleared.")
		}

		appLogger.Info("Analyzing codebase...")

		focusList, err := validateFocusFiles(focusFile)
		if err != nil {
			return err
		}

		opts := contextgen.Options{
			SrcDir:      srcDir,
			FocusFiles:  focusList,
			Minify:      minify,
			IncludeExts: includeExts,
			IgnoreDirs:  ignoreDirs,
		}

		codeContext, err := contextgen.Generate(cmd.Context(), opts, appLogger)
		if err != nil {
			return err
		}

		appLogger.Info(fmt.Sprintf("Context loaded (%d chars). Starting chat...", len(codeContext)))
		fmt.Println("\n💬 Interactive Chat Mode (type 'exit' to quit, '/clear' to reset)")
		fmt.Println(strings.Repeat("─", 60))

		client := openrouter.NewClient(apiKey)

		// Load history or init new
		history := loadHistory()

		// Update system prompt with fresh context (always replace the first message)
		systemMsg := openrouter.ChatMessage{
			Role: "system",
			Content: fmt.Sprintf("You are an expert coding assistant. Date: %s.\nCodebase Context:\n%s",
				time.Now().Format("2006-01-02"), codeContext),
		}

		if len(history) == 0 {
			history = []openrouter.ChatMessage{
				systemMsg,
				{Role: "assistant", Content: "I've analyzed your code. What would you like to know?"},
			}
		} else {
			// Update the system prompt in existing history
			history[0] = systemMsg
			fmt.Printf("📝 Resumed conversation with %d previous messages.\n", len(history)-1)
		}

		scanner := bufio.NewScanner(os.Stdin)
		for {
			fmt.Print("\n👉 You: ")
			if !scanner.Scan() {
				break
			}

			input := strings.TrimSpace(scanner.Text())
			if input == "" {
				continue
			}
			if input == "exit" || input == "quit" {
				break
			}
			if input == "/clear" {
				history = []openrouter.ChatMessage{
					systemMsg,
					{Role: "assistant", Content: "Context cleared. What's next?"},
				}
				_ = os.Remove(historyFile)
				fmt.Println("🧹 History cleared.")
				continue
			}

			history = append(history, openrouter.ChatMessage{Role: "user", Content: input})

			req := openrouter.ChatCompletionRequest{
				Model:    chatModel,
				Messages: history,
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

			history = append(history, openrouter.ChatMessage{Role: "assistant", Content: responseBuf.String()})
			saveHistory(history)
		}
		return nil
	},
}

func loadHistory() []openrouter.ChatMessage {
	data, err := os.ReadFile(historyFile)
	if err != nil {
		return nil
	}
	var history []openrouter.ChatMessage
	if err := json.Unmarshal(data, &history); err != nil {
		return nil
	}
	return history
}

func saveHistory(history []openrouter.ChatMessage) {
	// Ensure directory exists
	_ = os.MkdirAll(filepath.Dir(historyFile), 0755)

	// Truncate history if it gets too large (simple approach: keep last 20 messages + system prompt)
	if len(history) > 21 {
		kept := append([]openrouter.ChatMessage{history[0]}, history[len(history)-20:]...)
		history = kept
	}

	data, _ := json.MarshalIndent(history, "", "  ")
	_ = os.WriteFile(historyFile, data, 0644)
}

func init() {
	rootCmd.AddCommand(chatCmd)
	chatCmd.Flags().StringVarP(&chatModel, "model", "m", constants.DefaultModel, "Model ID")
	chatCmd.Flags().StringVarP(&focusFile, "focus", "f", "", "Comma-separated list of files")
	chatCmd.Flags().BoolVarP(&clearHistory, "clear", "c", false, "Clear previous chat history on startup")
}

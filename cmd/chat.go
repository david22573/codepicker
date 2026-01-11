package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/constants"
	"github.com/david22573/codepicker/internal/contextgen"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/spf13/cobra"
)

var chatModel string

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start an interactive chat session with your codebase",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Setup
		apiKey, err := validateAPIKey()
		if err != nil {
			return err
		}

		// 2. Prepare Context (Using the new internal package)
		appLogger.Info("Analyzing codebase...")

		// Parse focus files using the shared helper
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
		fmt.Println("\n💬 Interactive Chat Mode (type 'exit' to quit)")
		fmt.Println(strings.Repeat("─", 60))

		// 3. Init Chat History
		client := openrouter.NewClient(apiKey)
		history := []openrouter.ChatMessage{
			{
				Role: "system",
				Content: fmt.Sprintf("You are an expert coding assistant. Date: %s.\nCodebase Context:\n%s",
					time.Now().Format("2006-01-02"), codeContext),
			},
			{Role: "assistant", Content: "I've analyzed your code. What would you like to know?"},
		}

		// 4. REPL Loop
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

			history = append(history, openrouter.ChatMessage{Role: "user", Content: input})

			// Create Request
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
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(chatCmd)
	chatCmd.Flags().StringVarP(&chatModel, "model", "m", constants.DefaultModel, "Model ID")
	chatCmd.Flags().StringVarP(&focusFile, "focus", "f", "", "Comma-separated list of files")
}

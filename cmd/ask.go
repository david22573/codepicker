package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/david22573/codepicker/internal/constants"
	"github.com/david22573/codepicker/internal/contextgen"
	"github.com/david22573/codepicker/internal/planner"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/spf13/cobra"
)

var (
	askModel  string
	focusFile string
	smartMode bool
	rawOutput bool
	askCopy   bool // New flag variable
)

var askCmd = &cobra.Command{
	Use:   "ask [query]",
	Short: "Ask AI about the codebase",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")
		appLogger.Info(fmt.Sprintf("Ask command initiated with query: %s", query))

		apiKey, err := validateAPIKey()
		if err != nil {
			if !rawOutput {
				fmt.Fprintln(os.Stderr, "\n≡ƒÆí To fix this:")
				fmt.Fprintln(os.Stderr, "   1. Get your API key from https://openrouter.ai/settings/keys")
				fmt.Fprintln(os.Stderr, "   2. Set it: export OPENROUTER_API_KEY=your_key_here")
				fmt.Fprintln(os.Stderr, "   3. Or create a .env file with: OPENROUTER_API_KEY=your_key_here")
				fmt.Fprintln(os.Stderr, "\nΓÜá∩╕Å  WARNING: Never commit your API key!")
			}
			return err
		}
		appLogger.Info("API key validated")

		ctx := cmd.Context()

		// 1. Smart Mode: Pre-select files using the Planner
		if smartMode {
			planOpts := planner.Options{
				APIKey:      apiKey,
				Model:       askModel,
				SrcDir:      srcDir,
				Query:       query,
				IncludeExts: includeExts,
				IgnoreDirs:  ignoreDirs,
			}

			selectedFiles, err := planner.SelectRelevantFiles(ctx, planOpts, appLogger)
			if err != nil {
				appLogger.Warn(fmt.Sprintf("Smart planning failed: %v", err))
			} else if len(selectedFiles) > 0 {
				aiFiles := strings.Join(selectedFiles, ",")
				if focusFile != "" {
					focusFile = focusFile + "," + aiFiles
				} else {
					focusFile = aiFiles
				}
			} else {
				appLogger.Warn("Smart mode could not identify relevant files. Proceeding with user defaults.")
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 2. Context Generation
		appLogger.Info("Generating context...")

		var focusList []string
		if focusFile != "" {
			focusList, err = validateFocusFiles(focusFile)
			if err != nil {
				return err
			}
		}

		genOpts := contextgen.Options{
			SrcDir:      srcDir,
			FocusFiles:  focusList,
			Minify:      minify,
			IncludeExts: includeExts,
			IgnoreDirs:  ignoreDirs,
		}

		contextString, err := contextgen.Generate(ctx, genOpts, appLogger)
		if err != nil {
			return fmt.Errorf("failed to generate context: %w", err)
		}

		if len(contextString) == 0 {
			if smartMode {
				return fmt.Errorf("smart mode yielded no relevant files (try standard mode or broader query)")
			}
			return fmt.Errorf("no context generated (check your filters)")
		}

		appLogger.Info(fmt.Sprintf("Context generated: %d chars", len(contextString)))

		// 3. Construct the LLM Request
		client := openrouter.NewClient(apiKey)
		contextType := "Codebase"
		if len(focusList) > 0 {
			contextType = "Active File"
		}

		systemMsg := fmt.Sprintf(
			"You are an expert coding assistant. Date: %s. Use the provided %s Context to answer.\n"+
				"CRITICAL INSTRUCTION: Output clean, multi-line, properly indented code. DO NOT minify.",
			time.Now().Format("2006-01-02"), contextType,
		)

		userMsg := fmt.Sprintf("Context:\n%s\n\nQuestion: %s", contextString, query)

		req := openrouter.ChatCompletionRequest{
			Model: askModel,
			Messages: []openrouter.ChatMessage{
				{Role: "system", Content: systemMsg},
				{Role: "user", Content: userMsg},
			},
			Stream: true,
		}

		appLogger.Info(fmt.Sprintf("Sending request to model: %s", askModel))

		stream, err := client.CreateChatCompletionStream(ctx, req)
		if err != nil {
			appLogger.Error(fmt.Sprintf("API Error: %v", err))
			appLogger.Info("≡ƒÆí Check your API key and network connection")
			return err
		}
		defer stream.Close()

		if !rawOutput {
			fmt.Println("\n≡ƒñû AI Response:")
			fmt.Println(strings.Repeat("ΓöÇ", 60))
		}

		// 4. Stream and Capture
		var responseBuf strings.Builder // Buffer to capture full response for clipboard

		for {
			resp, err := stream.Recv()
			if err != nil {
				break
			}
			if len(resp.Choices) > 0 && resp.Choices[0].Delta != nil {
				content := resp.Choices[0].Delta.Content
				if str, ok := content.(string); ok {
					fmt.Print(str)
					responseBuf.WriteString(str) // Capture to memory
				}
			}
		}

		if !rawOutput {
			fmt.Println()
		}

		appLogger.Info("Response streaming completed")

		// 5. Handle Clipboard Copy
		if askCopy {
			cleanResponse := responseBuf.String()
			if cleanResponse != "" {
				if err := clipboard.WriteAll(cleanResponse); err != nil {
					appLogger.Warn(fmt.Sprintf("Failed to copy to clipboard: %v", err))
				} else {
					appLogger.Info("≡ƒôï Response copied to clipboard!")
				}
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(askCmd)
	askCmd.Flags().StringVarP(&askModel, "model", "m", constants.DefaultModel, "Model ID")
	askCmd.Flags().StringVarP(&focusFile, "focus", "f", "", "Comma-separated list of files to scan")
	askCmd.Flags().BoolVarP(&smartMode, "smart", "S", false, "Use AI to intelligently select relevant files")
	askCmd.Flags().BoolVarP(&rawOutput, "raw", "r", false, "Output only the raw AI response (no headers) for piping")
	askCmd.Flags().BoolVarP(&askCopy, "copy", "C", false, "Copy the AI response to system clipboard")
}

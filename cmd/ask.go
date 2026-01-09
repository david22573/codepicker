package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/scanner"
	"github.com/david22573/codepicker/internal/writer"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/spf13/cobra"
)

var (
	askModel  string
	focusFile string
)

var askCmd = &cobra.Command{
	Use:   "ask [query]",
	Short: "Ask AI about the codebase",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		query := strings.Join(args, " ")
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			fmt.Println("Error: OPENROUTER_API_KEY is not set")
			os.Exit(1)
		}

		// Create a temp file for context
		tmpFile, _ := os.CreateTemp("", "agent_context_*.md")
		tmpPath := tmpFile.Name()
		tmpFile.Close()
		defer os.Remove(tmpPath)

		// FIX 1: Set minify to false so the AI receives and generates readable code
		w := writer.NewConcatStrategy(tmpPath, false)
		w.Init()

		if focusFile != "" {
			// Fast path: Only scan specific files
			files := strings.Split(focusFile, ",")

			// FIX 2: Commented out debug print so it doesn't appear in Neovim buffer
			// fmt.Printf("🔍 Focused Context: %v\n", files)

			for _, f := range files {
				abs, err := filepath.Abs(f)
				if err == nil {
					w.Write(abs, f)
				}
			}
		} else {
			// Slow path: Scan entire directory
			absSrc, _ := filepath.Abs(srcDir)
			cfg := config.NewConfig()
			if includeExts != "" {
				cfg.AddAllowedExtensions(strings.Split(includeExts, ","))
			}
			if ignoreDirs != "" {
				cfg.AddIgnoredDirs(strings.Split(ignoreDirs, ","))
			}
			s := scanner.NewScanner(absSrc, w, cfg)
			s.Scan()
		}
		w.Close()

		// Read the context file
		contextBytes, err := os.ReadFile(tmpPath)
		if err != nil {
			fmt.Printf("Error reading context: %v\n", err)
			os.Exit(1)
		}

		// Prepare OpenRouter Client
		client := openrouter.NewClient(apiKey)

		contextType := "Codebase"
		if focusFile != "" {
			contextType = "Active File"
		}

		systemMsg := fmt.Sprintf(
			"You are an expert coding assistant. Date: %s. Use the provided %s Context to answer.",
			time.Now().Format("2006-01-02"), contextType,
		)

		userMsg := fmt.Sprintf("Context:\n%s\n\nQuestion: %s", string(contextBytes), query)

		req := openrouter.ChatCompletionRequest{
			Model: askModel,
			Messages: []openrouter.ChatMessage{
				{Role: "system", Content: systemMsg},
				{Role: "user", Content: userMsg},
			},
			Stream: true,
		}

		ctx := context.Background()
		stream, err := client.CreateChatCompletionStream(ctx, req)
		if err != nil {
			fmt.Printf("API Error: %v\n", err)
			os.Exit(1)
		}
		defer stream.Close()

		for {
			resp, err := stream.Recv()
			if err != nil {
				break
			}
			if len(resp.Choices) > 0 && resp.Choices[0].Delta != nil {
				content := resp.Choices[0].Delta.Content
				if str, ok := content.(string); ok {
					fmt.Print(str)
				}
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(askCmd)
	askCmd.Flags().StringVarP(&askModel, "model", "m", "xiaomi/mimo-v2-flash:free", "Model ID")
	askCmd.Flags().StringVarP(&focusFile, "focus", "f", "", "Comma-separated list of files to scan")
}


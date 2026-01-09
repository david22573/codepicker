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
	focusFile string // New flag for single file context
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

		// Temp file for context
		tmpFile, _ := os.CreateTemp("", "agent_context_*.md")
		tmpPath := tmpFile.Name()
		tmpFile.Close()
		defer os.Remove(tmpPath)

		// --- CONTEXT GATHERING STRATEGY ---
		w := writer.NewConcatStrategy(tmpPath, true) // Always minify
		w.Init()                                     // Manually init since we might not use Scanner's Scan()

		if focusFile != "" {
			// FAST PATH: Only scan the specific file(s) requested
			files := strings.Split(focusFile, ",")
			fmt.Printf("🔍 Focused Context: %v\n", files)

			for _, f := range files {
				abs, err := filepath.Abs(f)
				if err == nil {
					// Reuse writer logic to format/minify it
					w.Write(abs, f)
				}
			}
		} else {
			// SLOW PATH: Scan entire directory
			absSrc, _ := filepath.Abs(srcDir)
			cfg := config.NewConfig()
			if includeExts != "" {
				cfg.AddAllowedExtensions(strings.Split(includeExts, ","))
			}
			if ignoreDirs != "" {
				cfg.AddIgnoredDirs(strings.Split(ignoreDirs, ","))
			}
			s := scanner.NewScanner(absSrc, w, cfg)
			// Note: Scanner calls w.Init/Close internally, but we called Init above.
			// Ideally, refactor Scanner to not strictly own Init/Close,
			// but for now, let's just use the scanner normally if no focus.
			s.Scan()
		}
		w.Close()

		// Read the context
		contextBytes, err := os.ReadFile(tmpPath)
		if err != nil {
			fmt.Printf("Error reading context: %v\n", err)
			os.Exit(1)
		}

		// --- API CALL ---
		client := openrouter.NewClient(apiKey)

		// Updated System Prompt with "Current File" awareness if focused
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
	askCmd.Flags().StringVarP(&focusFile, "focus", "f", "", "Comma-separated list of files to scan (ignores directory scan)")
}


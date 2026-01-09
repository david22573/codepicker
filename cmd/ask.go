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

		tmpFile, _ := os.CreateTemp("", "agent_context_*.md")
		tmpPath := tmpFile.Name()
		tmpFile.Close()
		defer os.Remove(tmpPath)

		// CRITICAL FIX: Set minify to FALSE so AI returns clean multi-line code
		w := writer.NewConcatStrategy(tmpPath, false)
		w.Init()

		if focusFile != "" {
			files := strings.Split(focusFile, ",")

			// CRITICAL FIX: Commented out debug print to prevent "Ghost Lines" in Neovim Diff
			// fmt.Printf("🔍 Focused Context: %v\n", files)

			for _, f := range files {
				abs, err := filepath.Abs(f)
				if err == nil {
					w.Write(abs, f)
				}
			}
		} else {
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

		contextBytes, err := os.ReadFile(tmpPath)
		if err != nil {
			fmt.Printf("Error reading context: %v\n", err)
			os.Exit(1)
		}

		client := openrouter.NewClient(apiKey)

		contextType := "Codebase"
		if focusFile != "" {
			contextType = "Active File"
		}

		// Prompt updated to strictly forbid minification
		systemMsg := fmt.Sprintf(
			"You are an expert coding assistant. Date: %s. Use the provided %s Context to answer.\n"+
				"CRITICAL INSTRUCTION: Output clean, multi-line, properly indented code. DO NOT minify.",
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


package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/scanner"
	"github.com/david22573/codepicker/internal/writer"
	"github.com/david22573/codepicker/pkg/openrouter" // Imported from your copied package
	"github.com/spf13/cobra"
)

var askModel string

var askCmd = &cobra.Command{
	Use:   "ask [query]",
	Short: "Ask AI about the codebase using OpenRouter",
	Long:  `Scans the current directory (respecting .gitignore) and sends the context + query to OpenRouter.`,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		query := strings.Join(args, " ")
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			fmt.Println("Error: OPENROUTER_API_KEY is not set")
			os.Exit(1)
		}

		// --- PHASE 1: THE ENGINE (Gather Context) ---
		// We use a temporary file to store the scan result
		tmpFile, _ := os.CreateTemp("", "agent_context_*.md")
		tmpPath := tmpFile.Name()
		tmpFile.Close()
		defer os.Remove(tmpPath) // Clean up after we are done

		// Configure scanner (reusing your existing logic)
		absSrc, _ := filepath.Abs(srcDir)
		w := writer.NewConcatStrategy(tmpPath, true) // minify=true to save tokens
		cfg := config.NewConfig()

		// Apply flags if set (reusing global flags from root.go)
		if includeExts != "" {
			cfg.AddAllowedExtensions(strings.Split(includeExts, ","))
		}
		if ignoreDirs != "" {
			cfg.AddIgnoredDirs(strings.Split(ignoreDirs, ","))
		}

		s := scanner.NewScanner(absSrc, w, cfg)
		if err := s.Scan(); err != nil {
			fmt.Printf("Scan failed: %v\n", err)
			os.Exit(1)
		}

		// Read the massive context file
		contextBytes, err := os.ReadFile(tmpPath)
		if err != nil {
			fmt.Printf("Read failed: %v\n", err)
			os.Exit(1)
		}

		// Safety Check: Warn if context is massive (> 500k chars approx 125k tokens)
		if len(contextBytes) > 2000000 {
			fmt.Printf("⚠️  Warning: Context is very large (%d bytes). This might timeout on free tier.\n", len(contextBytes))
		}

		// --- PHASE 2: THE BRAIN (Call OpenRouter) ---
		client := openrouter.NewClient(apiKey)

		systemMsg := "You are an expert coding assistant. Use the provided Codebase Context to answer the user's question accurately."
		userMsg := fmt.Sprintf("Codebase Context:\n%s\n\nQuestion: %s", string(contextBytes), query)

		req := openrouter.ChatCompletionRequest{
			Model: askModel,
			Messages: []openrouter.ChatMessage{
				{Role: "system", Content: systemMsg},
				{Role: "user", Content: userMsg},
			},
			Stream: true, // Essential for Neovim feel
		}

		ctx := context.Background()
		stream, err := client.CreateChatCompletionStream(ctx, req)
		if err != nil {
			fmt.Printf("API Error: %v\n", err)
			os.Exit(1)
		}
		defer stream.Close()

		// Stream stdout so Neovim can capture it line-by-line
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
	// Default to the 256k context model you wanted
	askCmd.Flags().StringVarP(&askModel, "model", "m", "xiaomi/mimo-v2-flash:free", "Model ID")
}

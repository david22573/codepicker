package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/constants"
	"github.com/david22573/codepicker/internal/paths"
	"github.com/david22573/codepicker/internal/scanner"
	"github.com/david22573/codepicker/internal/writer"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/spf13/cobra"
)

var (
	askModel  string
	focusFile string
	smartMode bool
)

type PathCollector struct {
	Paths []string
}

func (p *PathCollector) Init() error                 { return nil }
func (p *PathCollector) Write(abs, rel string) error { p.Paths = append(p.Paths, rel); return nil }
func (p *PathCollector) Close() error                { return nil }
func (p *PathCollector) ShouldSkip(path string) bool { return false }
func (p *PathCollector) Name() string                { return "Collector" }

func validateAPIKey() (string, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")

	if apiKey == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY environment variable is not set")
	}

	if len(apiKey) < constants.MinAPIKeyLength {
		return "", fmt.Errorf("API key appears invalid (length < %d)", constants.MinAPIKeyLength)
	}

	return apiKey, nil
}

func validateFocusFiles(focusList string) ([]string, error) {
	if focusList == "" {
		return nil, nil
	}

	files := strings.Split(focusList, ",")
	var validated []string

	for _, f := range files {
		clean, err := paths.Sanitize(f)
		if err != nil {
			return nil, fmt.Errorf("invalid focus file path '%s': %w", f, err)
		}

		info, err := os.Stat(clean)
		if err != nil {
			appLogger.Warn(fmt.Sprintf("Focus file not found (skipping): %s", clean))
			continue
		}

		if info.IsDir() {
			return nil, fmt.Errorf("focus file is a directory (use -s for directories): %s", clean)
		}

		validated = append(validated, clean)
		appLogger.Debug(fmt.Sprintf("Validated focus file: %s", clean))
	}

	return validated, nil
}

func callLLMForPaths(apiKey, model, sysMsg, userMsg string) []string {
	client := openrouter.NewClient(apiKey)
	req := openrouter.ChatCompletionRequest{
		Model: model,
		Messages: []openrouter.ChatMessage{
			{Role: "system", Content: sysMsg},
			{Role: "user", Content: userMsg},
		},
		ResponseFormat: &openrouter.ResponseFormat{Type: "json_object"},
	}

	ctx := context.Background()
	resp, err := client.CreateChatCompletion(ctx, req)
	if err != nil {
		appLogger.Warn(fmt.Sprintf("Smart planning failed (API error): %v. Falling back to normal scan.", err))
		return nil
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message == nil {
		return nil
	}

	contentStr, ok := resp.Choices[0].Message.Content.(string)
	if !ok {
		appLogger.Warn("Failed to parse AI response content (not a string)")
		return nil
	}

	content := strings.TrimSpace(contentStr)
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
	}

	var resultObj struct {
		Files []string `json:"files"`
	}
	if err := json.Unmarshal([]byte(content), &resultObj); err == nil && len(resultObj.Files) > 0 {
		return resultObj.Files
	}

	var paths []string
	if err := json.Unmarshal([]byte(content), &paths); err == nil {
		return paths
	}

	appLogger.Warn(fmt.Sprintf("Failed to parse AI planning JSON. Response was: %s", content))
	return nil
}

var askCmd = &cobra.Command{
	Use:   "ask [query]",
	Short: "Ask AI about the codebase",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")
		appLogger.Info(fmt.Sprintf("Ask command initiated with query: %s", query))

		apiKey, err := validateAPIKey()
		if err != nil {
			fmt.Println("\n💡 To fix this:")
			fmt.Println("   1. Get your API key from https://openrouter.ai/settings/keys")
			fmt.Println("   2. Set it: export OPENROUTER_API_KEY=your_key_here")
			fmt.Println("   3. Or create a .env file with: OPENROUTER_API_KEY=your_key_here")
			fmt.Println("\n⚠️  WARNING: Never commit your API key!")
			return err
		}
		appLogger.Info("API key validated")

		if smartMode && focusFile == "" {
			appLogger.Info("🧠 Smart mode enabled: Planning context...")

			absSrc, err := paths.Sanitize(srcDir)
			if err != nil {
				return fmt.Errorf("invalid source directory: %w", err)
			}

			collector := &PathCollector{}
			cfg := config.NewConfig()
			if includeExts != "" {
				cfg.AddAllowedExtensions(strings.Split(includeExts, ","))
			}
			if ignoreDirs != "" {
				cfg.AddIgnoredDirs(strings.Split(ignoreDirs, ","))
			}

			s := scanner.NewScanner(absSrc, collector, cfg, appLogger)

			if err := s.Scan(cmd.Context()); err == nil && len(collector.Paths) > 0 {
				fileList := strings.Join(collector.Paths, "\n")
				appLogger.Info(fmt.Sprintf("Found %d files. Asking AI to select relevant ones...", len(collector.Paths)))

				sysMsg := `You are a senior developer. You have a list of files in a codebase.
Based on the user's query, identify exactly which files contain the relevant code to answer the question.
Return ONLY a valid JSON object with a single key "files" containing the list of strings.
Example: { "files": ["cmd/main.go", "internal/utils.go"] }
If no specific code is needed, return { "files": [] }.`

				userMsg := fmt.Sprintf("Files:\n%s\n\nQuery: %s", fileList, query)

				selectedFiles := callLLMForPaths(apiKey, askModel, sysMsg, userMsg)

				if len(selectedFiles) > 0 {
					focusFile = strings.Join(selectedFiles, ",")
					appLogger.Info(fmt.Sprintf("🤖 AI selected %d files: %v", len(selectedFiles), selectedFiles))
				} else {
					appLogger.Info("🤖 AI decided no files are needed (or failed to pick), proceeding with full context.")
				}
			} else {
				appLogger.Warn("Scanner found no files for planning. Proceeding normally.")
			}
		}

		tmpFile, err := os.CreateTemp("", "agent_context_*.md")
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()
		defer func() {
			if err := os.Remove(tmpPath); err != nil {
				appLogger.Warn(fmt.Sprintf("Failed to remove temp file: %v", err))
			}
		}()

		appLogger.Debug(fmt.Sprintf("Temporary context file: %s", tmpPath))

		w := writer.NewConcatStrategy(tmpPath, minify)
		if err := w.Init(); err != nil {
			return fmt.Errorf("failed to initialize writer: %w", err)
		}

		if focusFile != "" {
			validatedFiles, err := validateFocusFiles(focusFile)
			if err != nil {
				return err
			}

			if len(validatedFiles) == 0 {
				appLogger.Warn("No valid files in focus list. Generating empty context.")
			} else {
				appLogger.Info(fmt.Sprintf("Focus mode: %d file(s) selected", len(validatedFiles)))
				for _, f := range validatedFiles {
					abs, err := filepath.Abs(f)
					if err == nil {
						rel, _ := filepath.Rel(".", abs)
						fmt.Printf("   + %s\n", rel)
						if err := w.Write(abs, rel); err != nil {
							appLogger.Warn(fmt.Sprintf("Failed to write %s: %v", rel, err))
						}
					}
				}
			}
		} else {
			absSrc, err := paths.Sanitize(srcDir)
			if err != nil {
				return fmt.Errorf("invalid source directory: %w", err)
			}

			cfg := config.NewConfig()
			if includeExts != "" {
				cfg.AddAllowedExtensions(strings.Split(includeExts, ","))
			}
			if ignoreDirs != "" {
				cfg.AddIgnoredDirs(strings.Split(ignoreDirs, ","))
			}

			s := scanner.NewScanner(absSrc, w, cfg, appLogger)

			if err := s.Scan(cmd.Context()); err != nil {
				return fmt.Errorf("scan failed: %w", err)
			}
		}

		if err := w.Close(); err != nil {
			return fmt.Errorf("failed to write context: %w", err)
		}

		contextBytes, err := os.ReadFile(tmpPath)
		if err != nil {
			return fmt.Errorf("failed to read context: %w", err)
		}

		if len(contextBytes) == 0 && !smartMode {
			return fmt.Errorf("no context generated (check your filters)")
		}

		appLogger.Info(fmt.Sprintf("Context generated: %d bytes", len(contextBytes)))

		client := openrouter.NewClient(apiKey)
		contextType := "Codebase"
		if focusFile != "" {
			contextType = "Active File"
		}

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

		appLogger.Info(fmt.Sprintf("Sending request to model: %s", askModel))

		ctx := context.Background()
		stream, err := client.CreateChatCompletionStream(ctx, req)
		if err != nil {
			appLogger.Error(fmt.Sprintf("API Error: %v", err))
			appLogger.Info("💡 Check your API key and network connection")
			return err
		}
		defer stream.Close()

		fmt.Println("\n🤖 AI Response:")
		fmt.Println(strings.Repeat("─", 60))

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
		fmt.Println()
		appLogger.Info("Response streaming completed")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(askCmd)
	askCmd.Flags().StringVarP(&askModel, "model", "m", constants.DefaultModel, "Model ID")
	askCmd.Flags().StringVarP(&focusFile, "focus", "f", "", "Comma-separated list of files to scan")
	askCmd.Flags().BoolVarP(&smartMode, "smart", "S", false, "Use AI to intelligently select relevant files")
}

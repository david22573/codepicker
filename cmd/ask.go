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

// PathCollector is a lightweight writer strategy that just collects filenames
type PathCollector struct {
	Paths []string
}

func (p *PathCollector) Init() error                 { return nil }
func (p *PathCollector) Write(abs, rel string) error { p.Paths = append(p.Paths, rel); return nil }
func (p *PathCollector) Close() error                { return nil }
func (p *PathCollector) ShouldSkip(path string) bool { return false }
func (p *PathCollector) Name() string                { return "Collector" }

func validateAPIKey() string {
	apiKey := os.Getenv("OPENROUTER_API_KEY")

	if apiKey == "" {
		logError("OPENROUTER_API_KEY environment variable is not set")
		fmt.Println("\n💡 To fix this:")
		fmt.Println("   1. Get your API key from https://openrouter.ai/settings/keys")
		fmt.Println("   2. Set it: export OPENROUTER_API_KEY=your_key_here")
		fmt.Println("   3. Or create a .env file with: OPENROUTER_API_KEY=your_key_here")
		fmt.Println("\n⚠️  WARNING: Never commit your API key!")
		os.Exit(1)
	}

	if len(apiKey) < 10 {
		logError(fmt.Sprintf("API key appears invalid (too short): %s", apiKey[:5]+"..."))
		os.Exit(1)
	}

	return apiKey
}

func validateFocusFiles(focusList string) []string {
	if focusList == "" {
		return nil
	}

	files := strings.Split(focusList, ",")
	var validated []string

	for _, f := range files {
		clean, err := sanitizePath(f)
		if err != nil {
			logError(fmt.Sprintf("Invalid focus file path '%s': %v", f, err))
			os.Exit(1)
		}

		info, err := os.Stat(clean)
		if err != nil {
			// In smart mode, the LLM might hallucinate a file, so we warn instead of exit
			logWarn(fmt.Sprintf("Focus file not found (skipping): %s", clean))
			continue
		}

		if info.IsDir() {
			logError(fmt.Sprintf("Focus file is a directory (use -s for directories): %s", clean))
			os.Exit(1)
		}

		validated = append(validated, clean)
		logDebug(fmt.Sprintf("Validated focus file: %s", clean))
	}

	return validated
}

// callLLMForPaths handles the "Planning" step: sending a file list to the LLM and parsing the JSON response.
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
		logWarn(fmt.Sprintf("Smart planning failed (API error): %v. Falling back to normal scan.", err))
		return nil
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message == nil {
		return nil
	}

	contentStr, ok := resp.Choices[0].Message.Content.(string)
	if !ok {
		logWarn("Failed to parse AI response content (not a string)")
		return nil
	}

	// Clean up potential markdown formatting in the JSON response
	content := strings.TrimSpace(contentStr)
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
	}

	// Try parsing as object { "files": [...] }
	var resultObj struct {
		Files []string `json:"files"`
	}
	if err := json.Unmarshal([]byte(content), &resultObj); err == nil && len(resultObj.Files) > 0 {
		return resultObj.Files
	}

	// Try parsing as direct array [...]
	var paths []string
	if err := json.Unmarshal([]byte(content), &paths); err == nil {
		return paths
	}

	logWarn(fmt.Sprintf("Failed to parse AI planning JSON. Response was: %s", content))
	return nil
}

var askCmd = &cobra.Command{
	Use:   "ask [query]",
	Short: "Ask AI about the codebase",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		query := strings.Join(args, " ")
		logInfo(fmt.Sprintf("Ask command initiated with query: %s", query))

		apiKey := validateAPIKey()
		logInfo("API key validated")

		// --- SMART MODE PLANNING START ---
		if smartMode && focusFile == "" {
			logInfo("🧠 Smart mode enabled: Planning context...")

			absSrc, err := sanitizePath(srcDir)
			if err != nil {
				logError(fmt.Sprintf("Invalid source directory: %v", err))
				os.Exit(1)
			}

			// Scan purely for file paths
			collector := &PathCollector{}
			cfg := config.NewConfig()
			if includeExts != "" {
				cfg.AddAllowedExtensions(strings.Split(includeExts, ","))
			}
			if ignoreDirs != "" {
				cfg.AddIgnoredDirs(strings.Split(ignoreDirs, ","))
			}

			s := scanner.NewScanner(absSrc, collector, cfg)
			if err := s.Scan(); err == nil && len(collector.Paths) > 0 {
				fileList := strings.Join(collector.Paths, "\n")
				logInfo(fmt.Sprintf("Found %d files. Asking AI to select relevant ones...", len(collector.Paths)))

				sysMsg := `You are a senior developer. You have a list of files in a codebase.
Based on the user's query, identify exactly which files contain the relevant code to answer the question.
Return ONLY a valid JSON object with a single key "files" containing the list of strings.
Example: { "files": ["cmd/main.go", "internal/utils.go"] }
If no specific code is needed, return { "files": [] }.`

				userMsg := fmt.Sprintf("Files:\n%s\n\nQuery: %s", fileList, query)

				selectedFiles := callLLMForPaths(apiKey, askModel, sysMsg, userMsg)

				if len(selectedFiles) > 0 {
					// Update focusFile so the logic below treats it as a manual focus
					focusFile = strings.Join(selectedFiles, ",")
					logInfo(fmt.Sprintf("🤖 AI selected %d files: %v", len(selectedFiles), selectedFiles))
				} else {
					logInfo("🤖 AI decided no files are needed (or failed to pick), proceeding with full context.")
				}
			} else {
				logWarn("Scanner found no files for planning. Proceeding normally.")
			}
		}
		// --- SMART MODE PLANNING END ---

		tmpFile, err := os.CreateTemp("", "agent_context_*.md")
		if err != nil {
			logError(fmt.Sprintf("Failed to create temp file: %v", err))
			os.Exit(1)
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()
		defer func() {
			if err := os.Remove(tmpPath); err != nil {
				logWarn(fmt.Sprintf("Failed to remove temp file: %v", err))
			}
		}()

		logDebug(fmt.Sprintf("Temporary context file: %s", tmpPath))

		w := writer.NewConcatStrategy(tmpPath, minify)
		if err := w.Init(); err != nil {
			logError(fmt.Sprintf("Failed to initialize writer: %v", err))
			os.Exit(1)
		}

		if focusFile != "" {
			validatedFiles := validateFocusFiles(focusFile)
			if len(validatedFiles) == 0 {
				logWarn("No valid files in focus list. Generating empty context.")
			} else {
				logInfo(fmt.Sprintf("Focus mode: %d file(s) selected", len(validatedFiles)))
				for _, f := range validatedFiles {
					abs, err := filepath.Abs(f)
					if err == nil {
						rel, _ := filepath.Rel(".", abs)
						fmt.Printf("   + %s\n", rel)
						if err := w.Write(abs, rel); err != nil {
							logWarn(fmt.Sprintf("Failed to write %s: %v", rel, err))
						}
					}
				}
			}
		} else {
			absSrc, err := sanitizePath(srcDir)
			if err != nil {
				logError(fmt.Sprintf("Invalid source directory: %v", err))
				os.Exit(1)
			}

			cfg := config.NewConfig()
			if includeExts != "" {
				cfg.AddAllowedExtensions(strings.Split(includeExts, ","))
			}
			if ignoreDirs != "" {
				cfg.AddIgnoredDirs(strings.Split(ignoreDirs, ","))
			}

			s := scanner.NewScanner(absSrc, w, cfg)
			if err := s.Scan(); err != nil {
				logError(fmt.Sprintf("Scan failed: %v", err))
				os.Exit(1)
			}
		}

		if err := w.Close(); err != nil {
			logError(fmt.Sprintf("Failed to write context: %v", err))
			os.Exit(1)
		}

		contextBytes, err := os.ReadFile(tmpPath)
		if err != nil {
			logError(fmt.Sprintf("Failed to read context: %v", err))
			os.Exit(1)
		}

		// Allow empty context if AI selected nothing (might be a general question)
		if len(contextBytes) == 0 && !smartMode {
			logError("No context generated (check your filters)")
			os.Exit(1)
		}

		logInfo(fmt.Sprintf("Context generated: %d bytes", len(contextBytes)))

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

		logInfo(fmt.Sprintf("Sending request to model: %s", askModel))

		ctx := context.Background()
		stream, err := client.CreateChatCompletionStream(ctx, req)
		if err != nil {
			logError(fmt.Sprintf("API Error: %v", err))
			logInfo("💡 Check your API key and network connection")
			os.Exit(1)
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
		logInfo("Response streaming completed")
	},
}

func init() {
	rootCmd.AddCommand(askCmd)
	askCmd.Flags().StringVarP(&askModel, "model", "m", "xiaomi/mimo-v2-flash:free", "Model ID")
	askCmd.Flags().StringVarP(&focusFile, "focus", "f", "", "Comma-separated list of files to scan")
	askCmd.Flags().BoolVarP(&smartMode, "smart", "S", false, "Use AI to intelligently select relevant files")
}

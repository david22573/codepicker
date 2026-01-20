package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/contextgen"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/paths"
	"github.com/david22573/codepicker/internal/scanner"
	"github.com/david22573/codepicker/internal/tokenizer"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type PathCollector struct {
	Paths []string
}

func (p *PathCollector) Init() error                 { return nil }
func (p *PathCollector) Write(abs, rel string) error { p.Paths = append(p.Paths, rel); return nil }
func (p *PathCollector) Close() error                { return nil }
func (p *PathCollector) ShouldSkip(path string) bool { return false }
func (p *PathCollector) Name() string                { return "Collector" }

type Options struct {
	APIKey      string
	Model       string
	SrcDir      string
	Query       string
	IncludeExts string
	IgnoreDirs  string
}

func SelectRelevantFiles(ctx context.Context, opts Options, log logger.Logger) ([]string, error) {
	log.Info("🔮 Smart mode enabled: Planning context...")

	absSrc, err := paths.Sanitize(opts.SrcDir)
	if err != nil {
		return nil, fmt.Errorf("invalid source directory")
	}

	// 1. Generate Tree View (High-level context, low tokens)
	// This helps the AI understand the project structure without listing every single file path initially
	projectTree, err := contextgen.GenerateTree(absSrc)
	if err != nil {
		log.Warn("Failed to generate project tree for planning: " + err.Error())
		projectTree = "(Tree generation failed)"
	}

	// 2. Scan for actual candidate files
	collector := &PathCollector{}
	cfg := config.NewConfig()
	if opts.IncludeExts != "" {
		cfg.AddAllowedExtensions(strings.Split(opts.IncludeExts, ","))
	}
	if opts.IgnoreDirs != "" {
		cfg.AddIgnoredDirs(strings.Split(opts.IgnoreDirs, ","))
	}

	s := scanner.NewScanner(absSrc, collector, cfg, log)
	if err := s.Scan(ctx); err != nil {
		return nil, fmt.Errorf("scan failed during planning")
	}

	if len(collector.Paths) == 0 {
		log.Warn("Scanner found no files for planning.")
		return nil, nil
	}

	// 3. Safety Truncation
	// If the repo is massive, we truncate the candidates list to prevent context explosion.
	// The Tree View provided earlier helps mitigate the loss of visibility.
	maxFiles := 1000
	if len(collector.Paths) > maxFiles {
		log.Warn(fmt.Sprintf("Too many files (%d). Truncating candidate list to top %d.", len(collector.Paths), maxFiles))
		collector.Paths = collector.Paths[:maxFiles]
	}

	fileList := strings.Join(collector.Paths, "\n")
	log.Info(fmt.Sprintf("Found %d candidate files. Asking AI to select relevant ones...", len(collector.Paths)))

	// 4. Construct Prompt
	sysMsg := `You are a senior developer.
You have a PROJECT STRUCTURE and a list of CANDIDATE FILES.
Based on the user's query, identify exactly which files contain the relevant code to answer the question.
Return ONLY a valid JSON object with a single key "files" containing the list of strings.
Example: { "files": ["cmd/main.go", "internal/utils.go"] }
If no specific code is needed, return { "files": [] }.`

	userMsg := fmt.Sprintf("PROJECT STRUCTURE:\n%s\n\nCANDIDATE FILES:\n%s\n\nQuery: %s", projectTree, fileList, opts.Query)

	selectedFiles := callLLM(ctx, opts.APIKey, opts.Model, sysMsg, userMsg, log)

	validFiles := validatePaths(selectedFiles, absSrc, log)

	if len(validFiles) > 0 {
		log.Info(fmt.Sprintf("🤖 AI selected %d files: %v", len(validFiles), validFiles))
		estimateTokenUsage(ctx, opts.APIKey, opts.Model, validFiles, log)
	} else {
		log.Info("🤖 AI decided no files are needed (or failed to pick).")
	}

	return validFiles, nil
}

func validatePaths(files []string, root string, log logger.Logger) []string {
	var safe []string
	for _, f := range files {

		clean, err := paths.Sanitize(f)
		if err != nil {
			log.Debug(fmt.Sprintf("Skipping unsafe path suggested by LLM: %s", f))
			continue
		}

		if _, err := os.Stat(clean); err != nil {
			log.Debug(fmt.Sprintf("Skipping non-existent path suggested by LLM: %s", f))
			continue
		}

		safe = append(safe, f)
	}
	return safe
}

func callLLM(ctx context.Context, apiKey, model, sysMsg, userMsg string, log logger.Logger) []string {
	client := openrouter.NewClient(apiKey)
	req := openrouter.ChatCompletionRequest{
		Model: model,
		Messages: []openrouter.ChatMessage{
			{Role: "system", Content: sysMsg},
			{Role: "user", Content: userMsg},
		},
		ResponseFormat: &openrouter.ResponseFormat{Type: "json_object"},
	}

	resp, err := client.CreateChatCompletion(ctx, req)
	if err != nil {
		log.Warn("Smart planning request failed (check API key/network). Falling back to normal scan.")
		return nil
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message == nil {
		return nil
	}

	contentStr, ok := resp.Choices[0].Message.Content.(string)
	if !ok {
		log.Warn("Failed to parse AI response content (not a string)")
		return nil
	}

	return parseJSONResponse(contentStr, log)
}

func parseJSONResponse(content string, log logger.Logger) []string {
	content = strings.TrimSpace(content)

	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
			if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
				lines = lines[:len(lines)-1]
			}
			content = strings.Join(lines, "\n")
		}
	}

	var resultObj struct {
		Files []string `json:"files"`
	}
	if err := json.Unmarshal([]byte(content), &resultObj); err == nil {
		return resultObj.Files
	}

	var paths []string
	if err := json.Unmarshal([]byte(content), &paths); err == nil {
		return paths
	}

	log.Warn("Failed to parse AI planning JSON. Proceeding without smart selection.")
	return nil
}

func estimateTokenUsage(ctx context.Context, apiKey, model string, files []string, log logger.Logger) {
	client := openrouter.NewClient(apiKey)

	modelInfo, err := client.GetModelInfo(ctx, model)
	if err != nil {
		log.Debug(fmt.Sprintf("Failed to fetch model info for token estimation: %v", err))
		return
	}

	var totalTokens int
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err == nil {
			totalTokens += tokenizer.CountTokens(string(content))
		}
	}

	estimatedTotal := totalTokens + 1000
	limit := modelInfo.ContextLength

	log.Info(fmt.Sprintf("📊 Estimated context: %d tokens (Model limit: %d)", totalTokens, limit))

	if limit > 0 && estimatedTotal > limit {
		log.Warn(fmt.Sprintf("⚠️  WARNING: Context size (%d) exceeds model limit (%d). Output may be truncated.", estimatedTotal, limit))
	} else if limit > 0 && float64(estimatedTotal) > float64(limit)*0.9 {
		usagePercent := int(float64(estimatedTotal) / float64(limit) * 100)
		log.Warn(fmt.Sprintf("⚠️  WARNING: Approaching model context limit (%d%% used).", usagePercent))
	}
}

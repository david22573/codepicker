package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/paths"
	"github.com/david22573/codepicker/internal/scanner"
	"github.com/david22573/codepicker/internal/tokenizer"
	"github.com/david22573/codepicker/pkg/openrouter"
)

// PathCollector implements writer.OutputStrategy to collect paths 
// instead of writing file content.
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

// SelectRelevantFiles scans the source directory and consults the AI to pick 
// files relevant to the user's query.
func SelectRelevantFiles(ctx context.Context, opts Options, log logger.Logger) ([]string, error) {
	log.Info("ƒºá Smart mode enabled: Planning context...")

	// 1. Scan the directory to get the full file list
	absSrc, err := paths.Sanitize(opts.SrcDir)
	if err != nil {
		return nil, fmt.Errorf("invalid source directory: %w", err)
	}

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
		return nil, fmt.Errorf("scan failed during planning: %w", err)
	}

	if len(collector.Paths) == 0 {
		log.Warn("Scanner found no files for planning.")
		return nil, nil
	}

	// 2. Construct the Prompt
	fileList := strings.Join(collector.Paths, "\n")
	log.Info(fmt.Sprintf("Found %d files. Asking AI to select relevant ones...", len(collector.Paths)))

	sysMsg := `You are a senior developer.
You have a list of files in a codebase.
Based on the user's query, identify exactly which files contain the relevant code to answer the question.
Return ONLY a valid JSON object with a single key "files" containing the list of strings.
Example: { "files": ["cmd/main.go", "internal/utils.go"] }
If no specific code is needed, return { "files": [] }.`

	userMsg := fmt.Sprintf("Files:\n%s\n\nQuery: %s", fileList, opts.Query)

	// 3. Call the LLM
	selectedFiles := callLLM(ctx, opts.APIKey, opts.Model, sysMsg, userMsg, log)
	
	// 4. Validate and Estimate Tokens
	if len(selectedFiles) > 0 {
		log.Info(fmt.Sprintf("ƒñû AI selected %d files: %v", len(selectedFiles), selectedFiles))
		estimateTokenUsage(ctx, opts.APIKey, opts.Model, selectedFiles, log)
	} else {
		log.Info("ƒñû AI decided no files are needed (or failed to pick).")
	}

	return selectedFiles, nil
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
		log.Warn(fmt.Sprintf("Smart planning failed: %v. Falling back to normal scan.", err))
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

	// Strip markdown code blocks if present
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
	}

	// Try standard format { "files": [...] }
	var resultObj struct {
		Files []string `json:"files"`
	}
	if err := json.Unmarshal([]byte(content), &resultObj); err == nil && len(resultObj.Files) > 0 {
		return resultObj.Files
	}

	// Try fallback format [...]
	var paths []string
	if err := json.Unmarshal([]byte(content), &paths); err == nil {
		return paths
	}

	log.Warn(fmt.Sprintf("Failed to parse AI planning JSON. Response was: %s", content))
	return nil
}

func estimateTokenUsage(ctx context.Context, apiKey, model string, files []string, log logger.Logger) {
	client := openrouter.NewClient(apiKey)
	modelInfo, err := client.GetModelInfo(ctx, model)
	if err != nil {
		return // Ignore model info errors (optional feature)
	}

	var totalTokens int
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err == nil {
			totalTokens += tokenizer.CountTokens(string(content))
		}
	}

	estimatedTotal := totalTokens + 1000 // Buffer for system prompt/overhead
	limit := modelInfo.ContextLength

	log.Info(fmt.Sprintf("ƒôè Estimated context: %d tokens (Model limit: %d)", totalTokens, limit))

	if limit > 0 && estimatedTotal > limit {
		log.Warn(fmt.Sprintf("ΓÜáÅ  WARNING: Context size (%d) exceeds model limit (%d). Output may be truncated.", estimatedTotal, limit))
	} else if limit > 0 && float64(estimatedTotal) > float64(limit)*0.9 {
		usagePercent := int(float64(estimatedTotal) / float64(limit) * 100)
		log.Warn(fmt.Sprintf("ΓÜáÅ  WARNING: Approaching model context limit (%d%% used).", usagePercent))
	}
}

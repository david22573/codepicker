package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/internal/contextgen"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type GenerateContextTool struct {
	Root   string
	Logger logger.Logger
}

type genContextArgs struct {
	TargetDir string `json:"target_dir"`
	Focus     string `json:"focus_files,omitempty"` // Optional comma-separated list
}

func (t *GenerateContextTool) Name() string { return "scan_package" }

func (t *GenerateContextTool) Description() string {
	return "Generates a comprehensive markdown context for a specific directory or package. Use this to quickly understand an entire module without reading files one by one."
}

func (t *GenerateContextTool) Capabilities() []Capability {
	return []Capability{CapRead}
}

func (t *GenerateContextTool) Definition() openrouter.Tool {
	return openrouter.Tool{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"target_dir": { "type": "string", "description": "Relative path to the directory (e.g. 'internal/database')" },
					"focus_files": { "type": "string", "description": "Optional: Comma-separated list of specific files to focus on within that dir" }
				},
				"required": ["target_dir"]
			}`),
		},
	}
}

func (t *GenerateContextTool) Execute(ctx context.Context, argsJSON string, rt *RuntimeContext) (string, error) {
	var args genContextArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	// 1. Resolve Path
	targetPath := filepath.Join(t.Root, args.TargetDir)

	// Security check: Ensure we are still inside the project root
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %v", err)
	}
	absRoot, _ := filepath.Abs(t.Root)
	if !strings.HasPrefix(absTarget, absRoot) {
		return "", fmt.Errorf("access denied: cannot scan outside project root")
	}

	// 2. Parse Focus Files
	var focusList []string
	if args.Focus != "" {
		parts := strings.Split(args.Focus, ",")
		for _, p := range parts {
			clean := strings.TrimSpace(p)
			if clean != "" {
				// Ensure focus files are joined with the target dir if they are relative
				if !strings.Contains(clean, "/") {
					clean = filepath.Join(targetPath, clean)
				}
				focusList = append(focusList, clean)
			}
		}
	}

	// 3. Configure Generator Options
	opts := contextgen.Options{
		SrcDir:     targetPath,
		FocusFiles: focusList,
		Minify:     true, // Force minify to save tokens for the agent
	}

	// 4. Run Generation
	// We use a NoOp logger here to keep the CLI stdout clean,
	// or pass t.Logger if you want debug info visible.
	content, err := contextgen.Generate(ctx, opts, &logger.NoOpLogger{})
	if err != nil {
		return "", fmt.Errorf("failed to generate context: %w", err)
	}

	// 5. Output Handling
	// If it's too huge, we might truncate it to prevent context overflow errors.
	if len(content) > 60000 { // Safety cap (~15k tokens)
		return fmt.Sprintf("Context generated but truncated (size: %d bytes):\n%s\n...(truncated)", len(content), content[:60000]), nil
	}

	return fmt.Sprintf("Context for '%s':\n%s", args.TargetDir, content), nil
}

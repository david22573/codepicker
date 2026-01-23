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

type ScanPackageTool struct {
	Root   string
	Logger logger.Logger
}

type scanArgs struct {
	TargetDir string `json:"target_dir"`
}

func (t *ScanPackageTool) Name() string { return "scan_package" }

func (t *ScanPackageTool) Description() string {
	return "Generates a consolidated markdown summary of an entire directory or package. Use this FIRST to understand a module before reading individual files."
}

func (t *ScanPackageTool) Capabilities() []Capability {
	return []Capability{CapRead}
}

func (t *ScanPackageTool) Definition() openrouter.Tool {
	return openrouter.Tool{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"target_dir": { "type": "string", "description": "Relative path to the directory (e.g. 'internal/database')" }
				},
				"required": ["target_dir"]
			}`),
		},
	}
}

func (t *ScanPackageTool) Execute(ctx context.Context, argsJSON string, rt *RuntimeContext) (string, error) {
	var args scanArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	// Safety: Prevent scanning the root recursively if the project is huge
	if args.TargetDir == "." || args.TargetDir == "./" || args.TargetDir == "" {
		return "", fmt.Errorf("scanning the entire root is too large. Please specify a subdirectory (e.g., 'cmd' or 'internal/agent').")
	}

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

	// Configure the generator to dump context
	opts := contextgen.Options{
		SrcDir: targetPath,
		Minify: true, // Force minify to save tokens
	}

	// Use a NoOp logger to keep the CLI clean during tool execution
	content, err := contextgen.Generate(ctx, opts, &logger.NoOpLogger{})
	if err != nil {
		return "", fmt.Errorf("failed to generate context: %w", err)
	}

	// Hard token limit safety (approx 15k tokens) to prevent context overflow
	if len(content) > 60000 {
		return fmt.Sprintf("Context generated but truncated (size: %d bytes):\n%s\n...(truncated)", len(content), content[:60000]), nil
	}

	return fmt.Sprintf("Context for '%s':\n%s", args.TargetDir, content), nil
}

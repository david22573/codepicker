package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/david22573/codepicker/internal/code"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type SkeletonizeTool struct{}

type skelArgs struct {
	Path string `json:"path"`
}

func (t *SkeletonizeTool) Name() string { return "read_skeleton" }
func (t *SkeletonizeTool) Description() string {
	return "Read the skeleton (signatures only) of a Go file. Use this to understand large files without wasting context."
}

func (t *SkeletonizeTool) Capabilities() []Capability { return []Capability{CapRead} }

func (t *SkeletonizeTool) Definition() openrouter.Tool {
	return openrouter.Tool{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": { "type": "string", "description": "Relative path to the Go file" }
				},
				"required": ["path"]
			}`),
		},
	}
}

func (t *SkeletonizeTool) Execute(ctx context.Context, argsJSON string, rt *RuntimeContext) (string, error) {
	var args skelArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	contentBytes, err := rt.FS.ReadFile(args.Path)
	if err != nil {
		return "", fmt.Errorf("error reading '%s': %w", args.Path, err)
	}

	// Attempt to skeletonize
	skel, err := code.Skeletonize(args.Path, contentBytes)
	if err != nil {
		// If it fails (e.g., syntax error or non-Go file), we warn the agent but return the original content
		// so it isn't blocked.
		return fmt.Sprintf("⚠️ Skeletonization failed (syntax error?); returning full content instead.\nError: %v\n\n--- FILE: %s ---\n%s", err, args.Path, string(contentBytes)), nil
	}

	// We add the path to memory tracking, but not the content, since it's incomplete.
	// This ensures the agent knows it has "seen" the file structure.
	_ = rt.Memory.Add(args.Path)

	return fmt.Sprintf("--- SKELETON: %s ---\n%s", args.Path, string(skel)), nil
}

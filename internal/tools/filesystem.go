package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/pkg/openrouter"
)

// ReadFileTool allows the agent to read file content, optionally with line ranges.
type ReadFileTool struct{}

type readFileArgs struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

func (t *ReadFileTool) Name() string { return "read_file" }

func (t *ReadFileTool) Description() string {
	return "Read the contents of a specific file from the project. Use optional start_line/end_line to read specific sections."
}

func (t *ReadFileTool) Definition() openrouter.Tool {
	return openrouter.Tool{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": { "type": "string", "description": "Relative path to the file (e.g., 'cmd/main.go')" },
					"start_line": { "type": "integer", "description": "Start line number (1-based, optional)" },
					"end_line": { "type": "integer", "description": "End line number (optional)" }
				},
				"required": ["path"]
			}`),
		},
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, argsJSON string, rt *RuntimeContext) (string, error) {
	var args readFileArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	contentBytes, err := rt.FS.ReadFile(args.Path)
	if err != nil {
		return "", fmt.Errorf("error reading '%s': %w", args.Path, err)
	}

	// Handle line range logic
	if args.StartLine > 0 || args.EndLine > 0 {
		lines := strings.Split(string(contentBytes), "\n")
		start := args.StartLine - 1
		if start < 0 {
			start = 0
		}

		end := args.EndLine
		if end == 0 || end > len(lines) {
			end = len(lines)
		}

		if start >= len(lines) {
			return "", fmt.Errorf("start line %d is beyond file length (%d lines)", args.StartLine, len(lines))
		}
		if start > end {
			return "", fmt.Errorf("invalid range: start %d > end %d", args.StartLine, args.EndLine)
		}

		subset := strings.Join(lines[start:end], "\n")
		return fmt.Sprintf("--- FILE: %s (Lines %d-%d) ---\n%s", args.Path, args.StartLine, args.EndLine, subset), nil
	}

	// If no range, load into memory context
	if err := rt.Memory.Add(args.Path); err != nil {
		return "", fmt.Errorf("failed to add to memory: %w", err)
	}

	return fmt.Sprintf("✓ File '%s' loaded into active context", args.Path), nil
}

// WriteShadowFileTool allows the agent to propose changes by writing to the shadow FS.
type WriteShadowFileTool struct{}

type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (t *WriteShadowFileTool) Name() string { return "write_shadow_file" }

func (t *WriteShadowFileTool) Description() string {
	return "Write code to the shadow workspace. Use this to propose changes or create new files."
}

func (t *WriteShadowFileTool) Definition() openrouter.Tool {
	return openrouter.Tool{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": { "type": "string", "description": "Relative path to the file" },
					"content": { "type": "string", "description": "The full content of the file" }
				},
				"required": ["path", "content"]
			}`),
		},
	}
}

func (t *WriteShadowFileTool) Execute(ctx context.Context, argsJSON string, rt *RuntimeContext) (string, error) {
	var args writeArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	path, err := rt.FS.WriteFile(args.Path, []byte(args.Content))
	if err != nil {
		return "", fmt.Errorf("error writing shadow file: %w", err)
	}

	return fmt.Sprintf("Changes written to shadow file: %s", path), nil
}

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/internal/shadow"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type ReadFileTool struct{}

type readFileArgs struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

func (t *ReadFileTool) Name() string { return "read_file" }
func (t *ReadFileTool) Description() string {
	return "Read the contents of a specific file from the project."
}

func (t *ReadFileTool) Capabilities() []Capability { return []Capability{CapRead} }

func (t *ReadFileTool) Definition() openrouter.Tool {
	return openrouter.Tool{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": { "type": "string", "description": "Relative path to the file" },
					"start_line": { "type": "integer" },
					"end_line": { "type": "integer" }
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
			return "", fmt.Errorf("start line %d is beyond file length", args.StartLine)
		}
		subset := strings.Join(lines[start:end], "\n")
		return fmt.Sprintf("--- FILE: %s (Lines %d-%d) ---\n%s", args.Path, args.StartLine, args.EndLine, subset), nil
	}

	if err := rt.Memory.Add(args.Path); err != nil {
		return "", fmt.Errorf("failed to add to memory: %w", err)
	}

	return fmt.Sprintf("✓ File '%s' loaded into active context", args.Path), nil
}

// WriteShadowFileTool now includes the Shadow manager field
type WriteShadowFileTool struct {
	Shadow *shadow.Manager
}

type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (t *WriteShadowFileTool) Name() string { return "write_shadow_file" }
func (t *WriteShadowFileTool) Description() string {
	return "Write code to the shadow workspace."
}

func (t *WriteShadowFileTool) Capabilities() []Capability { return []Capability{CapWrite} }

func (t *WriteShadowFileTool) Definition() openrouter.Tool {
	return openrouter.Tool{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": { "type": "string" },
					"content": { "type": "string" }
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

	// Use the struct field if available (Registry uses this), fallback to RuntimeContext (Agent uses this)
	var shadowMgr *shadow.Manager
	if t.Shadow != nil {
		shadowMgr = t.Shadow
	} else if overlay, ok := rt.FS.(interface{ GetShadowManager() *shadow.Manager }); ok {
		shadowMgr = overlay.GetShadowManager()
	}

	if shadowMgr == nil {
		return "", fmt.Errorf("shadow manager not available")
	}

	path, err := shadowMgr.WriteFile(args.Path, []byte(args.Content))
	if err != nil {
		return "", fmt.Errorf("error writing shadow file: %w", err)
	}

	return fmt.Sprintf("Changes written to shadow file: %s", path), nil
}

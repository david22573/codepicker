package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/david22573/codepicker/pkg/openrouter"
)

type RunShellTool struct{}

type shellArgs struct {
	Command string `json:"command"`
}

func (t *RunShellTool) Name() string { return "run_shell" }
func (t *RunShellTool) Description() string {
	return "Execute a shell command. Use this for 'ls', 'go test', etc."
}

func (t *RunShellTool) Capabilities() []Capability {
	return []Capability{CapExecute, CapRead}
}

func (t *RunShellTool) Definition() openrouter.Tool {
	return openrouter.Tool{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"command": { "type": "string" }
				},
				"required": ["command"]
			}`),
		},
	}
}

func (t *RunShellTool) Execute(ctx context.Context, argsJSON string, rt *RuntimeContext) (string, error) {
	if rt.Sentinel == nil {
		return "", fmt.Errorf("shell execution disabled")
	}

	var args shellArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	_, _, binary, cmdArgs := rt.Sentinel.CheckCommand(args.Command)
	out, err := rt.Sentinel.Execute(binary, cmdArgs)
	if err != nil {
		return fmt.Sprintf("Command failed: %v\nOutput: %s", err, out), nil
	}
	return out, nil
}

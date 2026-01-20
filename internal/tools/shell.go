package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/david22573/codepicker/pkg/openrouter"
)

type RunShellTool struct {
	// ApprovalCallback allows the tool to ask the user (or system) for permission
	// This mirrors the previous logic but inside the tool.
	OnApproval func(command, reason string) bool
}

type shellArgs struct {
	Command string `json:"command"`
}

func (t *RunShellTool) Name() string { return "run_shell" }

func (t *RunShellTool) Description() string {
	return "Execute a shell command. Use this for 'ls', 'go test', etc. Prefer search_code for finding code."
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
					"command": { "type": "string", "description": "The full shell command string" }
				},
				"required": ["command"]
			}`),
		},
	}
}

func (t *RunShellTool) Execute(ctx context.Context, argsJSON string, rt *RuntimeContext) (string, error) {
	if rt.Sentinel == nil {
		return "", fmt.Errorf("shell execution disabled (no sentinel provided)")
	}

	var args shellArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	// 1. Security Check
	needsApproval, reason, binary, cmdArgs := rt.Sentinel.CheckCommand(args.Command)

	// 2. Interaction / Approval
	if needsApproval {
		if t.OnApproval != nil && !t.OnApproval(args.Command, reason) {
			return "Command denied by user/policy.", nil
		}
	}

	// 3. Execution
	out, err := rt.Sentinel.Execute(binary, cmdArgs)
	if err != nil {
		return fmt.Sprintf("Command failed: %v\nOutput: %s", err, out), nil
	}

	return out, nil
}

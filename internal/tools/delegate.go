package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/pkg/openrouter"
)

type DelegateTaskTool struct{}

type delegateArgs struct {
	Instruction  string `json:"instruction"`
	ContextFiles string `json:"context_files"`
}

func (t *DelegateTaskTool) Name() string { return "delegate_task" }
func (t *DelegateTaskTool) Description() string {
	return "Delegate a sub-task to a worker agent."
}

// [Fixed] Added Capabilities
func (t *DelegateTaskTool) Capabilities() []Capability {
	return []Capability{CapRead, CapWrite}
}

func (t *DelegateTaskTool) Definition() openrouter.Tool {
	return openrouter.Tool{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"instruction": { "type": "string" },
					"context_files": { "type": "string" }
				},
				"required": ["instruction"]
			}`),
		},
	}
}

func (t *DelegateTaskTool) Execute(ctx context.Context, argsJSON string, rt *RuntimeContext) (string, error) {
	if rt.Worker == nil {
		return "Delegation unavailable (no worker assigned)", nil
	}
	var args delegateArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	files := strings.Split(args.ContextFiles, ",")
	for i := range files {
		files[i] = strings.TrimSpace(files[i])
	}

	result, err := rt.Worker.Run(ctx, args.Instruction, files)
	if err != nil {
		return fmt.Sprintf("Worker failed: %v", err), nil
	}
	return fmt.Sprintf("Worker Output:\n%s", result), nil
}

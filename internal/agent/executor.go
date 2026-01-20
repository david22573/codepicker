package agent

import (
	"context"
	"fmt"

	"github.com/david22573/codepicker/internal/tools"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type ToolExecutor struct {
	Tools          map[string]tools.Tool
	RuntimeContext *tools.RuntimeContext
	DryRun         bool // Re-added for CLI compatibility
}

func NewToolExecutor(
	availableTools []tools.Tool,
	rt *tools.RuntimeContext,
) *ToolExecutor {
	toolMap := make(map[string]tools.Tool)
	for _, t := range availableTools {
		toolMap[t.Name()] = t
	}

	return &ToolExecutor{
		Tools:          toolMap,
		RuntimeContext: rt,
		DryRun:         false,
	}
}

func (e *ToolExecutor) Execute(ctx context.Context, call openrouter.ToolCall) string {
	tool, exists := e.Tools[call.Function.Name]
	if !exists {
		return fmt.Sprintf("Error: Tool '%s' is not available in this context.", call.Function.Name)
	}

	if e.DryRun {
		return fmt.Sprintf("[DRY RUN] Would execute tool '%s' with args: %s", call.Function.Name, call.Function.Arguments)
	}

	// Note: We pass the JSON arguments directly to the tool.
	// The tool is responsible for unmarshaling and validation.
	result, err := tool.Execute(ctx, call.Function.Arguments, e.RuntimeContext)
	if err != nil {
		return fmt.Sprintf("Tool execution failed: %v", err)
	}

	return result
}

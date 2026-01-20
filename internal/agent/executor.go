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
	Enforcer       *PolicyEnforcer // [1.2] Added Enforcer dependency
	DryRun         bool
}

func NewToolExecutor(
	availableTools []tools.Tool,
	rt *tools.RuntimeContext,
	enforcer *PolicyEnforcer, // [1.2] Inject Enforcer
) *ToolExecutor {
	toolMap := make(map[string]tools.Tool)
	for _, t := range availableTools {
		toolMap[t.Name()] = t
	}

	return &ToolExecutor{
		Tools:          toolMap,
		RuntimeContext: rt,
		Enforcer:       enforcer,
		DryRun:         false,
	}
}

func (e *ToolExecutor) Execute(ctx context.Context, call openrouter.ToolCall) string {
	// [1.2] Enforce policy at executor level
	// This captures ALL tool usage, regardless of which tool it is.
	req := ApprovalRequest{
		Tool: call.Function.Name,
		Args: call.Function.Arguments,
	}

	if !e.Enforcer.AllowTool(req) {
		return "Action denied by security policy."
	}

	tool, exists := e.Tools[call.Function.Name]
	if !exists {
		return fmt.Sprintf("Error: Tool '%s' is not available in this context.", call.Function.Name)
	}

	if e.DryRun {
		return fmt.Sprintf("[DRY RUN] Would execute tool '%s' with args: %s", call.Function.Name, call.Function.Arguments)
	}

	result, err := tool.Execute(ctx, call.Function.Arguments, e.RuntimeContext)
	if err != nil {
		return fmt.Sprintf("Tool execution failed: %v", err)
	}

	return result
}

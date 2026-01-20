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
	Enforcer       *PolicyEnforcer
	Trace          bool // [Phase 5]
}

func NewToolExecutor(
	availableTools []tools.Tool,
	rt *tools.RuntimeContext,
	enforcer *PolicyEnforcer,
	trace bool,
) *ToolExecutor {
	toolMap := make(map[string]tools.Tool)
	for _, t := range availableTools {
		toolMap[t.Name()] = t
	}

	return &ToolExecutor{
		Tools:          toolMap,
		RuntimeContext: rt,
		Enforcer:       enforcer,
		Trace:          trace,
	}
}

func (e *ToolExecutor) Execute(ctx context.Context, call openrouter.ToolCall) string {
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

	if e.Trace {
		fmt.Printf("\n[TRACE] >>> EXECUTE %s\nARGS: %s\n", call.Function.Name, call.Function.Arguments)
	}

	result, err := tool.Execute(ctx, call.Function.Arguments, e.RuntimeContext)

	if e.Trace {
		if err != nil {
			fmt.Printf("[TRACE] <<< ERROR: %v\n", err)
		} else {
			out := result
			if len(out) > 500 {
				out = out[:500] + "..."
			}
			fmt.Printf("[TRACE] <<< RESULT: %s\n", out)
		}
	}

	if err != nil {
		return fmt.Sprintf("Tool execution failed: %v", err)
	}

	return result
}

package agent

import (
	"context"
	"fmt"

	"github.com/david22573/codepicker/internal/tools"
	"github.com/david22573/codepicker/pkg/openrouter"
)

// ToolMiddleware defines hooks that run before and after tool execution.
type ToolMiddleware interface {
	// BeforeExecute is called before the tool runs. If it returns an error, execution stops.
	BeforeExecute(toolName string, args string) error

	// AfterExecute is called after the tool runs successfully.
	AfterExecute(toolName string, result string) error
}

type ToolExecutor struct {
	Tools          map[string]tools.Tool
	RuntimeContext *tools.RuntimeContext
	Enforcer       *PolicyEnforcer
	Middlewares    []ToolMiddleware
	Trace          bool
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
		Middlewares:    make([]ToolMiddleware, 0),
		Trace:          trace,
	}
}

func (e *ToolExecutor) AddMiddleware(m ToolMiddleware) {
	e.Middlewares = append(e.Middlewares, m)
}

func (e *ToolExecutor) Execute(ctx context.Context, call openrouter.ToolCall) string {
	// 1. Policy Check
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

	// 2. Middleware: BeforeExecute
	for _, mw := range e.Middlewares {
		if err := mw.BeforeExecute(call.Function.Name, call.Function.Arguments); err != nil {
			return fmt.Sprintf("Action blocked by middleware: %v", err)
		}
	}

	// 3. Execution
	if e.Trace {
		fmt.Printf("\n[TRACE] >>> EXECUTE %s\nARGS: %s\n", call.Function.Name, call.Function.Arguments)
	}

	result, err := tool.Execute(ctx, call.Function.Arguments, e.RuntimeContext)
	if err != nil {
		return fmt.Sprintf("Tool execution failed: %v", err)
	}

	if e.Trace {
		out := result
		if len(out) > 500 {
			out = out[:500] + "..."
		}
		fmt.Printf("[TRACE] <<< RESULT: %s\n", out)
	}

	// 4. Middleware: AfterExecute
	for _, mw := range e.Middlewares {
		// We log errors from AfterExecute but do not fail the tool execution itself
		// as the action has already occurred.
		if mwErr := mw.AfterExecute(call.Function.Name, result); mwErr != nil {
			if e.Trace {
				fmt.Printf("[TRACE] Middleware Post-Hook Error: %v\n", mwErr)
			}
		}
	}

	return result
}

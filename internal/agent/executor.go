package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/david22573/codepicker/internal/tools"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type ToolMiddleware interface {
	BeforeExecute(toolName string, args string) error
	// AfterExecute can return error to modify result/log, but usually just logs
	AfterExecute(toolName string, result string) error
}

type ToolExecutor struct {
	Tools          map[string]tools.Tool
	RuntimeContext *tools.RuntimeContext
	Enforcer       *PolicyEnforcer
	Middlewares    []ToolMiddleware
	Trace          bool
}

// Phase 2: Structured Tool Results
type ToolResponse struct {
	Status string `json:"status"` // "success", "error", "policy_blocked"
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
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

	req := ApprovalRequest{
		Tool: call.Function.Name,
		Args: call.Function.Arguments,
	}

	// 1. Policy Check
	if !e.Enforcer.AllowTool(req) {
		return e.formatError("policy_blocked", "Action denied by security policy.")
	}

	tool, exists := e.Tools[call.Function.Name]
	if !exists {
		return e.formatError("error", fmt.Sprintf("Tool '%s' is not available in this context.", call.Function.Name))
	}

	// 2. Middleware Pre-Check (Fail-Closed)
	for _, mw := range e.Middlewares {
		if err := mw.BeforeExecute(call.Function.Name, call.Function.Arguments); err != nil {
			return e.formatError("policy_blocked", fmt.Sprintf("Action blocked by safety middleware: %v", err))
		}
	}

	if e.Trace {
		fmt.Printf("\n[TRACE] >>> EXECUTE %s\nARGS: %s\n", call.Function.Name, call.Function.Arguments)
	}

	// 3. Execution
	result, err := tool.Execute(ctx, call.Function.Arguments, e.RuntimeContext)
	if err != nil {
		return e.formatError("error", fmt.Sprintf("Tool execution failed: %v", err))
	}

	if e.Trace {
		out := result
		if len(out) > 500 {
			out = out[:500] + "..."
		}
		fmt.Printf("[TRACE] <<< RESULT: %s\n", out)
	}

	// 4. Middleware Post-Hook
	for _, mw := range e.Middlewares {
		// Post-hooks are generally cosmetic (formatting/logging), so we don't hard fail here
		// unless necessary.
		if mwErr := mw.AfterExecute(call.Function.Name, result); mwErr != nil {
			if e.Trace {
				fmt.Printf("[TRACE] Middleware Post-Hook Error: %v\n", mwErr)
			}
		}
	}

	return result
}

func (e *ToolExecutor) formatError(status, msg string) string {
	resp := ToolResponse{
		Status: status,
		Error:  msg,
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

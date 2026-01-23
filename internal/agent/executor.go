package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/david22573/codepicker/internal/tools"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type ToolMiddleware interface {
	BeforeExecute(toolName string, args string) error
	AfterExecute(toolName string, result string) error
}

type ToolExecutor struct {
	Tools          map[string]tools.Tool
	RuntimeContext *tools.RuntimeContext
	Enforcer       *PolicyEnforcer
	Middlewares    []ToolMiddleware
	Trace          bool
}

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
	cleanName := sanitizeToolName(call.Function.Name)
	cleanArgs := sanitizeJSON(call.Function.Arguments)

	if cleanName == "" {
		if e.Trace {
			fmt.Printf("[Executor] Ignored garbage tool call: '%s'\n", call.Function.Name)
		}
		return e.formatError("error", "Invalid tool name received. Please try again.")
	}

	req := ApprovalRequest{
		Tool: cleanName,
		Args: cleanArgs,
	}

	if !e.Enforcer.AllowTool(req) {
		return e.formatError("policy_blocked", "Action denied by security policy.")
	}

	tool, exists := e.Tools[cleanName]
	if !exists {
		// Friendly suggestions for common hallucinations
		suggestion := ""
		if strings.Contains(cleanName, "replace") || strings.Contains(cleanName, "edit") {
			suggestion = " Hint: You do not have a patch/replace tool. You must use 'write_shadow_file' to overwrite the file completely."
		} else if strings.Contains(cleanName, "search") {
			suggestion = " Hint: Did you mean 'search_code'?"
		}

		msg := fmt.Sprintf("Tool '%s' does not exist.%s Available tools: %s",
			cleanName, suggestion, e.listToolNames())

		return e.formatError("error", msg)
	}

	for _, mw := range e.Middlewares {
		if err := mw.BeforeExecute(cleanName, cleanArgs); err != nil {
			return e.formatError("policy_blocked", fmt.Sprintf("Action blocked by safety middleware: %v", err))
		}
	}

	if e.Trace {
		fmt.Printf("\n[TRACE] >>> EXECUTE %s\nRAW ARGS: %s\n", cleanName, call.Function.Arguments)
	}

	result, err := tool.Execute(ctx, cleanArgs, e.RuntimeContext)
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

	for _, mw := range e.Middlewares {
		if mwErr := mw.AfterExecute(cleanName, result); mwErr != nil {
			if e.Trace {
				fmt.Printf("[TRACE] Middleware Post-Hook Error: %v\n", mwErr)
			}
		}
	}

	return result
}

func (e *ToolExecutor) listToolNames() string {
	var names []string
	for k := range e.Tools {
		names = append(names, k)
	}
	return strings.Join(names, ", ")
}

func (e *ToolExecutor) formatError(status, msg string) string {
	resp := ToolResponse{
		Status: status,
		Error:  msg,
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

func sanitizeToolName(s string) string {
	// FIX: Remove DeepSeek/LLM specific hallucinated tokens
	s = strings.ReplaceAll(s, "tool_call_begin", "")
	s = strings.ReplaceAll(s, "tool_call_end", "")
	s = strings.ReplaceAll(s, "func_call_begin", "")

	re := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	cleaned := re.ReplaceAllString(s, "")
	return cleaned
}

func sanitizeJSON(s string) string {
	s = strings.TrimSpace(s)
	// Simple repair for cut-off JSON (common with token limits)
	if !strings.HasSuffix(s, "}") && strings.HasPrefix(s, "{") {
		s += "}"
	}

	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")

	if start != -1 && end != -1 && end > start {
		return s[start : end+1]
	}

	return s
}

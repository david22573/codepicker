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

	// 1. SANITIZE NAME (Strict Allowlist: a-z, A-Z, 0-9, _)
	cleanName := sanitizeToolName(call.Function.Name)

	// 2. SANITIZE ARGS (Extract { ... })
	cleanArgs := sanitizeJSON(call.Function.Arguments)

	// If the tool name was purely garbage (became empty), ignore it safely
	if cleanName == "" {
		// Log it for debug, but return a gentle error to the model
		if e.Trace {
			fmt.Printf("[Executor] Ignored garbage tool call: '%s'\n", call.Function.Name)
		}
		return e.formatError("error", "Invalid tool name received (parsing error). Please try again.")
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
		// SMART RECOVERY: Help the model if it hallucinates or makes a typo
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
		fmt.Printf("\n[TRACE] >>> EXECUTE %s\nRAW ARGS: %s\nCLEAN ARGS: %s\n", cleanName, call.Function.Arguments, cleanArgs)
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

// -------------------------------------------------------------------------
// SANITIZATION HELPERS
// -------------------------------------------------------------------------

// sanitizeToolName strips everything except alphanumeric chars and underscores.
// This removes '[[tool_call_end]]', '}', ']', etc.
func sanitizeToolName(s string) string {
	// 1. Remove specific known hallucinated tokens
	s = strings.ReplaceAll(s, "tool_call_begin", "")
	s = strings.ReplaceAll(s, "tool_call_end", "")
	s = strings.ReplaceAll(s, "func_call_begin", "") // Catch other variants

	// 2. Remove any remaining non-alphanumeric characters (existing logic)
	re := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	cleaned := re.ReplaceAllString(s, "")

	return cleaned
}

// sanitizeJSON extracts the substring between the first '{' and the last '}'.
// It ignores conversational text that some models append (e.g., "Here is the code...").
func sanitizeJSON(s string) string {
	s = strings.TrimSpace(s)

	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")

	// If we found a valid JSON object structure
	if start != -1 && end != -1 && end > start {
		return s[start : end+1]
	}

	// If no braces found, return original string (it will likely fail unmarshal,
	// but that error will be handled by the specific tool).
	return s
}

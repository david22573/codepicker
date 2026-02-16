package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/event"
	infraCtx "github.com/david22573/codepicker/infra/context"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/logging"
	"github.com/david22573/codepicker/infra/ratelimit"
)

// ReActAgent implements the autonomous agent loop using Native Tool Calling.
type ReActAgent struct {
	model       *llm.OpenRouterAdapter
	tools       map[string]agent.Tool
	toolSchemas []llm.ToolDefinition
	bus         *event.DataBus
	logger      *logging.Logger
	policy      agent.Policy
	controller  *AdaptiveController
	processor   *ObservationProcessor
	rateLimiter *ratelimit.ToolRateLimiter
	budgetGuard *llm.BudgetGuard // FIX: Guard against cost overruns
	memory      *TurnMemory
	history     []llm.Message
	sysMsg      string
	verbose     bool
}

// NewReActAgent initializes the agent with a native tool-calling configuration.
func NewReActAgent(
	model *llm.OpenRouterAdapter,
	tools []agent.Tool,
	bus *event.DataBus,
	logger *logging.Logger,
	policy agent.Policy,
	costTracker *llm.CostTracker,
	rateLimiter *ratelimit.ToolRateLimiter,
	budget float64,
) *ReActAgent {
	toolMap := make(map[string]agent.Tool)
	var schemas []llm.ToolDefinition

	// 1. Map Domain Tools to LLM Schemas using Reflection
	for _, t := range tools {
		name := t.Name()
		toolMap[name] = t

		inputStruct, exists := toolInputRegistry[name]
		if !exists {
			// Fallback for unknown tools
			inputStruct = struct {
				Input string `json:"input" desc:"The input argument for the tool"`
			}{}
		}

		schemas = append(schemas, llm.GenerateToolDefinition(
			name,
			t.Description(),
			inputStruct,
		))
	}

	// FIX: Initialize BudgetGuard
	bg := llm.NewBudgetGuard(costTracker, budget)

	return &ReActAgent{
		model:       model,
		tools:       toolMap,
		toolSchemas: schemas,
		bus:         bus,
		logger:      logger,
		policy:      policy,
		controller:  NewAdaptiveController(10, 30, costTracker, budget),
		budgetGuard: bg,
		processor:   NewObservationProcessor(4000),
		rateLimiter: rateLimiter,
		memory:      NewTurnMemory(16000), // Increased memory buffer for larger contexts
		sysMsg: `You are CodePicker, an autonomous code execution agent with direct filesystem access.
🎯 PRIMARY MODE: EXECUTION WITH TOOLS
Your default behavior is to EXECUTE tasks using tools.
You are not a consultant - you are a doer.
CRITICAL RULES:
1. ALWAYS use tools to accomplish tasks - NEVER just describe what should be done
2. To modify any file, you MUST call write_file with the COMPLETE new file content
3. To read any file, you MUST call read_file - never assume or guess file contents
4. You work iteratively: read → analyze → write → verify
5. When writing files, provide the FULL, COMPLETE file content - no partial updates or snippets
6. The ONLY acceptable "Final Answer" is after you've used tools to complete the work

AVAILABLE TOOLS:
• read_file: Read any file to understand its current state (MANDATORY before modifications)
• write_file: Write complete file content (MANDATORY for making any code changes)
• list_dir: List directory contents to explore the project structure
• search_code: Semantic search across the codebase to find relevant code
• run_cmd: Execute shell commands (use cautiously, mainly for verification)

EXECUTION WORKFLOW:
Step 1: Call read_file("service.go") to see the current implementation
Step 2: Analyze what needs to change
Step 3: Call write_file("service.go", "<COMPLETE modified file content>")
Step 4: (Optional) Verify with read_file or run_cmd
Step 5: Respond with "Final Answer: Successfully updated service.go..."

FORBIDDEN BEHAVIORS:
❌ Responding "I would modify line 45 to..." without calling write_file
❌ Providing code snippets/diffs without calling write_file
❌ Making assumptions about file contents without calling read_file first
❌ Writing partial updates like "replace this function with..."
❌ Using tools only to "check" without making changes when changes are requested

DEFAULT BEHAVIOR: Execute with tools.
Actions speak louder than words.`,
	}
}

func (a *ReActAgent) Name() string { return "CodePicker-Native-v1" }

func (a *ReActAgent) UpdateSystemPrompt(msg string) {
	a.sysMsg = msg
}

func (a *ReActAgent) SetVerbose(verbose bool) {
	a.verbose = verbose
}

// Run executes the ReAct loop using native function calling.
func (a *ReActAgent) Run(ctx context.Context, taskInput string) (string, error) {
	maxTurns := a.controller.CalculateAllowedTurns(0.5)

	a.history = []llm.Message{
		{Role: "system", Content: a.sysMsg},
		{Role: "user", Content: taskInput},
	}

	// Use range over integer (Go 1.22+)
	for i := range maxTurns {
		if infraCtx.IsCancelled(ctx) {
			a.bus.Publish(event.Event{Type: event.EventError, Payload: map[string]any{"error": "cancelled"}})
			return "", fmt.Errorf("agent cancelled: %w", ctx.Err())
		}

		// FIX: Check Budget BEFORE calling LLM
		estimatedInputTokens := a.memory.estimateTokens(a.history)
		if err := a.budgetGuard.CheckBeforeCall(estimatedInputTokens); err != nil {
			a.bus.Publish(event.Event{Type: event.EventError, Payload: map[string]any{"error": "budget_exceeded"}})
			return "", fmt.Errorf("halted by budget guard: %w", err)
		}

		respMsg, _, err := a.model.ChatNative(ctx, a.history, a.toolSchemas)
		if err != nil {
			a.bus.Publish(event.Event{Type: event.EventError, Payload: map[string]any{"error": err.Error()}})
			return "", err
		}

		a.history = append(a.history, respMsg)

		a.bus.Publish(event.Event{
			Type: event.EventAgentThought,
			Payload: map[string]any{
				"turn":    i,
				"content": respMsg.Content,
			},
			Timestamp: time.Now().Unix(),
		})

		if len(respMsg.ToolCalls) == 0 {
			// If the model produces a "Final Answer" or we've run at least one turn, accept it.
			if strings.Contains(respMsg.Content, "Final Answer:") || i > 0 {
				a.bus.Publish(event.Event{Type: event.EventAgentFinish, Payload: map[string]any{"result": respMsg.Content}})
				return respMsg.Content, nil
			}
			// If no tool calls and no final answer in the very first turn, it's likely a chatty model.
			// We continue to let it potentially self-correct or receive a prompt to use tools.
		}

		// Parallel Execution Block
		var wg sync.WaitGroup
		var mu sync.Mutex

		type toolResult struct {
			callID  string
			name    string
			content string
		}
		results := make([]toolResult, 0, len(respMsg.ToolCalls))

		for _, tc := range respMsg.ToolCalls {
			wg.Add(1)
			go func(call llm.ToolCall) {
				defer wg.Done()

				if infraCtx.IsCancelled(ctx) {
					return
				}

				// Check Rate Limiter
				if err := a.rateLimiter.Wait(ctx, call.Function.Name); err != nil {
					mu.Lock()
					results = append(results, toolResult{call.ID, call.Function.Name, fmt.Sprintf("Error: %v", err)})
					mu.Unlock()
					return
				}

				// Check Policy
				if a.policy != nil {
					allowed, reason := a.policy.CanExecute(call.Function.Name, call.Function.Arguments)
					if !allowed {
						mu.Lock()
						results = append(results, toolResult{call.ID, call.Function.Name, fmt.Sprintf("Error: Policy Violation - %s", reason)})
						mu.Unlock()
						return
					}
				}

				tool, exists := a.tools[call.Function.Name]
				if !exists {
					mu.Lock()
					results = append(results, toolResult{call.ID, call.Function.Name, "Error: Tool not found"})
					mu.Unlock()
					return
				}

				if a.verbose {
					fmt.Printf("   🔧 [TOOL] Calling: %s\n", call.Function.Name)
					fmt.Printf("   📥 Input: %s\n", truncate(call.Function.Arguments, 200))
				}

				a.bus.Publish(event.Event{
					Type:    event.EventToolStart,
					Payload: map[string]any{"tool": call.Function.Name, "input": call.Function.Arguments},
				})

				output, toolErr := tool.Execute(ctx, call.Function.Arguments)
				if toolErr != nil {
					output = fmt.Sprintf("Error: %v", toolErr)
				}

				processedOutput := a.processor.Process(output)

				a.bus.Publish(event.Event{
					Type:    event.EventToolEnd,
					Payload: map[string]any{"tool": call.Function.Name, "status": "finished", "output": processedOutput},
				})

				if a.verbose {
					status := "✅"
					if toolErr != nil {
						status = "❌"
					}
					fmt.Printf("   %s Output: %s\n", status, truncate(processedOutput, 300))
				}

				mu.Lock()
				results = append(results, toolResult{call.ID, call.Function.Name, processedOutput})
				mu.Unlock()

			}(tc)
		}

		wg.Wait()

		// If no tools were actually executed (e.g. empty list), we skip recording
		if len(results) > 0 {
			for _, res := range results {
				a.recordToolResult(res.callID, res.name, res.content)
			}
			// FIX: Prune history here to prevent unbounded growth
			a.pruneHistory()
		} else if len(respMsg.ToolCalls) > 0 {
			// Tool calls existed but failed to produce results (e.g. rate limit/policy block on all)
			// We must record something or the LLM gets confused waiting for a tool output
			for _, tc := range respMsg.ToolCalls {
				a.recordToolResult(tc.ID, tc.Function.Name, "Error: Execution blocked or failed internally.")
			}
			// FIX: Prune history here as well
			a.pruneHistory()
		}
	}

	return "", fmt.Errorf("max turns exceeded (%d)", maxTurns)
}

func (a *ReActAgent) recordToolResult(callID, name, content string) {
	a.history = append(a.history, llm.Message{
		Role:       "tool",
		ToolCallID: callID,
		Name:       name,
		Content:    content,
	})
}

func (a *ReActAgent) pruneHistory() {
	a.history = a.memory.Prune(a.history)
}

// Global registry of tool input schemas.
// keys must match Tool.Name() return values exactly.
var toolInputRegistry = map[string]any{
	"read_file": struct {
		Path string `json:"path" desc:"Relative path to the file"`
	}{},
	"write_file": struct {
		Path    string `json:"path" desc:"Relative path to the file"`
		Content string `json:"content" desc:"The complete content to write"`
	}{},
	"list_dir": struct {
		Path string `json:"path" desc:"Directory path to list"`
	}{},
	"search_code": struct {
		Query string `json:"query" desc:"Natural language query"`
	}{},
	"search_definition": struct {
		Name string `json:"name" desc:"Symbol name to find"`
	}{},
	"run_cmd": struct {
		Command string `json:"command" desc:"Shell command to execute"`
	}{},
	"read_skeleton": struct {
		Path string `json:"path" desc:"File or directory path"`
	}{},
	"git_diff": struct {
	}{},
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

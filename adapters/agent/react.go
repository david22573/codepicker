package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	domainAgent "github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/event"
	infraCtx "github.com/david22573/codepicker/infra/context"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/logging"
	"github.com/david22573/codepicker/infra/ratelimit"
)

const (
	MaxObservationLength = 4000
	ExpectedOutputTokens = 1024
	DefaultMemoryTokens  = 16000
)

// ReActAgent implements the autonomous agent loop using Native Tool Calling.
type ReActAgent struct {
	model       *llm.OpenRouterAdapter
	tools       map[string]domainAgent.Tool
	toolSchemas []llm.ToolDefinition
	bus         *event.DataBus
	logger      *logging.Logger
	policy      domainAgent.Policy
	controller  *AdaptiveController
	processor   *ObservationProcessor
	rateLimiter *ratelimit.ToolRateLimiter
	budgetGuard *llm.BudgetGuard
	costTracker *llm.CostTracker
	memory      *TurnMemory
	history     []llm.Message
	sysMsg      string
	verbose     bool
}

// NewReActAgent initializes the agent with a native tool-calling configuration.
func NewReActAgent(
	model *llm.OpenRouterAdapter,
	tools []domainAgent.Tool,
	bus *event.DataBus,
	logger *logging.Logger,
	policy domainAgent.Policy,
	costTracker *llm.CostTracker,
	rateLimiter *ratelimit.ToolRateLimiter,
	budget float64,
	maxTurns int,
) *ReActAgent {
	toolMap := make(map[string]domainAgent.Tool)
	var schemas []llm.ToolDefinition

	for _, t := range tools {
		name := t.Name()
		toolMap[name] = t

		inputStruct, exists := toolInputRegistry[name]
		if !exists {
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

	bg := llm.NewBudgetGuard(costTracker, budget)

	baseTurns := maxTurns / 2
	if baseTurns < 10 {
		baseTurns = 10
	}

	return &ReActAgent{
		model:       model,
		tools:       toolMap,
		toolSchemas: schemas,
		bus:         bus,
		logger:      logger,
		policy:      policy,
		costTracker: costTracker,
		controller:  NewAdaptiveController(baseTurns, maxTurns, costTracker, budget),
		budgetGuard: bg,
		processor:   NewObservationProcessor(MaxObservationLength),
		rateLimiter: rateLimiter,
		memory:      NewTurnMemory(DefaultMemoryTokens),
		sysMsg: `<role>
You are CodePicker, an autonomous code execution agent with direct filesystem access.
You are a doer, not a consultant. Your primary mode is EXECUTION WITH TOOLS.
</role>

<critical_rules>
1. ALWAYS use tools to accomplish tasks - NEVER just describe what should be done.
2. To MODIFY an EXISTING file, you MUST use the edit_file tool with SEARCH/REPLACE blocks.
3. To CREATE a NEW file, you MUST use the write_file tool.
4. To read any file, you MUST call read_file - never assume or guess file contents.
5. You work iteratively: read → analyze → edit → verify.
6. The ONLY acceptable "Final Answer" is after you have actually used tools to complete the work.
</critical_rules>

<tools_usage>
• read_file: Read a file to understand its current state (MANDATORY before modifications).
• edit_file: Modify an existing file using SEARCH/REPLACE blocks.
• write_file: Create a completely new file.
• list_dir: List directory contents.
• search_code: Semantic search across the codebase.
• run_cmd: Execute shell commands for verification.
</tools_usage>

<formatting_edit_file>
When using the edit_file tool, your "blocks" argument MUST use this exact format:
<<<<
exact original code lines here
====
new replacement code lines here
>>>>
- The SEARCH block MUST match the file exactly, including whitespace and indentation.
- You can include multiple blocks in one call.
</formatting_edit_file>

<forbidden_behaviors>
❌ Using write_file to modify an existing file (use edit_file instead!).
❌ Responding "I would modify line 45 to..." without calling a tool.
❌ Providing code snippets in your thought process without calling a tool.
❌ Making assumptions about file contents without calling read_file first.
</forbidden_behaviors>

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

func (a *ReActAgent) GetSystemPrompt() string {
	return a.sysMsg
}

// Run executes the ReAct loop using native function calling.
func (a *ReActAgent) Run(ctx context.Context, taskInput string) (string, error) {
	maxTurns := a.controller.CalculateAllowedTurns(0.5)

	sysMsg := llm.Message{
		Role:         "system",
		Content:      a.sysMsg,
		CacheControl: &llm.CacheControl{Type: "ephemeral"},
	}

	a.history = []llm.Message{
		sysMsg,
		{Role: "user", Content: taskInput},
	}

	for i := 0; i < maxTurns; i++ {
		if infraCtx.IsCancelled(ctx) {
			if a.bus != nil {
				a.bus.Publish(event.Event{Type: event.EventError, Payload: map[string]any{"error": "cancelled"}})
			}
			return "", fmt.Errorf("agent cancelled: %w", ctx.Err())
		}

		inputTokens := a.memory.Estimate(a.history)
		estimatedCost := a.costTracker.PredictCost(inputTokens, ExpectedOutputTokens)

		if err := a.budgetGuard.Reserve(estimatedCost); err != nil {
			if a.bus != nil {
				a.bus.Publish(event.Event{Type: event.EventError, Payload: map[string]any{"error": "budget_exceeded"}})
			}
			return "", fmt.Errorf("halted by budget guard: %w", err)
		}

		respMsg, err := func() (llm.Message, error) {
			defer a.budgetGuard.Commit(estimatedCost)
			msg, _, err := a.model.ChatNative(ctx, a.history, a.toolSchemas)
			return msg, err
		}()

		if err != nil {
			if a.bus != nil {
				a.bus.Publish(event.Event{Type: event.EventError, Payload: map[string]any{"error": err.Error()}})
			}
			return "", err
		}

		a.history = append(a.history, respMsg)

		if a.bus != nil {
			a.bus.Publish(event.Event{
				Type: event.EventAgentThought,
				Payload: map[string]any{
					"turn":    i,
					"content": respMsg.Content,
				},
				Timestamp: time.Now().Unix(),
			})
		}

		if len(respMsg.ToolCalls) == 0 {
			if strings.Contains(respMsg.Content, "Final Answer:") || i > 0 {
				if a.bus != nil {
					a.bus.Publish(event.Event{Type: event.EventAgentFinish, Payload: map[string]any{"result": respMsg.Content}})
				}
				return respMsg.Content, nil
			}
		}

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

				if err := a.rateLimiter.Wait(ctx, call.Function.Name); err != nil {
					mu.Lock()
					results = append(results, toolResult{call.ID, call.Function.Name, fmt.Sprintf("Error: %v", err)})
					mu.Unlock()
					return
				}

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

				if a.bus != nil {
					a.bus.Publish(event.Event{
						Type:    event.EventToolStart,
						Payload: map[string]any{"tool": call.Function.Name, "input": call.Function.Arguments},
					})
				}

				output, toolErr := tool.Execute(ctx, call.Function.Arguments)
				if toolErr != nil {
					output = fmt.Sprintf("Error: %v", toolErr)
				}

				processedOutput := a.processor.Process(call.Function.Name, output)

				if a.bus != nil {
					a.bus.Publish(event.Event{
						Type:    event.EventToolEnd,
						Payload: map[string]any{"tool": call.Function.Name, "status": "finished", "output": processedOutput},
					})
				}

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

		if len(results) > 0 {
			for _, res := range results {
				a.recordToolResult(res.callID, res.name, res.content)
			}
			a.pruneHistory()
		} else if len(respMsg.ToolCalls) > 0 {
			for _, tc := range respMsg.ToolCalls {
				a.recordToolResult(tc.ID, tc.Function.Name, "Error: Execution blocked or failed internally.")
			}
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

var toolInputRegistry = map[string]any{
	"read_file": struct {
		Path string `json:"path"`
	}{},
	"write_file": struct {
		Path    string `json:"path" desc:"The file path to write to"`
		Content string `json:"content" desc:"The complete file content to write"`
	}{},
	"edit_file": struct {
		Path   string `json:"path" desc:"The existing file path to edit"`
		Blocks string `json:"blocks" desc:"The SEARCH/REPLACE blocks"`
	}{},
	"list_dir": struct {
		Path string `json:"path"`
	}{},
	"search_code": struct {
		Query string `json:"query"`
	}{},
	"search_definition": struct {
		Name string `json:"name"`
	}{},
	"run_cmd": struct {
		Command string `json:"command"`
	}{},
	"read_skeleton": struct {
		Path string `json:"path"`
	}{},
	"git_diff": struct{}{},
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
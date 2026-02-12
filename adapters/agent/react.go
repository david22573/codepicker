package agent

import (
	"context"
	"fmt"
	"strings"
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
	controller  *AdaptiveController
	processor   *ObservationProcessor
	rateLimiter *ratelimit.ToolRateLimiter
	memory      *TurnMemory
	history     []llm.Message
	sysMsg      string
}

// NewReActAgent initializes the agent with a native tool-calling configuration.
func NewReActAgent(
	model *llm.OpenRouterAdapter,
	tools []agent.Tool,
	bus *event.DataBus,
	logger *logging.Logger,
	costTracker *llm.CostTracker,
	rateLimiter *ratelimit.ToolRateLimiter,
	budget float64,
) *ReActAgent {
	toolMap := make(map[string]agent.Tool)
	var schemas []llm.ToolDefinition

	// 1. Map Domain Tools to LLM Schemas using Reflection
	for _, t := range tools {
		toolMap[t.Name()] = t

		// Look up the strictly typed input struct for this tool
		inputStruct, exists := toolInputRegistry[t.Name()]
		if !exists {
			// Fallback: If unknown tool, assume a generic "command" string
			// This prevents crashes for custom tools not yet in the registry
			inputStruct = struct {
				Input string `json:"input" desc:"The input for the tool"`
			}{}
		}

		// Dynamically generate JSON Schema
		schemas = append(schemas, llm.GenerateToolDefinition(
			t.Name(),
			t.Description(),
			inputStruct,
		))
	}

	return &ReActAgent{
		model:       model,
		tools:       toolMap,
		toolSchemas: schemas,
		bus:         bus,
		logger:      logger,
		controller:  NewAdaptiveController(10, 30, costTracker, budget),
		processor:   NewObservationProcessor(8000),
		rateLimiter: rateLimiter,
		memory:      NewTurnMemory(4000),
		sysMsg:      "You are CodePicker, an expert Go developer. Use the provided tools to solve the task.",
	}
}

func (a *ReActAgent) Name() string { return "CodePicker-Native-v1" }

// UpdateSystemPrompt allows dynamic persona changes between analyst and worker roles.
func (a *ReActAgent) UpdateSystemPrompt(msg string) {
	a.sysMsg = msg
}

// Run executes the ReAct loop using native function calling.
func (a *ReActAgent) Run(ctx context.Context, taskInput string) (string, error) {
	maxTurns := a.controller.CalculateAllowedTurns(0.5)

	// Initialize Conversation History
	a.history = []llm.Message{
		{Role: "system", Content: a.sysMsg},
		{Role: "user", Content: taskInput},
	}

	for i := 0; i < maxTurns; i++ {
		// 1. Safety: Check for context cancellation
		if infraCtx.IsCancelled(ctx) {
			a.bus.Publish(event.Event{Type: event.EventError, Payload: map[string]interface{}{"error": "cancelled"}})
			return "", fmt.Errorf("agent cancelled: %w", ctx.Err())
		}

		// 2. Turn Execution: Request Native Tool Call
		respMsg, _, err := a.model.ChatNative(ctx, a.history, a.toolSchemas)
		if err != nil {
			a.bus.Publish(event.Event{Type: event.EventError, Payload: map[string]interface{}{"error": err.Error()}})
			return "", err
		}

		// 3. Record Assistant Response in History
		a.history = append(a.history, respMsg)

		// Emit Thought Event
		a.bus.Publish(event.Event{
			Type: event.EventAgentThought,
			Payload: map[string]interface{}{
				"turn":    i,
				"content": respMsg.Content,
			},
			Timestamp: time.Now().Unix(),
		})

		// 4. Check for Completion
		if len(respMsg.ToolCalls) == 0 {
			if strings.Contains(respMsg.Content, "Final Answer:") || i > 1 {
				a.bus.Publish(event.Event{Type: event.EventAgentFinish, Payload: map[string]interface{}{"result": respMsg.Content}})
				return respMsg.Content, nil
			}
		}

		// 5. Execute Tool Calls
		for _, tc := range respMsg.ToolCalls {
			if infraCtx.IsCancelled(ctx) {
				return "", fmt.Errorf("interrupted during tool execution")
			}

			// Rate Limiting
			if err := a.rateLimiter.Wait(ctx, tc.Function.Name); err != nil {
				a.recordToolResult(tc.ID, tc.Function.Name, fmt.Sprintf("Error: %v", err))
				continue
			}

			// Tool Lookup
			tool, exists := a.tools[tc.Function.Name]
			if !exists {
				a.recordToolResult(tc.ID, tc.Function.Name, "Error: Tool not found")
				continue
			}

			// Logging and Events
			a.bus.Publish(event.Event{
				Type:    event.EventToolStart,
				Payload: map[string]interface{}{"tool": tc.Function.Name, "input": tc.Function.Arguments},
			})

			// Execute Tool
			output, toolErr := tool.Execute(ctx, tc.Function.Arguments)
			if toolErr != nil {
				output = fmt.Sprintf("Error: %v", toolErr)
			}

			// Truncate and process output
			processedOutput := a.processor.Process(output)

			// Record Result back into Conversation History
			a.recordToolResult(tc.ID, tc.Function.Name, processedOutput)

			a.bus.Publish(event.Event{
				Type:    event.EventToolEnd,
				Payload: map[string]interface{}{"tool": tc.Function.Name, "status": "finished", "output": processedOutput},
			})
		}

		// 6. Memory Management: Prune history to stay within context window
		a.pruneHistory()
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
	if len(a.history) > 12 {
		preserved := make([]llm.Message, 0)
		preserved = append(preserved, a.history[0], a.history[1]) // Keep System & Task
		preserved = append(preserved, a.history[len(a.history)-10:]...)
		a.history = preserved
	}
}

// --- Tool Registry ---
// This bridges the generic agent.Tool interface with strict structs for LLM generation.
var toolInputRegistry = map[string]interface{}{
	"read_file": struct {
		Path string `json:"path"`
	}{},
	"write_file": struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}{},
	"list_files": struct {
		Path string `json:"path"`
	}{},
	"search_code": struct {
		Query string `json:"query"`
		Path  string `json:"path"`
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
}

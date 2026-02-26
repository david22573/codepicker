package agent

import (
	"context"
	"fmt"
	"sync"

	domainAgent "github.com/david22573/codepicker/domain/agent"
	infraCtx "github.com/david22573/codepicker/infra/context"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/ratelimit"
)

// ToolExecutor manages concurrent tool dispatching, rate limiting, and result processing.
type ToolExecutor struct {
	tools       map[string]domainAgent.Tool
	policy      domainAgent.Policy
	rateLimiter *ratelimit.ToolRateLimiter
	processor   *ObservationProcessor
	emitter     *EventEmitter
	verbose     bool
}

func NewToolExecutor(
	tools map[string]domainAgent.Tool,
	policy domainAgent.Policy,
	rateLimiter *ratelimit.ToolRateLimiter,
	processor *ObservationProcessor,
	emitter *EventEmitter,
	verbose bool,
) *ToolExecutor {
	return &ToolExecutor{
		tools:       tools,
		policy:      policy,
		rateLimiter: rateLimiter,
		processor:   processor,
		emitter:     emitter,
		verbose:     verbose,
	}
}

// ExecuteConcurrent runs the requested tool calls in parallel and returns their results
// in a deterministic order matching the input slice.
func (te *ToolExecutor) ExecuteConcurrent(ctx context.Context, calls []llm.ToolCall) []llm.Message {
	if len(calls) == 0 {
		return nil
	}

	results := make([]llm.Message, len(calls))
	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, tc llm.ToolCall) {
			defer wg.Done()

			output := te.executeSingle(ctx, tc)
			
			results[idx] = llm.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
				Content:    output,
			}
		}(i, call)
	}

	wg.Wait()
	return results
}

func (te *ToolExecutor) executeSingle(ctx context.Context, call llm.ToolCall) string {
	if infraCtx.IsCancelled(ctx) {
		return "Error: execution cancelled"
	}

	if err := te.rateLimiter.Wait(ctx, call.Function.Name); err != nil {
		return fmt.Sprintf("Error: rate limit exceeded - %v", err)
	}

	if te.policy != nil {
		allowed, reason := te.policy.CanExecute(call.Function.Name, call.Function.Arguments)
		if !allowed {
			return fmt.Sprintf("Error: Policy Violation - %s", reason)
		}
	}

	tool, exists := te.tools[call.Function.Name]
	if !exists {
		return "Error: Tool not found"
	}

	if te.verbose {
		fmt.Printf("   🔧 [TOOL] Calling: %s\n", call.Function.Name)
		fmt.Printf("   📥 Input: %s\n", truncate(call.Function.Arguments, 200))
	}

	te.emitter.ToolStart(call.Function.Name, call.Function.Arguments)

	output, err := tool.Execute(ctx, call.Function.Arguments)
	if err != nil {
		output = fmt.Sprintf("Error: %v", err)
	}

	processedOutput := te.processor.Process(call.Function.Name, output)

	te.emitter.ToolEnd(call.Function.Name, processedOutput)

	if te.verbose {
		status := "✅"
		if err != nil {
			status = "❌"
		}
		fmt.Printf("   %s Output: %s\n", status, truncate(processedOutput, 300))
	}

	return processedOutput
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
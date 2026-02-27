package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	domainAgent "github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/errors"
	"github.com/david22573/codepicker/domain/event"
	infraCtx "github.com/david22573/codepicker/infra/context"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/logging"
	"github.com/david22573/codepicker/infra/metrics"
	"github.com/david22573/codepicker/infra/prompts"
	"github.com/david22573/codepicker/infra/ratelimit"
	"github.com/david22573/codepicker/runtime"
)

type ReActAgent struct {
	model       llm.Provider
	toolSchemas []llm.ToolDefinition
	logger      *logging.Logger
	controller  *AdaptiveController
	budgetGuard *llm.BudgetGuard
	costTracker *llm.CostTracker
	memory      *TurnMemory
	emitter     *EventEmitter
	executor    *ToolExecutor
	history     []llm.Message
	sysMsg      string
	verbose     bool
}

func NewReActAgent(
	model llm.Provider,
	tools []domainAgent.Tool,
	bus *event.DataBus,
	logger *logging.Logger,
	policy domainAgent.Policy,
	costTracker *llm.CostTracker,
	rateLimiter *ratelimit.ToolRateLimiter,
	toolPool ToolWorkerPool,
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
		schemas = append(schemas, llm.GenerateToolDefinition(name, t.Description(), inputStruct))
	}

	baseTurns := maxTurns / 2
	if baseTurns < 10 {
		baseTurns = 10
	}

	emitter := NewEventEmitter(bus)
	processor := NewObservationProcessor(runtime.Global.MaxObservationLength)
	sysMsg, _ := prompts.Render("agent_system", nil)

	return &ReActAgent{
		model:       model,
		toolSchemas: schemas,
		logger:      logger,
		costTracker: costTracker,
		controller:  NewAdaptiveController(baseTurns, maxTurns, costTracker, budget),
		budgetGuard: llm.NewBudgetGuard(costTracker, budget),
		memory:      NewTurnMemory(runtime.Global.DefaultMemoryTokens),
		emitter:     emitter,
		executor:    NewToolExecutor(toolMap, policy, rateLimiter, processor, emitter, toolPool, false),
		sysMsg:      sysMsg,
	}
}

func (a *ReActAgent) Name() string { return "CodePicker-Native-v2" }

func (a *ReActAgent) UpdateSystemPrompt(msg string) { a.sysMsg = msg }

func (a *ReActAgent) SetVerbose(verbose bool) {
	a.verbose = verbose
	a.executor.verbose = verbose
}

func (a *ReActAgent) GetSystemPrompt() string { return a.sysMsg }

func (a *ReActAgent) Run(ctx context.Context, taskInput string) (string, error) {
	// Phase 5: Emit the final calculated session metrics upon exit
	defer func() {
		stats := a.costTracker.GetMetrics()
		a.emitter.SessionCostUpdate("react-session", stats.TotalTokens, stats.TotalCost, stats.TotalCost, 0.0)
	}()

	maxTurns := a.controller.CalculateAllowedTurns(0.5)

	a.history = []llm.Message{
		{Role: "system", Content: a.sysMsg, CacheControl: &llm.CacheControl{Type: "ephemeral"}},
		{Role: "user", Content: taskInput},
	}

	metrics.GetRegistry().ObserveValue("codepicker_budget_limit", a.budgetGuard.Remaining())

	for i := 0; i < maxTurns; i++ {
		if infraCtx.IsCancelled(ctx) {
			a.emitter.Cancelled()
			return "", errors.NewExecutionError(errors.CodeCancellation, "ReActAgent.Run", "agent cancelled by user/system", ctx.Err())
		}

		inputTokens := a.memory.Estimate(a.history)
		metrics.GetRegistry().ObserveValue("codepicker_estimated_input_tokens", float64(inputTokens))

		estimatedCost := a.costTracker.PredictCost(inputTokens, runtime.Global.ExpectedOutputTokens)

		if err := a.budgetGuard.Reserve(estimatedCost); err != nil {
			a.emitter.BudgetExceeded()
			return "", errors.NewExecutionError(errors.CodeBudgetExceeded, "ReActAgent.Run", "halted by budget guard", err)
		}
		a.emitter.BudgetReserved(estimatedCost)
		metrics.GetRegistry().ObserveValue("codepicker_budget_reserved", estimatedCost)

		llmStart := time.Now()
		respMsg, err := a.invokeModel(ctx, estimatedCost)
		metrics.GetRegistry().ObserveDuration("codepicker_llm_latency_seconds", time.Since(llmStart))

		if err != nil {
			a.emitter.Error(err)
			return "", errors.NewExecutionError(errors.CodeLLM, "ReActAgent.Run", "model invocation failed", err)
		}

		a.history = append(a.history, respMsg)
		a.emitter.Thought(i, respMsg.Content)

		if len(respMsg.ToolCalls) == 0 {
			if strings.Contains(respMsg.Content, "Final Answer:") || i > 0 {
				a.emitter.Finish(respMsg.Content)
				return respMsg.Content, nil
			}
		}

		toolResults := a.executor.ExecuteConcurrent(ctx, respMsg.ToolCalls)

		if len(toolResults) > 0 {
			a.history = append(a.history, toolResults...)
		} else if len(respMsg.ToolCalls) > 0 {
			for _, tc := range respMsg.ToolCalls {
				a.history = append(a.history, llm.Message{
					Role:       "tool",
					ToolCallID: tc.ID,
					Name:       tc.Function.Name,
					Content:    "Error: Execution blocked or failed internally.",
				})
			}
		}

		prunedCount := 0
		a.history, prunedCount = a.memory.Prune(a.history)
		if prunedCount > 0 {
			metrics.GetRegistry().IncCounter("codepicker_memory_prunes_total", nil)
		}
		a.emitter.MemoryPruned(prunedCount)
	}

	a.emitter.TurnLimitReached(maxTurns)
	return "", errors.NewExecutionError(errors.CodeTurnLimitExceeded, "ReActAgent.Run", fmt.Sprintf("max turns exceeded (%d)", maxTurns), nil)
}

func (a *ReActAgent) invokeModel(ctx context.Context, estimatedCost float64) (llm.Message, error) {
	defer func() {
		a.budgetGuard.Commit(estimatedCost)
		a.emitter.BudgetCommitted(estimatedCost)
		metrics.GetRegistry().ObserveValue("codepicker_budget_actual", a.costTracker.GetMetrics().TotalCost)
	}()
	
	msg, usage, err := a.model.ChatNative(ctx, a.history, a.toolSchemas)
	
	if err == nil {
		metrics.GetRegistry().AddCounter("codepicker_tokens_in_total", float64(usage.PromptTokens), nil)
		metrics.GetRegistry().AddCounter("codepicker_tokens_out_total", float64(usage.CompletionTokens), nil)
	}
	
	return msg, err
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
package agent

import (
	"context"
	"fmt"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/internal/prompts"
	"github.com/david22573/codepicker/internal/shadow"
	"github.com/david22573/codepicker/internal/tools"
	"github.com/david22573/codepicker/internal/tracking"
	"github.com/david22573/codepicker/internal/vfs"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type Engine struct {
	Client       *openrouter.Client
	Model        string
	SystemPrompt string

	Enforcer    *PolicyEnforcer
	Executor    *ToolExecutor
	Sentinel    *Sentinel
	CostTracker *tracking.CostTracker

	Config *config.ConfigFile
	Memory *WorkingMemory
	Limits *config.Limits
	Logger logger.Logger
}

func NewEngine(
	client *openrouter.Client,
	model, srcRoot string,
	log logger.Logger,
	limits *config.Limits,
	store *database.Store,
	cfg *config.ConfigFile,
) (*Engine, error) {
	shadowMgr, err := shadow.NewManager(srcRoot)
	if err != nil {
		return nil, err
	}
	fs := vfs.NewOverlayFS(srcRoot, shadowMgr)
	memory := NewMemory(store, fs)
	sentinel := NewSentinel(limits)
	costTracker := tracking.NewCostTracker(limits.DailyCostLimit)

	workerModel := "deepseek/deepseek-chat"
	if cfg != nil && cfg.AI.WorkerModel != "" {
		workerModel = cfg.AI.WorkerModel
	}
	workerRunner := NewWorkerRunner(client, workerModel, fs, log)

	runtimeCtx := &tools.RuntimeContext{
		FS:       fs,
		Memory:   memory,
		Sentinel: sentinel,
		Config:   cfg,
		Worker:   workerRunner,
	}

	enforcer := NewPolicyEnforcer(policy.Batch, log, sentinel)

	// Initialize Executor with empty tools; rebuildTools will populate it
	executor := NewToolExecutor(nil, runtimeCtx, enforcer)

	e := &Engine{
		Client:       client,
		Model:        model,
		SystemPrompt: prompts.Supervisor,
		Enforcer:     enforcer,
		Executor:     executor,
		Sentinel:     sentinel,
		Config:       cfg,
		Memory:       memory,
		CostTracker:  costTracker,
		Logger:       log,
		Limits:       limits,
	}

	// [3.1] Initial Tool Build
	e.rebuildTools(tools.SetStandard)

	return e, nil
}

// [3.1] Extract tool rebuild logic
func (e *Engine) rebuildTools(toolSet tools.ToolSet) {
	srcRoot := "."
	if overlay, ok := e.Executor.RuntimeContext.FS.(*vfs.OverlayFS); ok {
		srcRoot = overlay.SrcRoot
	}

	registry := tools.NewRegistry(srcRoot, e.Config)
	newTools := registry.GetImplementation(toolSet)

	// Clear existing tools map
	e.Executor.Tools = make(map[string]tools.Tool)

	for _, t := range newTools {
		// 1. Register with Executor
		e.Executor.Tools[t.Name()] = t

		// 2. Register with Enforcer (for Capability checks)
		e.Enforcer.RegisterTool(t)
	}

	e.Logger.Debug(fmt.Sprintf("Tools rebuilt. Set: %s. Count: %d", toolSet, len(newTools)))
}

func (e *Engine) SetPolicy(p policy.ExecutionPolicy) {
	e.Logger.Debug(fmt.Sprintf("Engine policy set to: %s (Mode: %s)", p.Name, p.Mode))

	e.Enforcer.Policy = p
	if p.Mode == policy.LevelInteractive {
		e.Enforcer.SetInteractionHandler(DefaultCLIInteraction)
	}

	var toolSet tools.ToolSet
	switch p.Name {
	case "Architect":
		toolSet = tools.SetReadOnly
	case "Batch":
		toolSet = tools.SetStandard
	default:
		toolSet = tools.SetAdmin
	}

	// [3.1] Use centralized logic
	e.rebuildTools(toolSet)
}

// ... Run() method remains unchanged ...
func (e *Engine) Run(ctx context.Context, task string, updateHistory func(openrouter.ChatMessage)) (string, error) {
	// (See previous Engine.Run implementation - no changes needed here for this phase)
	ctx, cancel := context.WithTimeout(ctx, e.Limits.AgentTimeout)
	defer cancel()

	messages := []openrouter.ChatMessage{{Role: "user", Content: task}}

	var activeTools []openrouter.Tool
	for _, t := range e.Executor.Tools {
		activeTools = append(activeTools, t.Definition())
	}

	for i := 0; i < e.Limits.AgentMaxTurns; i++ {
		currentContext := e.Memory.FormatContext()
		fullSystemMsg := e.SystemPrompt + "\n" + currentContext

		req := openrouter.ChatCompletionRequest{
			Model:    e.Model,
			Messages: append([]openrouter.ChatMessage{{Role: "system", Content: fullSystemMsg}}, messages...),
			Tools:    activeTools,
		}

		cost, _ := e.CostTracker.GetStats()
		if cost >= e.Limits.DailyCostLimit {
			return "", fmt.Errorf("daily cost limit exceeded ($%.2f)", e.Limits.DailyCostLimit)
		}

		resp, err := e.Client.CreateChatCompletion(ctx, req)
		if err != nil {
			return "", fmt.Errorf("LLM error: %w", err)
		}

		if resp.Usage != nil {
			e.CostTracker.RecordRequest(resp.Usage.PromptTokens, resp.Usage.CompletionTokens, e.Model)
		}

		msg := resp.Choices[0].Message
		messages = append(messages, *msg)
		if updateHistory != nil {
			updateHistory(*msg)
		}

		if len(msg.ToolCalls) == 0 {
			return fmt.Sprintf("%v", msg.Content), nil
		}

		for _, tool := range msg.ToolCalls {
			e.Logger.Debug(fmt.Sprintf("🔨 Executing Tool: %s", tool.Function.Name))
			resultStr := e.Executor.Execute(ctx, tool)

			toolMsg := openrouter.ChatMessage{
				Role:       "tool",
				ToolCallID: tool.ID,
				Content:    resultStr,
			}
			messages = append(messages, toolMsg)
			if updateHistory != nil {
				updateHistory(toolMsg)
			}
		}
	}
	return "", fmt.Errorf("agent exceeded max turns (%d)", e.Limits.AgentMaxTurns)
}

package agent

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/mcp"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/internal/prompts"
	"github.com/david22573/codepicker/internal/shadow"
	"github.com/david22573/codepicker/internal/tools"
	"github.com/david22573/codepicker/internal/tracking"
	"github.com/david22573/codepicker/internal/vfs"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type DebugConfig struct {
	Policy bool
	Tools  bool // Used as "Trace" mode to reveal internal thoughts
	Memory bool
}

type Engine struct {
	Client       *openrouter.Client
	Model        string // The "Brain" (Supervisor/Reasoning Model)
	WorkerModel  string // The "Hands" (Delegated Execution Model)
	SystemPrompt string

	Enforcer    *PolicyEnforcer
	Executor    *ToolExecutor
	Sentinel    *Sentinel
	CostTracker *tracking.CostTracker
	MCPManager  *mcp.Manager

	Config *config.ConfigFile
	Memory *WorkingMemory
	Limits *config.Limits
	Logger logger.Logger
	Debug  DebugConfig
}

func NewEngine(
	client *openrouter.Client,
	model, srcRoot string,
	log logger.Logger,
	limits *config.Limits,
	store *database.Store,
	cfg *config.ConfigFile,
	debug DebugConfig,
) (*Engine, error) {
	shadowMgr, err := shadow.NewManager(srcRoot)
	if err != nil {
		return nil, err
	}
	fs := vfs.NewOverlayFS(srcRoot, shadowMgr)

	memory := NewMemory(store, fs, debug.Memory)

	sentinel := NewSentinel(limits)
	costTracker := tracking.NewCostTracker(limits.DailyCostLimit)

	primaryModel := model
	if cfg != nil && cfg.AI.Model != "" {
		if primaryModel == "" || primaryModel == "default" {
			primaryModel = cfg.AI.Model
		}
	}
	if primaryModel == "" {
		primaryModel = "deepseek/deepseek-chat"
	}

	workerModel := primaryModel
	if cfg != nil && cfg.AI.WorkerModel != "" {
		workerModel = cfg.AI.WorkerModel
	} else {
		// Auto-downgrade worker for reasoning models to save cost/time
		if isReasoningModel(primaryModel) {
			fallbackWorker := "deepseek/deepseek-chat"
			log.Info(fmt.Sprintf("🧠 Reasoning model detected (%s). Auto-assigning worker to %s for stability.", primaryModel, fallbackWorker))
			workerModel = fallbackWorker
		}
	}

	log.Debug(fmt.Sprintf("Engine Init: Supervisor=%s, Worker=%s", primaryModel, workerModel))

	workerRunner := NewWorkerRunner(client, workerModel, fs, log)

	runtimeCtx := &tools.RuntimeContext{
		FS:       fs,
		Memory:   memory,
		Sentinel: sentinel,
		Config:   cfg,
		Worker:   workerRunner,
	}

	enforcer := NewPolicyEnforcer(policy.Batch, log, sentinel, debug.Policy)

	mcpMgr := mcp.NewManager(log)
	if cfg != nil && len(cfg.MCPServers) > 0 {
		go mcpMgr.StartServers(context.Background(), cfg.MCPServers)
	}

	// Fix: Pass correct arguments to NewToolExecutor
	// We pass nil for 'availableTools' initially; they are populated in rebuildTools
	executor := NewToolExecutor(nil, runtimeCtx, enforcer, debug.Tools)

	executor.AddMiddleware(NewSafetyLogMiddleware(log))
	executor.AddMiddleware(NewFormattingMiddleware(shadowMgr.ShadowRoot, log))

	e := &Engine{
		Client:       client,
		Model:        primaryModel,
		WorkerModel:  workerModel,
		SystemPrompt: prompts.Supervisor,
		Enforcer:     enforcer,
		Executor:     executor,
		Sentinel:     sentinel,
		MCPManager:   mcpMgr,
		Config:       cfg,
		Memory:       memory,
		CostTracker:  costTracker,
		Logger:       log,
		Limits:       limits,
		Debug:        debug,
	}

	e.rebuildTools(tools.SetStandard)

	return e, nil
}

func (e *Engine) rebuildTools(toolSet tools.ToolSet) {
	srcRoot := "."
	var shadowMgr *shadow.Manager

	if overlay, ok := e.Executor.RuntimeContext.FS.(*vfs.OverlayFS); ok {
		srcRoot = overlay.SrcRoot
		shadowMgr = overlay.GetShadowManager()
	}

	// Initialize the registry to get tool implementations
	registry := tools.NewRegistry(srcRoot, e.Config, shadowMgr)
	newTools := registry.GetImplementation(toolSet)

	if e.MCPManager != nil {
		mcpTools, err := e.MCPManager.GetTools(context.Background())
		if err == nil {
			for _, mt := range mcpTools {
				adapter := tools.NewMCPToolAdapter(mt.ServerName, mt.Tool, e.MCPManager)
				newTools = append(newTools, adapter)
				e.Logger.Debug(fmt.Sprintf("🔗 Bridged MCP Tool: %s", adapter.Name()))
			}
		}
	}

	e.Executor.Tools = make(map[string]tools.Tool)

	for _, t := range newTools {
		e.Executor.Tools[t.Name()] = t
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

	e.rebuildTools(toolSet)
}

func cleanMemory(content string) string {
	// Remove <think> blocks from reasoning models to keep context clean
	re := regexp.MustCompile(`(?s)<think>.*?</think>`)
	cleaned := re.ReplaceAllString(content, "")
	return cleaned
}

func isReasoningModel(modelID string) bool {
	return regexp.MustCompile(`(?i)(reasoner|deepseek-r1|o1-preview|nemotron)`).MatchString(modelID)
}

func (e *Engine) getDynamicCapabilities() string {
	var builder strings.Builder
	builder.WriteString("\n### ACTIVE TOOLSET (You have access to these tools):\n")

	var names []string
	for k := range e.Executor.Tools {
		names = append(names, k)
	}
	sort.Strings(names)

	for _, name := range names {
		tool := e.Executor.Tools[name]
		builder.WriteString(fmt.Sprintf("- %s: %s\n", name, tool.Description()))
	}

	builder.WriteString("\nReminder: You must use the tools exactly as named above.")
	return builder.String()
}

func trimMessageHistory(messages []openrouter.ChatMessage, maxRecent int) []openrouter.ChatMessage {
	if len(messages) <= maxRecent+1 {
		return messages
	}

	// Keep system prompt (index 0) and the most recent N messages
	trimmed := make([]openrouter.ChatMessage, 0, maxRecent+1)
	trimmed = append(trimmed, messages[0])

	startIdx := len(messages) - maxRecent
	trimmed = append(trimmed, messages[startIdx:]...)

	return trimmed
}

func (e *Engine) Run(ctx context.Context, task string, updateHistory func(openrouter.ChatMessage)) (string, error) {

	startTime := time.Now()

	if len(task) > e.Limits.MaxQueryLength {
		e.Logger.Warn(fmt.Sprintf("Task description truncated from %d to %d chars", len(task), e.Limits.MaxQueryLength))
		task = task[:e.Limits.MaxQueryLength] + "...(truncated)"
	}

	messages := []openrouter.ChatMessage{{Role: "user", Content: task}}

	var activeTools []openrouter.Tool
	for _, t := range e.Executor.Tools {
		activeTools = append(activeTools, t.Definition())
	}

	seenActions := make(map[string]bool)
	hardLimit := e.Limits.AgentMaxTurns * 3
	totalTurns := 0

	toolCapabilities := e.getDynamicCapabilities()

	const maxRecentMessages = 20

	for i := 0; i < e.Limits.AgentMaxTurns; i++ {
		totalTurns++
		if totalTurns >= hardLimit {
			return "", fmt.Errorf("agent hit HARD safety limit (%d turns) despite making progress", hardLimit)
		}

		if time.Since(startTime) > e.Limits.AgentTimeout {
			return "", fmt.Errorf("agent exceeded global session timeout of %v", e.Limits.AgentTimeout)
		}

		if i > 0 {
			time.Sleep(500 * time.Millisecond)
		}

		cost, _ := e.CostTracker.GetStats()
		if cost >= e.Limits.DailyCostLimit {
			return "", fmt.Errorf("daily cost limit exceeded ($%.2f). Stopping execution to prevent billing overrun", e.Limits.DailyCostLimit)
		}

		currentContext := e.Memory.FormatContext()

		fullSystemMsg := e.SystemPrompt + "\n" + toolCapabilities + "\n" + currentContext

		trimmedMessages := trimMessageHistory(messages, maxRecentMessages)

		req := openrouter.ChatCompletionRequest{
			Model:     e.Model,
			Messages:  append([]openrouter.ChatMessage{{Role: "system", Content: fullSystemMsg}}, trimmedMessages...),
			Tools:     activeTools,
			MaxTokens: e.Limits.MaxStepTokens,
		}

		stepCtx, stepCancel := context.WithTimeout(ctx, e.Limits.CommandTimeout*5)
		resp, err := e.Client.CreateChatCompletion(stepCtx, req)
		stepCancel()

		if err != nil {
			return "", fmt.Errorf("LLM error (Model: %s): %w", e.Model, err)
		}

		if resp.Usage != nil {
			e.CostTracker.RecordRequest(resp.Usage.PromptTokens, resp.Usage.CompletionTokens, e.Model)
		}

		msg := resp.Choices[0].Message

		shouldReport := len(msg.ToolCalls) > 0 || e.Debug.Tools
		if updateHistory != nil && shouldReport {
			updateHistory(*msg)
		}

		memoryMsg := *msg
		if memoryMsg.Content != nil {
			text := fmt.Sprintf("%v", memoryMsg.Content)
			memoryMsg.Content = cleanMemory(text)
		}
		messages = append(messages, memoryMsg)

		if len(msg.ToolCalls) == 0 {
			content := fmt.Sprintf("%v", msg.Content)
			return content, nil
		}

		isProgressMade := false

		for _, tool := range msg.ToolCalls {
			e.Logger.Debug(fmt.Sprintf("🔨 Executing Tool: %s", tool.Function.Name))

			// Deduplication logic
			actionSig := fmt.Sprintf("%s:%s", tool.Function.Name, tool.Function.Arguments)
			if !seenActions[actionSig] {
				seenActions[actionSig] = true
				isProgressMade = true
			}

			resultStr := e.Executor.Execute(ctx, tool)

			if len(resultStr) > e.Limits.MaxToolOutput {
				e.Logger.Warn(fmt.Sprintf("Tool output truncated: %s (%d bytes)", tool.Function.Name, len(resultStr)))
				resultStr = resultStr[:e.Limits.MaxToolOutput] + "\n...(output truncated by security limit)"
			}

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

		if isProgressMade {
			if i > 0 {
				i-- // Reward progress by not counting this turn against the limit
				e.Logger.Debug("🔄 Progress detected (unique tool usage). Turn budget extended.")
			}
		} else {
			messages = append(messages, openrouter.ChatMessage{
				Role:    "user",
				Content: "System Warning: You are repeating the exact same tool call. Stop and reflect. Try a different approach.",
			})
		}

		if len(messages) > maxRecentMessages*2 {
			messages = trimMessageHistory(messages, maxRecentMessages)
			e.Logger.Debug(fmt.Sprintf("🧹 Trimmed message history to %d messages to prevent memory leak", len(messages)))
		}
	}

	return "", fmt.Errorf("agent exceeded max turns (%d) without sufficient unique progress", e.Limits.AgentMaxTurns)
}

func (e *Engine) RunSingleTurn(
	ctx context.Context,
	task string,
	updateHistory func(openrouter.ChatMessage),
) (string, error) {

	cost, _ := e.CostTracker.GetStats()
	if cost >= e.Limits.DailyCostLimit {
		return "", fmt.Errorf("daily cost limit exceeded ($%.2f)", e.Limits.DailyCostLimit)
	}

	currentContext := e.Memory.FormatContext()

	toolCapabilities := e.getDynamicCapabilities()
	fullSystemMsg := e.SystemPrompt + "\n" + toolCapabilities + "\n" + currentContext

	var activeTools []openrouter.Tool
	for _, t := range e.Executor.Tools {
		activeTools = append(activeTools, t.Definition())
	}

	messages := []openrouter.ChatMessage{
		{Role: "system", Content: fullSystemMsg},
		{Role: "user", Content: task},
	}

	req := openrouter.ChatCompletionRequest{
		Model:     e.Model,
		Messages:  messages,
		Tools:     activeTools,
		MaxTokens: e.Limits.MaxStepTokens,
	}

	resp, err := e.Client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("LLM error: %w", err)
	}

	if resp.Usage != nil {
		e.CostTracker.RecordRequest(
			resp.Usage.PromptTokens,
			resp.Usage.CompletionTokens,
			e.Model,
		)
	}

	msg := resp.Choices[0].Message
	if updateHistory != nil {
		updateHistory(*msg)
	}

	if len(msg.ToolCalls) == 0 {
		return fmt.Sprintf("%v", msg.Content), nil
	}

	e.Logger.Debug(fmt.Sprintf("Executing %d tools in this turn", len(msg.ToolCalls)))

	for _, tool := range msg.ToolCalls {
		e.Logger.Debug(fmt.Sprintf("🔨 Tool: %s", tool.Function.Name))

		resultStr := e.Executor.Execute(ctx, tool)

		if len(resultStr) > e.Limits.MaxToolOutput {
			e.Logger.Warn(fmt.Sprintf("Tool output truncated: %s (%d bytes)",
				tool.Function.Name, len(resultStr)))
			resultStr = resultStr[:e.Limits.MaxToolOutput] +
				"\n...(output truncated by security limit)"
		}

		if updateHistory != nil {
			toolMsg := openrouter.ChatMessage{
				Role:       "tool",
				ToolCallID: tool.ID,
				Content:    resultStr,
			}
			updateHistory(toolMsg)
		}
	}

	return fmt.Sprintf("%v", msg.Content), nil
}

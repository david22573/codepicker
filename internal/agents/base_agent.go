package agents

import (
	"context"
	"fmt"
	"time"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/shadow"
	"github.com/david22573/codepicker/internal/tools"
	"github.com/david22573/codepicker/internal/vfs"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type BaseAgent struct {
	Type         AgentType
	Client       *openrouter.Client
	Model        string
	SystemPrompt string
	Tools        []openrouter.Tool

	Executor *agent.ToolExecutor
	Logger   logger.Logger
	Limits   *config.Limits
	Memory   *agent.WorkingMemory
}

func NewBaseAgent(
	aType AgentType,
	client *openrouter.Client,
	model string,
	prompt string,
	memory *agent.WorkingMemory,
	fs vfs.VirtualFileSystem,
	sentinel *agent.Sentinel,
	log logger.Logger,
	limits *config.Limits,
	cfg *config.ConfigFile,
) *BaseAgent {

	// Create a runtime context for this agent
	rt := &tools.RuntimeContext{
		FS:       fs,
		Memory:   memory,
		Sentinel: sentinel,
		Config:   cfg,
	}

	srcDir := "."
	if cfg != nil && cfg.Src != "" {
		srcDir = cfg.Src
	}
	registry := tools.NewRegistry(srcDir)

	var toolSet tools.ToolSet
	switch aType {
	case AgentModifier:
		toolSet = tools.SetStandard
	case AgentSystem:
		toolSet = tools.SetAdmin
	case AgentOrchestrator:
		toolSet = tools.SetOrchestrator
	default:
		toolSet = tools.SetReadOnly
	}

	myTools := registry.GetImplementation(toolSet)

	// FIX: Use the new constructor signature
	exec := agent.NewToolExecutor(myTools, rt)

	// Convert to OpenRouter definitions
	var defs []openrouter.Tool
	for _, t := range myTools {
		defs = append(defs, t.Definition())
	}

	return &BaseAgent{
		Type:         aType,
		Client:       client,
		Model:        model,
		SystemPrompt: prompt,
		Tools:        defs,
		Executor:     exec,
		Logger:       log,
		Limits:       limits,
		Memory:       memory,
	}
}

func pruneHistory(messages []openrouter.ChatMessage, keepLast int) []openrouter.ChatMessage {
	if len(messages) <= keepLast+1 {
		return messages
	}

	systemPrompt := messages[0]
	recentMessages := messages[len(messages)-keepLast:]

	return append([]openrouter.ChatMessage{systemPrompt}, recentMessages...)
}

func (a *BaseAgent) Execute(ctx context.Context, task string) (string, error) {
	a.Logger.Info(fmt.Sprintf("🤖 [%s] Starting task: %s", a.Type, task))

	messages := []openrouter.ChatMessage{
		{Role: "user", Content: task},
	}

	maxTurns := a.Limits.AgentMaxTurns
	if maxTurns == 0 {
		maxTurns = 50
	}

	for i := 0; i < maxTurns; i++ {

		contextStr := a.Memory.FormatContext()
		sysMsg := fmt.Sprintf("%s\n\n%s", a.SystemPrompt, contextStr)

		currentRequestMsgs := append([]openrouter.ChatMessage{{Role: "system", Content: sysMsg}}, messages...)

		req := openrouter.ChatCompletionRequest{
			Model:    a.Model,
			Messages: currentRequestMsgs,
			Tools:    a.Tools,
		}

		resp, err := a.Client.CreateChatCompletion(ctx, req)
		if err != nil {
			return "", fmt.Errorf("[%s] LLM error: %w", a.Type, err)
		}

		msg := resp.Choices[0].Message

		messages = append(messages, *msg)

		if len(msg.ToolCalls) == 0 {
			return fmt.Sprintf("%v", msg.Content), nil
		}

		for _, tool := range msg.ToolCalls {
			a.Logger.Info(fmt.Sprintf("🔨 [%s] Executing Tool: %s", a.Type, tool.Function.Name))

			// FIX: Pass context to Execute
			resultStr := a.Executor.Execute(ctx, tool)

			messages = append(messages, openrouter.ChatMessage{
				Role:       "tool",
				ToolCallID: tool.ID,
				Content:    resultStr,
			})
		}

		if len(messages) > 20 {
			messages = pruneHistory(messages, 20)
		}

		time.Sleep(200 * time.Millisecond)
	}

	return "", fmt.Errorf("[%s] Exceeded max turns (%d)", a.Type, maxTurns)
}

func NewEnvironment(srcDir, apiKey string, log logger.Logger) (*database.Store, vfs.VirtualFileSystem, *agent.WorkingMemory, *config.Limits, error) {
	store, err := database.New(".codepicker")
	if err != nil {
		return nil, nil, nil, nil, err
	}

	shadowMgr, err := shadow.NewManager(srcDir)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	fs := vfs.NewOverlayFS(srcDir, shadowMgr)

	mem := agent.NewMemory(store, fs)
	limits := config.DefaultLimits()

	return store, fs, mem, limits, nil
}

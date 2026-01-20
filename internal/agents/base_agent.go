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

	exec := agent.NewToolExecutor(memory, fs, sentinel, cfg)

	return &BaseAgent{
		Type:         aType,
		Client:       client,
		Model:        model,
		SystemPrompt: prompt,
		Tools:        getToolsFor(aType),
		Executor:     exec,
		Logger:       log,
		Limits:       limits,
		Memory:       memory,
	}
}

// pruneHistory keeps the System Prompt (index 0) and the last N messages.
// This implements a "Sliding Window" to save tokens on long-running tasks.
func pruneHistory(messages []openrouter.ChatMessage, keepLast int) []openrouter.ChatMessage {
	if len(messages) <= keepLast+1 { // +1 for System prompt which we always keep
		return messages
	}

	// Always keep the System Prompt (index 0) so the agent remembers who it is
	systemPrompt := messages[0]

	// Keep the last N messages to maintain recent conversational context
	recentMessages := messages[len(messages)-keepLast:]

	// Reconstruct: System + Recent
	return append([]openrouter.ChatMessage{systemPrompt}, recentMessages...)
}

func (a *BaseAgent) Execute(ctx context.Context, task string) (string, error) {
	a.Logger.Info(fmt.Sprintf("🤖 [%s] Starting task: %s", a.Type, task))

	messages := []openrouter.ChatMessage{
		{Role: "user", Content: task},
	}

	maxTurns := a.Limits.AgentMaxTurns
	if maxTurns == 0 {
		maxTurns = 50 // Safe default
	}

	for i := 0; i < maxTurns; i++ {

		// 1. Refresh Dynamic Context (Working Memory)
		contextStr := a.Memory.FormatContext()
		sysMsg := fmt.Sprintf("%s\n\n%s", a.SystemPrompt, contextStr)

		// Construct the request: [System + Context] + [Conversation History]
		// We create a temporary slice so we don't duplicate the massive context string in history
		currentRequestMsgs := append([]openrouter.ChatMessage{{Role: "system", Content: sysMsg}}, messages...)

		req := openrouter.ChatCompletionRequest{
			Model:    a.Model,
			Messages: currentRequestMsgs,
			Tools:    a.Tools,
		}

		// 2. Call LLM
		resp, err := a.Client.CreateChatCompletion(ctx, req)
		if err != nil {
			return "", fmt.Errorf("[%s] LLM error: %w", a.Type, err)
		}

		msg := resp.Choices[0].Message

		// Add the assistant's reply to history
		messages = append(messages, *msg)

		// 3. Check for completion (No tools called = Done)
		if len(msg.ToolCalls) == 0 {
			return fmt.Sprintf("%v", msg.Content), nil
		}

		// 4. Execute Tools
		for _, tool := range msg.ToolCalls {
			a.Logger.Info(fmt.Sprintf("🔨 [%s] Executing Tool: %s", a.Type, tool.Function.Name))

			resultStr := a.Executor.Execute(tool)

			// Add tool output to history
			messages = append(messages, openrouter.ChatMessage{
				Role:       "tool",
				ToolCallID: tool.ID,
				Content:    resultStr,
			})
		}

		// 5. OPTIMIZATION: Prune history
		// Keep last 20 messages to balance context vs tokens
		if len(messages) > 20 {
			messages = pruneHistory(messages, 20)
		}

		// Small delay to be polite to the API (though Client handles 429s now)
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

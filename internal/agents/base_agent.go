package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/david22573/codepicker/internal/agent" // Importing legacy for shared types (Memory, Sentinel)
	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/shadow"
	"github.com/david22573/codepicker/pkg/openrouter"
)

// BaseAgent encapsulates the shared logic for all specialized agents
type BaseAgent struct {
	Type         AgentType
	Client       *openrouter.Client
	Model        string
	SystemPrompt string
	Tools        []openrouter.Tool

	// Shared Resources
	Memory   *agent.WorkingMemory
	Shadow   *shadow.Manager
	Sentinel *agent.Sentinel
	Logger   logger.Logger
	Limits   *config.Limits
}

func NewBaseAgent(
	aType AgentType,
	client *openrouter.Client,
	model string,
	prompt string,
	memory *agent.WorkingMemory,
	shadow *shadow.Manager,
	sentinel *agent.Sentinel,
	log logger.Logger,
	limits *config.Limits,
) *BaseAgent {
	return &BaseAgent{
		Type:         aType,
		Client:       client,
		Model:        model,
		SystemPrompt: prompt,
		Tools:        getToolsFor(aType),
		Memory:       memory,
		Shadow:       shadow,
		Sentinel:     sentinel,
		Logger:       log,
		Limits:       limits,
	}
}

// Execute runs the agent loop for a specific task
func (a *BaseAgent) Execute(ctx context.Context, task string) (string, error) {
	a.Logger.Info(fmt.Sprintf("🤖 [%s] Starting task: %s", a.Type, task))

	messages := []openrouter.ChatMessage{
		{Role: "user", Content: task},
	}

	maxTurns := a.Limits.AgentMaxTurns
	if maxTurns == 0 {
		maxTurns = 10
	}

	for i := 0; i < maxTurns; i++ {
		// 1. Prepare Context
		contextStr := a.Memory.FormatContext()
		sysMsg := fmt.Sprintf("%s\n\n%s", a.SystemPrompt, contextStr)

		// 2. Build Request (Prepend System Prompt)
		reqMessages := append([]openrouter.ChatMessage{{Role: "system", Content: sysMsg}}, messages...)

		req := openrouter.ChatCompletionRequest{
			Model:    a.Model,
			Messages: reqMessages,
			Tools:    a.Tools,
		}

		// 3. Call LLM
		resp, err := a.Client.CreateChatCompletion(ctx, req)
		if err != nil {
			return "", fmt.Errorf("[%s] LLM error: %w", a.Type, err)
		}

		msg := resp.Choices[0].Message
		messages = append(messages, *msg)

		// 4. Handle Tools or Return Content
		if len(msg.ToolCalls) == 0 {
			return fmt.Sprintf("%v", msg.Content), nil
		}

		for _, tool := range msg.ToolCalls {
			a.Logger.Info(fmt.Sprintf("🔨 [%s] Executing Tool: %s", a.Type, tool.Function.Name))
			resultStr := a.executeTool(tool)

			messages = append(messages, openrouter.ChatMessage{
				Role:       "tool",
				ToolCallID: tool.ID,
				Content:    resultStr,
			})
		}

		time.Sleep(500 * time.Millisecond)
	}

	return "", fmt.Errorf("[%s] Exceeded max turns (%d)", a.Type, maxTurns)
}

func (a *BaseAgent) executeTool(tool openrouter.ToolCall) string {
	switch tool.Function.Name {
	case "read_file":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("Invalid arguments: %v", err)
		}
		if err := a.Memory.Add(args.Path); err != nil {
			return fmt.Sprintf("Error reading '%s': %v", args.Path, err)
		}
		return fmt.Sprintf("✓ File '%s' loaded into context", args.Path)

	case "search_code":
		var args struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("Invalid arguments: %v", err)
		}
		// Reuse the PerformSearch logic from legacy agent package
		results, err := agent.PerformSearch(a.Memory.SrcRoot, args.Query)
		if err != nil {
			return fmt.Sprintf("Search error: %v", err)
		}
		return results

	case "write_shadow_file":
		var args struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("Invalid arguments: %v", err)
		}
		path, err := a.Shadow.WriteFile(args.Path, []byte(args.Content))
		if err != nil {
			return fmt.Sprintf("Error writing shadow file: %v", err)
		}
		return fmt.Sprintf("Changes written to shadow file: %s", path)

	case "run_shell":
		var args struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("Invalid arguments: %v", err)
		}

		// Security Check
		needsApproval, reason, binary, cmdArgs := a.Sentinel.CheckCommand(args.Command)
		if needsApproval {
			// In Phase 1, we default to "Ask User" via CLI, but for pure agents we might block.
			// For now, we log strict warning.
			a.Logger.Warn(fmt.Sprintf("⚠️ [%s] Running high-risk command: %s (Reason: %s)", a.Type, args.Command, reason))
		}

		out, err := a.Sentinel.Execute(binary, cmdArgs)
		if err != nil {
			return fmt.Sprintf("Command failed: %v\nOutput: %s", err, out)
		}
		return out

	default:
		return fmt.Sprintf("Tool %s not supported by this agent", tool.Function.Name)
	}
}

// Helper to create a fully initialized environment for testing/usage
func NewEnvironment(srcDir, apiKey string, log logger.Logger) (*database.Store, *shadow.Manager, *agent.WorkingMemory, *config.Limits, error) {
	store, err := database.New(".codepicker")
	if err != nil {
		return nil, nil, nil, nil, err
	}

	shadowMgr, err := shadow.NewManager(srcDir)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	mem := agent.NewMemory(srcDir, store, shadowMgr)
	limits := config.DefaultLimits()

	return store, shadowMgr, mem, limits, nil
}

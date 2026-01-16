package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/shadow"
	"github.com/david22573/codepicker/internal/tracking"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type Engine struct {
	Client           *openrouter.Client
	Model            string
	Sentinel         *Sentinel
	Shadow           *shadow.Manager
	Memory           *WorkingMemory
	Logger           logger.Logger
	SrcRoot          string
	ApprovalCallback func(command string, reason string) bool
	CostTracker      *tracking.CostTracker
	Limits           *config.Limits
	Config           *config.ConfigFile // Added Config reference
}

func NewEngine(client *openrouter.Client, model, srcRoot string, log logger.Logger, limits *config.Limits) (*Engine, error) {
	shadowMgr, err := shadow.NewManager(srcRoot)
	if err != nil {
		return nil, err
	}

	// Load config for custom tools
	cfg, _ := config.LoadConfigFile("")

	return &Engine{
		Client:           client,
		Model:            model,
		Sentinel:         NewSentinel(limits),
		Shadow:           shadowMgr,
		Memory:           NewMemory(srcRoot),
		Logger:           log,
		SrcRoot:          srcRoot,
		ApprovalCallback: func(c, r string) bool { return false },
		CostTracker:      tracking.NewCostTracker(limits.DailyCostLimit),
		Limits:           limits,
		Config:           cfg,
	}, nil
}

func (e *Engine) Run(ctx context.Context, task string, updateHistory func(openrouter.ChatMessage)) (string, error) {

	ctx, cancel := context.WithTimeout(ctx, e.Limits.AgentTimeout)
	defer cancel()

	baseSystemPrompt := `You are an autonomous AI developer agent.
RULES:
1. Code context is provided in "ACTIVE SOURCE FILES".
2. If you need to see a file not listed there, use 'read_file' to add it to context.
3. DO NOT output code for files already in context unless you are changing them.
4. Use 'write_shadow_file' to propose changes.`

	messages := []openrouter.ChatMessage{
		{Role: "user", Content: task},
	}

	// Load dynamic tools
	activeTools := GetTools(e.Config)

	for i := 0; i < e.Limits.AgentMaxTurns; i++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		currentContext := e.Memory.FormatContext()
		fullSystemMsg := baseSystemPrompt + "\n" + currentContext

		requestMessages := append([]openrouter.ChatMessage{{Role: "system", Content: fullSystemMsg}}, messages...)

		req := openrouter.ChatCompletionRequest{
			Model:    e.Model,
			Messages: requestMessages,
			Tools:    activeTools,
		}

		e.Logger.Info(fmt.Sprintf("Agent thinking (Turn %d/%d)...", i+1, e.Limits.AgentMaxTurns))

		cost, _ := e.CostTracker.GetStats()
		if cost >= e.Limits.DailyCostLimit {
			return "", fmt.Errorf("daily cost limit exceeded ($%.2f)", e.Limits.DailyCostLimit)
		}

		resp, err := e.Client.CreateChatCompletion(ctx, req)
		if err != nil {
			return "", fmt.Errorf("LLM error: %w", err)
		}

		if resp.Usage != nil {
			if err := e.CostTracker.RecordRequest(
				resp.Usage.PromptTokens,
				resp.Usage.CompletionTokens,
				e.Model,
			); err != nil {
				e.Logger.Warn(fmt.Sprintf("Cost tracking alert: %v", err))
			}
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
			resultStr := ""
			e.Logger.Info(fmt.Sprintf("🔨 Executing Tool: %s", tool.Function.Name))

			// Handle Built-ins
			switch tool.Function.Name {
			case "read_file":
				var args struct {
					Path string `json:"path"`
				}
				if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
					resultStr = fmt.Sprintf("Invalid arguments: %v", err)
					break
				}
				if err := e.Memory.Add(args.Path); err != nil {
					resultStr = fmt.Sprintf("Error reading '%s': %v", args.Path, err)
				} else {
					resultStr = fmt.Sprintf("✓ File '%s' loaded into context", args.Path)
				}

			case "write_shadow_file":
				var args struct {
					Path    string `json:"path"`
					Content string `json:"content"`
				}
				if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
					resultStr = fmt.Sprintf("Invalid arguments: %v", err)
					break
				}
				path, err := e.Shadow.WriteFile(args.Path, []byte(args.Content))
				if err != nil {
					resultStr = fmt.Sprintf("Error writing shadow file: %v", err)
				} else {
					resultStr = fmt.Sprintf("Changes written to shadow file: %s", path)
				}

			case "run_shell":
				var args struct {
					Command string `json:"command"`
				}
				if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
					resultStr = fmt.Sprintf("Invalid arguments: %v", err)
					break
				}

				needsApproval, reason, binary, cmdArgs := e.Sentinel.CheckCommand(args.Command)
				if needsApproval {
					if !e.ApprovalCallback(args.Command, reason) {
						resultStr = "Command denied by user."
						break
					}
				}
				// Execute with recovery logic
				recoveryResult := e.ExecuteWithRecovery(binary, cmdArgs, e.Limits.MaxRecoveryAttempts)
				if recoveryResult.Success {
					resultStr = recoveryResult.FinalOutput
				} else {
					resultStr = fmt.Sprintf("Command failed: %v\nOutput: %s", recoveryResult.FinalError, recoveryResult.FinalOutput)
				}

			default:
				// Handle Custom Plugins
				out, err := ExecuteCustomTool(tool.Function.Name, tool.Function.Arguments, e.Config)
				if err != nil {
					resultStr = fmt.Sprintf("Tool execution error: %v", err)
				} else {
					resultStr = out
				}
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
	}
	return "", fmt.Errorf("agent exceeded max turns (%d)", e.Limits.AgentMaxTurns)
}

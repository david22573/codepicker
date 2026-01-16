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
}

func NewEngine(client *openrouter.Client, model, srcRoot string, log logger.Logger, limits *config.Limits) (*Engine, error) {
	shadowMgr, err := shadow.NewManager(srcRoot)
	if err != nil {
		return nil, err
	}

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
			Tools:    Tools,
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

			switch tool.Function.Name {
			case "read_file":
				var args struct {
					Path string `json:"path"`
				}
				if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
					e.Logger.Error(fmt.Sprintf("Failed to parse read_file args: %v", err))
					resultStr = fmt.Sprintf("Invalid arguments for read_file: %v", err)
					break
				}
				if err := e.Memory.Add(args.Path); err != nil {
					e.Logger.Warn(fmt.Sprintf("Failed to read file %s: %v", args.Path, err))
					resultStr = fmt.Sprintf("Error: Could not read '%s'. %v", args.Path, err)
				} else {
					e.Logger.Info(fmt.Sprintf("Added to context: %s", args.Path))
					resultStr = fmt.Sprintf("✓ File '%s' loaded into context", args.Path)
				}

			case "write_shadow_file":
				var args struct {
					Path    string `json:"path"`
					Content string `json:"content"`
				}
				if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
					e.Logger.Error(fmt.Sprintf("Failed to parse write_shadow_file args: %v", err))
					resultStr = fmt.Sprintf("Invalid arguments for write_shadow_file: %v", err)
					break
				}
				path, err := e.Shadow.WriteFile(args.Path, []byte(args.Content))
				if err != nil {
					e.Logger.Warn(fmt.Sprintf("Failed to write shadow file %s: %v", args.Path, err))
					resultStr = fmt.Sprintf("Error writing shadow file: %v", err)
				} else {
					e.Logger.Info(fmt.Sprintf("Written to shadow: %s", args.Path))
					resultStr = fmt.Sprintf("Changes written to shadow file: %s", path)
				}

			case "run_shell":
				var args struct {
					Command string `json:"command"`
				}
				if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
					e.Logger.Error(fmt.Sprintf("Failed to parse run_shell args: %v", err))
					resultStr = fmt.Sprintf("Error parsing arguments: %v", err)
					break
				}

				needsApproval, reason, binary, cmdArgs := e.Sentinel.CheckCommand(args.Command)
				if needsApproval {
					if !e.ApprovalCallback(args.Command, reason) {
						resultStr = "Command denied by user."
						break
					}
				}

				// Phase 1: Recovery Integrated Execution
				recoveryResult := e.ExecuteWithRecovery(binary, cmdArgs, e.Limits.MaxRecoveryAttempts)

				if recoveryResult.Success {
					resultStr = recoveryResult.FinalOutput
					if recoveryResult.Attempted {
						resultStr = fmt.Sprintf("[Auto-recovery: %s]\n%s",
							recoveryResult.StrategyUsed, recoveryResult.FinalOutput)
					}
				} else {
					resultStr = fmt.Sprintf("Command failed: %v\nOutput:\n%s",
						recoveryResult.FinalError, recoveryResult.FinalOutput)

					if recoveryResult.Attempted {
						resultStr += fmt.Sprintf("\n\n(Recovery attempted via %s but failed. Actions taken: %v)",
							recoveryResult.StrategyUsed, recoveryResult.ActionsTaken)
					}
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

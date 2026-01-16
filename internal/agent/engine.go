package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/shadow"
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
}

func NewEngine(client *openrouter.Client, model, srcRoot string, log logger.Logger) (*Engine, error) {
	shadowMgr, err := shadow.NewManager(srcRoot)
	if err != nil {
		return nil, err
	}

	return &Engine{
		Client:           client,
		Model:            model,
		Sentinel:         NewSentinel(),
		Shadow:           shadowMgr,
		Memory:           NewMemory(srcRoot),
		Logger:           log,
		SrcRoot:          srcRoot,
		ApprovalCallback: func(c, r string) bool { return false },
	}, nil
}

func (e *Engine) Run(ctx context.Context, task string, updateHistory func(openrouter.ChatMessage)) (string, error) {

	baseSystemPrompt := `You are an autonomous AI developer agent.
RULES:
1. Code context is provided in "ACTIVE SOURCE FILES".
2. If you need to see a file not listed there, use 'read_file' to add it to context.
3. DO NOT output code for files already in context unless you are changing them.
4. Use 'write_shadow_file' to propose changes.`

	messages := []openrouter.ChatMessage{
		{Role: "user", Content: task},
	}

	const maxTurns = 15

	for i := 0; i < maxTurns; i++ {
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

		e.Logger.Info(fmt.Sprintf("Agent thinking (Turn %d)...", i+1))

		resp, err := e.Client.CreateChatCompletion(ctx, req)
		if err != nil {
			return "", fmt.Errorf("LLM error: %w", err)
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
				// ERROR HANDLING FIX: Check for invalid JSON from LLM
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
				// ERROR HANDLING FIX
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
				// ERROR HANDLING FIX
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

				output, err := e.Sentinel.Execute(binary, cmdArgs)
				if err != nil {
					// We return the partial output and the error so the agent knows what happened
					resultStr = fmt.Sprintf("Command failed: %v\nOutput so far:\n%s", err, output)
				} else {
					resultStr = output
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
	return "", fmt.Errorf("agent exceeded max turns")
}

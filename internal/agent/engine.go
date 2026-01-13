package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/shadow"
	"github.com/david22573/codepicker/pkg/openrouter"
)

var toolErr error

type Engine struct {
	Client   *openrouter.Client
	Model    string
	Sentinel *Sentinel
	Shadow   *shadow.Manager
	Logger   logger.Logger
	SrcRoot  string

	// Callback for when the agent needs user permission
	// Returns true if approved, false if denied
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
		Logger:           log,
		SrcRoot:          srcRoot,
		ApprovalCallback: func(c, r string) bool { return false }, // Default deny
	}, nil
}

// Run starts the agent loop on a specific task
func (e *Engine) Run(ctx context.Context, task string, updateHistory func(openrouter.ChatMessage)) (string, error) {
	messages := []openrouter.ChatMessage{
		{
			Role: "system",
			Content: `You are an autonomous AI developer agent. 
			You are operating in a Termux environment.
			
			RULES:
			1. Always explore the codebase first using 'run_shell' (ls, grep) or 'read_file'.
			2. DO NOT hallucinate file paths. Verify them.
			3. To edit code, use 'write_shadow_file'. This writes to a safe sandbox.
			4. When you are done, output the final answer or summary of changes.`,
		},
		{Role: "user", Content: task},
	}

	// Max turns to prevent infinite loops
	const maxTurns = 10

	for i := 0; i < maxTurns; i++ {
		// Check cancellation
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		req := openrouter.ChatCompletionRequest{
			Model:    e.Model,
			Messages: messages,
			Tools:    Tools,
		}

		e.Logger.Info(fmt.Sprintf("🤖 Agent thinking (Turn %d)...", i+1))

		resp, err := e.Client.CreateChatCompletion(ctx, req)
		if err != nil {
			return "", fmt.Errorf("LLM error: %w", err)
		}

		msg := resp.Choices[0].Message
		messages = append(messages, *msg)
		if updateHistory != nil {
			updateHistory(*msg) // Send thought back to UI if needed
		}

		// If no tool calls, the agent is done (or asking a question)
		if len(msg.ToolCalls) == 0 {
			return fmt.Sprintf("%v", msg.Content), nil
		}

		// Handle Tool Calls
		for _, tool := range msg.ToolCalls {
			e.Logger.Info(fmt.Sprintf("🛠️  Tool Call: %s", tool.Function.Name))

			resultStr := ""

			switch tool.Function.Name {
			case "read_file":
				var args struct {
					Path string `json:"path"`
				}
				json.Unmarshal([]byte(tool.Function.Arguments), &args)
				// Read from REAL source for context
				fullPath := filepath.Join(e.SrcRoot, args.Path)
				content, err := os.ReadFile(fullPath)
				if err != nil {
					resultStr = fmt.Sprintf("Error reading file: %v", err)
				} else {
					resultStr = string(content)
				}

			case "write_shadow_file":
				var args struct {
					Path    string `json:"path"`
					Content string `json:"content"`
				}
				json.Unmarshal([]byte(tool.Function.Arguments), &args)
				shadowPath, err := e.Shadow.WriteFile(args.Path, []byte(args.Content))
				if err != nil {
					toolErr = err
					resultStr = fmt.Sprintf("Error writing shadow file: %v", err)
				} else {
					resultStr = fmt.Sprintf("Successfully wrote to shadow file: %s", shadowPath)
				}

			case "run_shell":
				var args struct {
					Command string `json:"command"`
				}
				json.Unmarshal([]byte(tool.Function.Arguments), &args)

				needsApproval, reason := e.Sentinel.CheckCommand(args.Command)

				allowed := true
				if needsApproval {
					e.Logger.Warn(fmt.Sprintf("🔒 Command requires approval: %s (%s)", args.Command, reason))
					// Trigger the callback (This effectively pauses execution waiting for user)
					allowed = e.ApprovalCallback(args.Command, reason)
				}

				if allowed {
					out, err := e.Sentinel.Execute(args.Command)
					if err != nil {
						resultStr = fmt.Sprintf("Command failed: %v\nOutput: %s", err, out)
					} else {
						resultStr = fmt.Sprintf("Output:\n%s", out)
					}
				} else {
					resultStr = "Permission denied by user."
				}
			}

			// Feed result back to LLM
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

	return "", fmt.Errorf("agent exceeded max turns (%d)", maxTurns)
}

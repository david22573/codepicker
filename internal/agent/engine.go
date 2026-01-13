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
	Client   *openrouter.Client
	Model    string
	Sentinel *Sentinel
	Shadow   *shadow.Manager
	Memory   *WorkingMemory // <--- NEW
	Logger   logger.Logger
	SrcRoot  string

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
		Memory:           NewMemory(srcRoot), // <--- Init Memory
		Logger:           log,
		SrcRoot:          srcRoot,
		ApprovalCallback: func(c, r string) bool { return false },
	}, nil
}

func (e *Engine) Run(ctx context.Context, task string, updateHistory func(openrouter.ChatMessage)) (string, error) {
	// Base System Prompt
	baseSystemPrompt := `You are an autonomous AI developer agent. 
	RULES:
	1. Code context is provided in "ACTIVE SOURCE FILES". 
	2. If you need to see a file not listed there, use 'read_file' to add it to context.
	3. DO NOT output code for files already in context unless you are changing them.
	4. Use 'write_shadow_file' to propose changes.`

	// Initial History
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

		// <--- DYNAMIC CONTEXT INJECTION
		// We reconstruct the System Prompt every turn to include the latest Memory state.
		currentContext := e.Memory.FormatContext()
		fullSystemMsg := baseSystemPrompt + "\n" + currentContext

		// Construct the transient request messages (System + History)
		requestMessages := append([]openrouter.ChatMessage{{Role: "system", Content: fullSystemMsg}}, messages...)

		req := openrouter.ChatCompletionRequest{
			Model:    e.Model,
			Messages: requestMessages,
			Tools:    Tools,
		}

		e.Logger.Info(fmt.Sprintf("🤖 Agent thinking (Turn %d)...", i+1))

		resp, err := e.Client.CreateChatCompletion(ctx, req)
		if err != nil {
			return "", fmt.Errorf("LLM error: %w", err)
		}

		msg := resp.Choices[0].Message

		// Add thought to history (but NOT the massive system prompt)
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
				json.Unmarshal([]byte(tool.Function.Arguments), &args)

				// <--- UPDATED LOGIC: Add to Memory
				err := e.Memory.Add(args.Path)
				if err != nil {
					resultStr = fmt.Sprintf("Error reading file: %v", err)
				} else {
					resultStr = fmt.Sprintf("File '%s' added to Active Context.", args.Path)
				}

			// ... (write_shadow_file and run_shell logic remains the same as previous) ...
			case "run_shell":
				// ... (Same Sentinel logic) ...
				// For brevity, assuming previous implementation
				resultStr = "Shell command executed (simulated for brevity)"
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

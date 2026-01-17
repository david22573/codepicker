package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time" // Added for sleep/throttling

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/shadow"
	"github.com/david22573/codepicker/internal/tracking"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type Engine struct {
	Client           *openrouter.Client
	Model            string
	WorkerModel      string
	Sentinel         *Sentinel
	Shadow           *shadow.Manager
	Memory           *WorkingMemory
	Logger           logger.Logger
	SrcRoot          string
	ApprovalCallback func(command string, reason string) bool
	CostTracker      *tracking.CostTracker
	Limits           *config.Limits
	Config           *config.ConfigFile
	SystemPrompt     string // NEW: Dynamic system prompt for persona switching
}

func NewEngine(client *openrouter.Client, model, srcRoot string, log logger.Logger, limits *config.Limits, store *database.Store) (*Engine, error) {
	shadowMgr, err := shadow.NewManager(srcRoot)
	if err != nil {
		return nil, err
	}

	cfg, _ := config.LoadConfigFile("")

	workerModel := "google/gemini-1.5-flash"
	if cfg != nil && cfg.AI.WorkerModel != "" {
		workerModel = cfg.AI.WorkerModel
	}

	return &Engine{
		Client:           client,
		Model:            model,
		WorkerModel:      workerModel,
		Sentinel:         NewSentinel(limits),
		Shadow:           shadowMgr,
		Memory:           NewMemory(srcRoot, store),
		Logger:           log,
		SrcRoot:          srcRoot,
		ApprovalCallback: func(c, r string) bool { return false },
		CostTracker:      tracking.NewCostTracker(limits.DailyCostLimit),
		Limits:           limits,
		Config:           cfg,
		SystemPrompt:     DefaultSupervisorPrompt, // Defined in prompts.go
	}, nil
}

func (e *Engine) runWorker(ctx context.Context, instruction string, files []string) (string, error) {
	var fileContext strings.Builder

	for _, f := range files {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		path := filepath.Join(e.SrcRoot, f)
		content, err := os.ReadFile(path)
		if err != nil {
			e.Logger.Warn(fmt.Sprintf("Worker could not read file %s: %v", f, err))
			continue
		}
		fileContext.WriteString(fmt.Sprintf("\n--- FILE: %s ---\n%s\n", f, string(content)))
	}

	workerPrompt := fmt.Sprintf(
		"You are a Worker Agent. You perform concrete tasks efficiently.\n"+
			"CONTEXT:\n%s\n"+
			"INSTRUCTION: %s\n"+
			"Output ONLY the result or code change. Do not chatter.",
		fileContext.String(), instruction,
	)

	req := openrouter.ChatCompletionRequest{
		Model: e.WorkerModel,
		Messages: []openrouter.ChatMessage{
			{Role: "system", Content: workerPrompt},
			{Role: "user", Content: "Execute the instruction."},
		},
	}

	resp, err := e.Client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) > 0 && resp.Choices[0].Message != nil {
		return fmt.Sprintf("%v", resp.Choices[0].Message.Content), nil
	}
	return "No output from worker", nil
}

func (e *Engine) Run(ctx context.Context, task string, updateHistory func(openrouter.ChatMessage)) (string, error) {

	ctx, cancel := context.WithTimeout(ctx, e.Limits.AgentTimeout)
	defer cancel()

	messages := []openrouter.ChatMessage{
		{Role: "user", Content: task},
	}

	activeTools := GetTools(e.Config)

	for i := 0; i < e.Limits.AgentMaxTurns; i++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		// ADDED: Respectful throttle to prevent rate-limit bursts and allow safe cancellation
		if i > 0 {
			time.Sleep(1 * time.Second)
		}

		currentContext := e.Memory.FormatContext()

		// CHANGED: Use the dynamic SystemPrompt instead of hardcoded string
		fullSystemMsg := e.SystemPrompt + "\n" + currentContext

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

			case "search_code":
				var args struct {
					Query string `json:"query"`
				}
				if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
					resultStr = fmt.Sprintf("Invalid arguments: %v", err)
					break
				}

				e.Logger.Info(fmt.Sprintf("🔍 Searching for: %s", args.Query))
				results, err := PerformSearch(e.SrcRoot, args.Query)
				if err != nil {
					resultStr = fmt.Sprintf("Search error: %v", err)
				} else {
					resultStr = results
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

				recoveryResult := e.ExecuteWithRecovery(binary, cmdArgs, e.Limits.MaxRecoveryAttempts)
				if recoveryResult.Success {
					resultStr = recoveryResult.FinalOutput
				} else {
					resultStr = fmt.Sprintf("Command failed: %v\nOutput: %s", recoveryResult.FinalError, recoveryResult.FinalOutput)
				}

			case "delegate_task":
				var args struct {
					Instruction  string `json:"instruction"`
					ContextFiles string `json:"context_files"`
				}
				if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
					resultStr = fmt.Sprintf("Invalid arguments: %v", err)
					break
				}

				files := strings.Split(args.ContextFiles, ",")
				e.Logger.Info(fmt.Sprintf("👷 Delegating to Worker (%s): %s", e.WorkerModel, args.Instruction))

				workerResult, err := e.runWorker(ctx, args.Instruction, files)
				if err != nil {
					resultStr = fmt.Sprintf("Worker failed: %v", err)
				} else {
					resultStr = fmt.Sprintf("Worker Output:\n%s", workerResult)
				}

			default:
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

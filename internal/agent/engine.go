package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/internal/shadow"
	"github.com/david22573/codepicker/internal/tracking"
	"github.com/david22573/codepicker/internal/vfs"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type Engine struct {
	Client *openrouter.Client
	Model  string

	// Sub-components
	Enforcer *PolicyEnforcer
	Worker   *WorkerRunner
	Executor *ToolExecutor
	Sentinel *Sentinel

	// State & Infra
	Memory      *WorkingMemory
	Shadow      *shadow.Manager
	FS          vfs.VirtualFileSystem
	CostTracker *tracking.CostTracker

	// Config
	Logger       logger.Logger
	Limits       *config.Limits
	Config       *config.ConfigFile
	SystemPrompt string
}

func NewEngine(
	client *openrouter.Client,
	model, srcRoot string,
	log logger.Logger,
	limits *config.Limits,
	store *database.Store,
	cfg *config.ConfigFile, // NEW: Explicit dependency
) (*Engine, error) {
	shadowMgr, err := shadow.NewManager(srcRoot)
	if err != nil {
		return nil, err
	}

	workerModel := "deepseek/deepseek-chat"
	if cfg != nil && cfg.AI.WorkerModel != "" {
		workerModel = cfg.AI.WorkerModel
	}

	// 1. Infrastructure
	fs := vfs.NewOverlayFS(srcRoot, shadowMgr)
	memory := NewMemory(store, fs)
	sentinel := NewSentinel(limits)
	costTracker := tracking.NewCostTracker(limits.DailyCostLimit)

	// 2. Sub-components
	worker := NewWorkerRunner(client, workerModel, fs, log)

	// Default Enforcer (Policy set later)
	enforcer := NewPolicyEnforcer(policy.Batch, log)

	executor := NewToolExecutor(memory, fs, sentinel, cfg)

	// Wire Enforcer into Executor
	executor.OnApproval = enforcer.Check

	return &Engine{
		Client: client,
		Model:  model,

		Enforcer: enforcer,
		Worker:   worker,
		Executor: executor,
		Sentinel: sentinel,

		Memory:      memory,
		Shadow:      shadowMgr,
		FS:          fs,
		CostTracker: costTracker,

		Logger:       log,
		Limits:       limits,
		Config:       cfg,
		SystemPrompt: DefaultSupervisorPrompt,
	}, nil
}

func (e *Engine) SetPolicy(p policy.ExecutionPolicy) {
	e.Logger.Debug(fmt.Sprintf("Engine policy set to: %s (Mode: %s)", p.Name, p.Mode))

	// Update the Enforcer
	e.Enforcer.Policy = p

	// If interactive, use CLI default.
	// (Server mode can override this by calling e.Enforcer.SetInteractionHandler directly)
	if p.Mode == policy.LevelInteractive {
		e.Enforcer.SetInteractionHandler(DefaultCLIInteraction)
	}
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

		if i > 0 {
			time.Sleep(1 * time.Second)
		}

		currentContext := e.Memory.FormatContext()
		fullSystemMsg := e.SystemPrompt + "\n" + currentContext

		requestMessages := append([]openrouter.ChatMessage{{Role: "system", Content: fullSystemMsg}}, messages...)

		req := openrouter.ChatCompletionRequest{
			Model:    e.Model,
			Messages: requestMessages,
			Tools:    activeTools,
		}

		e.Logger.Debug(fmt.Sprintf("Agent thinking (Turn %d/%d)...", i+1, e.Limits.AgentMaxTurns))

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

		// 1. Check if this is the final answer (no tool calls)
		if len(msg.ToolCalls) == 0 {
			return fmt.Sprintf("%v", msg.Content), nil
		}

		// 2. Log internal thoughts at Debug level (hidden from user)
		if msg.Content != nil {
			contentStr := fmt.Sprintf("%v", msg.Content)
			if contentStr != "" {
				e.Logger.Debug(fmt.Sprintf("💭 Thought: %s", contentStr))
			}
		}

		// 3. Process tools with selective logging
		for _, tool := range msg.ToolCalls {
			resultStr := ""

			// Selective Logging: Only INFO for major actions, DEBUG for read/search
			if tool.Function.Name == "delegate_task" {
				e.Logger.Info("👷 Delegating task to worker agent...")
			} else if tool.Function.Name == "write_shadow_file" {
				e.Logger.Info("📝 Writing code to shadow workspace...")
			} else {
				e.Logger.Debug(fmt.Sprintf("🔨 Executing Tool: %s", tool.Function.Name))
			}

			if tool.Function.Name == "delegate_task" {
				var args struct {
					Instruction  string `json:"instruction"`
					ContextFiles string `json:"context_files"`
				}
				if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
					resultStr = fmt.Sprintf("Invalid arguments: %v", err)
				} else {
					files := strings.Split(args.ContextFiles, ",")
					// Detailed debug log for troubleshooting
					e.Logger.Debug(fmt.Sprintf("👷 Delegation Details: %s", args.Instruction))

					// Use extracted WorkerRunner
					workerResult, err := e.Worker.Run(ctx, args.Instruction, files)

					if err != nil {
						resultStr = fmt.Sprintf("Worker failed: %v", err)
					} else {
						resultStr = fmt.Sprintf("Worker Output:\n%s", workerResult)
					}
				}
			} else {
				resultStr = e.Executor.Execute(tool)
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

package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/audit"
	domainContext "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/domain/errors"
	"github.com/david22573/codepicker/domain/validation"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/logging"
	"go.uber.org/zap"
)

// ReActAgent implements domain.agent.Agent using the ReAct pattern.
type ReActAgent struct {
	model       agent.LLMClient
	tools       map[string]agent.Tool
	policy      agent.Policy
	repo        agent.Repository
	logger      *logging.Logger
	costTracker *llm.CostTracker // Phase 3: Cost Tracking
	sysMsg      string
	maxTurn     int
}

func NewReActAgent(
	model agent.LLMClient,
	tools []agent.Tool,
	policy agent.Policy,
	repo agent.Repository,
	logger *logging.Logger,
	costTracker *llm.CostTracker, // Injected dependency
) *ReActAgent {
	toolMap := make(map[string]agent.Tool)
	var toolDescs strings.Builder

	for _, t := range tools {
		toolMap[t.Name()] = t
		toolDescs.WriteString(fmt.Sprintf("- %s: %s\n", t.Name(), t.Description()))
	}

	systemPrompt := fmt.Sprintf(`You are CodePicker, an autonomous coding agent.
You verify every step. You never hallucinate filenames.
You operate in a loop: THOUGHT -> ACTION -> OBSERVATION.

AVAILABLE TOOLS:
%s

FORMAT RULES:
1. Output "Thought:", then "Action:", then "Input:".
2. "Action" must be a single tool name (e.g. read_file).
3. "Input" must be valid JSON single-line.
4. STRICTLY FORBIDDEN: Do not use XML tags like <invoke> or <function_calls>.
5. Do NOT output Markdown code blocks for the whole response.
6. Wait for the [SYSTEM] Observation before proceeding.

EXAMPLE INTERACTION:
Thought: I need to read the main file to understand the entry point.
Action: read_file
Input: {"path": "main.go"}

Begin.`, toolDescs.String())

	return &ReActAgent{
		model:       model,
		tools:       toolMap,
		policy:      policy,
		repo:        repo,
		logger:      logger,
		costTracker: costTracker,
		sysMsg:      systemPrompt,
		maxTurn:     15,
	}
}

func (a *ReActAgent) Name() string {
	return "CodePicker-ReAct"
}

func (a *ReActAgent) Run(ctx context.Context, taskInput string) (string, error) {
	// Phase 1: Validation
	validator := validation.NewValidator()
	if err := validator.ValidateTask(taskInput); err != nil {
		return "", fmt.Errorf("invalid task: %w", err)
	}

	// Phase 3: Audit Trail Initialization
	execID := fmt.Sprintf("exec-%d", time.Now().Unix())
	auditTrail := audit.NewAuditTrail(execID)

	// Create a context-aware logger
	logger := a.logger.WithContext(ctx)

	execution := agent.NewExecution(execID, "adhoc-plan")
	logger.Info("Agent Run Started", zap.String("task", taskInput), zap.String("execution_id", execID))

	if err := a.repo.SaveExecution(ctx, execution); err != nil {
		return "", err
	}

	auditTrail.Record("start", map[string]interface{}{"task": taskInput})
	currentContext := fmt.Sprintf("TASK: %s\n", taskInput)

	for i := 0; i < a.maxTurn; i++ {
		logger.Debug("Starting Turn", zap.Int("turn", i+1))

		// Phase 3: Cost Tracking Integration
		// We use type assertion to check if the model supports usage tracking
		var response string
		var err error
		var usage domainContext.TokenUsage

		if usageClient, ok := a.model.(interface {
			ChatWithUsage(context.Context, string, string) (string, domainContext.TokenUsage, error)
		}); ok {
			response, usage, err = usageClient.ChatWithUsage(ctx, a.sysMsg, currentContext)
			if err == nil {
				cost := a.costTracker.RecordUsage(usage.PromptTokens, usage.CompletionTokens)
				logger.Debug("LLM Cost", zap.Float64("cost", cost))
			}
		} else {
			// Fallback for models without usage support
			response, err = a.model.Chat(ctx, a.sysMsg, currentContext)
		}

		if err != nil {
			return "", errors.NewLLM("agent.Run", err)
		}

		thought, toolName, toolArgs := parseReActResponse(response)

		auditTrail.Record("turn", map[string]interface{}{
			"turn_id": i + 1,
			"thought": thought,
			"tool":    toolName,
			"args":    toolArgs,
		})

		logger.Info("Agent Thought", zap.String("thought", thought))

		if toolName == "" {
			execution.Finish()
			_ = a.repo.SaveExecution(ctx, execution)

			// Save Audit Trail to disk
			auditTrail.Record("finish", map[string]interface{}{"result": response})
			_ = auditTrail.Save(filepath.Join(".codepicker", "audit", fmt.Sprintf("%s.json", execID)))

			return response, nil
		}

		// Phase 1: Security & Policy Check
		allowed, reason := a.policy.CanExecute(toolName, toolArgs)
		if !allowed {
			toolOut := fmt.Sprintf("Error: Policy Violation: %s", reason)
			logger.Warn("Guardrail Blocked Action", zap.String("tool", toolName), zap.String("reason", reason))

			currentContext += fmt.Sprintf("\nThought: %s\nAction: %s\nInput: %s\nObservation: %s\n", thought, toolName, toolArgs, toolOut)
			continue
		}

		// Tool Execution
		tool, exists := a.tools[toolName]
		var toolOut string
		startTime := time.Now()

		if !exists {
			toolOut = fmt.Sprintf("Error: Tool '%s' not found.", toolName)
		} else {
			toolOut, err = tool.Execute(ctx, toolArgs)
			duration := time.Since(startTime)
			logger.LogToolExecution(toolName, toolArgs, duration, err)

			if err != nil {
				toolOut = fmt.Sprintf("Error: %v", err)
			}
		}

		execution.RecordTurn(thought, toolName, toolArgs, toolOut)
		_ = a.repo.SaveExecution(ctx, execution)

		currentContext += fmt.Sprintf("\nThought: %s\nAction: %s\nInput: %s\nObservation: %s\n", thought, toolName, toolArgs, toolOut)
	}

	return "", errors.NewSystem("agent.Run", fmt.Sprintf("Max turns (%d) exceeded", a.maxTurn), nil)
}

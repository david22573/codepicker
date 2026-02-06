package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/errors"
	"github.com/david22573/codepicker/infra/logging"
	"go.uber.org/zap"
)

// ReActAgent implements domain.agent.Agent using the ReAct pattern
type ReActAgent struct {
	model   agent.LLMClient
	tools   map[string]agent.Tool
	policy  agent.Policy
	repo    agent.Repository
	logger  *logging.Logger
	sysMsg  string
	maxTurn int
}

func NewReActAgent(
	model agent.LLMClient,
	tools []agent.Tool,
	policy agent.Policy,
	repo agent.Repository,
	logger *logging.Logger,
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
		model:   model,
		tools:   toolMap,
		policy:  policy,
		repo:    repo,
		logger:  logger,
		sysMsg:  systemPrompt,
		maxTurn: 15,
	}
}

func (a *ReActAgent) Name() string {
	return "CodePicker-ReAct"
}

func (a *ReActAgent) Run(ctx context.Context, taskInput string) (string, error) {
	// Create a context-aware logger for this run
	logger := a.logger.WithContext(ctx)

	execID := fmt.Sprintf("exec-%d", time.Now().Unix())
	execution := agent.NewExecution(execID, "adhoc-plan")

	// Log the start of the run
	logger.Info("Agent Run Started",
		zap.String("task", taskInput),
		zap.String("execution_id", execID))

	if err := a.repo.SaveExecution(ctx, execution); err != nil {
		logger.Error("Failed to persist execution start", zap.Error(err))
		return "", err
	}

	currentContext := fmt.Sprintf("TASK: %s\n", taskInput)

	for i := 0; i < a.maxTurn; i++ {

		// Log the turn start
		logger.Debug("Starting Turn", zap.Int("turn", i+1))

		response, err := a.model.Chat(ctx, a.sysMsg, currentContext)
		if err != nil {
			logger.Error("LLM Chat Failed", zap.Error(err))
			return "", errors.NewLLM("agent.Run", err)
		}

		thought, toolName, toolArgs := parseReActResponse(response)

		// Log the Agent's reasoning
		logger.Info("Agent Thought", zap.String("thought", thought))

		if toolName == "" {
			logger.Info("Agent Finished", zap.String("response", response))
			execution.Finish()
			_ = a.repo.SaveExecution(ctx, execution)
			return response, nil
		}

		// Log the intent to act
		logger.Info("Tool Request",
			zap.String("tool", toolName),
			zap.String("args", toolArgs))

		// 1. Policy Check
		allowed, reason := a.policy.CanExecute(toolName, toolArgs)
		if !allowed {
			toolOut := fmt.Sprintf("Error: Policy Violation: %s", reason)

			// Critical Security Log
			logger.Warn("Guardrail Blocked Action",
				zap.String("tool", toolName),
				zap.String("reason", reason))

			currentContext += fmt.Sprintf("\nThought: %s\nAction: %s\nInput: %s\nObservation: %s\n", thought, toolName, toolArgs, toolOut)
			continue
		}

		// 2. Execution
		tool, exists := a.tools[toolName]
		var toolOut string
		startTime := time.Now()

		if !exists {
			toolOut = fmt.Sprintf("Error: Tool '%s' not found.", toolName)
			logger.Warn("Tool Not Found", zap.String("tool", toolName))
		} else {
			// Execute and measure
			toolOut, err = tool.Execute(ctx, toolArgs)
			duration := time.Since(startTime)

			// Log the result using the standardized helper
			logger.LogToolExecution(toolName, toolArgs, duration, err)

			if err != nil {
				toolOut = fmt.Sprintf("Error: %v", err)
			}
		}

		execution.RecordTurn(thought, toolName, toolArgs, toolOut)
		_ = a.repo.SaveExecution(ctx, execution)

		currentContext += fmt.Sprintf("\nThought: %s\nAction: %s\nInput: %s\nObservation: %s\n", thought, toolName, toolArgs, toolOut)
	}

	logger.Error("Max Turns Exceeded")
	return "", errors.NewSystem("agent.Run", fmt.Sprintf("Max turns (%d) exceeded without final answer", a.maxTurn), nil)
}

package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/errors"
)

// ReActAgent implements domain.agent.Agent using the ReAct pattern
type ReActAgent struct {
	model   agent.LLMClient
	tools   map[string]agent.Tool
	policy  agent.Policy
	repo    agent.Repository
	sysMsg  string
	maxTurn int
}

// NewReActAgent constructs the agent with all dependencies injected
func NewReActAgent(
	model agent.LLMClient,
	tools []agent.Tool,
	policy agent.Policy,
	repo agent.Repository,
) *ReActAgent {
	// Index tools for O(1) lookup
	toolMap := make(map[string]agent.Tool)
	var toolDescs strings.Builder

	for _, t := range tools {
		toolMap[t.Name()] = t
		toolDescs.WriteString(fmt.Sprintf("- %s: %s\n", t.Name(), t.Description()))
	}

	// The System Prompt defines the Agent's personality and protocols
	systemPrompt := fmt.Sprintf(`You are CodePicker, an autonomous coding agent.
You verify every step. You never hallucinate filenames.
You operate in a loop: THOUGHT -> ACTION -> OBSERVATION.

AVAILABLE TOOLS:
%s

FORMAT:
Thought: <your reasoning>
Action: <tool_name>
Input: <json_arguments>

Example:
Thought: I need to read the main file to understand the entrypoint.
Action: read_file
Input: {"path": "main.go"}

Begin.`, toolDescs.String())

	return &ReActAgent{
		model:   model,
		tools:   toolMap,
		policy:  policy,
		repo:    repo,
		sysMsg:  systemPrompt,
		maxTurn: 15, // Safety limit
	}
}

func (a *ReActAgent) Name() string {
	return "CodePicker-ReAct"
}

// Run executes the main ReAct loop
func (a *ReActAgent) Run(ctx context.Context, taskInput string) (string, error) {
	// 1. Setup Execution State
	execID := fmt.Sprintf("exec-%d", time.Now().Unix())
	execution := agent.NewExecution(execID, "adhoc-plan")

	if err := a.repo.SaveExecution(ctx, execution); err != nil {
		return "", err
	}

	// 2. Initialize Context
	currentContext := fmt.Sprintf("TASK: %s\n", taskInput)

	// 3. The ReAct Loop
	for i := 0; i < a.maxTurn; i++ {
		// A. Get LLM Response
		response, err := a.model.Chat(ctx, a.sysMsg, currentContext)
		if err != nil {
			return "", errors.NewLLM("agent.Run", err)
		}

		// B. Parse Response
		thought, toolName, toolArgs := parseReActResponse(response)

		// If no tool action is detected, we assume the agent is providing the final answer
		if toolName == "" {
			execution.Finish()
			_ = a.repo.SaveExecution(ctx, execution)
			return response, nil
		}

		// C. Policy Check (Security Firewall)
		allowed, reason := a.policy.CanExecute(toolName, toolArgs)
		if !allowed {
			toolOut := fmt.Sprintf("Error: Policy Violation: %s", reason)
			currentContext += fmt.Sprintf("\nThought: %s\nAction: %s\nInput: %s\nObservation: %s\n", thought, toolName, toolArgs, toolOut)
			continue
		}

		// D. Execute Tool
		tool, exists := a.tools[toolName]
		var toolOut string

		if !exists {
			toolOut = fmt.Sprintf("Error: Tool '%s' not found. Check available tools list.", toolName)
		} else {
			// Actually run the code
			toolOut, err = tool.Execute(ctx, toolArgs)
			if err != nil {
				toolOut = fmt.Sprintf("Error: %v", err)
			}
		}

		// E. Update History & State
		execution.RecordTurn(thought, toolName, toolArgs, toolOut)
		_ = a.repo.SaveExecution(ctx, execution)

		// Append Observation to context so LLM sees the result in the next turn
		currentContext += fmt.Sprintf("\nThought: %s\nAction: %s\nInput: %s\nObservation: %s\n", thought, toolName, toolArgs, toolOut)
	}

	return "", errors.NewSystem("agent.Run", fmt.Sprintf("Max turns (%d) exceeded without final answer", a.maxTurn), nil)
}

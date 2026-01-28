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

func NewReActAgent(
	model agent.LLMClient,
	tools []agent.Tool,
	policy agent.Policy,
	repo agent.Repository,
) *ReActAgent {
	toolMap := make(map[string]agent.Tool)
	var toolDescs strings.Builder

	for _, t := range tools {
		toolMap[t.Name()] = t
		toolDescs.WriteString(fmt.Sprintf("- %s: %s\n", t.Name(), t.Description()))
	}

	// --- UNIVERSAL PROMPT ENGINEERING ---
	// 1. Persona & Safety
	// 2. Tools
	// 3. Format Guidelines
	// 4. ONE-SHOT EXAMPLE (Critical for weak models)
	systemPrompt := fmt.Sprintf(`You are CodePicker, an autonomous coding agent.
You verify every step. You never hallucinate filenames.
You operate in a loop: THOUGHT -> ACTION -> OBSERVATION.

AVAILABLE TOOLS:
%s

FORMAT RULES:
1. Output "Thought:", then "Action:", then "Input:".
2. "Action" must be a single tool name.
3. "Input" must be valid JSON.
4. Do NOT output Markdown code blocks for the whole response.
5. Wait for the [SYSTEM] Observation before proceeding.

EXAMPLE INTERACTION (Follow this format):
Thought: I need to read the main file to understand the entry point.
Action: read_file
Input: {"path": "main.go"}

Begin.`, toolDescs.String())

	return &ReActAgent{
		model:   model,
		tools:   toolMap,
		policy:  policy,
		repo:    repo,
		sysMsg:  systemPrompt,
		maxTurn: 15,
	}
}

func (a *ReActAgent) Name() string {
	return "CodePicker-ReAct"
}

func (a *ReActAgent) Run(ctx context.Context, taskInput string) (string, error) {
	execID := fmt.Sprintf("exec-%d", time.Now().Unix())
	execution := agent.NewExecution(execID, "adhoc-plan")

	if err := a.repo.SaveExecution(ctx, execution); err != nil {
		return "", err
	}

	currentContext := fmt.Sprintf("TASK: %s\n", taskInput)

	for i := 0; i < a.maxTurn; i++ {
		// A. Get LLM Response
		response, err := a.model.Chat(ctx, a.sysMsg, currentContext)
		if err != nil {
			return "", errors.NewLLM("agent.Run", err)
		}

		// B. Parse Response
		thought, toolName, toolArgs := parseReActResponse(response)

		// --- UX HARDENING: Distinct Agent Output ---
		fmt.Printf("\n🤖 [AGENT] Thought: %s\n", thought)
		// -------------------------------------------

		if toolName == "" {
			fmt.Printf("🏁 [AGENT] Final Answer: %s\n", response)
			execution.Finish()
			_ = a.repo.SaveExecution(ctx, execution)
			return response, nil
		}

		// --- UX HARDENING: Request Visualization ---
		fmt.Printf("⚡ [AGENT] Request: %s %s\n", toolName, toolArgs)
		// -------------------------------------------

		// C. Policy Check
		allowed, reason := a.policy.CanExecute(toolName, toolArgs)
		if !allowed {
			toolOut := fmt.Sprintf("Error: Policy Violation: %s", reason)

			// --- UX HARDENING: Security Alert ---
			fmt.Printf("🛡️  [GUARDRAIL] BLOCKED: %s\n", reason)
			// ------------------------------------

			currentContext += fmt.Sprintf("\nThought: %s\nAction: %s\nInput: %s\nObservation: %s\n", thought, toolName, toolArgs, toolOut)
			continue
		}

		// D. Execute Tool
		tool, exists := a.tools[toolName]
		var toolOut string

		if !exists {
			toolOut = fmt.Sprintf("Error: Tool '%s' not found.", toolName)
		} else {
			// --- UX HARDENING: System Action Label ---
			fmt.Printf("⚙️  [SYSTEM] Executing %s...\n", toolName)
			// -----------------------------------------

			toolOut, err = tool.Execute(ctx, toolArgs)
			if err != nil {
				toolOut = fmt.Sprintf("Error: %v", err)
				fmt.Printf("❌ [SYSTEM] Error: %v\n", err)
			} else {
				fmt.Printf("✅ [SYSTEM] OK.\n")
			}
		}

		// E. Update History
		execution.RecordTurn(thought, toolName, toolArgs, toolOut)
		_ = a.repo.SaveExecution(ctx, execution)

		currentContext += fmt.Sprintf("\nThought: %s\nAction: %s\nInput: %s\nObservation: %s\n", thought, toolName, toolArgs, toolOut)
	}

	return "", errors.NewSystem("agent.Run", fmt.Sprintf("Max turns (%d) exceeded without final answer", a.maxTurn), nil)
}

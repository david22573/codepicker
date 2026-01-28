package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/domain/agent"
)

type Explainer struct {
	model agent.LLMClient
	repo  agent.Repository
}

func NewExplainer(model agent.LLMClient, repo agent.Repository) *Explainer {
	return &Explainer{
		model: model,
		repo:  repo,
	}
}

// Explain analyzes a specific execution ID and returns a natural language summary
func (e *Explainer) Explain(ctx context.Context, executionID string) (string, error) {
	// 1. Fetch the raw history
	exec, err := e.repo.GetExecution(ctx, executionID)
	if err != nil {
		return "", fmt.Errorf("failed to load execution: %w", err)
	}

	// 2. Format the history into a readable trace for the LLM
	var trace strings.Builder
	trace.WriteString(fmt.Sprintf("Execution ID: %s\n", exec.ID))
	trace.WriteString(fmt.Sprintf("Status: %s\n\n", exec.Status))

	for _, turn := range exec.History {
		trace.WriteString(fmt.Sprintf("TURN %d:\n", turn.TurnID))
		trace.WriteString(fmt.Sprintf("Thought: %s\n", turn.Thought))
		trace.WriteString(fmt.Sprintf("Action: %s(%s)\n", turn.ToolName, turn.ToolArgs))
		// Truncate output to save tokens, we care about decision flow
		out := turn.ToolOut
		if len(out) > 200 {
			out = out[:200] + "...(truncated)"
		}
		trace.WriteString(fmt.Sprintf("Result: %s\n\n", out))
	}

	// 3. Prompt the LLM for analysis
	systemPrompt := `You are an AI Explainability Specialist. 
Your goal is to analyze the execution trace of an autonomous coding agent.
Explain the agent's strategy, identify any errors in reasoning, and summarize the outcome.
Be concise and objective.`

	userPrompt := fmt.Sprintf("Analyze this execution trace:\n\n%s", trace.String())

	fmt.Println("🤔 Analyzing execution trace...")
	return e.model.Chat(ctx, systemPrompt, userPrompt)
}

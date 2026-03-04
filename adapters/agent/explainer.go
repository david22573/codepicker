package agent

import (
	"context"
	"fmt"
	"strings"

	domainAgent "github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/prompts"
	"github.com/david22573/codepicker/runtime"
)

type Explainer struct {
	model       domainAgent.LLMClient
	repo        domainAgent.Repository
	budgetGuard *llm.BudgetGuard
}

func NewExplainer(model domainAgent.LLMClient, repo domainAgent.Repository, costTracker *llm.CostTracker, budget float64) *Explainer {
	return &Explainer{
		model:       model,
		repo:        repo,
		budgetGuard: llm.NewBudgetGuard(costTracker, budget),
	}
}

// Explain analyzes a specific execution ID and returns a natural language summary
func (e *Explainer) Explain(ctx context.Context, executionID string) (string, error) {
	exec, err := e.repo.GetExecution(ctx, executionID)
	if err != nil {
		return "", fmt.Errorf("failed to load execution: %w", err)
	}

	var trace strings.Builder
	trace.WriteString(fmt.Sprintf("Execution ID: %s\n", exec.ID))
	trace.WriteString(fmt.Sprintf("Status: %s\n\n", exec.Status))

	for _, turn := range exec.History {
		trace.WriteString(fmt.Sprintf("TURN %d:\n", turn.TurnID))
		trace.WriteString(fmt.Sprintf("Thought: %s\n", turn.Thought))
		trace.WriteString(fmt.Sprintf("Action: %s(%s)\n", turn.ToolName, turn.ToolArgs))
		out := turn.ToolOut
		if len(out) > 200 {
			out = out[:200] + "...(truncated)"
		}
		trace.WriteString(fmt.Sprintf("Result: %s\n\n", out))
	}

	systemPrompt, err := prompts.Render("explainer_system", nil)
	if err != nil {
		return "", err
	}

	userPrompt := fmt.Sprintf("<execution_trace>\n%s\n</execution_trace>", trace.String())

	estCost := runtime.Global.ExplainerEstCost
	if err := e.budgetGuard.Reserve(estCost); err != nil {
		return "", fmt.Errorf("explanation halted by budget: %w", err)
	}
	defer e.budgetGuard.Commit(estCost)

	fmt.Println("🤔 Analyzing execution trace...")
	return e.model.Chat(ctx, systemPrompt, userPrompt)
}

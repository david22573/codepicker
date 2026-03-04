package context

import (
	"context"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/domain/agent"
	domainCtx "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/prompts"
	"github.com/david22573/codepicker/runtime"
)

type Reranker struct {
	model       llm.StructuredLLM
	budgetGuard *llm.BudgetGuard
}

func NewReranker(client agent.LLMClient, costTracker *llm.CostTracker, budget float64) *Reranker {
	return &Reranker{
		model:       llm.NewStructuredAdapter(client),
		budgetGuard: llm.NewBudgetGuard(costTracker, budget),
	}
}

// RankResponse defines the LLM output structure
type RankResponse struct {
	RankedIDs []string `json:"ranked_ids"`
}

// Rank filters and orders slices based on their relevance to the task.
func (r *Reranker) Rank(ctx context.Context, task string, candidates []domainCtx.CodeSlice) ([]domainCtx.CodeSlice, error) {
	if len(candidates) <= 5 {
		return candidates, nil // Don't bother reranking small sets
	}

	// 1. Prepare Prompt
	var sb strings.Builder

	sliceMap := make(map[string]domainCtx.CodeSlice)
	for _, s := range candidates {
		preview := s.Content
		lines := strings.Split(preview, "\n")
		if len(lines) > 3 {
			preview = strings.Join(lines[:3], "\n") + "\n..."
		}

		sb.WriteString(fmt.Sprintf("<snippet id=\"%s\" file=\"%s\">\n%s\n</snippet>\n\n", s.ID, s.FilePath, preview))
		sliceMap[s.ID] = s
	}

	system, err := prompts.Render("reranker_system", nil)
	if err != nil {
		return candidates, nil
	}

	user := fmt.Sprintf("<task>\n%s\n</task>\n\n<candidates>\n%s</candidates>", task, sb.String())

	// --- BUDGET PROTECTION ---
	estCost := runtime.Global.RerankerEstCost
	if err := r.budgetGuard.Reserve(estCost); err != nil {
		return candidates, nil
	}
	defer r.budgetGuard.Commit(estCost)
	// -------------------------

	// 2. Call LLM
	var resp RankResponse
	err = r.model.ChatJSON(ctx, system, user, &resp)
	if err != nil {
		return candidates, nil
	}

	// 3. Reconstruct List
	var ranked []domainCtx.CodeSlice
	seen := make(map[string]bool)

	for _, id := range resp.RankedIDs {
		if s, ok := sliceMap[id]; ok {
			ranked = append(ranked, s)
			seen[id] = true
		}
	}

	// Append any missing candidates at the end (fallback)
	for _, s := range candidates {
		if !seen[s.ID] {
			ranked = append(ranked, s)
		}
	}

	return ranked, nil
}

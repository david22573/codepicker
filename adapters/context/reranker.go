package context

import (
	"context"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/domain/agent"
	domainCtx "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/infra/llm"
)

type Reranker struct {
	model llm.StructuredLLM
}

func NewReranker(client agent.LLMClient) *Reranker {
	return &Reranker{
		model: llm.NewStructuredAdapter(client),
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
	sb.WriteString("CANDIDATES:\n")

	sliceMap := make(map[string]domainCtx.CodeSlice)
	for _, s := range candidates {
		// Provide minimal context to save tokens during ranking
		// Only Header + First 3 lines
		preview := s.Content
		lines := strings.Split(preview, "\n")
		if len(lines) > 3 {
			preview = strings.Join(lines[:3], "\n") + "\n..."
		}

		sb.WriteString(fmt.Sprintf("ID: %s | File: %s | Code: %s\n\n", s.ID, s.FilePath, preview))
		sliceMap[s.ID] = s
	}

	system := `You are a Senior Tech Lead.
Rank the provided code snippets by their relevance to the user's TASK.
Return a JSON object with a list of IDs in descending order of importance.
Example: {"ranked_ids": ["main.go-Func-10", "utils.go-Struct-5"]}`

	user := fmt.Sprintf("TASK: %s\n\n%s", task, sb.String())

	// 2. Call LLM
	var resp RankResponse
	err := r.model.ChatJSON(ctx, system, user, &resp)
	if err != nil {
		// Fallback: Return original order if LLM fails
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

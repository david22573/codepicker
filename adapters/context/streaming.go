package context

import (
	"context"
	"fmt"
	"strings"

	domainCtx "github.com/david22573/codepicker/domain/context"
)

// StreamingBuilder handles context construction for large codebases.
// It prioritizes fitting the most relevant slices into a strict token budget.
type StreamingBuilder struct {
	store     domainCtx.SliceStore
	maxTokens int
}

func NewStreamingBuilder(store domainCtx.SliceStore, maxTokens int) *StreamingBuilder {
	return &StreamingBuilder{
		store:     store,
		maxTokens: maxTokens,
	}
}

// BuildContextWithBudget generates a prompt-ready context string that fits within specific constraints.
func (b *StreamingBuilder) BuildContextWithBudget(ctx context.Context, query string, budget int) (string, error) {
	// 1. Fetch more candidates than we need (oversampling) to allow for prioritization
	// We ask for up to 50 slices initially.
	candidates, err := b.store.SearchSlices(ctx, query, 50)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}

	if len(candidates) == 0 {
		return "No relevant code context found.", nil
	}

	// 2. Packing Strategy
	var sb strings.Builder
	sb.WriteString("### RELEVANT CODE CONTEXT\n")

	usedTokens := 0
	includedCount := 0

	for _, slice := range candidates {
		// Estimate tokens: roughly 4 chars per token + formatting overhead
		// Header overhead: "--- File: ... (Lines ..) ---\n\n\n" is approx 10-15 tokens
		contentTokens := len(slice.Content) / 4
		headerTokens := 15
		sliceCost := contentTokens + headerTokens

		if usedTokens+sliceCost > budget {
			continue // Skip this slice, try next (or break if strict order matters)
		}

		// 3. Format the slice
		sb.WriteString(fmt.Sprintf("--- File: %s (Lines %d-%d) [%s] ---\n",
			slice.FilePath, slice.StartLine, slice.EndLine, slice.SliceType))
		sb.WriteString(slice.Content)
		sb.WriteString("\n\n")

		usedTokens += sliceCost
		includedCount++
	}

	if includedCount < len(candidates) {
		sb.WriteString(fmt.Sprintf("... (Truncated %d less relevant snippets to fit context window)\n", len(candidates)-includedCount))
	}

	return sb.String(), nil
}

// EstimateTokens provides a quick utility for other components
func EstimateTokens(text string) int {
	return len(text) / 4
}

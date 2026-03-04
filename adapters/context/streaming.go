package context

import (
	"context"
	"fmt"
	"strings"

	domainCtx "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/infra/llm"
)

// StreamingBuilder handles context construction for large codebases.
// It prioritizes fitting the most relevant slices into a strict token budget.
type StreamingBuilder struct {
	store     domainCtx.SliceStore
	maxTokens int
	estimator llm.TokenEstimator
}

func NewStreamingBuilder(store domainCtx.SliceStore, maxTokens int) *StreamingBuilder {
	return &StreamingBuilder{
		store:     store,
		maxTokens: maxTokens,
		estimator: llm.NewDefaultEstimator(),
	}
}

// BuildContextWithBudget generates a prompt-ready context string that fits within specific constraints.
func (b *StreamingBuilder) BuildContextWithBudget(ctx context.Context, query string, budget int) (string, error) {
	// 1. Fetch more candidates than we need (oversampling) to allow for prioritization
	candidates, err := b.store.SearchSlices(ctx, query, 50)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}

	if len(candidates) == 0 {
		return "<relevant_code_context>\n  \n</relevant_code_context>", nil
	}

	// 2. Packing Strategy with XML Formatting
	var sb strings.Builder
	sb.WriteString("<relevant_code_context>\n")

	usedTokens := 0
	includedCount := 0

	for _, slice := range candidates {
		contentTokens := b.estimator.EstimateText(slice.Content)
		headerTokens := 30 // Increased for XML overhead
		sliceCost := contentTokens + headerTokens

		if usedTokens+sliceCost > budget {
			continue
		}

		// 3. Format the slice
		sb.WriteString(fmt.Sprintf("  <file path=\"%s\">\n", slice.FilePath))
		sb.WriteString(fmt.Sprintf("    <slice start_line=\"%d\" end_line=\"%d\" type=\"%s\">\n", slice.StartLine, slice.EndLine, slice.SliceType))
		sb.WriteString(slice.Content)
		if !strings.HasSuffix(slice.Content, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("    </slice>\n  </file>\n\n")

		usedTokens += sliceCost
		includedCount++
	}

	if includedCount < len(candidates) {
		sb.WriteString(fmt.Sprintf("  \n", len(candidates)-includedCount))
	}

	sb.WriteString("</relevant_code_context>\n")

	return sb.String(), nil
}

// EstimateTokens provides a quick utility for other components.
// Maintained for backward compatibility but routes to the centralized estimator.
func EstimateTokens(text string) int {
	estimator := llm.NewDefaultEstimator()
	return estimator.EstimateText(text)
}

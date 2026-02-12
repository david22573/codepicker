package context

import (
	"context"
	"fmt"
	"strings"

	domainCtx "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/domain/errors"
)

// SliceBasedBuilder implements the streaming context logic for large repositories.
// UPDATED: Now uses token budgeting instead of naive loading.
type SliceBasedBuilder struct {
	repo      domainCtx.SliceStore
	maxTokens int
}

// NewSliceBasedBuilder creates a builder that respects a specific token budget.
func NewSliceBasedBuilder(repo domainCtx.SliceStore, maxTokens int) *SliceBasedBuilder {
	return &SliceBasedBuilder{
		repo:      repo,
		maxTokens: maxTokens,
	}
}

// BuildForTask is a helper used by the CLI (cmd/run.go, cmd/context.go) to prepare context.
func (b *SliceBasedBuilder) BuildForTask(query string) (string, error) {
	return b.BuildContext(context.Background(), query)
}

// BuildContext pulls relevant code slices and formats them for the LLM turn.
func (b *SliceBasedBuilder) BuildContext(ctx context.Context, query string) (string, error) {
	// 1. Fetch more candidates than we need (oversampling) to allow for prioritization
	// We ask for up to 50 slices initially to filter down.
	candidates, err := b.repo.SearchSlices(ctx, query, 50)
	if err != nil {
		return "", errors.NewSystem("context.builder", "failed to search code slices", err)
	}

	if len(candidates) == 0 {
		return "No relevant code context found in the index for the given query.", nil
	}

	var builder strings.Builder
	builder.WriteString("### RELEVANT CODE CONTEXT\n")
	builder.WriteString("The following snippets are relevant to your current task:\n\n")

	usedTokens := 0
	includedCount := 0

	// 2. Packing Strategy: Strict Token Budgeting
	for _, slice := range candidates {
		// Estimate tokens: roughly 4 chars per token + formatting overhead
		// Header overhead: "--- File: ... (Lines ..) ---\n\n\n" is approx 15 tokens
		contentTokens := len(slice.Content) / 4
		headerTokens := 15
		sliceCost := contentTokens + headerTokens

		// Stop if this slice would exceed our budget
		if usedTokens+sliceCost > b.maxTokens {
			continue
		}

		builder.WriteString(fmt.Sprintf("--- File: %s (Lines %d-%d) [%s] ---\n",
			slice.FilePath, slice.StartLine, slice.EndLine, slice.SliceType))
		builder.WriteString(slice.Content)
		builder.WriteString("\n\n")

		usedTokens += sliceCost
		includedCount++
	}

	// 3. Add footer if we had to drop content
	if includedCount < len(candidates) {
		builder.WriteString(fmt.Sprintf("\n... (Truncated %d less relevant snippets to fit context window)\n", len(candidates)-includedCount))
	}

	return builder.String(), nil
}

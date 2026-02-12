package context

import (
	"context"
	"fmt"
	"strings"

	domainContext "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/domain/errors"
)

// SliceBasedBuilder implements the streaming context logic for large repositories.
type SliceBasedBuilder struct {
	repo      domainContext.SliceStore
	maxTokens int
}

// NewSliceBasedBuilder creates a builder that respects a specific token budget.
func NewSliceBasedBuilder(repo domainContext.SliceStore, maxTokens int) *SliceBasedBuilder {
	return &SliceBasedBuilder{
		repo:      repo,
		maxTokens: maxTokens,
	}
}

// BuildForTask is a helper used by the CLI (cmd/run.go, cmd/context.go) to prepare context.
// It wraps BuildContext with a background context for ease of use.
func (b *SliceBasedBuilder) BuildForTask(query string) (string, error) {
	return b.BuildContext(context.Background(), query)
}

// BuildContext pulls relevant code slices and formats them for the LLM turn.
func (b *SliceBasedBuilder) BuildContext(ctx context.Context, query string) (string, error) {
	// FIX: SearchSlices returns (slices, error), so we must capture both.
	// If you previously had `slices := ...`, that caused the "multiple-value in single-value context" error.
	slices, err := b.repo.SearchSlices(ctx, query, 20)
	if err != nil {
		return "", errors.NewSystem("context.builder", "failed to search code slices", err)
	}

	var builder strings.Builder
	builder.WriteString("### RELEVANT CODE CONTEXT\n")
	builder.WriteString("The following snippets are relevant to your current task:\n\n")

	currentTokens := 0
	for _, s := range slices {
		// Rough token estimation (1 token ≈ 4 characters).
		estTokens := len(s.Content) / 4

		if currentTokens+estTokens > b.maxTokens {
			builder.WriteString("\n... (remaining context truncated to fit token window)")
			break
		}

		builder.WriteString(fmt.Sprintf("--- File: %s (Lines %d-%d) ---\n", s.FilePath, s.StartLine, s.EndLine))
		builder.WriteString(s.Content)
		builder.WriteString("\n\n")

		currentTokens += estTokens
	}

	if currentTokens == 0 {
		return "No relevant code context found in the index for the given query.", nil
	}

	return builder.String(), nil
}

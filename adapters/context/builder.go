package context

import (
	"context"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/domain/errors"
	"github.com/david22573/codepicker/infra/indexer"
	"github.com/david22573/codepicker/infra/llm"
)

type SmartBuilder struct {
	repo      indexer.ContextRepository
	embedder  *llm.EmbeddingClient
	reranker  *Reranker
	maxTokens int
}

func NewSmartBuilder(repo indexer.ContextRepository, embedder *llm.EmbeddingClient, reranker *Reranker, maxTokens int) *SmartBuilder {
	return &SmartBuilder{
		repo:      repo,
		embedder:  embedder,
		reranker:  reranker,
		maxTokens: maxTokens,
	}
}

func (b *SmartBuilder) BuildForTask(query string) (string, error) {
	return b.BuildContext(context.Background(), query)
}

func (b *SmartBuilder) BuildContext(ctx context.Context, query string) (string, error) {
	// 1. Vector Retrieval (Fetch top 50 candidates)
	vectors, _, err := b.embedder.CreateEmbeddings(ctx, []string{query})
	if err != nil {
		return "", errors.NewSystem("context.build", "embedding failed", err)
	}

	// FIX: Check if we actually got a vector back
	if len(vectors) == 0 {
		return "", errors.NewSystem("context.build", "no embedding generated", nil)
	}

	candidates, err := b.repo.SearchByVector(ctx, vectors[0], 50)
	if err != nil {
		return "", errors.NewSystem("context.build", "vector search failed", err)
	}

	if len(candidates) == 0 {
		return "No relevant code found.", nil
	}

	// 2. Re-Ranking (LLM filters and orders them)
	ranked, err := b.reranker.Rank(ctx, query, candidates)
	if err != nil {
		// Log warning but continue with vector results
		ranked = candidates
	}

	// 3. Packing (Token Budgeting)
	var builder strings.Builder
	builder.WriteString("### RELEVANT CODE CONTEXT (RAG Optimized)\n")

	usedTokens := 0
	includedCount := 0

	for _, slice := range ranked {
		// Estimate tokens: 4 chars / token
		contentTokens := len(slice.Content) / 4
		overhead := 20
		cost := contentTokens + overhead

		if usedTokens+cost > b.maxTokens {
			continue
		}

		builder.WriteString(fmt.Sprintf("--- File: %s (Lines %d-%d) [%s] ---\n", slice.FilePath, slice.StartLine, slice.EndLine, slice.SliceType))
		builder.WriteString(slice.Content)
		builder.WriteString("\n\n")

		usedTokens += cost
		includedCount++
	}

	if includedCount < len(ranked) {
		builder.WriteString(fmt.Sprintf("\n... (Truncated %d less relevant snippets)\n", len(ranked)-includedCount))
	}

	return builder.String(), nil
}

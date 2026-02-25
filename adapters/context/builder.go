package context

import (
	"context"
	"fmt"
	"sort"
	"strings"

	domainCtx "github.com/david22573/codepicker/domain/context"
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
	// 1. Vector Retrieval (Increased to 100 candidates to cast a wider net)
	vectors, _, err := b.embedder.CreateEmbeddings(ctx, []string{query})
	if err != nil {
		return "", errors.NewSystem("context.build", "embedding failed", err)
	}

	if len(vectors) == 0 {
		return "", errors.NewSystem("context.build", "no embedding generated", nil)
	}

	candidates, err := b.repo.SearchByVector(ctx, vectors[0], 100)
	if err != nil {
		return "", errors.NewSystem("context.build", "vector search failed", err)
	}

	if len(candidates) == 0 {
		return "No relevant code found.", nil
	}

	// 2. Re-Ranking (LLM filters and orders them by semantic importance)
	ranked, err := b.reranker.Rank(ctx, query, candidates)
	if err != nil {
		ranked = candidates
	}

	// 3. Grouping and Chronological Sorting
	// Instead of scattering snippets, group them by file and sort by line number
	// to provide the LLM with contiguous, readable logic blocks.
	grouped := make(map[string][]domainCtx.CodeSlice)
	var orderedFiles []string
	seenFiles := make(map[string]bool)

	for _, slice := range ranked {
		grouped[slice.FilePath] = append(grouped[slice.FilePath], slice)

		// Maintain the LLM's file-level priority based on the first occurrence of a file in the ranked list
		if !seenFiles[slice.FilePath] {
			orderedFiles = append(orderedFiles, slice.FilePath)
			seenFiles[slice.FilePath] = true
		}
	}

	// 4. Packing (Token Budgeting)
	var builder strings.Builder
	builder.WriteString("### RELEVANT CODE CONTEXT (RAG Optimized)\n")

	usedTokens := 0
	includedCount := 0

	for _, filePath := range orderedFiles {
		slices := grouped[filePath]

		// Sort slices within the file chronologically by StartLine
		sort.Slice(slices, func(i, j int) bool {
			return slices[i].StartLine < slices[j].StartLine
		})

		fileHeaderAdded := false

		for _, slice := range slices {
			// Estimate tokens: 4 chars / token
			contentTokens := len(slice.Content) / 4
			overhead := 30
			cost := contentTokens + overhead

			if usedTokens+cost > b.maxTokens {
				continue
			}

			if !fileHeaderAdded {
				builder.WriteString(fmt.Sprintf("\n#### FILE: %s\n", filePath))
				fileHeaderAdded = true
			}

			builder.WriteString(fmt.Sprintf("--- Lines %d-%d [%s] ---\n", slice.StartLine, slice.EndLine, slice.SliceType))
			builder.WriteString(slice.Content)
			builder.WriteString("\n\n")

			usedTokens += cost
			includedCount++
		}
	}

	if includedCount < len(ranked) {
		builder.WriteString(fmt.Sprintf("\n... (Truncated %d less relevant snippets to fit token limits)\n", len(ranked)-includedCount))
	}

	return builder.String(), nil
}

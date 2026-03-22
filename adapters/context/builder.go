package context

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	domainCtx "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/domain/errors"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/indexer"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/metrics"
)

type SmartBuilder struct {
	repo      indexer.ContextRepository
	embedder  *llm.EmbeddingClient
	reranker  *Reranker
	maxTokens int
	estimator llm.TokenEstimator
	dedup     *SemanticDeduplicator
	shadow    *fs.ShadowManager
	mapper    *indexer.RepoMapper
}

func NewSmartBuilder(
	repo indexer.ContextRepository,
	embedder *llm.EmbeddingClient,
	reranker *Reranker,
	shadow *fs.ShadowManager,
	maxTokens int,
	mapper *indexer.RepoMapper,
) *SmartBuilder {
	return &SmartBuilder{
		repo:      repo,
		embedder:  embedder,
		reranker:  reranker,
		maxTokens: maxTokens,
		estimator: llm.NewDefaultEstimator(),
		dedup:     NewSemanticDeduplicator(),
		shadow:    shadow,
		mapper:    mapper,
	}
}

func (b *SmartBuilder) BuildForTask(query string) (string, error) {
	return b.BuildContext(context.Background(), query)
}

func (b *SmartBuilder) BuildContext(ctx context.Context, query string) (string, error) {
	start := time.Now()
	defer func() {
		metrics.GetRegistry().ObserveDuration("codepicker_context_build_latency_seconds", time.Since(start))
	}()

	var builder strings.Builder

	// Phase 1.2: Inject the Sparse Repo Map
	if b.mapper != nil {
		mapBudget := 1000
		if b.maxTokens < 4000 {
			mapBudget = b.maxTokens / 4 // Scale down if overall budget is super tight
		}
		builder.WriteString(b.mapper.RenderMap(mapBudget))
		builder.WriteString("\n\n")
		b.maxTokens -= mapBudget // Deduct map cost from remaining slice budget
	}

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
		builder.WriteString("<relevant_code_context>\n  \n</relevant_code_context>\n")
		return builder.String(), nil
	}

	ranked, err := b.reranker.Rank(ctx, query, candidates)
	if err != nil {
		ranked = candidates
	}

	grouped := make(map[string][]domainCtx.CodeSlice)
	var orderedFiles []string
	seenFiles := make(map[string]bool)

	for _, slice := range ranked {
		grouped[slice.FilePath] = append(grouped[slice.FilePath], slice)
		if !seenFiles[slice.FilePath] {
			orderedFiles = append(orderedFiles, slice.FilePath)
			seenFiles[slice.FilePath] = true
		}
	}

	builder.WriteString("<relevant_code_context>\n")

	usedTokens := 0
	includedCount := 0

	for _, filePath := range orderedFiles {
		// Phase 7.2: Incremental File Diff Injection
		if b.shadow != nil {
			diffSummary, err := b.shadow.Diff(filePath)
			if err == nil && diffSummary.Type != fs.ChangeNoOp {
				diffNotice := fmt.Sprintf("  <file path=\"%s\" state=\"modified_in_shadow\">\n    [NOTE: This file has pending edits. Lines modified: +%d/-%d. Use git_diff or read_file to view exact shadow state.]\n  </file>\n",
					filePath, diffSummary.NewLines, diffSummary.OldLines)

				noticeCost := b.estimator.EstimateText(diffNotice)
				if usedTokens+noticeCost <= b.maxTokens {
					builder.WriteString(diffNotice)
					usedTokens += noticeCost
				}
				continue // Skip injecting raw baseline slices since the file has diverged
			}
		}

		slices := grouped[filePath]
		sort.Slice(slices, func(i, j int) bool {
			return slices[i].StartLine < slices[j].StartLine
		})

		fileHeaderAdded := false

		for _, slice := range slices {
			// Phase 7.1: Semantic Context Deduplication
			if !b.dedup.IsUnique(slice.Content) {
				continue
			}

			contentTokens := b.estimator.EstimateText(slice.Content)
			overhead := 40
			cost := contentTokens + overhead

			if usedTokens+cost > b.maxTokens {
				continue
			}

			if !fileHeaderAdded {
				builder.WriteString(fmt.Sprintf("  <file path=\"%s\">\n", filePath))
				fileHeaderAdded = true
			}

			builder.WriteString(fmt.Sprintf("    <slice start_line=\"%d\" end_line=\"%d\" type=\"%s\">\n", slice.StartLine, slice.EndLine, slice.SliceType))
			builder.WriteString(slice.Content)
			if !strings.HasSuffix(slice.Content, "\n") {
				builder.WriteString("\n")
			}
			builder.WriteString("    </slice>\n")

			usedTokens += cost
			includedCount++
		}

		if fileHeaderAdded {
			builder.WriteString("  </file>\n")
		}
	}

	if includedCount < len(ranked) {
		fmt.Fprintf(&builder, "  \n", len(ranked)-includedCount)
	}

	builder.WriteString("</relevant_code_context>\n")

	return builder.String(), nil
}

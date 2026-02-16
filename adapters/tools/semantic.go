package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/domain/errors"
	"github.com/david22573/codepicker/infra/indexer"
	"github.com/david22573/codepicker/infra/llm"
)

type SemanticSearchTool struct {
	embedder *llm.EmbeddingClient
	repo     indexer.ContextRepository
}

func NewSemanticSearchTool(e *llm.EmbeddingClient, r indexer.ContextRepository) *SemanticSearchTool {
	return &SemanticSearchTool{embedder: e, repo: r}
}

func (t *SemanticSearchTool) Name() string { return "search_semantic" }
func (t *SemanticSearchTool) Description() string {
	return `Search codebase by meaning (concept) rather than exact text.
Input JSON: {"query": "string"}`
}

func (t *SemanticSearchTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", errors.NewValidation("tool.search_semantic", "invalid JSON")
	}

	if input.Query == "" {
		return "", errors.NewValidation("tool.search_semantic", "query empty")
	}

	// 1. Vectorize Query
	// We generate an embedding for the user's search query to match against code vectors.
	vectors, _, err := t.embedder.CreateEmbeddings(ctx, []string{input.Query})
	if err != nil {
		return "", fmt.Errorf("failed to vectorize query: %w", err)
	}
	if len(vectors) == 0 {
		return "", fmt.Errorf("no embedding generated")
	}

	// 2. Vector Search (Cosine Similarity)
	// We fetch top 10 results from the SQLite vector store.
	results, err := t.repo.SearchByVector(ctx, vectors[0], 10)
	if err != nil {
		return "", fmt.Errorf("vector search failed: %w", err)
	}

	if len(results) == 0 {
		return "No relevant code found.", nil
	}

	// 3. Format Output
	// Returns a readable list of matches for the LLM to consume.
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d relevant snippets:\n\n", len(results)))

	for _, s := range results {
		sb.WriteString(fmt.Sprintf("--- %s (%d-%d) ---\n", s.FilePath, s.StartLine, s.EndLine))
		sb.WriteString(s.Content)
		sb.WriteString("\n\n")
	}

	return sb.String(), nil
}
